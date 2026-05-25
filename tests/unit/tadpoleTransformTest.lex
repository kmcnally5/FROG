// tadpoleTransformTest.lex — smoke test for the tadPole transform MCP tool.
//
// The real handler in snowball/tadPole/tadPole.lex calls into ollama, which
// can't run in CI without a live daemon. This test mirrors the validation
// branches of that handler in a local stub, then exercises _mcpServeHTTP
// with the same schema + handler shape on a side-channel port to prove the
// MCP wiring accepts what tadpole registers.
//
// End-to-end testing with a real ollama backend happens by running tadpole
// and calling mcp__tadpole__transform from any MCP client.

let TRANSFORM_MAX_INPUT_BYTES = 32768

// Mirror of _mcpToolTransform's validation + success shape. Real handler
// makes the ollama call; this stub returns a deterministic success so we
// can lock the response keys.
fn stubTransform(args) {
    if !hasKey(args, "input") || type(args["input"]) != "STRING" {
        return {"ok": false, "error": "input is required (string)"}
    }
    if !hasKey(args, "instruction") || type(args["instruction"]) != "STRING" {
        return {"ok": false, "error": "instruction is required (string)"}
    }
    if len(args["input"]) > TRANSFORM_MAX_INPUT_BYTES {
        return {"ok": false, "error": "input too large (max 32 KB)"}
    }
    return {"ok": true, "output": "[stub] " + args["instruction"], "model": "stub-model", "elapsed_ms": 0}
}

let failed = 0

// ── Validation branches ────────────────────────────────────────────────────

let r1 = stubTransform({})
if r1["ok"] != false { failed = failed + 1  println("FAIL: empty args should return ok=false") }

let r2 = stubTransform({"input": "hello"})
if r2["ok"] != false { failed = failed + 1  println("FAIL: missing instruction should return ok=false") }

let r3 = stubTransform({"instruction": "tighten"})
if r3["ok"] != false { failed = failed + 1  println("FAIL: missing input should return ok=false") }

let r4 = stubTransform({"input": "hello", "instruction": 42})
if r4["ok"] != false { failed = failed + 1  println("FAIL: non-string instruction should return ok=false") }

// Build a 64 KB string via doubling — comfortably over the 32 KB cap.
let big = "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
let i = 0
while i < 10 {
    big = big + big
    i   = i + 1
}
let r5 = stubTransform({"input": big, "instruction": "trim"})
if r5["ok"] != false { failed = failed + 1  println("FAIL: oversized input should return ok=false") }

// ── Success shape ──────────────────────────────────────────────────────────

let r6 = stubTransform({"input": "hello world", "instruction": "uppercase"})
if r6["ok"] != true    { failed = failed + 1  println("FAIL: valid args should return ok=true") }
if !hasKey(r6, "output")     { failed = failed + 1  println("FAIL: success response missing 'output'") }
if !hasKey(r6, "model")      { failed = failed + 1  println("FAIL: success response missing 'model'") }
if !hasKey(r6, "elapsed_ms") { failed = failed + 1  println("FAIL: success response missing 'elapsed_ms'") }

// ── MCP wiring ─────────────────────────────────────────────────────────────
// Register the transform schema + handler shape with _mcpServeHTTP on an
// unused port. Catches typos in tool spec keys, bad handler signature, or
// schema rejection that would only show up at tadpole startup otherwise.

let srv, srvErr = _mcpServeHTTP({
    "name":    "tadpole-transform-test",
    "version": "0.0.1",
    "port":    9778,
    "tools": {
        "transform": {
            "description": "stub transform for wiring smoke test",
            "schema": {
                "type": "object",
                "properties": {
                    "input":       {"type": "string"},
                    "instruction": {"type": "string"},
                    "max_tokens":  {"type": "integer"},
                    "temperature": {"type": "number"},
                },
                "required": ["input", "instruction"],
            },
            "handler": stubTransform,
        },
    },
})
if srvErr != null {
    failed = failed + 1
    println("FAIL: MCP server should start, got: " + srvErr.message)
}
if srv != null {
    _mcpStopServer(srv)
}

if failed == 0 {
    println("tadpoleTransformTest: PASS")
} else {
    println("tadpoleTransformTest: " + str(failed) + " FAILURE(S)")
}
