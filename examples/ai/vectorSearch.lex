// vectorSearch.lex — semantic search demo using stdlib/ai/vector_store.lex.
//
// Builds a small corpus, embeds each entry with Ollama, then ranks the
// corpus by cosine similarity to a query. Showcases the new vector_store
// stdlib + the cosineSim Go builtin.
//
// Run:
//   ollama pull mxbai-embed-large   # one-time, ~670 MB
//   ollama serve                    # if not already
//   ./klex tests/examples/ai/vectorSearch.lex
//
// Override the embedding model with the OLLAMA_EMBED_MODEL env var.

import "stdlib/ai/ollama.lex"       as ollama
import "stdlib/ai/vector_store.lex" as vs

let embedModelName = env("OLLAMA_EMBED_MODEL")
if embedModelName == null || len(embedModelName) == 0 {
    embedModelName = "mxbai-embed-large"
}

let c = ollama.newClient(env("OLLAMA_HOST"), embedModelName)

// Verify the embedding model is installed.
let models, lerr = ollama.listModels(c)
if lerr != null {
    println("vectorSearch: cannot reach Ollama — " + lerr.message)
    _osExit(1)
}
let found = false
for m in models {
    if m["name"] == embedModelName || m["model"] == embedModelName { found = true }
}
if !found {
    println("vectorSearch: embedding model `" + embedModelName + "` is not installed.")
    println("              Pull it with:")
    println("                ollama pull " + embedModelName)
    println("              Or set OLLAMA_EMBED_MODEL to a model you have installed.")
    _osExit(1)
}


// ── Build a small corpus ──────────────────────────────────────────────────

let corpus = [
    "The quick brown fox jumps over the lazy dog.",
    "A swift fox leaps over a sleeping canine.",
    "The lazy dog sleeps in the afternoon sun.",
    "Frogs are amphibians that lay eggs in water.",
    "kLex is a strict, explicit scripting language built from scratch in Go.",
    "Python is a popular general-purpose interpreted language.",
    "Channels in kLex let concurrent tasks pass values safely.",
    "Cosine similarity ranks vectors by the angle between them.",
    "RAG retrieves relevant context before asking an LLM.",
    "Cats prefer warm windowsills and unattended keyboards.",
]

println("Embedding " + str(len(corpus)) + " documents with `" + embedModelName + "`…")
let store = vs.newStore()
let i = 0
while i < len(corpus) {
    let resp, eerr = ollama.embed(c, corpus[i])
    if eerr != null {
        println("embed failed for doc " + str(i) + ": " + eerr.message)
        _osExit(1)
    }
    let vector = resp["embeddings"][0]
    store.add("doc" + str(i), vector, {"text": corpus[i]})
    i = i + 1
}
println("Done — store has " + str(store.count()) + " entries.")
println("")


// ── Query ─────────────────────────────────────────────────────────────────

fn askAndPrint(query, k) {
    println("─── query: \"" + query + "\" ───")
    let qResp, qerr = ollama.embed(c, query)
    if qerr != null {
        println("  embed failed: " + qerr.message)
        return
    }
    let qv = qResp["embeddings"][0]
    let hits = store.search(qv, k)
    for h in hits {
        let scoreStr = substr(str(h["score"] + 0.00001), 0, 6)
        println("  " + scoreStr + "  " + h["metadata"]["text"])
    }
    println("")
}

askAndPrint("fast animal hops over a tired pet",     3)
askAndPrint("programming language with concurrency",  3)
askAndPrint("how does RAG work",                      3)
askAndPrint("amphibian biology",                      2)
