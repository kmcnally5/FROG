// rag.lex — retrieval-augmented generation demo.
//
// Combines:
//   - stdlib/ai/vector_store.lex (semantic retrieval)
//   - stdlib/ai/ollama.lex (embedding + chat)
//
// Pattern: index a small knowledge base, embed the user's question,
// retrieve the top-K most relevant facts, build a grounded prompt with
// those facts as context, ask the chat model to answer using ONLY the
// retrieved facts. The classic RAG pipeline in ~80 lines of kLex.
//
// Run:
//   ollama pull mxbai-embed-large   # embedding model (one-time)
//   ollama pull qwen3.5             # any chat model works
//   ./klex tests/examples/ai/rag.lex
//
// Env overrides:
//   OLLAMA_EMBED_MODEL — defaults to mxbai-embed-large
//   OLLAMA_MODEL       — defaults to qwen3.5:latest

import "stdlib/ai/ollama.lex"       as ollama
import "stdlib/ai/ai_common.lex"    as ai
import "stdlib/ai/vector_store.lex" as vs

embedModelName = env("OLLAMA_EMBED_MODEL")
if embedModelName == null || len(embedModelName) == 0 {
    embedModelName = "mxbai-embed-large"
}
chatModelName = env("OLLAMA_MODEL")
if chatModelName == null || len(chatModelName) == 0 {
    chatModelName = "qwen3.5:latest"
}

embedClient = ollama.newClient(env("OLLAMA_HOST"), embedModelName)
chatClient  = ollama.newClient(env("OLLAMA_HOST"), chatModelName)

// Confirm both models are installed.
models, lerr = ollama.listModels(embedClient)
if lerr != null {
    println("rag: cannot reach Ollama — " + lerr.message)
    _osExit(1)
}
have = {}
for m in models { have[m["name"]] = true }
missing = makeArray(0)
if !hasKey(have, embedModelName) { missing = concat(missing, [embedModelName]) }
if !hasKey(have, chatModelName)  { missing = concat(missing, [chatModelName]) }
if len(missing) > 0 {
    println("rag: missing model(s) — pull each:")
    for m in missing { println("       ollama pull " + m) }
    _osExit(1)
}


// ── Knowledge base ────────────────────────────────────────────────────────
// A small set of facts about kLex that the chat model wouldn't know from
// its training data. RAG should let the model answer questions about
// kLex correctly by grounding its responses in retrieved facts.

facts = [
    "kLex is a strict-typed, explicit scripting language implemented in Go.",
    "kLex was built from scratch as a learning project — lexer, parser, AST, evaluator, environment.",
    "kLex's concurrency model uses channels, async/await, and an actor pattern.",
    "kLex programs run as .lex files via the ./klex interpreter binary.",
    "The kLex stdlib lives in stdlib/ and is loaded via the KLEX_PATH env var or co-located paths.",
    "kLex has first-class graphics builtins: window, button, slider, image, charts, all in eval/builtins_*.go.",
    "kLex's AI library set ships anthropic.lex (Claude) and ollama.lex (local LLMs) under stdlib/ai/.",
    "kLex's structured-output API uses stdlib/ai/schema.lex to build JSON Schemas declaratively.",
    "kLex bridges to Python and Node via a JSON-over-stdin protocol with capability handshakes.",
    "kLex strings use \{expr} interpolation; raw strings use backticks; .ttc fonts don't work — only .ttf/.otf.",
    "kLex uses strict typing: no implicit coercion, no integer truthiness, null != T is always false.",
    "kLex was nicknamed FROG: Functional, Reactive, Opinionated, Go.",
    "Tadpole and SecretHunter are example kLex apps — multi-provider AI chat and credential scanning.",
]

println("Indexing " + str(len(facts)) + " facts with `" + embedModelName + "`…")
store = vs.newStore()
i = 0
while i < len(facts) {
    resp, eerr = ollama.embed(embedClient, facts[i])
    if eerr != null { println("embed fail: " + eerr.message)  _osExit(1) }
    store.add("fact" + str(i), resp["embeddings"][0], {"text": facts[i]})
    i = i + 1
}
println("Indexed. Store has " + str(store.count()) + " facts.")
println("")


// ── RAG query helper ──────────────────────────────────────────────────────

fn answerWithRAG(question, k) {
    println("─── Q: " + question + " ───")

    // 1. Embed the question
    qResp, qerr = ollama.embed(embedClient, question)
    if qerr != null { println("  embed fail: " + qerr.message)  return }

    // 2. Retrieve top-K relevant facts
    hits = store.search(qResp["embeddings"][0], k)

    // 3. Build a grounded prompt
    context = ""
    for h in hits {
        context = context + "  - " + h["metadata"]["text"] + "\n"
    }
    prompt = "Answer the question using ONLY the facts below. " +
             "If the facts don't contain the answer, say \"I don't know based on the provided facts.\"\n\n" +
             "Facts:\n" + context + "\n" +
             "Question: " + question + "\n\nAnswer:"

    // 4. Ask the chat model
    resp, cerr = ollama.chat(chatClient, {
        "messages": [ai.userMsg(prompt)],
        "think":    false,
        "options":  {"temperature": 0.2},
    })
    if cerr != null { println("  chat fail: " + cerr.message)  return }

    println("  retrieved (top " + str(k) + "):")
    for h in hits {
        scoreStr = substr(str(h["score"] + 0.00001), 0, 6)
        println("    " + scoreStr + "  " + h["metadata"]["text"])
    }
    println("")
    println("  A: " + trim(ollama.textOf(resp)))
    println("")
}


// ── Demo questions ────────────────────────────────────────────────────────

answerWithRAG("What concurrency primitives does kLex have?",     3)
answerWithRAG("How do I write a kLex program — what's the file format?", 3)
answerWithRAG("Does kLex support TrueType Collection fonts?",    3)
answerWithRAG("What does FROG stand for?",                       2)
answerWithRAG("Can kLex bridge to Java?",                        3)   // shouldn't know — only Python + Node
