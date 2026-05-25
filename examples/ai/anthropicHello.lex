// anthropicHello.lex — minimal "hello, Claude" demo for stdlib/ai/anthropic.lex.
//
// Run from the kLex project root with your Anthropic API key in the env:
//   ANTHROPIC_API_KEY=sk-ant-… ./klex tests/examples/ai/anthropicHello.lex
//
// Gracefully skips (no error, exit 0) if ANTHROPIC_API_KEY is unset — so
// this file is safe to run as part of a wider test suite without burning
// credits unless the caller deliberately opted in.

import "stdlib/ai/anthropic.lex" as claude
import "stdlib/ai/ai_common.lex" as ai

let apiKey = env("ANTHROPIC_API_KEY")
if apiKey == null || len(apiKey) == 0 {
    println("anthropicHello: ANTHROPIC_API_KEY not set — skipping.")
    println("                Set it in your shell to run this demo:")
    println("                  export ANTHROPIC_API_KEY=sk-ant-…")
    _osExit(0)
}

// Construct a client. Pass null for model to use the library default
// (DEFAULT_MODEL = claude-opus-4-7); pass a specific model id to override.
let c = claude.newClient(apiKey, null)

// Single-shot Q&A. opts is a hash; only "messages" is required.
let resp, err = claude.messages(c, {
    "system":     "You are concise. Answer in one short sentence.",
    "max_tokens": 256,
    "messages":   [ai.userMsg("In one sentence, what is kLex?")],
})
if err != null {
    println("error: " + err.message + " (code: " + err.code + ")")
    _osExit(1)
}

// Pull the assistant's text out of the response content blocks.
println("Claude says:")
println("  " + claude.textOf(resp))

// And token usage for the call.
let u = claude.usageOf(resp)
println("")
println("tokens: in=" + str(u["input_tokens"]) + " out=" + str(u["output_tokens"]))
println("stop reason: " + claude.stopReasonOf(resp))
