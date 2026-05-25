// anthropicBatch.lex — Message Batches demo for stdlib/ai/anthropic.lex.
//
// Anthropic's Message Batches API costs 50% of the regular Messages API.
// Submit up to 100,000 requests in one batch, poll until done, then fetch
// the JSONL results. Ideal for evals and dataset generation.
//
// This demo:
//   1. Builds 3 small prompts as a batch
//   2. Submits and waits with a progress callback
//   3. Prints each request's outcome + the assistant's reply
//
// Run from the kLex project root with your Anthropic API key in the env:
//   ANTHROPIC_API_KEY=sk-ant-… ./klex tests/examples/ai/anthropicBatch.lex
//
// Skips cleanly (exit 0) without the env var so this is safe to include in
// a broader test sweep without burning credits unless explicitly opted in.

import "stdlib/ai/anthropic.lex" as claude
import "stdlib/ai/ai_common.lex" as ai


let apiKey = env("ANTHROPIC_API_KEY")
if apiKey == null || len(apiKey) == 0 {
    println("anthropicBatch: ANTHROPIC_API_KEY not set — skipping.")
    println("                Set it in your shell to run this demo:")
    println("                  export ANTHROPIC_API_KEY=sk-ant-…")
    _osExit(0)
}

let c = claude.newClient(apiKey, null)

// ── 1. Build the requests ─────────────────────────────────────────────────

let prompts = [
    "In one short sentence, what is photosynthesis?",
    "In one short sentence, what is a black hole?",
    "In one short sentence, what is recursion?",
]

let reqs = makeArray(len(prompts), null)
for i, p in prompts {
    reqs[i] = claude.buildBatchRequest("req-" + str(i), {
        "messages":   [ai.userMsg(p)],
        "max_tokens": 128,
        "system":     "Answer concisely.",
    })
}

println("Submitting batch of " + str(len(reqs)) + " requests…")


// ── 2. Submit ─────────────────────────────────────────────────────────────

let batch, err = claude.createBatch(c, reqs)
if err != null {
    println("createBatch error: " + err.message + " (code: " + err.code + ")")
    _osExit(1)
}
let batchId = batch["id"]
println("Batch ID:           " + batchId)
println("Initial status:     " + batch["processing_status"])
println("Expires:            " + batch["expires_at"])
println("")


// ── 3. Wait — with a progress callback ───────────────────────────────────

println("Polling until done (Anthropic recommends ≥30s between polls)…")

let final, err = claude.awaitBatch(c, batchId, {
    "pollIntervalMs": 30000,
    "deadlineMs":     900000,        // 15 minutes — adjust for larger batches
    "onPoll": fn(s) {
        let rc = s["request_counts"]
        let proc = rc["processing"]
        let succ = rc["succeeded"]
        let errd = rc["errored"]
        println("  status=" + s["processing_status"] +
                " processing=" + str(proc) +
                " succeeded=" + str(succ) +
                " errored=" + str(errd))
    },
})
if err != null {
    println("awaitBatch error: " + err.message)
    _osExit(1)
}
println("")
println("Batch ended.  Final status: " + final["processing_status"])


// ── 4. Fetch + render results ────────────────────────────────────────────

let results, err = claude.batchResults(c, batchId)
if err != null {
    println("batchResults error: " + err.message)
    _osExit(1)
}

println("")
println("Results:")
for r in results {
    let cid     = r["custom_id"]
    let outcome = r["result"]
    let kind    = outcome["type"]

    if kind == "succeeded" {
        let text = claude.textOf(outcome["message"])
        println("  " + cid + "  ✓  " + text)
    } else if kind == "errored" {
        let e = outcome["error"]
        let msg = ""
        if type(e) == "HASH" && hasKey(e, "message") { msg = e["message"] }
        println("  " + cid + "  ✗  errored: " + msg)
    } else {
        println("  " + cid + "  ·  " + kind)
    }
}

// Approximate cost note for context — Anthropic bills at 50% of the
// per-token rate for batch requests. The exact spend lands on your usage
// dashboard.
println("")
println("Note: batch requests are billed at 50% of the regular Messages rate.")
