package eval

// builtins_mcp_server.go — Model Context Protocol (MCP) server primitive.
//
// The other side of builtins_mcp.go: lets a kLex program EXPOSE itself as
// an MCP server that any MCP client (Claude Code, Claude Desktop, custom
// agents) can drive over HTTP+SSE.
//
// Builtins:
//
//   _mcpServeHTTP(spec)        → (server, err)
//   _mcpStopServer(server)     → null
//
// spec hash shape (v0):
//
//   {
//     "name":    "klex-foo",        // required string
//     "version": "0.1.0",            // required string
//     "port":    7777,               // required int (1..65535)
//     "tools":   {                   // optional hash; missing = empty toolset
//       "tool_name": {
//         "description": "...",
//         "schema":      { ...JSON Schema... },
//         "handler":     <kLex callable>,
//       },
//       ...
//     }
//   }
//
// Returns IMMEDIATELY with the live MCPServer handle. The HTTP server
// runs on its own goroutine until _mcpStopServer is called or the kLex
// process exits. This is the "fire-and-keep-running" shape — the calling
// kLex code stays in control and can do other work (run a UI loop, an
// agentic loop, whatever).
//
// Transport (v0): SSE-style HTTP MCP.
//
//   GET  /sse                              opens an event stream; first event
//                                          is `endpoint` with the URL the
//                                          client should POST messages to.
//   POST /messages?sessionId=<id>          carries JSON-RPC requests.
//   Server pushes JSON-RPC responses back over the SSE stream as
//   `message` events.
//
// SSE-style was chosen over the newer streamable-HTTP single-endpoint shape
// because every MCP client in the wild today (Claude Code, Claude Desktop,
// MCP Inspector) supports SSE; streamable-HTTP support is still rolling out.
// When the ecosystem catches up we add a second transport, not switch.
//
// Threading model (v0): MCP request handlers run on the HTTP server's
// goroutines. When a tool call dispatches into a kLex callable, the call
// happens on that goroutine — same shape as async(). User code that
// mutates shared globals from a tool handler must accept the async
// semantics (env snapshot rules apply). For tadPole's v0 tool set
// (chat, list_history, current_state, list_providers), this is fine.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"klex/ast"
)

// mcpServerProtocolVersion is what we advertise in the initialize
// response. Matches what builtins_mcp.go (client side) uses, keeping
// both ends speaking the same dialect.
const mcpServerProtocolVersion = "2025-03-26"

// ── Types ───────────────────────────────────────────────────────────────────

// MCPServer is the live server handle returned to kLex. Holds the HTTP
// server, the tool registry, and per-client SSE sessions.
type MCPServer struct {
	name    string
	version string
	port    int

	tools []*mcpServerTool

	// toolsListJSON is the pre-marshalled tools/list result payload.
	// Computed once at server start; tools/list returns it verbatim.
	// Avoids re-marshalling on every list call (which Claude Code does
	// once per session, but other clients may poll).
	toolsListJSON json.RawMessage

	httpSrv *http.Server

	sessionsMu sync.Mutex
	sessions   map[string]*mcpSession

	// callMu serialises tool-call dispatch into the kLex runtime.
	// See handleToolsCall for the threading contract.
	callMu sync.Mutex

	shutdownOnce sync.Once
	shutdownCh   chan struct{}
	stopped      atomic.Bool
}

func (s *MCPServer) Type() ObjectType { return MCP_SERVER_OBJ }
func (s *MCPServer) Inspect() string {
	return fmt.Sprintf("mcp_server(%s/%s on :%d)", s.name, s.version, s.port)
}

// mcpServerTool is one registered tool: the metadata the client sees
// plus the kLex callable that runs when the tool is called.
type mcpServerTool struct {
	name        string
	description string
	schema      map[string]interface{} // pre-decoded JSON Schema
	handler     Object                 // the kLex callable (Function / Builtin / CompiledFunction)
}

// mcpSession is one connected SSE client. Holds the writer + flusher
// needed to push response events back, plus a write mutex so concurrent
// in-flight tool calls don't interleave SSE frames on the wire.
type mcpSession struct {
	id      string
	w       http.ResponseWriter
	flusher http.Flusher
	writeMu sync.Mutex
	closed  atomic.Bool
	done    chan struct{}
}

// writeEvent serialises one SSE event to the client. Returns an error
// if the connection is closed or the write fails — the caller should
// treat that as a terminal session error and drop the session.
func (sess *mcpSession) writeEvent(eventType, data string) error {
	if sess.closed.Load() {
		return fmt.Errorf("session closed")
	}
	sess.writeMu.Lock()
	defer sess.writeMu.Unlock()
	if _, err := fmt.Fprintf(sess.w, "event: %s\ndata: %s\n\n", eventType, data); err != nil {
		return err
	}
	sess.flusher.Flush()
	return nil
}

// ── Spec validation ─────────────────────────────────────────────────────────

// parseServerSpec validates the user-supplied spec hash and constructs
// the MCPServer skeleton (no HTTP server started yet). Returns either
// the partially-built server or an error suitable for the (val, err)
// tuple the kLex caller will destructure.
func parseServerSpec(spec *Hash) (*MCPServer, Object) {
	name, err := mcpReqString(spec, "name")
	if err != nil {
		return nil, err
	}
	version, err := mcpReqString(spec, "version")
	if err != nil {
		return nil, err
	}
	port, err := mcpReqInt(spec, "port")
	if err != nil {
		return nil, err
	}
	if port < 1 || port > 65535 {
		return nil, mcpServerErr("MCP_SERVER_BAD_SPEC",
			fmt.Sprintf("port must be 1..65535, got %d", port))
	}

	srv := &MCPServer{
		name:       name,
		version:    version,
		port:       port,
		sessions:   make(map[string]*mcpSession),
		shutdownCh: make(chan struct{}),
	}

	// Tools section is optional — a server can advertise zero tools and
	// still be a valid MCP server (it would only answer initialize).
	if toolsVal, ok := spec.Pairs[HashKey{Type: STRING_OBJ, Value: "tools"}]; ok {
		toolsHash, ok := toolsVal.Value.(*Hash)
		if !ok {
			return nil, mcpServerErr("MCP_SERVER_BAD_SPEC",
				fmt.Sprintf("'tools' must be a hash, got %s", toolsVal.Value.Type()))
		}
		for _, pair := range toolsHash.Pairs {
			nameKey, ok := pair.Key.(*String)
			if !ok {
				return nil, mcpServerErr("MCP_SERVER_BAD_SPEC",
					"tool names must be strings")
			}
			toolSpec, ok := pair.Value.(*Hash)
			if !ok {
				return nil, mcpServerErr("MCP_SERVER_BAD_SPEC",
					fmt.Sprintf("tool %q must be a hash with description/schema/handler", nameKey.Value))
			}
			tool, err := parseToolSpec(nameKey.Value, toolSpec)
			if err != nil {
				return nil, err
			}
			srv.tools = append(srv.tools, tool)
		}
	}

	// Pre-marshal the tools/list result so the HTTP handler can return
	// it verbatim without redoing the work on every request.
	listResult := map[string]interface{}{
		"tools": buildToolsListPayload(srv.tools),
	}
	raw, jerr := json.Marshal(listResult)
	if jerr != nil {
		return nil, mcpServerErr("MCP_SERVER_BAD_SPEC",
			"could not encode tools list: "+jerr.Error())
	}
	srv.toolsListJSON = raw

	return srv, nil
}

// parseToolSpec validates one entry from the spec's "tools" hash.
func parseToolSpec(name string, toolSpec *Hash) (*mcpServerTool, Object) {
	t := &mcpServerTool{name: name}

	if descVal, ok := toolSpec.Pairs[HashKey{Type: STRING_OBJ, Value: "description"}]; ok {
		if s, ok := descVal.Value.(*String); ok {
			t.description = s.Value
		}
	}

	if schVal, ok := toolSpec.Pairs[HashKey{Type: STRING_OBJ, Value: "schema"}]; ok {
		schemaGo, jerr := klexToJSON(schVal.Value)
		if jerr != nil {
			return nil, mcpServerErr("MCP_SERVER_BAD_SPEC",
				fmt.Sprintf("tool %q: schema must be JSON-compatible: %s", name, jerr.Error()))
		}
		if schemaHash, ok := schemaGo.(map[string]interface{}); ok {
			t.schema = schemaHash
		} else {
			t.schema = map[string]interface{}{
				"type": "object",
			}
		}
	} else {
		// Sensible default: accept any object.
		t.schema = map[string]interface{}{
			"type": "object",
		}
	}

	handlerVal, ok := toolSpec.Pairs[HashKey{Type: STRING_OBJ, Value: "handler"}]
	if !ok {
		return nil, mcpServerErr("MCP_SERVER_BAD_SPEC",
			fmt.Sprintf("tool %q: missing 'handler' (a kLex fn)", name))
	}
	t.handler = handlerVal.Value
	// Light callable check — we don't try to invoke it here, but we want
	// a useful error at registration time rather than at first tool call.
	switch t.handler.(type) {
	case *Function, *Builtin:
		// ok
	default:
		if t.handler.Type() != COMPILED_FUNCTION_OBJ {
			return nil, mcpServerErr("MCP_SERVER_BAD_SPEC",
				fmt.Sprintf("tool %q: handler must be a function, got %s", name, t.handler.Type()))
		}
	}

	return t, nil
}

// buildToolsListPayload constructs the array sent back for tools/list.
// Each entry follows the MCP spec: name + description + inputSchema.
func buildToolsListPayload(tools []*mcpServerTool) []map[string]interface{} {
	out := make([]map[string]interface{}, len(tools))
	for i, t := range tools {
		out[i] = map[string]interface{}{
			"name":        t.name,
			"description": t.description,
			"inputSchema": t.schema,
		}
	}
	return out
}

// ── Builtin: _mcpServeHTTP ──────────────────────────────────────────────────

func init() {
	// _mcpServeHTTP(spec) → (server, err)
	//
	// Validates spec, binds an HTTP listener, and starts background
	// goroutines for SSE + JSON-RPC. Returns the MCPServer handle
	// immediately — the kLex caller stays in control.
	Builtins["_mcpServeHTTP"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_mcpServeHTTP expects 1 argument (spec hash)", ast.Pos{})
		}
		spec, ok := args[0].(*Hash)
		if !ok {
			return typeError(fmt.Sprintf("_mcpServeHTTP: spec must be hash, got %s", args[0].Type()), ast.Pos{})
		}
		srv, errObj := parseServerSpec(spec)
		if errObj != nil {
			return &Tuple{Elements: []Object{NULL, errObj}}
		}

		// Bind first so a port conflict surfaces synchronously — we
		// don't want kLex code to think the server is up when it isn't.
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", srv.port))
		if err != nil {
			return &Tuple{Elements: []Object{NULL,
				mcpServerErr("MCP_SERVER_BIND_FAILED",
					fmt.Sprintf("bind 127.0.0.1:%d: %s", srv.port, err.Error()))}}
		}

		mux := http.NewServeMux()
		mux.HandleFunc("/sse", srv.handleSSE)
		mux.HandleFunc("/messages", srv.handleMessages)
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			// Friendly landing for browsers / health checks.
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprintf(w, "kLex MCP server: %s/%s\nendpoints: /sse (GET), /messages (POST)\n",
				srv.name, srv.version)
		})

		srv.httpSrv = &http.Server{
			Handler:           mux,
			ReadHeaderTimeout: 10 * time.Second,
		}

		go func() {
			_ = srv.httpSrv.Serve(ln)
		}()

		return &Tuple{Elements: []Object{srv, NULL}}
	}}

	// _mcpStopServer(server) → null
	//
	// Graceful shutdown: closes the listener, waits up to 5s for
	// in-flight handlers, then forces shutdown. Idempotent.
	Builtins["_mcpStopServer"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_mcpStopServer expects 1 argument (server)", ast.Pos{})
		}
		srv, ok := args[0].(*MCPServer)
		if !ok {
			return typeError(fmt.Sprintf("_mcpStopServer: argument must be mcp_server, got %s", args[0].Type()), ast.Pos{})
		}
		srv.shutdownOnce.Do(func() {
			srv.stopped.Store(true)
			close(srv.shutdownCh)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = srv.httpSrv.Shutdown(ctx)
			// Drop any lingering SSE sessions.
			srv.sessionsMu.Lock()
			for _, sess := range srv.sessions {
				sess.closed.Store(true)
				select {
				case <-sess.done:
				default:
					close(sess.done)
				}
			}
			srv.sessions = nil
			srv.sessionsMu.Unlock()
		})
		return NULL
	}}

	// _mcpServerInfo(server) → hash  — introspection helper, useful for tests
	Builtins["_mcpServerInfo"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_mcpServerInfo expects 1 argument (server)", ast.Pos{})
		}
		srv, ok := args[0].(*MCPServer)
		if !ok {
			return typeError(fmt.Sprintf("_mcpServerInfo: argument must be mcp_server, got %s", args[0].Type()), ast.Pos{})
		}
		toolNames := make([]Object, len(srv.tools))
		for i, t := range srv.tools {
			toolNames[i] = &String{Value: t.name}
		}
		h := &Hash{Pairs: map[HashKey]HashPair{}}
		h.Pairs[HashKey{Type: STRING_OBJ, Value: "name"}] = HashPair{
			Key:   &String{Value: "name"},
			Value: &String{Value: srv.name},
		}
		h.Pairs[HashKey{Type: STRING_OBJ, Value: "version"}] = HashPair{
			Key:   &String{Value: "version"},
			Value: &String{Value: srv.version},
		}
		h.Pairs[HashKey{Type: STRING_OBJ, Value: "port"}] = HashPair{
			Key:   &String{Value: "port"},
			Value: intObj(srv.port),
		}
		h.Pairs[HashKey{Type: STRING_OBJ, Value: "tools"}] = HashPair{
			Key:   &String{Value: "tools"},
			Value: &Array{Elements: toolNames},
		}
		h.Pairs[HashKey{Type: STRING_OBJ, Value: "stopped"}] = HashPair{
			Key:   &String{Value: "stopped"},
			Value: boolObj(srv.stopped.Load()),
		}
		return h
	}}
}

// ── HTTP handlers ───────────────────────────────────────────────────────────

// handleSSE accepts a GET, opens an SSE stream, and tells the client
// the message endpoint URL via an `endpoint` event. The session lives
// until the client disconnects or the server shuts down.
func (s *MCPServer) handleSSE(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	sessID := newSessionID()
	sess := &mcpSession{
		id:      sessID,
		w:       w,
		flusher: flusher,
		done:    make(chan struct{}),
	}
	s.sessionsMu.Lock()
	s.sessions[sessID] = sess
	s.sessionsMu.Unlock()

	// Tell the client where to POST messages. The MCP SSE transport
	// expects this `endpoint` event as the first thing on the stream.
	endpointURL := fmt.Sprintf("/messages?sessionId=%s", sessID)
	if err := sess.writeEvent("endpoint", endpointURL); err != nil {
		s.dropSession(sessID)
		return
	}

	// Keep the stream open until the client disconnects or the server
	// shuts down. Heartbeat every 15s so intermediaries don't time out.
	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()

	for {
		select {
		case <-r.Context().Done():
			s.dropSession(sessID)
			return
		case <-s.shutdownCh:
			s.dropSession(sessID)
			return
		case <-sess.done:
			return
		case <-tick.C:
			// SSE comment line as heartbeat (ignored by clients).
			sess.writeMu.Lock()
			_, err := fmt.Fprintf(w, ": ping\n\n")
			if err == nil {
				flusher.Flush()
			}
			sess.writeMu.Unlock()
			if err != nil {
				s.dropSession(sessID)
				return
			}
		}
	}
}

// handleMessages accepts a POST with a JSON-RPC request body.
// The response is pushed back over the matching SSE session as a
// `message` event; the HTTP response is just an ACK (status 202).
func (s *MCPServer) handleMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sessID := r.URL.Query().Get("sessionId")
	if sessID == "" {
		http.Error(w, "missing sessionId query param", http.StatusBadRequest)
		return
	}
	s.sessionsMu.Lock()
	sess, ok := s.sessions[sessID]
	s.sessionsMu.Unlock()
	if !ok {
		http.Error(w, "unknown sessionId — open /sse first", http.StatusNotFound)
		return
	}

	// Decode JSON-RPC envelope.
	var req jsonrpcRequest
	dec := json.NewDecoder(r.Body)
	dec.UseNumber()
	if err := dec.Decode(&req); err != nil {
		s.sendRPCError(sess, nil, -32700, "parse error: "+err.Error())
		http.Error(w, "parse error", http.StatusBadRequest)
		return
	}

	// Acknowledge immediately — the actual response goes out over SSE.
	w.WriteHeader(http.StatusAccepted)

	// Notifications (no id) don't get a response per JSON-RPC.
	if req.ID == nil {
		// "notifications/initialized" is the standard handshake follow-up
		// from the client; nothing to do, just don't reply.
		return
	}

	// Dispatch — synchronously on this goroutine, but the response
	// flows back via the SSE stream rather than this HTTP response.
	s.dispatch(sess, &req)
}

// dropSession removes a session from the registry and closes its done
// channel. Idempotent — safe to call multiple times.
func (s *MCPServer) dropSession(id string) {
	s.sessionsMu.Lock()
	sess, ok := s.sessions[id]
	if ok {
		delete(s.sessions, id)
	}
	s.sessionsMu.Unlock()
	if ok && !sess.closed.Swap(true) {
		close(sess.done)
	}
}

// ── JSON-RPC dispatch ───────────────────────────────────────────────────────

type jsonrpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonrpcResponse struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      json.RawMessage  `json:"id"`
	Result  interface{}      `json:"result,omitempty"`
	Error   *jsonrpcErrorObj `json:"error,omitempty"`
}

type jsonrpcErrorObj struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// dispatch routes one JSON-RPC request to the matching MCP method and
// sends the response back over the SSE session.
func (s *MCPServer) dispatch(sess *mcpSession, req *jsonrpcRequest) {
	switch req.Method {
	case "initialize":
		s.sendRPCResult(sess, req.ID, map[string]interface{}{
			"protocolVersion": mcpServerProtocolVersion,
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{},
			},
			"serverInfo": map[string]interface{}{
				"name":    s.name,
				"version": s.version,
			},
		})
	case "tools/list":
		// Pre-marshalled, decoded on the fly so json.Marshal in
		// sendRPCResult sees a Go value, not a RawMessage.
		var listResult interface{}
		_ = json.Unmarshal(s.toolsListJSON, &listResult)
		s.sendRPCResult(sess, req.ID, listResult)
	case "tools/call":
		s.handleToolsCall(sess, req)
	case "ping":
		s.sendRPCResult(sess, req.ID, map[string]interface{}{})
	default:
		s.sendRPCError(sess, req.ID, -32601, "method not found: "+req.Method)
	}
}

// handleToolsCall routes a tools/call JSON-RPC request to the
// registered kLex callable, marshals the result back, and pushes the
// response over the SSE stream.
//
// MCP result shape (per 2024-11-05 / 2025-03-26 spec):
//
//	{
//	  "content": [{"type":"text","text":"<json-stringified result>"}],
//	  "isError": false
//	}
//
// On kLex-side error we return isError:true with the error message in
// the text block. We DON'T use the JSON-RPC error channel for tool
// errors — that's reserved for protocol-level failures (unknown tool,
// bad args, server crash). MCP clients distinguish "the tool ran and
// reports failure" from "the call itself failed" based on this split.
func (s *MCPServer) handleToolsCall(sess *mcpSession, req *jsonrpcRequest) {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &p); err != nil {
			s.sendRPCError(sess, req.ID, -32602, "invalid params: "+err.Error())
			return
		}
	}
	if p.Name == "" {
		s.sendRPCError(sess, req.ID, -32602, "missing 'name' in tools/call params")
		return
	}

	// Linear scan — tool counts are small (single digits to low dozens
	// in realistic kLex servers); no need for a map.
	var tool *mcpServerTool
	for _, t := range s.tools {
		if t.name == p.Name {
			tool = t
			break
		}
	}
	if tool == nil {
		s.sendRPCError(sess, req.ID, -32601, "unknown tool: "+p.Name)
		return
	}

	// Build the args hash. MCP allows arguments to be omitted entirely
	// (zero-arg tools); pass an empty hash in that case so the kLex fn
	// always sees a hash and never a null surprise.
	var argsObj Object
	if len(p.Arguments) == 0 || string(p.Arguments) == "null" {
		argsObj = &Hash{Pairs: map[HashKey]HashPair{}}
	} else {
		var argsRaw interface{}
		if err := json.Unmarshal(p.Arguments, &argsRaw); err != nil {
			s.sendRPCError(sess, req.ID, -32602, "invalid arguments JSON: "+err.Error())
			return
		}
		argsObj = goToKlex(argsRaw)
		// MCP arguments must be an object (hash). If the client sent
		// something else (array, primitive), wrap it so the kLex fn
		// always gets a hash — keeps the kLex side's contract simple.
		if argsObj.Type() != HASH_OBJ {
			wrap := &Hash{Pairs: map[HashKey]HashPair{}}
			wrap.Pairs[HashKey{Type: STRING_OBJ, Value: "value"}] = HashPair{
				Key:   &String{Value: "value"},
				Value: argsObj,
			}
			argsObj = wrap
		}
	}

	// Serialise kLex runtime entry across concurrent MCP requests.
	// Without this lock two simultaneous tool calls would race on
	// whatever globals the handlers touch (history arrays, provider
	// state). Mutex held for the full handler duration — including any
	// network I/O the handler does — so callers see consistent state.
	//
	// CAVEAT: this lock does NOT synchronise with the kLex MAIN thread
	// (UI loop, etc.). If your tool handler mutates state the main loop
	// also touches, you still need explicit concurrency primitives on
	// the kLex side (atomicHash, channels). Documented in
	// stdlib/mcp_server.lex.
	s.callMu.Lock()
	result, errObj := callCallable(tool.handler, []Object{argsObj})
	s.callMu.Unlock()

	// kLex-side error → MCP "tool reports failure" response (isError=true).
	if errObj != nil {
		var msg string
		if e, ok := errObj.(*Error); ok {
			msg = e.Message
			if e.Code != "" {
				msg = e.Code + ": " + msg
			}
		} else {
			msg = errObj.Inspect()
		}
		s.sendRPCResult(sess, req.ID, mcpToolErrorResult(msg))
		return
	}

	// Convert the kLex return value to JSON and wrap in the MCP text
	// content envelope. We always JSON-stringify the kLex result —
	// even a plain String becomes {"text":"<the string>"} JSON-quoted
	// — because that keeps the response shape uniform on the client
	// side. Clients that want structured data parse the JSON; clients
	// that want plain text strip the outer quotes.
	jsonVal, jerr := klexToJSON(result)
	if jerr != nil {
		s.sendRPCResult(sess, req.ID,
			mcpToolErrorResult("kLex result not JSON-serialisable: "+jerr.Error()))
		return
	}
	raw, mErr := json.Marshal(jsonVal)
	if mErr != nil {
		s.sendRPCResult(sess, req.ID,
			mcpToolErrorResult("marshal kLex result: "+mErr.Error()))
		return
	}
	s.sendRPCResult(sess, req.ID, mcpToolOkResult(string(raw)))
}

// mcpToolOkResult builds the standard {"content":[...],"isError":false}
// envelope for a successful tool call.
func mcpToolOkResult(text string) map[string]interface{} {
	return map[string]interface{}{
		"content": []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": text,
			},
		},
		"isError": false,
	}
}

// mcpToolErrorResult builds the {"content":[...],"isError":true} envelope.
func mcpToolErrorResult(msg string) map[string]interface{} {
	return map[string]interface{}{
		"content": []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": msg,
			},
		},
		"isError": true,
	}
}

// sendRPCResult marshals + sends a successful JSON-RPC response over SSE.
func (s *MCPServer) sendRPCResult(sess *mcpSession, id json.RawMessage, result interface{}) {
	resp := jsonrpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	raw, err := json.Marshal(&resp)
	if err != nil {
		// Last-ditch — send a parse error instead. Should be unreachable
		// for our own well-formed results.
		s.sendRPCError(sess, id, -32603, "internal marshal error: "+err.Error())
		return
	}
	_ = sess.writeEvent("message", string(raw))
}

// sendRPCError marshals + sends a JSON-RPC error response over SSE.
func (s *MCPServer) sendRPCError(sess *mcpSession, id json.RawMessage, code int, msg string) {
	resp := jsonrpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &jsonrpcErrorObj{
			Code:    code,
			Message: msg,
		},
	}
	raw, _ := json.Marshal(&resp)
	_ = sess.writeEvent("message", string(raw))
}

// ── Helpers ─────────────────────────────────────────────────────────────────

// mcpReqString reads a required string field from a spec hash.
func mcpReqString(h *Hash, key string) (string, Object) {
	v, ok := h.Pairs[HashKey{Type: STRING_OBJ, Value: key}]
	if !ok {
		return "", mcpServerErr("MCP_SERVER_BAD_SPEC",
			fmt.Sprintf("spec missing required field %q", key))
	}
	s, ok := v.Value.(*String)
	if !ok {
		return "", mcpServerErr("MCP_SERVER_BAD_SPEC",
			fmt.Sprintf("spec field %q must be string, got %s", key, v.Value.Type()))
	}
	return s.Value, nil
}

// mcpReqInt reads a required integer field from a spec hash.
func mcpReqInt(h *Hash, key string) (int, Object) {
	v, ok := h.Pairs[HashKey{Type: STRING_OBJ, Value: key}]
	if !ok {
		return 0, mcpServerErr("MCP_SERVER_BAD_SPEC",
			fmt.Sprintf("spec missing required field %q", key))
	}
	switch x := v.Value.(type) {
	case *Integer:
		return x.Value, nil
	case *Float:
		// Tolerate integer-valued floats (kLex hash literals often parse
		// numeric literals as Float by default).
		if x.Value == float64(int(x.Value)) {
			return int(x.Value), nil
		}
	}
	return 0, mcpServerErr("MCP_SERVER_BAD_SPEC",
		fmt.Sprintf("spec field %q must be integer, got %s", key, v.Value.Type()))
}

// mcpServerErr builds a typed kLex error object suitable for the
// second slot of a (val, err) tuple.
func mcpServerErr(code, msg string) Object {
	return &Error{
		IsUserError: true,
		Code:        code,
		Message:     msg,
	}
}

// newSessionID returns a 16-hex-char random identifier suitable for
// the ?sessionId= query parameter. crypto/rand because session IDs
// double as opaque capability tokens — guessing one lets you POST to
// someone else's stream.
func newSessionID() string {
	var buf [8]byte
	_, _ = rand.Read(buf[:])
	return hex.EncodeToString(buf[:])
}

// Ensure strconv is referenced so go vet doesn't whine if we trim
// usage in future edits. (Used implicitly by tests; keeps the import
// stable for the next commit which will use it for parsing tool args.)
var _ = strconv.Itoa
