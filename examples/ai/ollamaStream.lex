// ollamaStream.lex — streaming local-LLM demo for stdlib/ai/ollama.lex.
//
// Same shape as anthropicStream.lex but talks to a local Ollama daemon.
// Backed by _httpStream("...", "lines") for NDJSON parsing — Ollama
// streams newline-delimited JSON, not SSE.
//
// Run:
//   ollama serve            # if not already
//   ollama pull llama3.2    # one-time
//   ./klex tests/examples/ai/ollamaStream.lex

import "stdlib/ai/ollama.lex"    as ollama
import "stdlib/ai/ai_common.lex" as ai

let modelName = env("OLLAMA_MODEL")
if modelName == null || len(modelName) == 0 { modelName = "gemma4:latest" }

let c = ollama.newClient(env("OLLAMA_HOST"), modelName)

// Cheap reachability check — also surfaces "Ollama not running" cleanly.
let _, lerr = ollama.listModels(c)
if lerr != null {
    println("ollamaStream: cannot reach Ollama — " + lerr.message)
    _osExit(1)
}

println("Streaming a short poem from " + modelName + "…")
println("──────────────────────────────────────────────")

let ch, err = ollama.stream(c, {
    "system":   "You write concise, evocative poetry.",
    "messages": [ai.userMsg(
        "Write a four-line poem about a programming language called kLex " +
        "that combines clarity, concurrency, and the playful spirit of a frog."
        )],
    "think":   false,                   // skip chain-of-thought (qwen3, r1, …) for a tight demo
    "options": {"num_predict": 400},
})
if err != null {
    println("stream error: " + err.message)
    _osExit(1)
}

let fullText      = ""
let thinkingChars = 0
let doneReason    = ""
let usage         = null

for evt in ch {
    if evt["type"] == "text" {
        print(evt["text"])
        fullText = fullText + evt["text"]
    } else if evt["type"] == "thinking" {
        // Reasoning models (qwen3, deepseek-r1, …) emit chain-of-thought
        // in `thinking` before the answer. Just print a dim indicator so
        // the user knows something is happening without dumping the trace.
        thinkingChars = thinkingChars + len(evt["text"])
        print(".")
    } else if evt["type"] == "done" {
        if hasKey(evt, "done_reason") { doneReason = evt["done_reason"] }
        if hasKey(evt, "usage")       { usage      = evt["usage"] }
    } else if evt["type"] == "error" {
        println("")
        println("⚠ " + evt["message"])
    }
}

println("")
println("──────────────────────────────────────────────")
println("done_reason:     " + doneReason)
println("answer chars:    " + str(len(fullText)))
if thinkingChars > 0 {
    println("thinking chars:  " + str(thinkingChars) + " (chain-of-thought, hidden)")
}
if usage != null {
    if hasKey(usage, "input_tokens")  { println("input tokens:    " + str(usage["input_tokens"])) }
    if hasKey(usage, "output_tokens") { println("output tokens:   " + str(usage["output_tokens"])) }
}
