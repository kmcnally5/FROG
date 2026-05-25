// mcp_server.lex — Model Context Protocol SERVER for kLex.
// @module    mcp_server
// @version   0.1.0
// @since     klex 0.3.36
// @author    karl
// @summary   Expose a kLex program as an MCP server (HTTP+SSE).
//
// Companion to stdlib/mcp.lex (the CLIENT side). Where that module lets a
// kLex program TALK to an MCP server, this module lets a kLex program BE
// an MCP server — so any MCP client (Claude Code, Claude Desktop, the
// MCP Inspector, your own scripts) can drive it.
//
// Transport: HTTP + SSE (Server-Sent Events). One GET endpoint opens a
// session stream; messages are POSTed back. Standard MCP wire protocol
// (2025-03-26 spec).
//
// Threading caveat — READ THIS:
// Tool handlers run on the HTTP server's goroutines, NOT on the kLex
// program's main thread. This means:
//   • Concurrent MCP requests are serialised (one tool at a time)
//   • The kLex main thread (e.g. a UI loop) is NOT synchronised with
//     tool calls. If your tool mutates shared state, use kLex's
//     concurrency primitives (atomicHash, channels) for safety.
// For most tools — query-shaped or self-contained — this is invisible.
// For tools that touch UI state from a render loop, design carefully.
//
// All functions return (value, err) tuples. err is null on success;
// otherwise err.code names the failure class and err.message has prose.
//
// Error codes:
//   MCP_SERVER_BAD_SPEC      — caller-side validation failure
//   MCP_SERVER_BIND_FAILED   — port already in use, permission denied
//
// Usage:
//   import "stdlib/mcp_server.lex" as mcpd
//
//   fn echo(args) {
//       return {"you_sent": args}
//   }
//
//   let srv, err = mcpd.serveHTTP({
//       "name":    "my-klex-app",
//       "version": "0.1.0",
//       "port":    7777,
//       "tools": {
//           "echo": {
//               "description": "Echo whatever you send",
//               "schema":      {"type": "object"},
//               "handler":     echo,
//           },
//       },
//   })
//   if err != null { println(err.message)  exit(1) }
//
//   // ...run your program; the server lives on background goroutines...
//
//   mcpd.stop(srv)


// ── Constants ─────────────────────────────────────────────────────────────

// Default port if the caller omits one. 7777 is unused by any standard
// service and easy to remember; we never silently pick a random port
// because that defeats the purpose of advertising a stable endpoint.
const DEFAULT_PORT = 7777


// ── serveHTTP ─────────────────────────────────────────────────────────────

// serveHTTP starts the MCP server on the given port and returns
// immediately. The server runs on background goroutines until stop() is
// called or the kLex process exits.
//
// spec hash shape:
//   "name":    string  (required) — server identity reported via initialize
//   "version": string  (required) — server version reported via initialize
//   "port":    int     (optional, default 7777) — TCP port on 127.0.0.1
//   "tools":   hash    (optional) — tool name → {description, schema, handler}
//
// Each tool's entry is itself a hash:
//   "description": string         — human prose shown to the MCP client
//   "schema":      hash           — JSON Schema for the tool's arguments
//   "handler":     fn(args)       — kLex callable invoked on tools/call
//
// The handler receives the client-supplied arguments as a single hash
// argument and must return a JSON-serialisable kLex value (hash, array,
// string, number, bool, null). To report a tool-level failure, return
// an Error object — it will be sent as isError:true on the MCP side.
fn serveHTTP(spec) {
    if type(spec) != "HASH" {
        return null, error("MCP_SERVER_BAD_SPEC",
                           "serveHTTP: spec must be a hash")
    }
    if !hasKey(spec, "name") || type(spec["name"]) != "STRING" {
        return null, error("MCP_SERVER_BAD_SPEC",
                           "serveHTTP: spec.name must be a string")
    }
    if !hasKey(spec, "version") || type(spec["version"]) != "STRING" {
        return null, error("MCP_SERVER_BAD_SPEC",
                           "serveHTTP: spec.version must be a string")
    }
    if !hasKey(spec, "port") {
        spec["port"] = DEFAULT_PORT
    }
    if type(spec["port"]) != "INTEGER" {
        return null, error("MCP_SERVER_BAD_SPEC",
                           "serveHTTP: spec.port must be an integer")
    }
    return _mcpServeHTTP(spec)
}


// ── stop ──────────────────────────────────────────────────────────────────

// stop gracefully shuts down a running MCP server. Closes the listener,
// drops every active SSE session, and waits up to 5s for in-flight tool
// calls to finish. Idempotent — calling twice on the same server is
// harmless.
fn stop(server) {
    if type(server) != "MCP_SERVER" {
        return error("MCP_SERVER_BAD_SPEC",
                     "stop: argument must be an MCP server handle")
    }
    return _mcpStopServer(server)
}


// ── info ──────────────────────────────────────────────────────────────────

// info returns a snapshot of the server's current state — useful for
// logging, status pages, or tests. Hash keys: name, version, port,
// tools (array of tool names), stopped (bool).
fn info(server) {
    if type(server) != "MCP_SERVER" {
        return null, error("MCP_SERVER_BAD_SPEC",
                           "info: argument must be an MCP server handle")
    }
    return _mcpServerInfo(server), null
}
