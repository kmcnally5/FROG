// quick.lex — friendly one-liner facade for kLex AI calls.
// @module    quick
// @version   1.0.0
// @since     klex 0.3.35
// @author    karl
// @summary   friendly one-liner facade for kLex AI calls.
//
// The provider libraries (anthropic.lex, ollama.lex, voyage.lex) are
// thorough, structured, and a little verbose for casual use. This module
// wraps the five most-used patterns behind dumb-easy entry points:
//
//   ai.ask(question)              → string answer
//   ai.executeQuery(q, tools)     → answer, with kLex functions as tools
//   ai.extract(text, schema)      → typed hash matching the schema
//   ai.classify(text, labels)     → one of the provided labels
//   ai.askCached(question)        → ask, with exact-match response cache
//   ai.askDocument(path, q)       → grounded answer using a local doc / PDF
//
// Provider switching is global state — `useClaude()` / `useOllama()` flips
// every subsequent call. Default is Claude (uses ANTHROPIC_API_KEY env var).
// `KLEX_AI_MODEL` env var overrides the default model on either provider.
//
// `executeQuery` works on either provider, but Ollama tool use is gated:
// call `ai.allowOllamaTools(true)` after switching to a tools-capable model
// (Gemma 3+, Llama 3.1+, Qwen 2.5+, etc.) to opt in.
//
// All functions return (value, err) tuples — use `?` for clean propagation:
//   answer = ai.ask("Why is the sky blue?")?
//
// Usage:
//   import "stdlib/ai/quick.lex"     as ai
//   import "stdlib/ai/ai_common.lex" as aic        // if you also want userMsg etc.
//
//   ans = ai.ask("In one sentence, what is kLex?")?
//   println(ans)
//
//   // Tool dispatch — natural language → kLex function call
//   fn _earthDiameterKm() { return 12742 }
//   ans = ai.executeQuery("How wide is the earth?", { "earthDiameter": _earthDiameterKm })?
//   println(ans)


import "stdlib/ai/anthropic.lex"     as claude
import "stdlib/ai/ollama.lex"        as ollama
import "stdlib/ai/ai_common.lex"     as aic
import "stdlib/ai/chunk.lex"         as chunk
import "stdlib/ai/vector_store.lex"  as vs
import "stdlib/pdf.lex"              as pdf
import "stdlib/cache.lex"            as cache


// ── Module state ──────────────────────────────────────────────────────────

// Active provider — switch with useClaude() / useOllama().
let _provider = "claude"

// Per-call model override. Null = use provider default (or KLEX_AI_MODEL env).
let _model = null

// Cached clients — built lazily on first use. Reset to null when the
// provider or model changes so the next call rebuilds with new settings.
let _claudeClientCache = null
let _ollamaClientCache = null

// Response cache for ai.askCached(). Keyed by canonical (provider, model,
// question, opts) hash → answer string. Persists for the lifetime of the
// kLex process. Call clearCache() to flush.
let _responseCache = cache.newCache()

// Opt-in toggle for executeQuery() on Ollama. Off by default because
// older / smaller Ollama models silently ignore tool definitions —
// the user gets back prose with no tool call attempted, which looks
// like a bug. Set to true via allowOllamaTools(true) when the active
// model genuinely supports tools (gemma3+, llama3.1+, qwen2.5+,
// mistral-nemo, …). Has no effect on Claude.
let _allowOllamaTools = false

// askDocument() caches the chunk+embedding vector store per document so
// repeated questions over the same file are cheap. Cache key embeds
// (path | embedModel | chunkSize); entries are invalidated when the file's
// mtime changes. Each entry: {"mtime": int, "store": VectorStore}.
let _docStores = cache.newCache()

// Per-(model,host) Ollama embedding clients — one connection reused across
// chunks. Keyed by "model|host" string.
let _embedClientCache = cache.newCache()


// ── Provider switching ───────────────────────────────────────────────────

// useClaude([modelName]) — switch all subsequent ai.* calls to Anthropic.
// modelName=null uses the library default (claude-opus-4-7) or whatever
// KLEX_AI_MODEL env var is set to.
fn useClaude(modelName = null) {
    _provider = "claude"
    _model    = modelName
    _claudeClientCache = null
    return null
}

// useOllama([modelName]) — switch to a local Ollama daemon. modelName=null
// falls back to KLEX_AI_MODEL env var or "llama3.2". Requires Ollama running
// (`ollama serve`) and the model pulled (`ollama pull <name>`).
fn useOllama(modelName = null) {
    _provider = "ollama"
    _model    = modelName
    _ollamaClientCache = null
    return null
}

// currentProvider() returns "claude" or "ollama" — the active backend.
fn currentProvider() {
    return _provider
}

// allowOllamaTools(allow) — opt in to ai.executeQuery() on Ollama. Off by
// default because tool-use quality varies wildly by model — older / smaller
// Ollama models tend to ignore tool definitions and answer in prose, which
// looks like a bug. Set this to true only when the active model is known
// to support tools well (Gemma 3+, Llama 3.1+, Qwen 2.5+, Mistral-Nemo).
//
// Verify with: ollama show <model>   →   look for "tools" in the
// Capabilities section.
//
// Has no effect on Claude — Claude always supports tools.
fn allowOllamaTools(allow) {
    _allowOllamaTools = allow
    return null
}


// ── Public API ───────────────────────────────────────────────────────────

// ask(question, [opts]) — fire-and-forget Q&A. Returns the assistant's
// text answer. opts is an optional hash with: system, max_tokens,
// temperature, model.
//
// Returns (string, err). Use `?` to propagate the error to your caller.
fn ask(question, opts = null) {
    if type(question) != "STRING" {
        return "", error("AI_BAD_ARGS", "ask: question must be a string")
    }
    let callOpts = _buildCallOpts(question, opts)
    return _runChat(callOpts)
}


// executeQuery(question, tools, [opts]) — natural-language tool dispatch.
//
// `tools` may be:
//   - a single FUNCTION:  ai.executeQuery("How wide?", _earthWidth)
//   - a hash {name: fn}:  ai.executeQuery("...", {"earthWidth": _earthWidth,
//                                                "earthMass":  _earthMass})
//   - an array of full tool specs (the same shape claude.runAgent accepts —
//     {name, description, input_schema, handler}).
//
// Auto-built tool defs assume 0-arity functions (no input parameters);
// the AI calls them with no args. Pass the array form when you need
// argument schemas, custom descriptions, etc.
//
// Claude is always supported. Ollama is gated behind allowOllamaTools(true)
// because older / smaller Ollama models silently ignore tool definitions
// and answer in prose — which would look like a bug to the caller. Opt in
// only when you've confirmed the active model genuinely supports tools
// (Gemma 3+, Llama 3.1+, Qwen 2.5+, Mistral-Nemo, …). Check via:
//   ollama show <model>   →   look for "tools" in Capabilities
fn executeQuery(question, tools, opts = null) {
    if type(question) != "STRING" {
        return "", error("AI_BAD_ARGS", "executeQuery: question must be a string")
    }
    if _provider == "ollama" && !_allowOllamaTools {
        return "", error("AI_PROVIDER_UNSUPPORTED",
            "executeQuery on Ollama is gated. Confirm your model supports tools " +
            "(`ollama show <model>` → Capabilities), then call " +
            "ai.allowOllamaTools(true) to opt in.")
    }

    let toolDefs = _normalizeTools(tools)
    if toolDefs == null {
        return "", error("AI_BAD_ARGS",
            "executeQuery: tools must be a FUNCTION, a {name: fn} hash, or an array of tool specs")
    }

    let callOpts = _buildCallOpts(question, opts)
    callOpts["tools"] = toolDefs

    if _provider == "claude" {
        let c, cerr = _claudeClient()
        if cerr != null { return "", cerr }
        let finalResp, conv, err = claude.runAgent(c, callOpts, 12)
        if err != null { return "", err }
        return claude.textOf(finalResp), null
    }

    // Ollama path. ollama.runAgent accepts Anthropic-shape tool hashes —
    // the same {name, description, input_schema, handler} layout _normalizeTools
    // produces — and normalises them to Ollama's wire format internally.
    let c, cerr = _ollamaClient()
    if cerr != null { return "", cerr }
    let finalResp, conv, err = ollama.runAgent(c, callOpts, 12)
    if err != null { return "", err }
    return ollama.textOf(finalResp), null
}


// extract(text, jsonSchema) — return a typed hash matching the supplied
// JSON Schema. Wraps the provider's structured-output endpoint. Build the
// schema with stdlib/ai/schema.lex.
//
//   import "stdlib/ai/schema.lex" as schema
//   PersonSchema = schema.object({
//       "name":  schema.string("Full name"),
//       "age":   schema.integer("Age", 0, 150),
//   })
//   person = ai.extract("Albert Einstein was born in Ulm in 1879", PersonSchema)?
//   println(person["name"])
fn extract(text, jsonSchema) {
    if type(text) != "STRING" {
        return null, error("AI_BAD_ARGS", "extract: text must be a string")
    }
    if _provider == "claude" {
        let c, cerr = _claudeClient()
        if cerr != null { return null, cerr }
        return claude.complete(c, jsonSchema, text)
    }
    let c, cerr = _ollamaClient()
    if cerr != null { return null, cerr }
    return ollama.complete(c, jsonSchema, text)
}


// classify(text, labels) — return one of the provided string labels.
//
//   mood = ai.classify("I had a great day!", ["happy", "sad", "neutral"])?
//   // → "happy"
fn classify(text, labels) {
    if type(text) != "STRING" {
        return "", error("AI_BAD_ARGS", "classify: text must be a string")
    }
    if type(labels) != "ARRAY" || len(labels) == 0 {
        return "", error("AI_BAD_ARGS", "classify: labels must be a non-empty array")
    }

    let sch = {
        "type": "object",
        "properties": {
            "label": {
                "type":        "string",
                "enum":        labels,
                "description": "The single most appropriate label from the provided list",
            },
        },
        "required": ["label"],
    }
    let prompt = "Classify the following text into one of the available labels. " +
             "Return only the label that best fits.\n\nText: " + text

    let result, err = extract(prompt, sch)
    if err != null { return "", err }
    if type(result) != "HASH" || !hasKey(result, "label") {
        return "", error("AI_PARSE", "classify: model did not return a label field")
    }
    return result["label"], null
}


// askCached(question, [opts]) — same as ask(), with an in-memory cache
// keyed by canonical (provider, model, question, opts). First call hits
// the API; subsequent calls with identical inputs are free.
//
// Errors are NOT cached — a transient failure followed by a successful
// retry caches the success normally. Call clearCache() to invalidate.
fn askCached(question, opts = null) {
    let key = cache.keyOf({
        "provider": _provider,
        "model":    _model,
        "q":        question,
        "opts":     opts,
    })
    let cached = _responseCache.get(key)
    if cached != null { return cached, null }

    let ans, err = ask(question, opts)
    if err != null { return "", err }

    _responseCache.set(key, ans)
    return ans, null
}


// clearCache() — drop every askCached entry. Useful after switching
// provider/model if you want to force fresh responses.
fn clearCache() {
    _responseCache.clear()
    return null
}


// cacheSize() returns the number of cached responses.
fn cacheSize() {
    return _responseCache.size()
}


// askDocument(path, question, [opts]) — RAG answer grounded in a local file.
//
//   ans = ai.askDocument("docs/KLEX_LANGUAGE.MD", "What is kLex?")?
//   ans = ai.askDocument("/path/to/manual.pdf",    "Recommended dose?")?
//
// Pipeline:
//   1. Extract text — PDFs via stdlib/pdf.lex, anything else via readFile
//   2. Chunk into paragraph-sized pieces (default 800 chars)
//   3. Embed each chunk via Ollama (default `mxbai-embed-large`)
//   4. Embed the question, retrieve top-K most similar chunks
//   5. Build a grounded prompt and ask the *active* chat provider
//      (so ai.useClaude() / ai.useOllama() switches who answers — embeddings
//       always use Ollama because they're local and free)
//
// The per-document vector store is cached by (path, embedModel, chunkSize)
// and invalidated when the file's mtime changes — repeat questions on the
// same doc are cheap, only the question embedding + chat call run again.
//
// Options (all optional):
//   embedModel : string  — Ollama embed model (default "mxbai-embed-large").
//                          Pull once: `ollama pull mxbai-embed-large`
//   embedHost  : string  — Ollama daemon host (null = localhost:11434)
//   chunkSize  : int     — chars per chunk (default 800)
//   topK       : int     — how many top chunks to feed the model (default 4)
//   system     : string  — override the default RAG system prompt
//   max_tokens : int     — answer length cap (default 1024)
//
// Errors:
//   AI_DOC_OPEN              — couldn't read the file
//   AI_DOC_EMPTY             — extracted text produced no chunks
//   AI_EMBED_OFFLINE         — Ollama daemon unreachable
//   AI_EMBED_MODEL_MISSING   — embedding model not pulled
//   AI_EMBED_FAILED          — any other embed failure
//   (plus any AI_NO_KEY / ANTHROPIC_* from the answering provider)
fn askDocument(path, question, opts = null) {
    if type(path) != "STRING" {
        return "", error("AI_BAD_ARGS", "askDocument: path must be a string")
    }
    if type(question) != "STRING" {
        return "", error("AI_BAD_ARGS", "askDocument: question must be a string")
    }

    // Defaults + opts overrides.
    let embedModel   = "mxbai-embed-large"
    let embedHost    = null
    let chunkSize    = 800
    let topK         = 4
    let customSystem = null
    let maxTokens    = 1024
    if opts != null {
        if hasKey(opts, "embedModel") { embedModel   = opts["embedModel"] }
        if hasKey(opts, "embedHost")  { embedHost    = opts["embedHost"] }
        if hasKey(opts, "chunkSize")  { chunkSize    = opts["chunkSize"] }
        if hasKey(opts, "topK")       { topK         = opts["topK"] }
        if hasKey(opts, "system")     { customSystem = opts["system"] }
        if hasKey(opts, "max_tokens") { maxTokens    = opts["max_tokens"] }
    }

    // Build or fetch the cached vector store for this document.
    let store, sErr = _docStoreFor(path, embedModel, embedHost, chunkSize)
    if sErr != null { return "", sErr }

    // Embed the question.
    let ec, ecErr = _embedClient(embedModel, embedHost)
    if ecErr != null { return "", ecErr }
    let qResp, qEmbedErr = ollama.embed(ec, question)
    if qEmbedErr != null { return "", _wrapEmbedErr(qEmbedErr, embedModel) }
    let qVec = qResp["embeddings"][0]

    // Retrieve the top-K most relevant chunks.
    let hits = store.search(qVec, topK)
    if len(hits) == 0 {
        return "", error("AI_DOC_EMPTY",
            "askDocument: no chunks matched — store may have failed to populate")
    }

    // Build the grounded prompt. Pre-allocate the parts array; concat-in-loop
    // would be O(n²) on the context block.
    let parts = makeArray(len(hits), "")
    for i, h in hits {
        let meta = h["metadata"]
        parts[i] = "[chunk " + str(meta["index"]) + "]\n" + meta["text"]
    }
    let contextBlock = join(parts, "\n\n")

    let system = customSystem
    if system == null { system = _ragSystemPrompt(path) }

    let grounded = "Excerpts from " + path + ":\n\n" + contextBlock +
               "\n\nQuestion: " + question

    return ask(grounded, {"system": system, "max_tokens": maxTokens})
}


// clearDocumentCache() — drop every cached doc store. Use if you want to
// force re-embedding (e.g. after switching embedModel mid-session).
fn clearDocumentCache() {
    _docStores.clear()
    _embedClientCache.clear()
    return null
}


// documentCacheSize() — number of cached documents.
fn documentCacheSize() {
    return _docStores.size()
}


// ── Internal helpers ─────────────────────────────────────────────────────

// _buildCallOpts assembles the wire-shape opts hash from the user's
// question + optional overrides. Defaults max_tokens to a generous-but-
// not-excessive 1024 so casual questions don't get cut off and casual
// "expand this prompt" calls don't burn pages of output.
fn _buildCallOpts(question, opts) {
    let out = {
        "max_tokens": 1024,
        "messages":   [aic.userMsg(question)],
    }
    if opts != null {
        if hasKey(opts, "system")      { out["system"]      = opts["system"] }
        if hasKey(opts, "max_tokens")  { out["max_tokens"]  = opts["max_tokens"] }
        if hasKey(opts, "temperature") { out["temperature"] = opts["temperature"] }
        if hasKey(opts, "model")       { out["model"]       = opts["model"] }
    }
    return out
}


// _runChat dispatches a chat call to the active provider. Returns
// (text, err). Used by ask() and askCached().
fn _runChat(callOpts) {
    if _provider == "claude" {
        let c, cerr = _claudeClient()
        if cerr != null { return "", cerr }
        let resp, err = claude.messages(c, callOpts)
        if err != null { return "", err }
        return claude.textOf(resp), null
    }
    let c, cerr = _ollamaClient()
    if cerr != null { return "", cerr }
    let resp, err = ollama.chat(c, callOpts)
    if err != null { return "", err }
    return ollama.textOf(resp), null
}


// _claudeClient returns a lazily-constructed Claude client, or an error
// if ANTHROPIC_API_KEY is missing.
fn _claudeClient() {
    if _claudeClientCache != null { return _claudeClientCache, null }
    let key = env("ANTHROPIC_API_KEY")
    if key == null || len(key) == 0 {
        return null, error("AI_NO_KEY",
            "ANTHROPIC_API_KEY env var is not set — required for Claude calls. " +
            "Either export the key, or call ai.useOllama(\"<model>\") to switch backends.")
    }
    let model = _model
    if model == null {
        let e = env("KLEX_AI_MODEL")
        if e != null && len(e) > 0 { model = e }
    }
    _claudeClientCache = claude.newClient(key, model)
    return _claudeClientCache, null
}


// _ollamaClient returns a lazily-constructed Ollama client. Uses
// OLLAMA_HOST env var if set, otherwise localhost:11434.
fn _ollamaClient() {
    if _ollamaClientCache != null { return _ollamaClientCache, null }
    let host  = env("OLLAMA_HOST")
    let model = _model
    if model == null {
        let e = env("KLEX_AI_MODEL")
        if e != null && len(e) > 0 { model = e }
    }
    if model == null { model = "llama3.2" }
    _ollamaClientCache = ollama.newClient(host, model)
    return _ollamaClientCache, null
}


// _makeZeroArityHandler returns a fresh closure capturing `f` per call.
// Using a helper instead of an inline closure inside a loop avoids the
// classic capture-by-reference trap where every iteration's closure ends
// up pointing at the loop's final value.
fn _makeZeroArityHandler(f) {
    return fn(input) { return f() }
}


// _ragSystemPrompt builds the default "use only what I gave you" system
// message for askDocument. Mentions the source path so the model can
// reference it naturally in its answer.
fn _ragSystemPrompt(path) {
    return "You are answering questions about the document at " + path + ". " +
           "Use ONLY the provided excerpts to answer. If the excerpts do not " +
           "contain the answer, say so plainly — do not speculate. Keep your " +
           "answer concise. Reference chunk indices (e.g. \"[chunk 7]\") when " +
           "directly quoting or paraphrasing a specific excerpt."
}


// _docStoreFor returns the cached vector store for `path`, rebuilding it
// (extract → chunk → embed) if the file is new or has been modified.
// Cache key includes (path, embedModel, chunkSize) so changing those
// invalidates correctly without manual clearDocumentCache().
fn _docStoreFor(path, embedModel, embedHost, chunkSize) {
    let key = path + "|" + embedModel + "|" + str(chunkSize)

    let info, statErr = _fsStat(path)
    if statErr != null {
        return null, error("AI_DOC_OPEN",
            "askDocument: cannot stat " + path + ": " + str(statErr))
    }
    let curMtime = info["modTime"]

    let cached = _docStores.get(key)
    if cached != null && cached["mtime"] == curMtime {
        return cached["store"], null
    }

    // Cold path — load, chunk, embed.
    let text, loadErr = _loadDocText(path)
    if loadErr != null { return null, loadErr }

    let chunks = chunk.byParagraphs(text, chunkSize)
    if len(chunks) == 0 {
        return null, error("AI_DOC_EMPTY",
            "askDocument: " + path + " produced no chunks (empty file?)")
    }

    let ec, ecErr = _embedClient(embedModel, embedHost)
    if ecErr != null { return null, ecErr }

    let store = vs.newStore()
    let i = 0
    while i < len(chunks) {
        let c = chunks[i]
        let eResp, eErr = ollama.embed(ec, c["text"])
        if eErr != null { return null, _wrapEmbedErr(eErr, embedModel) }
        store.add("c" + str(i), eResp["embeddings"][0], c)
        i = i + 1
    }

    _docStores.set(key, {"mtime": curMtime, "store": store})
    return store, null
}


// _loadDocText reads `path` into a single string. PDFs go through
// stdlib/pdf.lex; everything else is treated as plain text (works for
// .txt, .md, source code, etc.).
fn _loadDocText(path) {
    if len(path) >= 4 {
        let ext = substr(path, len(path) - 4, len(path))
        if ext == ".pdf" || ext == ".PDF" {
            return pdf.extractText(path)
        }
    }
    let text, err = safe(readFile, path)
    if err != null {
        return null, error("AI_DOC_OPEN",
            "askDocument: cannot read " + path + ": " + err.message)
    }
    return text, null
}


// _embedClient returns a cached Ollama embedding client. Keyed by
// (model, host) so swapping either spins up a fresh connection.
fn _embedClient(model, host) {
    let key = "ollama|" + model + "|"
    if host != null { key = key + host }
    let cached = _embedClientCache.get(key)
    if cached != null { return cached, null }
    let c = ollama.newClient(host, model)
    _embedClientCache.set(key, c)
    return c, null
}


// _wrapEmbedErr converts a raw ollama.embed error into a more actionable
// typed error code so callers can branch on err.code without parsing strings.
fn _wrapEmbedErr(err, model) {
    let msg = err.message
    if indexOf(msg, "not found") >= 0 || indexOf(msg, "no such model") >= 0 || indexOf(msg, "404") >= 0 {
        return error("AI_EMBED_MODEL_MISSING",
            "embedding model '" + model + "' not pulled — run: ollama pull " + model)
    }
    if err.code == "OLLAMA_OFFLINE" || indexOf(msg, "connection refused") >= 0 {
        return error("AI_EMBED_OFFLINE",
            "Ollama daemon not reachable — start it with `ollama serve` (or set OLLAMA_HOST)")
    }
    return error("AI_EMBED_FAILED", "embedding failed: " + msg)
}


// _normalizeTools turns the user's `tools` argument into the array of
// tool-spec hashes that claude.runAgent expects. Returns null if the
// shape isn't recognised.
fn _normalizeTools(t) {
    let tt = type(t)

    // Already in runAgent format.
    if tt == "ARRAY" { return t }

    // Single function reference — wrap as one 0-arity tool named "tool".
    if tt == "FUNCTION" || tt == "BUILTIN" {
        return [{
                "name":         "tool",
            "description":  "Call this function to answer the user's question.",
            "input_schema": {"type": "object", "properties": {}},
            "handler":      _makeZeroArityHandler(t),
        }]
    }

    if tt != "HASH" { return null }

    // Single full-spec hash (has a "handler" key) — wrap as a one-element array.
    if hasKey(t, "handler") { return [t] }

    // Hash of {name: fn} — build one tool spec per entry.
    let ks = keys(t)
    let n  = len(ks)
    let out = makeArray(n, null)
    let i = 0
    while i < n {
        let name  = ks[i]
        let value = t[name]
        let vt    = type(value)
        if vt == "FUNCTION" || vt == "BUILTIN" {
            out[i] = {
                "name":         name,
                "description":  "User-registered function: " + name,
                "input_schema": {"type": "object", "properties": {}},
                "handler":      _makeZeroArityHandler(value),
            }
        } else if vt == "HASH" {
            // Pre-built tool spec under this name — just stamp the name in.
            let spec = {}
            for k, v in value { spec[k] = v }
            spec["name"] = name
            out[i] = spec
        } else {
            return null
        }
        i = i + 1
    }
    return out
}
