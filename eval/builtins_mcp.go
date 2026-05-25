package eval

// builtins_mcp.go — Model Context Protocol (MCP) client primitive.
//
// MCP is a JSON-RPC 2.0 protocol over newline-delimited JSON on stdio. This
// file gives kLex four low-level builtins for talking to any MCP server
// subprocess (frogMcp, filesystem-mcp, github-mcp, etc.):
//
//   _mcpSpawn(cmd, args, opts?)         → (mcpClient, err)
//   _mcpCall(client, method, params, timeoutSec?) → (result_hash, err)
//   _mcpNotify(client, method, params)  → (null, err)
//   _mcpClose(client)                    → null
//   _mcpInfo(client)                     → hash  — server name/version/protocol
//   _mcpNotifications(client)            → channel of server-pushed notifications
//
// The stdlib/mcp.lex wrapper layers an idiomatic API (newClient, listTools,
// callTool, listResources, ...) on top of these. Scripts can also call the
// underscore primitives directly when fine control is needed.
//
// Architecture mirrors the bridge: one reader goroutine owns stdout; it
// decodes each line and routes responses (with "id") to per-call channels
// or notifications (without "id") to the notifCh. _mcpCall holds no mutex
// during the wait — only briefly during the id alloc and stdin write.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"klex/ast"
)

// -------- types ----------------------------------------------------------

// MCPClient is a persistent connection to an MCP server subprocess.
// Object interface: Type() / Inspect() — defined below alongside the struct.
type MCPClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Scanner

	mu       sync.Mutex
	nextID   int64
	pending  map[int64]chan *mcpResponse
	closed   bool
	closeMsg string

	// writeMu serialises stdin writes so concurrent calls don't interleave
	// bytes on the wire. Held only during the Write() call.
	writeMu sync.Mutex

	// notifCh delivers server-initiated notifications. Always allocated;
	// kLex scripts can drain or ignore. Bounded — drops oldest when full.
	notifCh    *Channel
	notifClose sync.Once

	// Populated from the initialize handshake.
	serverName    string
	serverVersion string
	protoVersion  string
}

func (c *MCPClient) Type() ObjectType { return MCP_CLIENT_OBJ }
func (c *MCPClient) Inspect() string {
	if c.serverName != "" {
		return fmt.Sprintf("mcp_client(%s/%s)", c.serverName, c.serverVersion)
	}
	return "mcp_client"
}

type mcpResponse struct {
	result json.RawMessage
	err    *mcpRPCError
}

type mcpRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// -------- helpers --------------------------------------------------------

// mcpError builds the typed (null, error) tuple kLex callers destructure.
func mcpError(code, msg string) *Tuple {
	return &Tuple{Elements: []Object{NULL, &Error{
		IsUserError: true,
		Code:        code,
		Message:     msg,
	}}}
}

// mcpOK builds a successful (value, null) tuple.
func mcpOK(v Object) *Tuple {
	return &Tuple{Elements: []Object{v, NULL}}
}

// jsonToKlex converts a parsed json.RawMessage into a kLex Object. Used to
// hand RPC results back to scripts. Mirrors the json.lex parser's value
// dispatch but operates on already-decoded Go types so the kLex side never
// re-parses a JSON string for every tool result.
func jsonToKlex(raw json.RawMessage) Object {
	if len(raw) == 0 || string(raw) == "null" {
		return NULL
	}
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		// Hand the raw bytes back as a string so the script can decide.
		return &String{Value: string(raw)}
	}
	return goToKlex(v)
}

// goToKlex maps a Go value produced by encoding/json into the kLex object
// tree. Numbers come through as float64 from json.Unmarshal; we narrow to
// Integer when the value is a whole number that fits in int — matches what
// stdlib/json.lex's parser does.
func goToKlex(v interface{}) Object {
	switch x := v.(type) {
	case nil:
		return NULL
	case bool:
		return boolObj(x)
	case float64:
		// Integer-valued floats become Integer; fractional stays Float.
		if x == float64(int64(x)) {
			return intObj(int(x))
		}
		return &Float{Value: x}
	case string:
		return &String{Value: x}
	case []interface{}:
		out := make([]Object, len(x))
		for i, el := range x {
			out[i] = goToKlex(el)
		}
		return &Array{Elements: out}
	case map[string]interface{}:
		h := &Hash{Pairs: make(map[HashKey]HashPair, len(x))}
		for k, val := range x {
			key := &String{Value: k}
			hk := HashKey{Type: STRING_OBJ, Value: k}
			h.Pairs[hk] = HashPair{Key: key, Value: goToKlex(val)}
		}
		return h
	default:
		return &String{Value: fmt.Sprintf("%v", v)}
	}
}

// klexToJSON converts a kLex Object into a Go value suitable for
// json.Marshal. Used when the script supplies params for an RPC call.
func klexToJSON(o Object) (interface{}, error) {
	switch x := o.(type) {
	case *Null:
		return nil, nil
	case *Boolean:
		return x.Value, nil
	case *Integer:
		return x.Value, nil
	case *Float:
		return x.Value, nil
	case *String:
		return x.Value, nil
	case *Array:
		out := make([]interface{}, len(x.Elements))
		for i, el := range x.Elements {
			v, err := klexToJSON(el)
			if err != nil {
				return nil, err
			}
			out[i] = v
		}
		return out, nil
	case *Hash:
		out := make(map[string]interface{}, len(x.Pairs))
		for _, pair := range x.Pairs {
			ks, ok := pair.Key.(*String)
			if !ok {
				return nil, fmt.Errorf("hash key must be string for MCP params, got %s", pair.Key.Type())
			}
			v, err := klexToJSON(pair.Value)
			if err != nil {
				return nil, err
			}
			out[ks.Value] = v
		}
		return out, nil
	case *Bytes:
		// MCP doesn't define a binary wire format for params; emit base64
		// so the server can decide. Most servers won't expect bytes here.
		return x.Value, nil
	default:
		return nil, fmt.Errorf("cannot serialise %s to JSON", o.Type())
	}
}

// -------- reader goroutine ----------------------------------------------

// readerLoop owns stdout. Decodes one JSON line at a time and routes:
//   - {"id": N, "result": ...}  → pending[N]
//   - {"id": N, "error": ...}   → pending[N]
//   - {"method": "...", ...}    → notifCh (server-initiated notification)
//
// Stops when the scanner errors or hits EOF. On exit, drains all pending
// callers with a typed close error and closes notifCh.
func (c *MCPClient) readerLoop() {
	for c.stdout.Scan() {
		line := c.stdout.Bytes()
		if len(line) == 0 {
			continue
		}
		var msg map[string]json.RawMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			// Malformed line — push to notifications as a debug string so
			// the script can see it if needed; don't tear down the connection.
			c.pushNotif(&Hash{Pairs: map[HashKey]HashPair{
				{Type: STRING_OBJ, Value: "_parse_error"}: {
					Key:   &String{Value: "_parse_error"},
					Value: &String{Value: err.Error()},
				},
				{Type: STRING_OBJ, Value: "_raw"}: {
					Key:   &String{Value: "_raw"},
					Value: &String{Value: string(line)},
				},
			}})
			continue
		}

		// Response (id present) or notification?
		idRaw, hasID := msg["id"]
		if !hasID || string(idRaw) == "null" {
			// Notification: build a kLex hash with method/params.
			notif := &Hash{Pairs: make(map[HashKey]HashPair)}
			if mRaw, ok := msg["method"]; ok {
				var s string
				_ = json.Unmarshal(mRaw, &s)
				notif.Pairs[HashKey{Type: STRING_OBJ, Value: "method"}] = HashPair{
					Key:   &String{Value: "method"},
					Value: &String{Value: s},
				}
			}
			if pRaw, ok := msg["params"]; ok {
				notif.Pairs[HashKey{Type: STRING_OBJ, Value: "params"}] = HashPair{
					Key:   &String{Value: "params"},
					Value: jsonToKlex(pRaw),
				}
			}
			c.pushNotif(notif)
			continue
		}

		var id int64
		if err := json.Unmarshal(idRaw, &id); err != nil {
			// Server returned a non-integer id (string?). MCP uses ints, but
			// fall back to ignoring rather than crashing.
			continue
		}

		// Deliver to the waiter (if any).
		c.mu.Lock()
		ch, ok := c.pending[id]
		if ok {
			delete(c.pending, id)
		}
		c.mu.Unlock()
		if !ok {
			// No waiter (timeout already fired, or duplicate response).
			continue
		}

		resp := &mcpResponse{}
		if errRaw, ok := msg["error"]; ok {
			var rpcErr mcpRPCError
			if err := json.Unmarshal(errRaw, &rpcErr); err != nil {
				rpcErr = mcpRPCError{Code: -32603, Message: "invalid error object: " + err.Error()}
			}
			resp.err = &rpcErr
		} else if resRaw, ok := msg["result"]; ok {
			resp.result = resRaw
		}
		// Non-blocking send — the waiter has a buffered chan of 1.
		select {
		case ch <- resp:
		default:
		}
	}
	// Scanner ended (EOF or error). Tear down: drain all pending waiters
	// with a typed close error so they don't deadlock.
	closeMsg := "MCP server stdout closed"
	if err := c.stdout.Err(); err != nil {
		closeMsg = "MCP server stdout error: " + err.Error()
	}
	c.mu.Lock()
	c.closed = true
	c.closeMsg = closeMsg
	pending := c.pending
	c.pending = map[int64]chan *mcpResponse{}
	c.mu.Unlock()
	for _, ch := range pending {
		select {
		case ch <- &mcpResponse{err: &mcpRPCError{Code: -32000, Message: closeMsg}}:
		default:
		}
	}
	c.notifClose.Do(func() {
		close(c.notifCh.ch)
	})
}

// pushNotif delivers a notification to notifCh, dropping it if the buffer
// is full so a slow consumer can't back-pressure the reader goroutine.
func (c *MCPClient) pushNotif(o Object) {
	select {
	case c.notifCh.ch <- o:
	default:
		// Buffer full — drop. Notifications are informational; losing one
		// shouldn't break the request/response flow.
	}
}

// -------- send paths -----------------------------------------------------

// sendRaw writes a single JSON-RPC message to stdin. Held under writeMu so
// concurrent callers don't interleave bytes.
func (c *MCPClient) sendRaw(payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if _, err := c.stdin.Write(payload); err != nil {
		return err
	}
	if _, err := c.stdin.Write([]byte{'\n'}); err != nil {
		return err
	}
	return nil
}

// callRaw issues an RPC request and waits for the response.
// timeout==0 means wait forever (used during the initialize handshake where
// the caller has its own outer budget).
func (c *MCPClient) callRaw(method string, params interface{}, timeout time.Duration) (json.RawMessage, *mcpRPCError) {
	c.mu.Lock()
	if c.closed {
		msg := c.closeMsg
		c.mu.Unlock()
		return nil, &mcpRPCError{Code: -32000, Message: msg}
	}
	c.nextID++
	id := c.nextID
	respCh := make(chan *mcpResponse, 1)
	c.pending[id] = respCh
	c.mu.Unlock()

	body, err := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	})
	if err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, &mcpRPCError{Code: -32700, Message: "marshal failed: " + err.Error()}
	}
	if err := c.sendRaw(body); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, &mcpRPCError{Code: -32000, Message: "write failed: " + err.Error()}
	}

	if timeout > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		select {
		case resp := <-respCh:
			return resp.result, resp.err
		case <-ctx.Done():
			c.mu.Lock()
			delete(c.pending, id)
			c.mu.Unlock()
			return nil, &mcpRPCError{Code: -32000, Message: fmt.Sprintf("call %q timed out after %s", method, timeout)}
		}
	}
	resp := <-respCh
	return resp.result, resp.err
}

// notifyRaw fires a notification (no id, no response expected).
func (c *MCPClient) notifyRaw(method string, params interface{}) error {
	body, err := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	})
	if err != nil {
		return err
	}
	return c.sendRaw(body)
}

// closeImpl tears down the subprocess. Idempotent.
func (c *MCPClient) closeImpl() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	c.closeMsg = "MCP client closed"
	c.mu.Unlock()

	// Close stdin so the server sees EOF and exits gracefully. Many MCP
	// servers exit on stdin close; for stubborn ones we Kill below.
	_ = c.stdin.Close()

	// Give the process a moment to exit cleanly, then SIGKILL.
	done := make(chan struct{})
	go func() {
		_ = c.cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		_ = c.cmd.Process.Kill()
		<-done
	}
}

// -------- builtins -------------------------------------------------------

func init() {
	// _mcpSpawn(cmd, args, opts?) → (mcpClient, err)
	//
	// Spawn an MCP server subprocess and complete the initialize handshake.
	// cmd  — string, executable name (resolved via PATH)
	// args — array of strings
	// opts — optional hash:
	//          env          : hash string→string of additional env vars
	//          timeout_sec  : initialize-handshake timeout (default 30s)
	//          notif_buffer : notification channel size (default 32)
	//
	// On success returns the MCPClient ready for _mcpCall. On failure the
	// subprocess is reaped and a typed error is returned. Error codes:
	//   MCP_SPAWN_FAILED — exec.Start error
	//   MCP_INIT_FAILED  — initialize RPC returned an error or timed out
	Builtins["_mcpSpawn"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 2 || len(args) > 3 {
			return runtimeError("_mcpSpawn expects 2 or 3 arguments (cmd, args [, opts])", ast.Pos{})
		}
		cmdName, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_mcpSpawn: cmd must be string, got %s", args[0].Type()), ast.Pos{})
		}
		argv, terr := objectToStringSlice("_mcpSpawn", args[1])
		if terr != nil {
			return terr
		}

		initTimeout := 30 * time.Second
		notifBuf := 32
		var envVars []string
		if len(args) == 3 && args[2].Type() == HASH_OBJ {
			opts := args[2].(*Hash)
			if v, ok := opts.Pairs[HashKey{Type: STRING_OBJ, Value: "timeout_sec"}]; ok {
				switch x := v.Value.(type) {
				case *Integer:
					initTimeout = time.Duration(x.Value) * time.Second
				case *Float:
					initTimeout = time.Duration(x.Value * float64(time.Second))
				}
			}
			if v, ok := opts.Pairs[HashKey{Type: STRING_OBJ, Value: "notif_buffer"}]; ok {
				if x, ok := v.Value.(*Integer); ok && x.Value > 0 {
					notifBuf = x.Value
				}
			}
			if v, ok := opts.Pairs[HashKey{Type: STRING_OBJ, Value: "env"}]; ok {
				if envH, ok := v.Value.(*Hash); ok {
					for _, pair := range envH.Pairs {
						k, kok := pair.Key.(*String)
						val, vok := pair.Value.(*String)
						if kok && vok {
							envVars = append(envVars, k.Value+"="+val.Value)
						}
					}
				}
			}
		}

		cmd := exec.Command(cmdName.Value, argv...)
		if len(envVars) > 0 {
			// Inherit existing env, append user's overrides (later wins).
			cmd.Env = append(cmd.Env, envVars...)
		}
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return mcpError("MCP_SPAWN_FAILED", "stdin pipe: "+err.Error())
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return mcpError("MCP_SPAWN_FAILED", "stdout pipe: "+err.Error())
		}
		// Discard stderr by default. Future opts.stderr_log can capture it.
		stderr, err := cmd.StderrPipe()
		if err != nil {
			return mcpError("MCP_SPAWN_FAILED", "stderr pipe: "+err.Error())
		}
		go io.Copy(io.Discard, stderr) //nolint:errcheck

		if err := cmd.Start(); err != nil {
			return mcpError("MCP_SPAWN_FAILED", "start: "+err.Error())
		}

		scanner := bufio.NewScanner(stdout)
		// MCP tool results can include large file contents (e.g. klex_get_source
		// on a big .lex file). Allow up to 16MB per line; initial buffer 64KB.
		scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)

		client := &MCPClient{
			cmd:     cmd,
			stdin:   stdin,
			stdout:  scanner,
			pending: make(map[int64]chan *mcpResponse),
			notifCh: &Channel{
				ch:   make(chan Object, notifBuf),
				done: make(chan struct{}),
			},
		}
		go client.readerLoop()

		// Initialize handshake.
		initParams := map[string]interface{}{
			"protocolVersion": "2025-03-26",
			"clientInfo": map[string]string{
				"name":    "kLex",
				"version": KLexVersion,
			},
			"capabilities": map[string]interface{}{},
		}
		raw, rpcErr := client.callRaw("initialize", initParams, initTimeout)
		if rpcErr != nil {
			client.closeImpl()
			return mcpError("MCP_INIT_FAILED", fmt.Sprintf("initialize: %s", rpcErr.Message))
		}

		var initResult struct {
			ProtocolVersion string `json:"protocolVersion"`
			ServerInfo      struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"serverInfo"`
		}
		if err := json.Unmarshal(raw, &initResult); err == nil {
			client.serverName = initResult.ServerInfo.Name
			client.serverVersion = initResult.ServerInfo.Version
			client.protoVersion = initResult.ProtocolVersion
		}

		// Confirm the handshake. Servers expect this notification before
		// they'll respond to tools/list etc.
		if err := client.notifyRaw("notifications/initialized", nil); err != nil {
			client.closeImpl()
			return mcpError("MCP_INIT_FAILED", "initialized notify: "+err.Error())
		}

		return mcpOK(client)
	}}

	// _mcpCall(client, method, params, timeoutSec?) → (result, err)
	//
	// Issue a JSON-RPC request and wait for the response. params may be a
	// hash, array, or null. timeoutSec defaults to 60. result is the parsed
	// MCP "result" value mapped to a kLex object (typically a hash).
	//
	// Error codes:
	//   MCP_CALL_RPC      — server returned an error response (code/message in err)
	//   MCP_CALL_TIMEOUT  — no response within timeoutSec
	//   MCP_CALL_CLOSED   — connection was closed before/during the call
	Builtins["_mcpCall"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 3 || len(args) > 4 {
			return runtimeError("_mcpCall expects 3 or 4 arguments (client, method, params [, timeoutSec])", ast.Pos{})
		}
		client, ok := args[0].(*MCPClient)
		if !ok {
			return typeError(fmt.Sprintf("_mcpCall: first argument must be an MCP client, got %s", args[0].Type()), ast.Pos{})
		}
		method, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_mcpCall: method must be string, got %s", args[1].Type()), ast.Pos{})
		}
		params, perr := klexToJSON(args[2])
		if perr != nil {
			return mcpError("MCP_CALL_RPC", "params: "+perr.Error())
		}
		timeout := 60 * time.Second
		if len(args) == 4 {
			switch x := args[3].(type) {
			case *Integer:
				timeout = time.Duration(x.Value) * time.Second
			case *Float:
				timeout = time.Duration(x.Value * float64(time.Second))
			case *Null:
				// keep default
			default:
				return typeError(fmt.Sprintf("_mcpCall: timeoutSec must be number or null, got %s", args[3].Type()), ast.Pos{})
			}
		}

		raw, rpcErr := client.callRaw(method.Value, params, timeout)
		if rpcErr != nil {
			code := "MCP_CALL_RPC"
			msg := rpcErr.Message
			if msg == "" {
				msg = fmt.Sprintf("RPC error %d", rpcErr.Code)
			}
			if rpcErr.Code == -32000 {
				code = "MCP_CALL_CLOSED"
			}
			return &Tuple{Elements: []Object{NULL, &Error{
				IsUserError: true,
				Code:        code,
				Message:     msg,
			}}}
		}
		return mcpOK(jsonToKlex(raw))
	}}

	// _mcpNotify(client, method, params) → (null, err)
	// Fire-and-forget notification. Server's reply (if any) goes to the
	// notification channel rather than back to the caller.
	Builtins["_mcpNotify"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 3 {
			return runtimeError("_mcpNotify expects 3 arguments (client, method, params)", ast.Pos{})
		}
		client, ok := args[0].(*MCPClient)
		if !ok {
			return typeError(fmt.Sprintf("_mcpNotify: first argument must be an MCP client, got %s", args[0].Type()), ast.Pos{})
		}
		method, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_mcpNotify: method must be string, got %s", args[1].Type()), ast.Pos{})
		}
		params, perr := klexToJSON(args[2])
		if perr != nil {
			return mcpError("MCP_NOTIFY", "params: "+perr.Error())
		}
		if err := client.notifyRaw(method.Value, params); err != nil {
			return mcpError("MCP_NOTIFY", err.Error())
		}
		return mcpOK(NULL)
	}}

	// _mcpClose(client) → null
	// Gracefully terminate the server: close stdin (server should exit on
	// EOF), wait up to 2s, then SIGKILL. Idempotent.
	Builtins["_mcpClose"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_mcpClose expects 1 argument (client)", ast.Pos{})
		}
		client, ok := args[0].(*MCPClient)
		if !ok {
			return typeError(fmt.Sprintf("_mcpClose: argument must be an MCP client, got %s", args[0].Type()), ast.Pos{})
		}
		client.closeImpl()
		return NULL
	}}

	// _mcpInfo(client) → hash {name, version, protocol}
	// Server identity as reported in the initialize response.
	Builtins["_mcpInfo"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_mcpInfo expects 1 argument (client)", ast.Pos{})
		}
		client, ok := args[0].(*MCPClient)
		if !ok {
			return typeError(fmt.Sprintf("_mcpInfo: argument must be an MCP client, got %s", args[0].Type()), ast.Pos{})
		}
		h := &Hash{Pairs: make(map[HashKey]HashPair)}
		put := func(k, v string) {
			h.Pairs[HashKey{Type: STRING_OBJ, Value: k}] = HashPair{
				Key:   &String{Value: k},
				Value: &String{Value: v},
			}
		}
		put("name", client.serverName)
		put("version", client.serverVersion)
		put("protocol", client.protoVersion)
		return h
	}}

	// _mcpNotifications(client) → channel
	// Returns the notification channel for this client. Each value is a
	// hash {"method": "...", "params": ...} matching the JSON-RPC envelope.
	// The channel is closed automatically when the server exits.
	Builtins["_mcpNotifications"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_mcpNotifications expects 1 argument (client)", ast.Pos{})
		}
		client, ok := args[0].(*MCPClient)
		if !ok {
			return typeError(fmt.Sprintf("_mcpNotifications: argument must be an MCP client, got %s", args[0].Type()), ast.Pos{})
		}
		return client.notifCh
	}}
}
