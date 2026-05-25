// askDocumentDemo.lex — RAG over kLex's own documentation.
//
// Solves the "Claude doesn't know kLex" gap that quickDemo.lex surfaced.
// Reads docs/KLEX_LANGUAGE.TXT, chunks + embeds it locally via Ollama,
// then answers questions grounded in the retrieved excerpts.
//
// Requirements:
//   - Ollama running (`ollama serve`)
//   - Embedding model pulled:  ollama pull mxbai-embed-large
//   - For the Claude path:     export ANTHROPIC_API_KEY=sk-ant-…
//     (or comment out the useClaude() line below to answer with Ollama
//     using your active chat model — set OLLAMA_HOST / KLEX_AI_MODEL
//     if you want overrides)
//
// Run from the kLex project root:
//   ANTHROPIC_API_KEY=sk-ant-… ./klex tests/examples/ai/askDocumentDemo.lex

import "stdlib/ai/quick.lex" as ai


// Default to Claude for the answer. If your key isn't set, switch to
// Ollama (whatever model you've configured) — embeddings still come from
// the local mxbai-embed-large either way.
if env("ANTHROPIC_API_KEY") == null || len(env("ANTHROPIC_API_KEY")) == 0 {
    println("(no ANTHROPIC_API_KEY — answering with Ollama chat model)")
    ai.useOllama(null)        // null → use KLEX_AI_MODEL or "llama3.2" default
}

let DOC = "docs/KLEX_LANGUAGE.TXT"

let questions = [
    "In one sentence, what is kLex?",
    "What are the strict typing rules?",
    "How does the async model work — do tasks share state with the caller?",
    "What does the `?` postfix operator do?",
]

println("=== askDocument over " + DOC + " ===")
println("(provider: " + ai.currentProvider() + ")")
println("")

for q in questions {
    println("Q: " + q)
    let start = _timeNanos()
    let ans, err = ai.askDocument(DOC, q)
    let elapsedMs = (_timeNanos() - start) / 1000000
    if err != null {
        println("   ERR (" + err.code + "): " + err.message)
        // Bail loud on the first error rather than silently spamming all 4.
        if err.code == "AI_EMBED_MODEL_MISSING" || err.code == "AI_EMBED_OFFLINE" {
            _osExit(1)
        }
    } else {
        println("A (" + str(elapsedMs) + "ms): " + ans)
    }
    println("")
}

println("documents cached: " + str(ai.documentCacheSize()))
println("(repeat questions over the same doc reuse the chunk+embed work)")
