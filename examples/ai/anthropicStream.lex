// anthropicStream.lex — streaming chat demo for stdlib/ai/anthropic.lex.
//
// Demonstrates the channel-based streaming API: tokens arrive on the
// channel as Claude generates them, so the response appears live in the
// terminal instead of all-at-once after the full generation completes.
//
// Run from the kLex project root with your API key set:
//   ANTHROPIC_API_KEY=sk-ant-… ./klex tests/examples/ai/anthropicStream.lex
//
// Cleanly skips if the env var is absent.

import "stdlib/ai/anthropic.lex" as claude
import "stdlib/ai/ai_common.lex" as ai

let apiKey = env("ANTHROPIC_API_KEY")
if apiKey == null || len(apiKey) == 0 {
    println("anthropicStream: ANTHROPIC_API_KEY not set — skipping.")
    _osExit(0)
}

let c = claude.newClient(apiKey, null)

println("Asking Claude to write a short poem about kLex…")
println("───────────────────────────────────────────────")

// Open the stream — returns immediately with a channel; the HTTP request
// is held open in the background and tokens arrive as Anthropic emits them.
let ch, err = claude.stream(c, {
    "system":     "You write concise, evocative poetry.",
    "max_tokens": 400,
    "messages":   [ai.userMsg(
        "Write a four-line poem about a programming language called kLex " +
        "that combines clarity, concurrency, and the playful spirit of a frog."
    )],
})
if err != null {
    println("stream error: " + err.message)
    _osExit(1)
}

// Accumulate text + remember the final stop_reason / usage. This is the
// canonical streaming-consumer pattern: print deltas live, capture final
// state when the "done" event arrives.
let fullText   = ""
let stopReason = ""
let usage      = null

for evt in ch {
    if evt["type"] == "text" {
        // Print incrementally — no newline so chunks join visually.
        print(evt["text"])
        fullText = fullText + evt["text"]
    } else if evt["type"] == "done" {
        if hasKey(evt, "stop_reason") { stopReason = evt["stop_reason"] }
        if hasKey(evt, "usage")       { usage      = evt["usage"] }
    } else if evt["type"] == "error" {
        println("")
        println("⚠ stream error: " + evt["message"])
    }
    // tool_use / tool_input not used in this demo
}

println("")
println("───────────────────────────────────────────────")
println("stop_reason: " + stopReason)
println("chars received: " + str(len(fullText)))
if usage != null && hasKey(usage, "output_tokens") {
    println("output tokens: " + str(usage["output_tokens"]))
}
