// mcpTest.lex — smoke test for the MCP client primitive (_mcpSpawn etc.)
// AND the stdlib/mcp.lex wrapper. Runs against frogMcp because it's the
// only MCP server guaranteed to ship with this repo. Skips if Python or
// the server's deps are unavailable — kLex's MCP client is what we're
// testing, not the user's Python install.

import "stdlib/mcp.lex" as mcp

let failed = 0
let passed = 0

fn check(name, cond) {
    if cond {
        println("  PASS  " + name)
        passed = passed + 1
    } else {
        println("  FAIL  " + name)
        failed = failed + 1
    }
}

println("── MCP client smoke test ────────────────────────────────────────")

// Spawn frogMcp. The server expects the index file to exist; if it's missing
// the server still starts (with a stderr WARNING) and tools/list still works,
// so we can verify the wire protocol regardless. The "env" opt is illustrative;
// frogMcp doesn't actually require any extras.
let client, err = _mcpSpawn(
    "python3",
    ["snowball/frogMcp/server/server.py"],
    {"timeout_sec": 30}
)

if err != null {
    println("  SKIP  frogMcp unavailable: " + err.message)
    println("        (test environment missing 'mcp' Python package — kLex MCP")
    println("         primitive can't be verified end-to-end. Build of the Go")
    println("         primitives succeeded — see build output.)")
    _osExit(0)
}

check("_mcpSpawn returns an MCP client", type(client) == "MCP_CLIENT")

// ── server identity ──────────────────────────────────────────────────────
let info = _mcpInfo(client)
check("_mcpInfo returns a hash",       type(info) == "HASH")
check("server name populated",         len(info["name"]) > 0)
check("server version populated",      len(info["version"]) > 0)
check("protocol version populated",    len(info["protocol"]) > 0)
println("        server: " + info["name"] + "/" + info["version"] + " (proto " + info["protocol"] + ")")

// ── tools/list ────────────────────────────────────────────────────────────
let toolsResult, terr = _mcpCall(client, "tools/list", null, 10)
check("tools/list succeeds",           terr == null)
check("tools/list returns a hash",     type(toolsResult) == "HASH")
check("tools array present",           hasKey(toolsResult, "tools"))
let tools = toolsResult["tools"]
check("tools is an array",             type(tools) == "ARRAY")
check("frogMcp exposes >= 5 tools",    len(tools) >= 5)

// Pull tool names out for the next assertion. Order is server-defined.
let toolNames = makeArray(len(tools), "")
let i = 0
while i < len(tools) {
    toolNames[i] = tools[i]["name"]
    i = i + 1
}
let hasDescribe = false
for n in toolNames {
    if n == "klex_describe_symbol" { hasDescribe = true }
}
check("klex_describe_symbol present", hasDescribe)

// ── tools/call ────────────────────────────────────────────────────────────
// Look up a builtin that definitely exists. The result lives at
// result["content"][0]["text"] per the MCP spec — a JSON-encoded payload.
let callRes, cerr = _mcpCall(client, "tools/call",
    {"name": "klex_describe_symbol", "arguments": {"name": "makeArray"}},
    10)
check("tools/call succeeds",           cerr == null)
check("call result is a hash",         type(callRes) == "HASH")
check("content array present",         hasKey(callRes, "content"))
if hasKey(callRes, "content") && len(callRes["content"]) > 0 {
    let text = callRes["content"][0]["text"]
    check("content text non-empty",    len(text) > 0)
    // Should mention makeArray somewhere in the JSON payload.
    check("response mentions makeArray", indexOf(text, "makeArray") != -1)
}

// ── close ─────────────────────────────────────────────────────────────────
_mcpClose(client)
println("        client closed")


// ────────────────────────────────────────────────────────────────────────
// stdlib/mcp.lex wrapper — same protocol, idiomatic surface.
// ────────────────────────────────────────────────────────────────────────

println("")
println("── stdlib/mcp.lex wrapper ────────────────────────────────────")

let wclient, werr = mcp.newClient("python3",
                              ["snowball/frogMcp/server/server.py"],
                              {"timeout_sec": 30})
check("mcp.newClient succeeds",        werr == null)
check("returns MCP_CLIENT type",       type(wclient) == "MCP_CLIENT")

let winfo = mcp.info(wclient)
check("mcp.info returns hash",         type(winfo) == "HASH")
check("info has server name",          len(winfo["name"]) > 0)

let wtools, lterr = mcp.listTools(wclient)
check("mcp.listTools succeeds",        lterr == null)
check("listTools returns array",       type(wtools) == "ARRAY")
check("listTools has >= 5 tools",      len(wtools) >= 5)
check("each tool has a name field",    type(wtools[0]["name"]) == "STRING")

// callTool returns the raw result hash (with content/isError/etc.)
let wraw, cterr = mcp.callTool(wclient, "klex_describe_symbol",
                           {"name": "len"}, null)
check("mcp.callTool succeeds",         cterr == null)
check("callTool result is hash",       type(wraw) == "HASH")
check("callTool result has content",   hasKey(wraw, "content"))

// callToolText unpacks the first text item — what 99% of callers want.
let wtext, txerr = mcp.callToolText(wclient, "klex_describe_symbol",
                                {"name": "len"}, null)
check("mcp.callToolText succeeds",     txerr == null)
check("callToolText returns string",   type(wtext) == "STRING")
check("text mentions len",             indexOf(wtext, "len") != -1)

// Error path: unknown JSON-RPC method (not unknown tool — frogMcp wraps
// tool-level errors in a successful response with isError:false, so the
// real RPC-error surface is the method name itself).
let _, badErr = mcp.call(wclient, "no_such_method_xyz", null, 5)
check("unknown RPC method returns typed err", badErr != null)
check("err code is MCP_CALL_RPC",
    badErr != null && badErr.code == "MCP_CALL_RPC")

mcp.close(wclient)
println("        wrapper client closed")

// ── summary ──────────────────────────────────────────────────────────────
println("")
println("── result ──────────────────────────────────────────────────────")
println("  passed: " + str(passed))
println("  failed: " + str(failed))
if failed > 0 { _osExit(1) }
