// mcp.lex — Model Context Protocol client for kLex.
// @module    mcp
// @version   1.0.0
// @since     klex 0.3.35
// @author    karl
// @summary   Model Context Protocol client for kLex.
//
// Wraps the Go-level _mcpSpawn / _mcpCall / _mcpNotify primitives in an
// idiomatic kLex API. Talks to any MCP server subprocess over stdio —
// frogMcp, filesystem-mcp, github-mcp, etc.
//
// All functions return (value, err) tuples. On success err is null;
// on failure value is null and err is a typed error (err.code is the
// machine-readable handle, err.message is human prose).
//
// Error codes:
//   MCP_BAD_OPTS    — caller-side argument validation failure
//   MCP_SPAWN_FAILED — subprocess could not be started
//   MCP_INIT_FAILED  — server didn't complete the initialize handshake
//   MCP_CALL_RPC     — server returned an RPC error (code/message in err)
//   MCP_CALL_TIMEOUT — no response within the timeout
//   MCP_CALL_CLOSED  — connection was closed before/during the call
//   MCP_NOTIFY       — fire-and-forget notification could not be sent
//
// Usage:
//   import "stdlib/mcp.lex" as mcp
//
//   client, err = mcp.newClient("python3", ["server.py"], null)
//   if err != null { println(err.message)  return }
//
//   tools, err = mcp.listTools(client)
//   for t in tools { println(t["name"] + ": " + t["description"]) }
//
//   text, err = mcp.callToolText(client, "klex_describe_symbol",
//                                {"name": "makeArray"})
//
//   mcp.close(client)


// ── Constants ─────────────────────────────────────────────────────────────

// Default per-call timeout in seconds. Tool calls that involve heavy work
// (klex_run, klex_test, go_doc) regularly exceed 10s; 60 gives generous
// headroom without being open-ended. Callers can override per-call via
// the timeoutSec arg on callTool / call.
const DEFAULT_CALL_TIMEOUT_SEC = 60

// Default handshake timeout. Python-based MCP servers (frogMcp) can take
// several seconds to import their deps before answering initialize.
const DEFAULT_HANDSHAKE_TIMEOUT_SEC = 30


// ── Spawn / lifecycle ─────────────────────────────────────────────────────

// newClient spawns an MCP server subprocess and completes the JSON-RPC
// initialize handshake. Returns (client, err) — on success client is an
// MCP_CLIENT object usable with the rest of this module.
//
// cmd  — executable name (resolved via PATH)
// args — array of strings; pass an empty array for none
// opts — hash or null; supported keys:
//          env          : hash string→string of additional env vars
//          timeout_sec  : initialize-handshake timeout (default 30s)
//          notif_buffer : server-notification channel size (default 32)
fn newClient(cmd, args, opts) {
    if type(cmd) != "STRING" || len(cmd) == 0 {
        return null, error("MCP_BAD_OPTS", "newClient: cmd must be a non-empty string")
    }
    if type(args) != "ARRAY" {
        return null, error("MCP_BAD_OPTS", "newClient: args must be an array of strings")
    }
    let spawnOpts = {"timeout_sec": DEFAULT_HANDSHAKE_TIMEOUT_SEC}
    if opts != null && type(opts) == "HASH" {
        // Pass through known keys verbatim — the Go primitive validates types.
        if hasKey(opts, "timeout_sec")  { spawnOpts["timeout_sec"]  = opts["timeout_sec"] }
        if hasKey(opts, "notif_buffer") { spawnOpts["notif_buffer"] = opts["notif_buffer"] }
        if hasKey(opts, "env")          { spawnOpts["env"]          = opts["env"] }
    }
    let client, err = _mcpSpawn(cmd, args, spawnOpts)
    if err != null { return null, err }
    return client, null
}


// close terminates the server gracefully (stdin EOF, 2s grace, then SIGKILL).
// Idempotent. Always returns null.
fn close(client) {
    _mcpClose(client)
    return null
}


// info returns a hash describing the connected server:
//   {name: string, version: string, protocol: string}
// All fields default to "" if the server didn't supply them.
fn info(client) {
    return _mcpInfo(client)
}


// notifications returns the kLex channel of server-initiated notifications.
// Each value is a hash {method: string, params: any}. The channel is closed
// automatically when the server exits. Drain it with recv() / recvNonBlock()
// in a polling loop; ignore it if you don't need pushed events.
fn notifications(client) {
    return _mcpNotifications(client)
}


// ── Generic RPC ───────────────────────────────────────────────────────────

// call issues an arbitrary JSON-RPC request. Most code should prefer the
// typed helpers below (listTools, callTool, listResources, readResource);
// use this when you need a method outside the standard MCP surface.
//
// timeoutSec is optional — pass null or omit to use DEFAULT_CALL_TIMEOUT_SEC.
fn call(client, method, params, timeoutSec) {
    if type(method) != "STRING" || len(method) == 0 {
        return null, error("MCP_BAD_OPTS", "call: method must be a non-empty string")
    }
    let t = DEFAULT_CALL_TIMEOUT_SEC
    if timeoutSec != null && (type(timeoutSec) == "INTEGER" || type(timeoutSec) == "FLOAT") {
        t = timeoutSec
    }
    return _mcpCall(client, method, params, t)
}


// notify fires a notification (no response expected). Returns (null, err).
fn notify(client, method, params) {
    if type(method) != "STRING" || len(method) == 0 {
        return null, error("MCP_BAD_OPTS", "notify: method must be a non-empty string")
    }
    return _mcpNotify(client, method, params)
}


// ── Tools surface ─────────────────────────────────────────────────────────

// listTools returns (tools, err). tools is an array of hashes; each entry
// has fields name, description, inputSchema (a JSON Schema). Returns an
// empty array if the server exposes no tools.
fn listTools(client) {
    let result, err = _mcpCall(client, "tools/list", null, DEFAULT_CALL_TIMEOUT_SEC)
    if err != null { return null, err }
    if type(result) != "HASH" || !hasKey(result, "tools") {
        return makeArray(0, null), null
    }
    return result["tools"], null
}


// callTool invokes a server tool by name with the given arguments hash.
// Returns the raw result hash, which conforms to the MCP spec:
//   {"content": [{"type": "text", "text": "..."}, ...], "isError"?: bool}
// Most callers want the convenience helper callToolText() that unpacks the
// first text item directly.
//
// timeoutSec is optional — null means DEFAULT_CALL_TIMEOUT_SEC.
fn callTool(client, name, toolArgs, timeoutSec) {
    if type(name) != "STRING" || len(name) == 0 {
        return null, error("MCP_BAD_OPTS", "callTool: name must be a non-empty string")
    }
    if toolArgs == null { toolArgs = {} }
    if type(toolArgs) != "HASH" {
        return null, error("MCP_BAD_OPTS", "callTool: arguments must be a hash or null")
    }
    let t = DEFAULT_CALL_TIMEOUT_SEC
    if timeoutSec != null && (type(timeoutSec) == "INTEGER" || type(timeoutSec) == "FLOAT") {
        t = timeoutSec
    }
    return _mcpCall(client, "tools/call",
                    {"name": name, "arguments": toolArgs},
                    t)
}


// callToolText is callTool() with the most common envelope unpacked: it
// extracts the text content of the first text-typed result item, so
// callers don't have to dig through result["content"][0]["text"].
//
// Returns (text, err). If the server marked the result as an error via
// "isError": true, the err is MCP_TOOL_ERROR with the text body as message.
// If the result contains no text item (e.g. only image content), err is
// MCP_NO_TEXT and value is the raw result hash for the caller to handle.
fn callToolText(client, name, toolArgs, timeoutSec) {
    let result, err = callTool(client, name, toolArgs, timeoutSec)
    if err != null { return null, err }
    if type(result) != "HASH" {
        return null, error("MCP_NO_TEXT", "callToolText: server returned non-hash result")
    }
    if hasKey(result, "isError") && result["isError"] == true {
        let msg = "tool reported an error"
        if hasKey(result, "content") && len(result["content"]) > 0 {
            let first = result["content"][0]
            if type(first) == "HASH" && hasKey(first, "text") {
                msg = first["text"]
            }
        }
        return null, error("MCP_TOOL_ERROR", msg)
    }
    if !hasKey(result, "content") || type(result["content"]) != "ARRAY" {
        return null, error("MCP_NO_TEXT", "callToolText: result missing content array")
    }
    let items = result["content"]
    let i = 0
    while i < len(items) {
        let it = items[i]
        if type(it) == "HASH" && hasKey(it, "type") && it["type"] == "text" {
            return it["text"], null
        }
        i = i + 1
    }
    return null, error("MCP_NO_TEXT", "callToolText: result has no text item")
}


// ── Resources surface ─────────────────────────────────────────────────────

// listResources returns (resources, err). resources is an array of hashes;
// each entry has uri, name, optional description and mimeType. Empty array
// if the server exposes no resources.
fn listResources(client) {
    let result, err = _mcpCall(client, "resources/list", null, DEFAULT_CALL_TIMEOUT_SEC)
    if err != null { return null, err }
    if type(result) != "HASH" || !hasKey(result, "resources") {
        return makeArray(0, null), null
    }
    return result["resources"], null
}


// readResource fetches the contents of a resource by URI. Returns the raw
// result hash from the server (typically {"contents": [{uri, mimeType, text|blob}]}).
fn readResource(client, uri) {
    if type(uri) != "STRING" || len(uri) == 0 {
        return null, error("MCP_BAD_OPTS", "readResource: uri must be a non-empty string")
    }
    return _mcpCall(client, "resources/read", {"uri": uri}, DEFAULT_CALL_TIMEOUT_SEC)
}


// ── Prompts surface ──────────────────────────────────────────────────────

// listPrompts returns server-defined prompt templates. Many MCP servers
// don't implement prompts; an unsupported-method error is returned for them.
fn listPrompts(client) {
    let result, err = _mcpCall(client, "prompts/list", null, DEFAULT_CALL_TIMEOUT_SEC)
    if err != null { return null, err }
    if type(result) != "HASH" || !hasKey(result, "prompts") {
        return makeArray(0, null), null
    }
    return result["prompts"], null
}


// getPrompt fetches a specific prompt template with arguments interpolated.
fn getPrompt(client, name, promptArgs) {
    if type(name) != "STRING" || len(name) == 0 {
        return null, error("MCP_BAD_OPTS", "getPrompt: name must be a non-empty string")
    }
    if promptArgs == null { promptArgs = {} }
    return _mcpCall(client, "prompts/get",
                    {"name": name, "arguments": promptArgs},
                    DEFAULT_CALL_TIMEOUT_SEC)
}
