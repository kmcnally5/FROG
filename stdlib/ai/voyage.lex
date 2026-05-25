// voyage.lex — Voyage AI embeddings as a first-class kLex library.
// @module    voyage
// @version   1.0.0
// @since     klex 0.3.35
// @author    karl
// @summary   Voyage AI embeddings as a first-class kLex library.
//
// Pure-HTTP implementation built on stdlib/rest.lex + stdlib/json.lex.
// Zero external dependencies — no Python or Node bridge required.
//
// Voyage is Anthropic's officially recommended embedding provider.
// Anthropic does not ship its own embedding model, so a Claude-primary
// stack pairs Anthropic for chat with Voyage for vectors.
//
// Reference: https://docs.voyageai.com/reference/embeddings-api
//
// Authentication: pass your API key to newClient() at construction time.
// Get a key from https://dash.voyageai.com/.
//
// Usage:
//   import "stdlib/ai/voyage.lex"       as voyage
//   import "stdlib/ai/vector_store.lex" as vs
//
//   c     = voyage.newClient(env("VOYAGE_API_KEY"), "voyage-3")
//   store = vs.newStore()
//
//   resp, err = voyage.embed(c, "the quick brown fox")
//   if err != null { println(err.message)  return }
//   store.add("doc0", resp["embeddings"][0], {"text": "the quick brown fox"})
//
// The response shape (`{embeddings: [[..], …]}`) matches `ollama.embed`,
// so the same `vector_store.add(...)` / `.search(...)` flow works
// unchanged regardless of which provider produced the vector.


import "stdlib/rest.lex"         as rest
import "stdlib/json.lex"         as json
import "stdlib/ai/ai_common.lex" as ai


// ── Constants ─────────────────────────────────────────────────────────────

const API_BASE_URL  = "https://api.voyageai.com/v1"
const DEFAULT_MODEL = "voyage-3"


// Pricing — USD per 1M tokens. Voyage charges only on input tokens
// (embeddings have no output tokens). Update here if Voyage revises.
// Source: https://docs.voyageai.com/docs/pricing
let _RATES = {
    "voyage-3":              0.06,
    "voyage-3-lite":         0.02,
    "voyage-3-large":        0.18,
    "voyage-code-3":         0.18,
    "voyage-finance-2":      0.12,
    "voyage-multilingual-2": 0.12,
    "voyage-law-2":          0.12,
}


// _rateFor returns the USD-per-1M-tokens input rate for a model. Falls
// back to voyage-3 pricing for unknown future model IDs — conservative
// so unknowns don't silently look free.
fn _rateFor(model) {
    if model != null && hasKey(_RATES, model) {
        return _RATES[model]
    }
    return 0.06
}


// ── Client ────────────────────────────────────────────────────────────────

// Client carries per-conversation defaults. Construct via newClient() and
// pass it to every API call.
struct Client {
    apiKey, model, baseUrl
}


// newClient creates a Client. Pass an empty string or null model to fall
// back to DEFAULT_MODEL.
fn newClient(apiKey, model) {
    let chosenModel = DEFAULT_MODEL
    if model != null {
        if type(model) == "STRING" && len(model) > 0 {
            chosenModel = model
        }
    }
    return Client {
        apiKey:  apiKey,
        model:   chosenModel,
        baseUrl: API_BASE_URL,
    }
}


// ── Internal helpers ──────────────────────────────────────────────────────

fn _headers(c) {
    return {
        "Authorization": "Bearer " + c.apiKey,
        "content-type":  "application/json",
    }
}


fn _classifyError(resp) {
    let code = "VOYAGE_HTTP_ERROR"
    let msg  = "Voyage API returned status " + str(resp.status)
    if type(resp.data) == "HASH" && hasKey(resp.data, "detail") {
        let d = resp.data["detail"]
        if type(d) == "STRING" { msg = d }
    }
    if resp.status == 401 { code = "VOYAGE_AUTH" }
    if resp.status == 429 { code = "VOYAGE_RATE_LIMIT" }
    return error(code, msg)
}


// _extractEmbeddings converts Voyage's wire format (data: [{embedding,
// index}, ...]) into a flat array of float-arrays so the response shape
// matches ollama.embed and drops straight into vector_store.
fn _extractEmbeddings(respData) {
    if type(respData) != "HASH" { return makeArray(0) }
    if !hasKey(respData, "data") { return makeArray(0) }
    let data = respData["data"]
    if type(data) != "ARRAY" { return makeArray(0) }
    let n = len(data)
    let out = makeArray(n, null)
    let i = 0
    while i < n {
        let entry = data[i]
        if type(entry) == "HASH" && hasKey(entry, "embedding") {
            out[i] = entry["embedding"]
        }
        i = i + 1
    }
    return out
}


// ── Embeddings ────────────────────────────────────────────────────────────

// embed produces vectors for `input` — a single string or an array of
// strings. Returns (response_hash, err) where response.embeddings is an
// array of float arrays (one per input). Use input_type via embedWith()
// to nudge Voyage toward document-side or query-side optimisation.
//
// Mirrors ollama.embed's return shape so vector_store, RAG helpers, and
// downstream eval scripts work without provider-specific branches.
fn embed(c, input) {
    return embedWith(c, input, null)
}


// embedWith is embed() plus per-call options:
//   model          — override the client's default model
//   input_type     — "document" | "query" (Voyage tunes the vector slightly
//                    depending on which side of a retrieval pair it's on)
//   truncation     — bool; if true, oversized inputs are silently truncated
//                    rather than rejected
//   output_dtype   — "float" | "int8" | "uint8" | "binary" | "ubinary"
//   output_dimension — request a reduced-dimension vector (model-dependent)
fn embedWith(c, input, opts) {
    if input == null {
        return null, error("VOYAGE_BAD_ARGS", "input is required")
    }
    if ai.budgetExceeded() { return null, ai._budgetError() }

    let body = {
        "model": c.model,
        "input": input,
    }
    if opts != null && type(opts) == "HASH" {
        if hasKey(opts, "model")            { body["model"]            = opts["model"] }
        if hasKey(opts, "input_type")       { body["input_type"]       = opts["input_type"] }
        if hasKey(opts, "truncation")       { body["truncation"]       = opts["truncation"] }
        if hasKey(opts, "output_dtype")     { body["output_dtype"]     = opts["output_dtype"] }
        if hasKey(opts, "output_dimension") { body["output_dimension"] = opts["output_dimension"] }
    }

    let resp, httpErr = rest.postWith(c.baseUrl + "/embeddings", body, _headers(c))
    if httpErr != null { return null, error("VOYAGE_NETWORK", httpErr) }
    if !rest.isOk(resp) { return null, _classifyError(resp) }

    let embeddings = _extractEmbeddings(resp.data)
    let totalTokens = 0
    if type(resp.data) == "HASH" && hasKey(resp.data, "usage") {
        let u = resp.data["usage"]
        if type(u) == "HASH" && hasKey(u, "total_tokens") {
            totalTokens = u["total_tokens"]
        }
    }

    // Voyage bills only input tokens (no output tokens for embeddings).
    let rate = _rateFor(body["model"])
    ai._record(body["model"], totalTokens, 0, rate, 0.0)

    return {
        "embeddings": embeddings,
        "model":      body["model"],
        "usage":      {"input_tokens": totalTokens, "output_tokens": 0, "cost_usd": null},
    }, null
}


// ── Response accessors ────────────────────────────────────────────────────

// usageOf pulls the canonical {input_tokens, output_tokens, cost_usd}
// hash out of a response — matches the shape provided by anthropic.lex
// and ollama.lex so reporting code stays provider-agnostic.
fn usageOf(resp) {
    if type(resp) != "HASH" { return {"input_tokens": 0, "output_tokens": 0, "cost_usd": null} }
    if !hasKey(resp, "usage") { return {"input_tokens": 0, "output_tokens": 0, "cost_usd": null} }
    return resp["usage"]
}


// embeddingsOf returns the [[float, …], …] array from a response. Handy
// for one-liner pipelines where you don't care about model/usage.
fn embeddingsOf(resp) {
    if type(resp) != "HASH" { return makeArray(0) }
    if !hasKey(resp, "embeddings") { return makeArray(0) }
    return resp["embeddings"]
}
