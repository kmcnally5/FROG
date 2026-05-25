// ollama.lex — Ollama local-LLM API as a first-class kLex library.
// @module    ollama
// @version   1.0.0
// @since     klex 0.3.35
// @author    karl
// @summary   Ollama local-LLM API as a first-class kLex library.
//
// Pure-HTTP implementation built on stdlib/rest.lex + stdlib/json.lex.
// Zero external dependencies — talks straight to a local Ollama daemon
// (or a remote one if you set the host).
//
// Default host: http://localhost:11434.  Override at client construction
// or via the OLLAMA_HOST env var (handled at the call site, not here).
//
// Reference: https://github.com/ollama/ollama/blob/main/docs/api.md
//
// Usage:
//   import "stdlib/ai/ollama.lex"     as ollama
//   import "stdlib/ai/ai_common.lex"  as ai
//
//   c = ollama.newClient(null, "llama3.2")    // null = default host
//   resp, err = ollama.chat(c, {
//       "messages": [ai.userMsg("Why is the sky blue?")],
//   })
//   if err != null { println(err.message)  return }
//   println(ollama.textOf(resp))


import "stdlib/rest.lex"         as rest
import "stdlib/json.lex"         as json
import "stdlib/ai/ai_common.lex" as ai


// ── Constants ─────────────────────────────────────────────────────────────

const DEFAULT_BASE_URL = "http://localhost:11434"
const DEFAULT_MODEL    = "llama3.2"
const DEFAULT_MAX_STEPS = 12       // agent-loop iteration cap

// Default per-call HTTP timeout. Local Ollama with a cold model can
// easily take >30s to load the model AND generate the response in a
// single non-streaming round-trip (especially on CPU-only machines or
// for larger models). 300s is generous but cheap — context-bound, only
// fires if something is genuinely wrong. Callers can override per-call
// via opts["timeout_sec"] on chat() / generate() / embed().
const DEFAULT_TIMEOUT_SEC = 300


// ── Client ────────────────────────────────────────────────────────────────

// Client carries per-conversation defaults. Construct via newClient() and
// pass it to every API call. Unlike cloud providers Ollama has no API key
// (it's a local daemon), so apiKey isn't on the struct — but the field is
// reserved for future remote/auth scenarios.
struct Client {
    baseUrl, model
}


// newClient creates a Client. baseUrl=null uses DEFAULT_BASE_URL;
// model=null/empty falls back to DEFAULT_MODEL.
fn newClient(baseUrl, model) {
    let chosenBase = DEFAULT_BASE_URL
    if baseUrl != null && type(baseUrl) == "STRING" && len(baseUrl) > 0 {
        chosenBase = baseUrl
    }
    let chosenModel = DEFAULT_MODEL
    if model != null && type(model) == "STRING" && len(model) > 0 {
        chosenModel = model
    }
    return Client { baseUrl: chosenBase, model: chosenModel }
}


// ── Internal helpers ──────────────────────────────────────────────────────

fn _classifyError(resp) {
    let code = "OLLAMA_HTTP_ERROR"
    let msg  = "Ollama API returned status " + str(resp.status)
    if resp.status == 404 {
        code = "OLLAMA_NOT_FOUND"
        msg  = "model or endpoint not found — is the model pulled? (`ollama pull <model>`)"
    } else if resp.status >= 500 {
        code = "OLLAMA_SERVER"
        msg  = "Ollama server error (status " + str(resp.status) + ")"
    }
    // Surface the API's own error message when present.
    if type(resp.data) == "HASH" && hasKey(resp.data, "error") {
        let e = resp.data["error"]
        if type(e) == "STRING" { msg = msg + ": " + e }
    }
    return error(code, msg)
}


// _classifyConnError handles the case where Ollama isn't running.
// rest's network-failure error string usually contains "connection refused".
fn _classifyConnError(rawErr) {
    if indexOf(rawErr, "connection refused") >= 0 {
        return error("OLLAMA_OFFLINE",
            "Ollama daemon not reachable — start it with `ollama serve` (or check OLLAMA_HOST)")
    }
    return error("OLLAMA_NETWORK", rawErr)
}


// _buildMessages normalises an array of ai.Message structs or plain
// message hashes into the wire format Ollama expects.
fn _buildMessages(msgs) {
    let out = makeArray(0)
    for m in msgs {
        if type(m) == "HASH" {
            out = concat(out, [m])
        } else {
            out = concat(out, [{ "role": m.role, "content": m.content }])
        }
    }
    return out
}


// _stripHandlers removes kLex-side `handler` fields from tool defs.
// Ollama's tool schema is OpenAI-compatible:
//   {"type": "function", "function": {"name", "description", "parameters"}}
// Callers may pass either that shape OR the simpler Anthropic-style hash
// {"name", "description", "input_schema", "handler"} — we accept both and
// normalise to Ollama's wire form.
fn _stripHandlers(tools) {
    let out = makeArray(0)
    for t in tools {
        // Already Ollama-shape?
        if hasKey(t, "type") && t["type"] == "function" && hasKey(t, "function") {
            let clean = {"type": "function", "function": {}}
            for k, v in t["function"] {
                if k != "handler" { clean["function"][k] = v }
            }
            out = concat(out, [clean])
            continue
        }
        // Anthropic-shape — translate to Ollama-shape.
        let fn_def = {}
        if hasKey(t, "name")         { fn_def["name"]        = t["name"] }
        if hasKey(t, "description")  { fn_def["description"] = t["description"] }
        if hasKey(t, "input_schema") { fn_def["parameters"]  = t["input_schema"] }
        if hasKey(t, "parameters")   { fn_def["parameters"]  = t["parameters"] }
        out = concat(out, [{"type": "function", "function": fn_def}])
    }
    return out
}


// _toolHandlersFromOpts builds a name→handler dispatch table from opts.tools.
// Accepts both Ollama-shape and Anthropic-shape tool hashes.
fn _toolHandlersFromOpts(opts) {
    let handlers = {}
    if !hasKey(opts, "tools") { return handlers }
    for t in opts["tools"] {
        // Ollama-shape
        if hasKey(t, "type") && t["type"] == "function" && hasKey(t, "function") {
            let f = t["function"]
            if hasKey(f, "name") && hasKey(f, "handler") {
                handlers[f["name"]] = f["handler"]
            }
            continue
        }
        // Anthropic-shape
        if hasKey(t, "name") && hasKey(t, "handler") {
            handlers[t["name"]] = t["handler"]
        }
    }
    return handlers
}


// ── Primary API ───────────────────────────────────────────────────────────

// chat calls /api/chat — Ollama's canonical chat-completion endpoint
// (non-streaming). Returns the raw parsed response on success.
//
// opts is a hash; required:
//   messages       — array of ai.Messages or message hashes
// optional:
//   model          — overrides client default
//   system         — system prompt (auto-prepended as a system message)
//   tools          — array of tool hashes (Ollama- or Anthropic-shape)
//   format         — "json" or a JSON Schema hash for structured output
//   options        — Ollama generation params hash (temperature, num_predict,
//                    top_p, top_k, num_ctx, seed, …) — pass through verbatim
//   keep_alive     — string or int controlling model unload time
//
// Response shape (Ollama wire):
//   { model, created_at, message: {role, content, tool_calls}, done,
//     done_reason, total_duration, prompt_eval_count, eval_count, … }
// Use textOf(resp) / toolCallsOf(resp) / usageOf(resp) for friendly access.
fn chat(c, opts) {
    if !hasKey(opts, "messages") {
        return null, error("OLLAMA_BAD_ARGS", "opts.messages is required")
    }
    let msgs = _buildMessages(opts["messages"])
    if hasKey(opts, "system") {
        msgs = concat([{"role": "system", "content": opts["system"]}], msgs)
    }
    let body = {
        "model":    c.model,
        "messages": msgs,
        "stream":   false,
    }
    if hasKey(opts, "model")      { body["model"]      = opts["model"] }
    if hasKey(opts, "format")     { body["format"]     = opts["format"] }
    if hasKey(opts, "options")    { body["options"]    = opts["options"] }
    if hasKey(opts, "keep_alive") { body["keep_alive"] = opts["keep_alive"] }
    if hasKey(opts, "think")      { body["think"]      = opts["think"] }
    if hasKey(opts, "tools")      { body["tools"]      = _stripHandlers(opts["tools"]) }

    let timeoutSec = DEFAULT_TIMEOUT_SEC
    if hasKey(opts, "timeout_sec") { timeoutSec = opts["timeout_sec"] }

    let resp, httpErr = rest.postWithTimeout(c.baseUrl + "/api/chat", body, timeoutSec)
    if httpErr != null { return null, _classifyConnError(httpErr) }
    if !rest.isOk(resp) { return null, _classifyError(resp) }

    // Record usage at $0 — Ollama is local, no money changes hands. Token
    // counts still flow into ai.usage() so reporting + eval scripts see
    // which models did the work.
    let u = usageOf(resp.data)
    ai._record(body["model"], u["input_tokens"], u["output_tokens"], 0.0, 0.0)

    return resp.data, null
}


// ── Structured output ────────────────────────────────────────────────────

// complete asks the model to return a structured response matching the
// supplied JSON Schema. Uses Ollama's native `format` parameter (which
// accepts a JSON Schema document since Ollama 0.5+) to constrain the
// model's output to valid JSON of the requested shape.
//
// Build the schema with stdlib/ai/schema.lex:
//
//   import "stdlib/ai/schema.lex" as schema
//
//   PersonSchema = schema.object({
//       "name":  schema.string("Full name"),
//       "age":   schema.integer("Age in years", 0, 150),
//       "email": schema.string("Email address"),
//   })
//
//   person, err = ollama.complete(c, PersonSchema, "Generate a fake profile")
//   if err != null { println(err.message)  return }
//   println(person["name"])
//
// Returns the parsed structured data as a kLex hash. For more control
// (system prompt, model override, num_predict, think, …) use
// completeWith(c, schema, prompt, opts).
fn complete(c, jsonSchema, prompt) {
    return completeWith(c, jsonSchema, prompt, null)
}

// completeWith(c, jsonSchema, prompt, opts) — structured-output completion with full control over options.
// opts hash may include: system, model, num_predict, think. Returns (parsedHash, err).
fn completeWith(c, jsonSchema, prompt, opts) {
    // Belt-and-braces: pass the schema via `format` for Ollama versions
    // that enforce it natively, AND inject a system prompt describing the
    // schema so older Ollama versions / less-capable models still produce
    // conforming output. Some models (qwen3.x variants, older llamas)
    // honour format="json" but not format=<schema>; the system prompt
    // covers them.
    let schemaJSON = json.stringify(jsonSchema)
    let systemMsg = "You return ONLY JSON that exactly conforms to this JSON Schema. " +
                "Do not include markdown fences, prose, or explanation — just raw JSON.\n\n" +
                "Schema:\n" + schemaJSON

    let callOpts = {
        "system":   systemMsg,
        "messages": [ai.userMsg(prompt)],
        "format":   jsonSchema,
    }
    if opts != null {
        if hasKey(opts, "system") {
            // Caller-supplied system goes FIRST, schema instructions appended
            callOpts["system"] = opts["system"] + "\n\n" + systemMsg
        }
        if hasKey(opts, "model")       { callOpts["model"]       = opts["model"] }
        if hasKey(opts, "options")     { callOpts["options"]     = opts["options"] }
        if hasKey(opts, "think")       { callOpts["think"]       = opts["think"] }
        if hasKey(opts, "messages")    { callOpts["messages"]    = opts["messages"] }
        if hasKey(opts, "keep_alive")  { callOpts["keep_alive"]  = opts["keep_alive"] }
        if hasKey(opts, "timeout_sec") { callOpts["timeout_sec"] = opts["timeout_sec"] }
    }

    let resp, err = chat(c, callOpts)
    if err != null { return null, err }

    let text = textOf(resp)
    if len(text) == 0 {
        return null, error("OLLAMA_NO_STRUCTURED_OUTPUT", "model returned empty content")
    }
    // Some models wrap output in ```json fences despite the instruction
    // — strip them if present before parsing.
    text = _stripJSONFences(text)
    let parsed, perr = json.parse(text)
    if perr != null {
        return null, error("OLLAMA_INVALID_JSON",
            "model output is not valid JSON: " + str(perr))
    }
    return parsed, null
}


// _stripJSONFences removes leading ```json / ``` markdown fences from a
// model's output. Some Ollama models emit fenced blocks even when told
// not to; we forgive that since the JSON inside is still valid.
fn _stripJSONFences(text) {
    let t = trim(text)
    // Leading ```json
    if len(t) >= 7 && substr(t, 0, 7) == "```json" {
        t = substr(t, 7, len(t))
    } else if len(t) >= 3 && substr(t, 0, 3) == "```" {
        t = substr(t, 3, len(t))
    }
    t = trim(t)
    // Trailing ```
    if len(t) >= 3 && substr(t, len(t) - 3, len(t)) == "```" {
        t = substr(t, 0, len(t) - 3)
    }
    return trim(t)
}


// generate calls /api/generate — Ollama's single-prompt completion
// endpoint (no chat history, simpler than chat()). Use when you want
// straight text-in, text-out.
//
// opts required: "prompt" (string).  Optional: model, system, format,
//   options, suffix, template, raw, keep_alive.
//
// Returns the parsed response: { model, response, done, … }.
fn generate(c, opts) {
    if !hasKey(opts, "prompt") {
        return null, error("OLLAMA_BAD_ARGS", "opts.prompt is required")
    }
    let body = {
        "model":  c.model,
        "prompt": opts["prompt"],
        "stream": false,
    }
    if hasKey(opts, "model")      { body["model"]      = opts["model"] }
    if hasKey(opts, "system")     { body["system"]     = opts["system"] }
    if hasKey(opts, "format")     { body["format"]     = opts["format"] }
    if hasKey(opts, "options")    { body["options"]    = opts["options"] }
    if hasKey(opts, "suffix")     { body["suffix"]     = opts["suffix"] }
    if hasKey(opts, "template")   { body["template"]   = opts["template"] }
    if hasKey(opts, "raw")        { body["raw"]        = opts["raw"] }
    if hasKey(opts, "keep_alive") { body["keep_alive"] = opts["keep_alive"] }

    let timeoutSec = DEFAULT_TIMEOUT_SEC
    if hasKey(opts, "timeout_sec") { timeoutSec = opts["timeout_sec"] }

    let resp, httpErr = rest.postWithTimeout(c.baseUrl + "/api/generate", body, timeoutSec)
    if httpErr != null { return null, _classifyConnError(httpErr) }
    if !rest.isOk(resp) { return null, _classifyError(resp) }
    return resp.data, null
}


// embed calls /api/embed — Ollama's embedding endpoint.  `input` may be
// a single string or an array of strings; the response carries one
// vector per input. Use for semantic search, classification, RAG, etc.
//
// Returns (response_hash, err) where response.embeddings is an array of
// float arrays (one per input).
fn embed(c, input) {
    if input == null {
        return null, error("OLLAMA_BAD_ARGS", "input is required")
    }
    let body = {
        "model": c.model,
        "input": input,
    }
    let resp, httpErr = rest.postWithTimeout(c.baseUrl + "/api/embed", body, DEFAULT_TIMEOUT_SEC)
    if httpErr != null { return null, _classifyConnError(httpErr) }
    if !rest.isOk(resp) { return null, _classifyError(resp) }
    return resp.data, null
}


// listModels calls /api/tags to enumerate locally-installed models.
// Returns (array of model hashes, err). Each hash carries
// {name, model, size, digest, modified_at, details}.
fn listModels(c) {
    let resp, httpErr = rest.get(c.baseUrl + "/api/tags")
    if httpErr != null { return null, _classifyConnError(httpErr) }
    if !rest.isOk(resp) { return null, _classifyError(resp) }
    if type(resp.data) == "HASH" && hasKey(resp.data, "models") {
        return resp.data["models"], null
    }
    return null, error("OLLAMA_PARSE", "no `models` field in /api/tags response")
}


// ps calls /api/ps to list currently-loaded models (those resident in
// memory, ready to serve requests with no warmup).
fn ps(c) {
    let resp, httpErr = rest.get(c.baseUrl + "/api/ps")
    if httpErr != null { return null, _classifyConnError(httpErr) }
    if !rest.isOk(resp) { return null, _classifyError(resp) }
    if type(resp.data) == "HASH" && hasKey(resp.data, "models") {
        return resp.data["models"], null
    }
    return null, error("OLLAMA_PARSE", "no `models` field in /api/ps response")
}


// show calls /api/show to return a model's full metadata: architecture,
// parameter count, context length, embedding length, quantisation,
// template, parameters, license, and crucially the `capabilities` array
// — `completion`, `tools`, `vision`, `audio`, `thinking`, etc.
//
// Pass an explicit modelName to introspect a specific tag, or null to
// use the client's configured model.
//
// Returns (hash, err). The hash mirrors Ollama's JSON shape.
//
// Example — detect if the active model supports tool-use before sending
// tool definitions:
//
//   info, err = ollama.show(c, null)
//   if err == null && hasCapability(info, "tools") {
//       // safe to pass `tools` to chat()/stream()
//   }
fn show(c, modelName) {
    let name = c.model
    if modelName != null && type(modelName) == "STRING" && len(modelName) > 0 {
        name = modelName
    }
    let resp, httpErr = rest.post(c.baseUrl + "/api/show", {"model": name})
    if httpErr != null { return null, _classifyConnError(httpErr) }
    if !rest.isOk(resp) { return null, _classifyError(resp) }
    if type(resp.data) != "HASH" {
        return null, error("OLLAMA_PARSE", "/api/show returned a non-hash body")
    }
    return resp.data, null
}


// hasCapability returns true if the show() response advertises the given
// capability — useful for gating tool-use, vision input, etc. before
// sending requests that the model would silently ignore.
//
// Known capability names from Ollama:
//   "completion"   — basic chat / generate
//   "tools"        — accepts and emits tool_calls
//   "vision"       — accepts image content blocks
//   "audio"        — accepts audio content blocks
//   "thinking"     — emits chain-of-thought in a separate `thinking` field
//   "embedding"    — exposed via /api/embed
//   "insert"       — fill-in-the-middle generation (model-specific)
fn hasCapability(showResp, capName) {
    if type(showResp) != "HASH" { return false }
    if !hasKey(showResp, "capabilities") { return false }
    let caps = showResp["capabilities"]
    if type(caps) != "ARRAY" { return false }
    for c in caps {
        if c == capName { return true }
    }
    return false
}


// ── Response accessors ────────────────────────────────────────────────────

// textOf returns the assistant text from a chat() response.
// For generate() responses use resp["response"] directly.
fn textOf(resp) {
    if type(resp) != "HASH"        { return "" }
    if !hasKey(resp, "message")    { return "" }
    let m = resp["message"]
    if type(m) != "HASH"           { return "" }
    if !hasKey(m, "content")       { return "" }
    return m["content"]
}


// toolCallsOf returns the tool_calls array from a chat() response, or
// an empty array if none. Each entry is a hash:
//   { "function": { "name": "...", "arguments": {...} } }
fn toolCallsOf(resp) {
    let out = makeArray(0)
    if type(resp) != "HASH"     { return out }
    if !hasKey(resp, "message") { return out }
    let m = resp["message"]
    if type(m) != "HASH"            { return out }
    if !hasKey(m, "tool_calls")     { return out }
    return m["tool_calls"]
}


// doneReasonOf returns the response's done_reason. Common values:
// "stop" (end of turn), "length" (hit token cap), "tool_calls" (when
// the model emitted tool calls and is waiting for results).
fn doneReasonOf(resp) {
    if type(resp) != "HASH" || !hasKey(resp, "done_reason") { return "" }
    return resp["done_reason"]
}


// usageOf returns a hash {input_tokens, output_tokens, cost_usd} from a
// chat()/generate() response. cost_usd stays null — Ollama is local-only,
// so cost is electricity.
//
// We return a hash (not the ai.Usage struct) because cross-module struct
// literals don't construct in kLex — `ai.Usage { … }` resolves to the
// StructDef itself, not an instance. Hash shape matches what callers want
// and works consistently across providers.
fn usageOf(resp) {
    let inT  = 0
    let outT = 0
    if type(resp) == "HASH" {
        if hasKey(resp, "prompt_eval_count") { inT  = resp["prompt_eval_count"] }
        if hasKey(resp, "eval_count")        { outT = resp["eval_count"] }
    }
    return {"input_tokens": inT, "output_tokens": outT, "cost_usd": null}
}


// ── Streaming ─────────────────────────────────────────────────────────────

// stream calls /api/chat with stream=true and returns a kLex channel of
// friendly event hashes. Each event has a `type` field, matching the
// shape used by stdlib/ai/anthropic.lex's stream() so consumers can be
// largely provider-agnostic:
//
//   {"type": "text",      "text": "..."}                           ← final-answer chunk
//   {"type": "thinking",  "text": "..."}                           ← reasoning trace (qwen3, r1, etc.)
//   {"type": "tool_use",  "name": "...", "input": {...}}           ← tool call (Ollama emits these whole)
//   {"type": "done",      "done_reason": "...", "usage": {...}}    ← terminal
//   {"type": "error",     "message": "..."}                        ← request/stream error
//
// Notes:
//   - Ollama emits tool calls as complete blocks (not streamed-by-delta
//     like Anthropic), so there's no "tool_input" event — the input hash
//     arrives whole in the tool_use event.
//   - Reasoning models split output into two phases: `thinking` (chain
//     of thought) emitted first, then `text` (the user-visible answer).
//     Set num_predict generously when prompting them or the model may
//     run out of tokens before reaching the answer phase.
//   - Same opts shape as chat().
fn stream(c, opts) {
    if !hasKey(opts, "messages") {
        return null, error("OLLAMA_BAD_ARGS", "opts.messages is required")
    }
    let msgs = _buildMessages(opts["messages"])
    if hasKey(opts, "system") {
        msgs = concat([{"role": "system", "content": opts["system"]}], msgs)
    }
    let body = {
        "model":    c.model,
        "messages": msgs,
        "stream":   true,
    }
    if hasKey(opts, "model")      { body["model"]      = opts["model"] }
    if hasKey(opts, "format")     { body["format"]     = opts["format"] }
    if hasKey(opts, "options")    { body["options"]    = opts["options"] }
    if hasKey(opts, "keep_alive") { body["keep_alive"] = opts["keep_alive"] }
    if hasKey(opts, "think")      { body["think"]      = opts["think"] }
    if hasKey(opts, "tools")      { body["tools"]      = _stripHandlers(opts["tools"]) }

    let headers = {"content-type": "application/json"}
    let rawCh, sErr = _httpStream("POST", c.baseUrl + "/api/chat",
                              headers, json.stringify(body), "lines")
    if sErr != null { return null, _classifyConnError(sErr) }

    let outCh = channel(32)
    async(fn() {
        for rawEvt in rawCh {
            let evt = _transformStreamEvent(rawEvt)
            if evt != null {
                if send(outCh, evt) == false { break }
            }
        }
        close(outCh)
    })
    return outCh, null
}


// _transformStreamEvent maps one NDJSON frame into a friendly event hash.
// Ollama's frame shape:
//   { model, created_at, message: {role, content, thinking, tool_calls},
//     done, done_reason?, eval_count?, prompt_eval_count?, … }
// We turn each frame into:
//   - "thinking" event when message.thinking is non-empty (reasoning models
//     like qwen3, deepseek-r1 emit a chain-of-thought before the answer)
//   - "text" event when message.content is non-empty (the final answer)
//   - "tool_use" event when message.tool_calls is non-empty
//   - "done" event when done=true
fn _transformStreamEvent(rawEvt) {
    // Mid-stream error (from _httpStream's error frame).
    if rawEvt["event"] == "error" {
        return {"type": "error", "message": rawEvt["data"]}
    }

    let payload, perr = json.parse(rawEvt["data"])
    if perr != null {
        return {"type": "error", "message": "stream parse error: " + str(perr)}
    }

    // Ollama embeds errors in the body too.
    if type(payload) == "HASH" && hasKey(payload, "error") {
        return {"type": "error", "message": payload["error"]}
    }

    // Surface any tool calls in this frame as discrete events.
    // (Ollama may emit them in the same frame as the done=true signal,
    // so we handle these before the done branch below.)
    if hasKey(payload, "message") && type(payload["message"]) == "HASH" {
        let m = payload["message"]
        if hasKey(m, "tool_calls") && len(m["tool_calls"]) > 0 {
            // Multi-tool: emit one tool_use event per call. The caller
            // collects them up to the "done" signal.
            // We can only return a single event from this function; for
            // simplicity v1 returns the FIRST tool call and ignores any
            // subsequent ones in the same frame (rare in practice).
            let tc = m["tool_calls"][0]
            if hasKey(tc, "function") && type(tc["function"]) == "HASH" {
                let fn_call = tc["function"]
                let out = {
                    "type": "tool_use",
                    "name": "",
                    "input": {},
                }
                if hasKey(fn_call, "name")      { out["name"]  = fn_call["name"] }
                if hasKey(fn_call, "arguments") { out["input"] = fn_call["arguments"] }
                return out
            }
        }
    }

    // Done frame — wraps up.
    if hasKey(payload, "done") && payload["done"] {
        let out = {"type": "done"}
        if hasKey(payload, "done_reason") { out["done_reason"] = payload["done_reason"] }
        // Build a usage hash for callers wanting per-token counts.
        let usage = {}
        if hasKey(payload, "prompt_eval_count") { usage["input_tokens"]  = payload["prompt_eval_count"] }
        if hasKey(payload, "eval_count")        { usage["output_tokens"] = payload["eval_count"] }
        out["usage"] = usage
        return out
    }

    // Content / thinking delta. Reasoning models emit chain-of-thought
    // in `thinking` first, then the final answer in `content`. Surface
    // both so the caller can decide what to show.
    if hasKey(payload, "message") && type(payload["message"]) == "HASH" {
        let m = payload["message"]
        if hasKey(m, "content") && len(m["content"]) > 0 {
            return {"type": "text", "text": m["content"]}
        }
        if hasKey(m, "thinking") && len(m["thinking"]) > 0 {
            return {"type": "thinking", "text": m["thinking"]}
        }
    }

    return null
}


// ── Agent loop ────────────────────────────────────────────────────────────

// runAgent executes a multi-turn tool-use loop against Ollama. Mirrors
// the shape of anthropic.lex's runAgent — same opts hash, same return
// shape — but uses Ollama's tool_calls/tool conversation format.
//
// Each tool hash should be either:
//   Anthropic-shape: {name, description, input_schema, handler}
//   Ollama-shape:    {type:"function", function:{name, description, parameters, handler}}
// _stripHandlers normalises to Ollama-shape before sending.
//
// Returns (final_response, conversation, err).
fn runAgent(c, opts, maxSteps) {
    let cap = maxSteps
    if cap <= 0 { cap = DEFAULT_MAX_STEPS }

    let handlers = _toolHandlersFromOpts(opts)
    let conv = _buildMessages(opts["messages"])
    if hasKey(opts, "system") {
        conv = concat([{"role": "system", "content": opts["system"]}], conv)
    }

    let step = 0
    while step < cap {
        let callOpts = {}
        for k, v in opts {
            // Skip "system" (we already merged it into conv) and "messages"
            // (we're overriding with the running conv).
            if k != "system" && k != "messages" {
                callOpts[k] = v
            }
        }
        callOpts["messages"] = conv

        let resp, err = chat(c, callOpts)
        if err != null { return null, conv, err }

        // Record the assistant turn in the conversation.
        let assistantMsg = resp["message"]
        if assistantMsg == null { assistantMsg = {"role": "assistant", "content": ""} }
        conv = concat(conv, [assistantMsg])

        let tcalls = toolCallsOf(resp)
        if len(tcalls) == 0 {
            return resp, conv, null
        }

        // Dispatch each tool call, appending a tool-role result message
        // per call. Ollama expects them as role="tool" messages.
        for tc in tcalls {
            if !hasKey(tc, "function") { continue }
            let f = tc["function"]
            let tname  = f["name"]
            let targs  = {}
            if hasKey(f, "arguments") { targs = f["arguments"] }

            let resultText = ""
            if !hasKey(handlers, tname) {
                resultText = "tool not registered: " + tname
            } else {
                let out, herr = safe(handlers[tname], targs)
                if herr != null {
                    resultText = "handler error: " + herr.message
                } else {
                    if type(out) == "STRING" { resultText = out }
                    else                     { resultText = str(out) }
                }
            }
            conv = concat(conv, [{"role": "tool", "content": resultText}])
        }
        step = step + 1
    }

    return null, conv, error("OLLAMA_MAX_STEPS",
        "agent exceeded maxSteps=" + str(cap) + " without terminating")
}
