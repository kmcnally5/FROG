// ollamaHello.lex — minimal local-LLM demo for stdlib/ai/ollama.lex.
//
// Run from the kLex project root. Requires Ollama running locally:
//   ollama serve        # starts the daemon (port 11434)
//   ollama pull llama3.2 # one-time, ~2GB
//   ./klex tests/examples/ai/ollamaHello.lex
//
// The model defaults to "llama3.2". Override via the OLLAMA_MODEL env var.

import "stdlib/ai/ollama.lex"    as ollama
import "stdlib/ai/ai_common.lex" as ai

let modelName = env("OLLAMA_MODEL")
if modelName == null || len(modelName) == 0 { modelName = "llama3.2" }

let c = ollama.newClient(env("OLLAMA_HOST"), modelName)

// Cheap reachability probe — listModels also tells us if the model we
// want to use is actually installed.
let models, lerr = ollama.listModels(c)
if lerr != null {
    println("ollamaHello: cannot reach Ollama — " + lerr.message)
    println("             start it with `ollama serve` and try again.")
    _osExit(1)
}
let installed = false
for m in models {
    if m["name"] == modelName || m["model"] == modelName { installed = true }
}
if !installed {
    println("ollamaHello: model `" + modelName + "` not installed locally.")
    println("             Run `ollama pull " + modelName + "` and try again.")
    println("             (or set OLLAMA_MODEL to one of these:)")
    for m in models {
        println("               - " + m["name"])
    }
    _osExit(1)
}

println("Asking " + modelName + ":  In one sentence, what is kLex?")
println("─────────────────────────────────────────────────────────")

let resp, err = ollama.chat(c, {
    "system":   "You are concise. Answer in one short sentence.",
    "messages": [ai.userMsg("In one sentence, what is kLex?")],
    "think":    false,                   // skip chain-of-thought for reasoning models
    "options":  {"temperature": 0.5},
})
if err != null {
    println("error: " + err.message + " (code: " + err.code + ")")
    _osExit(1)
}

println(ollama.textOf(resp))
println("─────────────────────────────────────────────────────────")

let u = ollama.usageOf(resp)
println("tokens: in=" + str(u["input_tokens"]) + " out=" + str(u["output_tokens"]))
println("done_reason: " + ollama.doneReasonOf(resp))
