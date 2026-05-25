// voyageImportTest.lex — parse/import smoke test for stdlib/ai/voyage.lex.
//
// Does NOT hit the Voyage network. Verifies the library imports cleanly,
// newClient() builds a sensible struct, _rateFor falls back safely on
// unknown models, and the local helpers extract data in the right shape.

import "stdlib/ai/voyage.lex"    as voyage
import "stdlib/ai/ai_common.lex" as ai


fn assertEq(label, got, want) {
    if got != want {
        println("FAIL " + label + ": got " + str(got) + " want " + str(want))
        exit(1)
    }
}


// ── newClient defaults ──────────────────────────────────────────────────
let c = voyage.newClient("test-key", null)
assertEq("default model",   c.model,   "voyage-3")
assertEq("apiKey carried",  c.apiKey,  "test-key")
assertEq("baseUrl set",     c.baseUrl, "https://api.voyageai.com/v1")

let c2 = voyage.newClient("k2", "voyage-code-3")
assertEq("explicit model honoured", c2.model, "voyage-code-3")

let c3 = voyage.newClient("k3", "")
assertEq("empty-string model falls back to default", c3.model, "voyage-3")


// ── _rateFor lookups ────────────────────────────────────────────────────
assertEq("known model rate",   voyage._rateFor("voyage-3"),      0.06)
assertEq("lite rate",          voyage._rateFor("voyage-3-lite"), 0.02)
assertEq("code rate",          voyage._rateFor("voyage-code-3"), 0.18)
assertEq("unknown falls back", voyage._rateFor("voyage-future"), 0.06)


// ── _extractEmbeddings handles voyage wire shape ────────────────────────
let wire = {
    "data": [
        {"embedding": [0.1, 0.2, 0.3], "index": 0},
        {"embedding": [0.4, 0.5, 0.6], "index": 1},
    ],
    "model": "voyage-3",
    "usage": {"total_tokens": 12},
}
let flat = voyage._extractEmbeddings(wire)
assertEq("flattened length",       len(flat),   2)
assertEq("first vector preserved", flat[0][0],  0.1)
assertEq("second vector preserved", flat[1][2], 0.6)


// ── shape matches ollama.embed: resp["embeddings"][i] is a float array ──
// Simulate what embedWith() returns on success so consumers can rely on
// the shape contract without a live API call.
let simulated = {
    "embeddings": flat,
    "model":      "voyage-3",
    "usage":      {"input_tokens": 12, "output_tokens": 0, "cost_usd": null},
}
assertEq("vector_store-compatible shape", len(simulated["embeddings"]), 2)
assertEq("usageOf passthrough",           voyage.usageOf(simulated)["input_tokens"], 12)
assertEq("embeddingsOf one-liner",        len(voyage.embeddingsOf(simulated)), 2)


// ── budget pre-flight is honoured ────────────────────────────────────────
ai.resetUsage()
ai.budget(0.0001)
ai._record("seed", 1000, 0, 1.0, 0.0)   // forces spent above cap
let resp, err = voyage.embed(c, "this should be blocked by the cap")
assertEq("budget pre-flight blocks call", err.code, "BUDGET_EXCEEDED")
ai.resetUsage()
ai.budget(null)


println("PASS voyageImportTest")
