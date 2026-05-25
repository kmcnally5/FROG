// anthropic.lex — Anthropic Claude API as a first-class kLex library.
// @module    anthropic
// @version   1.0.0
// @since     klex 0.3.35
// @author    karl
// @summary   Anthropic Claude API as a first-class kLex library.
//
// Pure-HTTP implementation built on stdlib/rest.lex + stdlib/json.lex.
// Zero external dependencies — no Python or Node bridge required.
//
// Reference: https://docs.anthropic.com/en/api/messages
//
// Authentication: pass your API key to newClient() at construction time.
// Get a key from https://console.anthropic.com/.
//
// Usage:
//   import "stdlib/ai/anthropic.lex" as claude
//   import "stdlib/ai/ai_common.lex"  as ai
//
//   c = claude.newClient(env("ANTHROPIC_API_KEY"), "claude-opus-4-7")
//   resp, err = claude.messages(c, {
//       "system":   "You are concise.",
//       "messages": [ai.userMsg("What's 2+2?")],
//   })
//   if err != null { println(err.message)  return }
//   println(claude.textOf(resp))


import "stdlib/rest.lex"         as rest
import "stdlib/json.lex"         as json
import "stdlib/ai/ai_common.lex" as ai


// ── Constants ─────────────────────────────────────────────────────────────

const API_BASE_URL       = "https://api.anthropic.com/v1"
const API_VERSION        = "2023-06-01"
const DEFAULT_MODEL      = "claude-opus-4-7"
const DEFAULT_MAX_TOKENS = 4096
const DEFAULT_MAX_STEPS  = 12     // agent-loop iteration cap


// Pricing table — USD per 1M tokens, [input_rate, output_rate]. Used by
// the ai cost ledger (see stdlib/ai/ai_common.lex). Rates as published
// on anthropic.com/pricing; update here if Anthropic revises. Prompt-
// cache reads/writes are billed at modified rates by Anthropic but we
// record them at the base input rate in v1 — close enough for budget
// safety; tighten later if accuracy matters more than simplicity.
let _RATES = {
    "claude-opus-4-7":          [15.0, 75.0],
    "claude-opus-4-5":          [15.0, 75.0],
    "claude-opus-4-1":          [15.0, 75.0],
    "claude-opus-4":            [15.0, 75.0],
    "claude-sonnet-4-6":        [3.0,  15.0],
    "claude-sonnet-4-5":        [3.0,  15.0],
    "claude-sonnet-4":          [3.0,  15.0],
    "claude-haiku-4-5":         [1.0,   5.0],
    "claude-haiku-4-5-20251001":[1.0,   5.0],
    "claude-3-5-haiku":         [0.80,  4.0],
    "claude-3-5-sonnet":        [3.0,  15.0],
    "claude-3-opus":            [15.0, 75.0],
}


// _rateFor returns [input_rate, output_rate] for a model name. Falls
// back to Sonnet pricing for unknown future model IDs — conservative
// (mid-tier) so unknowns don't silently look free.
fn _rateFor(model) {
    if model != null && hasKey(_RATES, model) {
        return _RATES[model]
    }
    // Try a prefix match for date-suffixed IDs like "claude-sonnet-4-6-20251022".
    for k in keys(_RATES) {
        if len(model) >= len(k) && substr(model, 0, len(k)) == k {
            return _RATES[k]
        }
    }
    return [3.0, 15.0]
}


// ── Client ────────────────────────────────────────────────────────────────

// Client carries per-conversation defaults. Construct via newClient() and
// pass it to every API call. Fields are intentionally minimal in v1 — room
// to grow for timeouts, proxy URLs, retry policy, etc.
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

// _headers builds the standard Anthropic header set from a client.
fn _headers(c) {
    return {
        "x-api-key":         c.apiKey,
        "anthropic-version": API_VERSION,
        "content-type":      "application/json",
    }
}


// _classifyError turns a non-2xx response into a typed kLex error.
fn _classifyError(resp) {
    let code = "ANTHROPIC_HTTP_ERROR"
    let msg  = "Anthropic API returned status " + str(resp.status)

    if resp.status == 401 {
        code = "ANTHROPIC_AUTH"
        msg  = "invalid or missing API key"
    } else if resp.status == 403 {
        code = "ANTHROPIC_FORBIDDEN"
        msg  = "access denied"
    } else if resp.status == 404 {
        code = "ANTHROPIC_NOT_FOUND"
        msg  = "endpoint or model not found"
    } else if resp.status == 429 {
        code = "ANTHROPIC_RATE_LIMIT"
        msg  = "rate limit exceeded"
    } else if resp.status >= 500 {
        code = "ANTHROPIC_SERVER"
        msg  = "Anthropic server error (status " + str(resp.status) + ")"
    }

    // Surface the API's own error message when present.
    if type(resp.data) == "HASH" && hasKey(resp.data, "error") {
        let e = resp.data["error"]
        if type(e) == "HASH" && hasKey(e, "message") {
            msg = msg + ": " + e["message"]
        }
    }
    return error(code, msg)
}


// _buildMessages normalises an array of either ai.Message structs or
// plain message hashes into the wire format Anthropic expects.
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


// _stripHandlers removes the kLex-side `handler` field from tool defs
// before sending to Anthropic — the API rejects unknown fields.
fn _stripHandlers(tools) {
    let out = makeArray(0)
    for t in tools {
        let clean = {}
        for k, v in t {
            if k != "handler" { clean[k] = v }
        }
        out = concat(out, [clean])
    }
    return out
}


// ── Primary API ───────────────────────────────────────────────────────────

// messages calls /v1/messages — Anthropic's canonical chat-completion
// endpoint. Returns the parsed response on success, typed error on failure.
//
// opts is a hash; the only required field is "messages" (array of Messages
// or message hashes). Optional fields with sensible defaults:
//   model           — overrides client default
//   max_tokens      — defaults to DEFAULT_MAX_TOKENS (4096)
//   system          — system prompt string
//   temperature     — float 0.0–1.0
//   top_p, top_k    — sampling controls
//   stop_sequences  — array of strings
//   tools           — array of tool hashes (see runAgent for handler-bearing form)
//   tool_choice     — {"type": "auto"} | {"type": "tool", "name": "..."}
//   metadata        — passthrough hash (user_id, etc.)
//
// Returns (response_hash, err). Response shape mirrors Anthropic's wire:
//   { id, type, role, content, model, stop_reason, stop_sequence, usage }
// Use textOf(resp) / toolCallsOf(resp) / usageOf(resp) for friendlier access.
fn messages(c, opts) {
    if !hasKey(opts, "messages") {
        return null, error("ANTHROPIC_BAD_ARGS", "opts.messages is required")
    }
    if ai.budgetExceeded() { return null, ai._budgetError() }

    let body = {
        "model":      c.model,
        "max_tokens": DEFAULT_MAX_TOKENS,
        "messages":   _buildMessages(opts["messages"]),
    }
    if hasKey(opts, "model")          { body["model"]          = opts["model"] }
    if hasKey(opts, "max_tokens")     { body["max_tokens"]     = opts["max_tokens"] }
    if hasKey(opts, "system")         { body["system"]         = opts["system"] }
    if hasKey(opts, "temperature")    { body["temperature"]    = opts["temperature"] }
    if hasKey(opts, "top_p")          { body["top_p"]          = opts["top_p"] }
    if hasKey(opts, "top_k")          { body["top_k"]          = opts["top_k"] }
    if hasKey(opts, "stop_sequences") { body["stop_sequences"] = opts["stop_sequences"] }
    if hasKey(opts, "tool_choice")    { body["tool_choice"]    = opts["tool_choice"] }
    if hasKey(opts, "metadata")       { body["metadata"]       = opts["metadata"] }
    if hasKey(opts, "tools") {
        body["tools"] = _stripHandlers(opts["tools"])
    }

    let resp, httpErr = rest.postWith(c.baseUrl + "/messages", body, _headers(c))
    if httpErr != null { return null, error("ANTHROPIC_NETWORK", httpErr) }
    if !rest.isOk(resp) { return null, _classifyError(resp) }

    // Record usage against the ai cost ledger so spent() / usage() /
    // budget() see this call.
    let u = usageOf(resp.data)
    let rates = _rateFor(body["model"])
    ai._record(body["model"], u["input_tokens"], u["output_tokens"], rates[0], rates[1])

    return resp.data, null
}


// ── Structured output ────────────────────────────────────────────────────

const _STRUCTURED_TOOL = "_structured_output"

// complete asks the model to return a structured response matching the
// supplied JSON Schema. Internally uses Anthropic's tool-use mechanism
// with a synthetic single-purpose tool — forces the model to emit JSON
// that conforms to the schema. Returns the parsed structured data as a
// kLex hash (bracket-access).
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
//   person, err = claude.complete(c, PersonSchema, "Generate a fake profile")
//   if err != null { println(err.message)  return }
//   println(person["name"])
//
// For more control (system prompt, temperature, model override, …) use
// completeWith(c, schema, prompt, opts).
fn complete(c, jsonSchema, prompt) {
    return completeWith(c, jsonSchema, prompt, null)
}

// completeWith(c, jsonSchema, prompt, opts) — structured-output completion with full control over options.
// opts hash may include: system, temperature, model, max_tokens. Returns (parsedHash, err).
fn completeWith(c, jsonSchema, prompt, opts) {
    let syntheticTool = {
        "name":         _STRUCTURED_TOOL,
        "description":  "Return the requested structured data via this tool. The data MUST conform exactly to input_schema.",
        "input_schema": jsonSchema,
    }
    let callOpts = {
        "messages":    [ai.userMsg(prompt)],
        "tools":       [syntheticTool],
        "tool_choice": {"type": "tool", "name": _STRUCTURED_TOOL},
        "max_tokens":  4096,
    }
    if opts != null {
        if hasKey(opts, "system")      { callOpts["system"]      = opts["system"] }
        if hasKey(opts, "max_tokens")  { callOpts["max_tokens"]  = opts["max_tokens"] }
        if hasKey(opts, "temperature") { callOpts["temperature"] = opts["temperature"] }
        if hasKey(opts, "model")       { callOpts["model"]       = opts["model"] }
        if hasKey(opts, "messages")    { callOpts["messages"]    = opts["messages"] }
    }

    let resp, err = messages(c, callOpts)
    if err != null { return null, err }

    // The tool_use block's `input` field IS the structured output.
    if hasKey(resp, "content") {
        for block in resp["content"] {
            if type(block) == "HASH" && hasKey(block, "type") {
                if block["type"] == "tool_use" && block["name"] == _STRUCTURED_TOOL {
                    return block["input"], null
                }
            }
        }
    }
    return null, error("ANTHROPIC_NO_STRUCTURED_OUTPUT",
        "model did not emit the structured-output tool call")
}


// countTokens calls /v1/messages/count_tokens — useful for budgeting before
// committing to a real generation. Same opts as messages() (minus
// max_tokens, which is irrelevant for counting). Returns (int, err).
fn countTokens(c, opts) {
    if !hasKey(opts, "messages") {
        return 0, error("ANTHROPIC_BAD_ARGS", "opts.messages is required")
    }
    let body = {
        "model":    c.model,
        "messages": _buildMessages(opts["messages"]),
    }
    if hasKey(opts, "model")  { body["model"]  = opts["model"] }
    if hasKey(opts, "system") { body["system"] = opts["system"] }
    if hasKey(opts, "tools")  { body["tools"]  = _stripHandlers(opts["tools"]) }

    let resp, httpErr = rest.postWith(c.baseUrl + "/messages/count_tokens", body, _headers(c))
    if httpErr != null { return 0, error("ANTHROPIC_NETWORK", httpErr) }
    if !rest.isOk(resp) { return 0, _classifyError(resp) }
    if type(resp.data) == "HASH" && hasKey(resp.data, "input_tokens") {
        return resp.data["input_tokens"], null
    }
    return 0, error("ANTHROPIC_PARSE", "no input_tokens in response")
}


// listModels calls /v1/models to enumerate available models. Returns
// (array_of_model_hashes, err).
fn listModels(c) {
    let resp, httpErr = rest.getWith(c.baseUrl + "/models", _headers(c))
    if httpErr != null { return null, error("ANTHROPIC_NETWORK", httpErr) }
    if !rest.isOk(resp) { return null, _classifyError(resp) }
    if type(resp.data) == "HASH" && hasKey(resp.data, "data") {
        return resp.data["data"], null
    }
    return null, error("ANTHROPIC_PARSE", "no data array in models response")
}


// ── Response accessors ────────────────────────────────────────────────────

// textOf returns the concatenated text from every "text" content block in
// a response. Returns "" if no text content (e.g. response is all tool_use).
fn textOf(resp) {
    if type(resp) != "HASH"      { return "" }
    if !hasKey(resp, "content")  { return "" }
    let out = ""
    for block in resp["content"] {
        if type(block) == "HASH" && hasKey(block, "type") {
            if block["type"] == "text" && hasKey(block, "text") {
                out = out + block["text"]
            }
        }
    }
    return out
}


// toolCallsOf returns the tool_use content blocks from a response — each
// is a hash {type, id, name, input}. Empty array if none.
fn toolCallsOf(resp) {
    let out = makeArray(0)
    if type(resp) != "HASH"     { return out }
    if !hasKey(resp, "content") { return out }
    for block in resp["content"] {
        if type(block) == "HASH" && hasKey(block, "type") && block["type"] == "tool_use" {
            out = concat(out, [block])
        }
    }
    return out
}


// stopReasonOf returns the response's stop_reason. Common values:
// "end_turn", "max_tokens", "stop_sequence", "tool_use".
fn stopReasonOf(resp) {
    if type(resp) != "HASH"          { return "" }
    if !hasKey(resp, "stop_reason")  { return "" }
    return resp["stop_reason"]
}


// usageOf returns a hash {input_tokens, output_tokens, cost_usd} from a
// response's usage block. cost_usd stays null — Anthropic doesn't report
// per-call cost; compute it from your account's rate table if needed.
//
// Hash shape (not ai.Usage struct) — cross-module struct literals don't
// construct in kLex.
fn usageOf(resp) {
    let inT  = 0
    let outT = 0
    if type(resp) == "HASH" && hasKey(resp, "usage") {
        let u = resp["usage"]
        if type(u) == "HASH" {
            if hasKey(u, "input_tokens")  { inT  = u["input_tokens"] }
            if hasKey(u, "output_tokens") { outT = u["output_tokens"] }
        }
    }
    return {"input_tokens": inT, "output_tokens": outT, "cost_usd": null}
}


// toolResultBlock builds a tool_result content block to send back to the
// model after dispatching a tool_use. `content` may be a string or an
// array of content blocks (text/image). Set isError=true on failure to
// hint to the model that the tool didn't succeed.
fn toolResultBlock(toolUseId, content, isError) {
    return {
        "type":        "tool_result",
        "tool_use_id": toolUseId,
        "content":     content,
        "is_error":    isError,
    }
}


// ── Streaming ─────────────────────────────────────────────────────────────

// stream calls /v1/messages with stream=true and returns a kLex channel
// of friendly event hashes. The channel closes when the stream ends or
// when the consumer breaks out of the for-in loop (which cancels the
// underlying HTTP connection — no leaked sockets).
//
// Same opts as messages(); v1 omits the streaming-specific fields
// Anthropic supports (none, currently — stream=true is what enables it).
//
// Returns (channel, err). The channel yields hashes with a `type` field:
//
//   {"type": "text",        "text": "..."}                          ← most common; concatenate to render
//   {"type": "tool_use",    "id": "...", "name": "..."}             ← tool call begins
//   {"type": "tool_input",  "partial_json": "..."}                  ← streaming tool-input JSON fragment
//   {"type": "done",        "stop_reason": "...", "usage": {...}}   ← terminal event
//   {"type": "error",       "message": "..."}                       ← request/stream error
//
// Notes:
//   - Tool inputs arrive as a sequence of "tool_input" events whose
//     `partial_json` strings must be concatenated and JSON-parsed to get
//     the final tool input hash. The final concatenation produces valid
//     JSON only after the matching content_block_stop event (i.e. when
//     no more "tool_input" events for that tool will be emitted).
//   - "done" always carries stop_reason; usage may include only
//     output_tokens (input_tokens are visible in messages() if needed).
fn stream(c, opts) {
    if !hasKey(opts, "messages") {
        return null, error("ANTHROPIC_BAD_ARGS", "opts.messages is required")
    }
    let body = {
        "model":      c.model,
        "max_tokens": DEFAULT_MAX_TOKENS,
        "messages":   _buildMessages(opts["messages"]),
        "stream":     true,
    }
    if hasKey(opts, "model")          { body["model"]          = opts["model"] }
    if hasKey(opts, "max_tokens")     { body["max_tokens"]     = opts["max_tokens"] }
    if hasKey(opts, "system")         { body["system"]         = opts["system"] }
    if hasKey(opts, "temperature")    { body["temperature"]    = opts["temperature"] }
    if hasKey(opts, "top_p")          { body["top_p"]          = opts["top_p"] }
    if hasKey(opts, "top_k")          { body["top_k"]          = opts["top_k"] }
    if hasKey(opts, "stop_sequences") { body["stop_sequences"] = opts["stop_sequences"] }
    if hasKey(opts, "tool_choice")    { body["tool_choice"]    = opts["tool_choice"] }
    if hasKey(opts, "metadata")       { body["metadata"]       = opts["metadata"] }
    if hasKey(opts, "tools")          { body["tools"]          = _stripHandlers(opts["tools"]) }

    let rawCh, sErr = _httpStream("POST", c.baseUrl + "/messages",
                              _headers(c), json.stringify(body))
    if sErr != null { return null, error("ANTHROPIC_NETWORK", sErr) }

    // Transform raw SSE frames into the friendly event shapes above.
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


// _transformStreamEvent maps a raw SSE frame ({event, data}) into one of
// the friendly event shapes documented on stream(). Returns null for
// events the consumer doesn't need (ping, bookkeeping, etc.).
fn _transformStreamEvent(rawEvt) {
    let evName = rawEvt["event"]
    let rawData = rawEvt["data"]

    if evName == "error" {
        // _httpStream's mid-stream error frame — data is a plain string,
        // not JSON. Pass through as a friendly error event.
        return {"type": "error", "message": rawData}
    }

    if evName == "ping" || evName == "message_start" || evName == "content_block_stop" || evName == "message_stop" {
        return null
    }

    let payload, perr = json.parse(rawData)
    if perr != null {
        return {"type": "error", "message": "stream parse error: " + str(perr)}
    }

    if evName == "content_block_start" {
        let block = payload["content_block"]
        if type(block) == "HASH" && hasKey(block, "type") && block["type"] == "tool_use" {
            return {
                "type": "tool_use",
                "id":   block["id"],
                "name": block["name"],
            }
        }
        return null
    }

    if evName == "content_block_delta" {
        let d = payload["delta"]
        if type(d) != "HASH" || !hasKey(d, "type") { return null }
        if d["type"] == "text_delta" {
            return {"type": "text", "text": d["text"]}
        }
        if d["type"] == "input_json_delta" {
            return {"type": "tool_input", "partial_json": d["partial_json"]}
        }
        return null
    }

    if evName == "message_delta" {
        let out = {"type": "done"}
        if hasKey(payload, "delta") && type(payload["delta"]) == "HASH" {
            if hasKey(payload["delta"], "stop_reason") {
                out["stop_reason"] = payload["delta"]["stop_reason"]
            }
        }
        if hasKey(payload, "usage") {
            out["usage"] = payload["usage"]
        }
        return out
    }

    return null
}


// ── Agent loop ────────────────────────────────────────────────────────────

// runAgent executes a multi-turn tool-use loop. The model is invoked with
// the conversation + tool registrations; when it emits tool_use blocks the
// loop dispatches them to the kLex handler functions in each tool, then
// feeds the results back as a user message containing tool_result blocks.
// Terminates when the model's stop_reason is not "tool_use".
//
// opts has the same shape as messages(). Each tool hash must include:
//   "name"         — string identifier (matched against tool_use blocks)
//   "description"  — string for the model
//   "input_schema" — JSON Schema hash describing the input
//   "handler"      — kLex function: fn(input_hash) -> any
//
// maxSteps caps the loop to prevent runaway costs. Pass 0 or a negative
// value to use DEFAULT_MAX_STEPS (12). Each step is one round-trip to the
// API — so maxSteps=12 means at most 12 model calls before bailing.
//
// Returns (final_response, conversation, err):
//   final_response is the last messages() response whose stop_reason
//                  was not "tool_use".
//   conversation   is the full message history — useful for logging,
//                  persistence, or feeding into a future turn.
//   err            is non-null on HTTP failure or step-limit exceeded.
fn runAgent(c, opts, maxSteps) {
    let cap = maxSteps
    if cap <= 0 { cap = DEFAULT_MAX_STEPS }

    // Build name → handler dispatch table from opts.tools.
    let handlers = {}
    if hasKey(opts, "tools") {
        for t in opts["tools"] {
            if type(t) == "HASH" && hasKey(t, "name") && hasKey(t, "handler") {
                handlers[t["name"]] = t["handler"]
            }
        }
    }

    // Accumulating conversation, starts from caller's messages.
    let conv = _buildMessages(opts["messages"])

    let step = 0
    while step < cap {
        // Copy opts but swap in our running conversation.
        let callOpts = {}
        for k, v in opts {
            callOpts[k] = v
        }
        callOpts["messages"] = conv

        let resp, err = messages(c, callOpts)
        if err != null { return null, conv, err }

        // Record the assistant turn verbatim (content blocks preserved).
        conv = concat(conv, [{ "role": "assistant", "content": resp["content"] }])

        if stopReasonOf(resp) != "tool_use" {
            return resp, conv, null
        }

        // Dispatch every tool_use block in the assistant turn, building
        // a single user message with all the tool_result blocks.
        let toolBlocks   = toolCallsOf(resp)
        let resultBlocks = makeArray(0)
        for tb in toolBlocks {
            let tname  = tb["name"]
            let tid    = tb["id"]
            let tinput = tb["input"]

            if !hasKey(handlers, tname) {
                resultBlocks = concat(resultBlocks,
                    [toolResultBlock(tid, "tool not registered: " + tname, true)])
            } else {
                let handler = handlers[tname]
                let out, herr = safe(handler, tinput)
                if herr != null {
                    resultBlocks = concat(resultBlocks,
                        [toolResultBlock(tid, "handler error: " + herr.message, true)])
                } else {
                    // Anthropic accepts strings or content-block arrays
                    // as tool_result content. Stringify anything else.
                    let payload = out
                    if type(out) != "STRING" && type(out) != "ARRAY" {
                        payload = str(out)
                    }
                    resultBlocks = concat(resultBlocks,
                        [toolResultBlock(tid, payload, false)])
                }
            }
        }

        conv = concat(conv, [{ "role": "user", "content": resultBlocks }])
        step = step + 1
    }

    return null, conv, error("ANTHROPIC_MAX_STEPS",
        "agent exceeded maxSteps=" + str(cap) + " without terminating")
}


// ── Message Batches API ──────────────────────────────────────────────────
//
// Anthropic's Message Batches lets you submit up to 100,000 messages in a
// single async batch at 50% of the regular Messages API cost. Batches
// complete within 24 hours (usually much faster). Results stay available
// for 29 days. Ideal for evals, dataset generation, and any throughput-
// tolerant workload.
//
// Reference: https://docs.anthropic.com/en/api/creating-message-batches
//
// Workflow:
//   1. buildBatchRequest(customId, opts)  — shape one request
//   2. createBatch(c, requests)           — submit
//   3. awaitBatch(c, batchId, pollOpts)   — poll until done
//   4. batchResults(c, batchId)           — fetch parsed results
//
// Example:
//   prompts = ["Summarise X", "Summarise Y", "Summarise Z"]
//   reqs    = makeArray(len(prompts), null)
//   for i, p in prompts {
//       reqs[i] = claude.buildBatchRequest("req-" + str(i), {
//           "messages":   [ai.userMsg(p)],
//           "max_tokens": 512,
//       })
//   }
//   batch, err = claude.createBatch(c, reqs)
//   if err != null { println(err.message)  _osExit(1) }
//
//   final, err = claude.awaitBatch(c, batch["id"], {"pollIntervalMs": 30000})
//   if err != null { println(err.message)  _osExit(1) }
//
//   results, err = claude.batchResults(c, batch["id"])
//   for r in results {
//       // r["custom_id"]              — your original ID
//       // r["result"]["type"]         — "succeeded" | "errored" | "canceled" | "expired"
//       // r["result"]["message"]      — full Message API response when succeeded
//       // r["result"]["error"]        — error hash when errored
//   }


// buildBatchRequest wraps a normal messages-style opts hash into the
// {custom_id, params} shape required by createBatch(). Default model +
// max_tokens are filled in from the library defaults (createBatch will
// also patch any missing model from the client's default before sending).
fn buildBatchRequest(customId, opts) {
    let params = {
        "model":      DEFAULT_MODEL,
        "max_tokens": DEFAULT_MAX_TOKENS,
    }
    if hasKey(opts, "messages")       { params["messages"]       = _buildMessages(opts["messages"]) }
    if hasKey(opts, "model")          { params["model"]          = opts["model"] }
    if hasKey(opts, "max_tokens")     { params["max_tokens"]     = opts["max_tokens"] }
    if hasKey(opts, "system")         { params["system"]         = opts["system"] }
    if hasKey(opts, "temperature")    { params["temperature"]    = opts["temperature"] }
    if hasKey(opts, "top_p")          { params["top_p"]          = opts["top_p"] }
    if hasKey(opts, "top_k")          { params["top_k"]          = opts["top_k"] }
    if hasKey(opts, "stop_sequences") { params["stop_sequences"] = opts["stop_sequences"] }
    if hasKey(opts, "tool_choice")    { params["tool_choice"]    = opts["tool_choice"] }
    if hasKey(opts, "metadata")       { params["metadata"]       = opts["metadata"] }
    if hasKey(opts, "tools") {
        params["tools"] = _stripHandlers(opts["tools"])
    }
    return {"custom_id": customId, "params": params}
}


// createBatch submits a batch of requests for async processing. Returns
// (batch_hash, err) where batch_hash carries id, processing_status,
// request_counts, created_at, expires_at, etc.
//
// requests is an array of {custom_id, params} hashes — use
// buildBatchRequest() to construct each one from familiar messages-style
// opts. Any request missing "model" in its params is patched with the
// client's default before sending (Anthropic requires it per-request).
fn createBatch(c, requests) {
    if type(requests) != "ARRAY" || len(requests) == 0 {
        return null, error("ANTHROPIC_BAD_ARGS",
            "createBatch: requests must be a non-empty array")
    }
    let out = makeArray(len(requests), null)
    for i, r in requests {
        if type(r) != "HASH" || !hasKey(r, "custom_id") || !hasKey(r, "params") {
            return null, error("ANTHROPIC_BAD_ARGS",
                "createBatch: requests[" + str(i) + "] must be \{custom_id, params\}")
        }
        let p = r["params"]
        if type(p) != "HASH" {
            return null, error("ANTHROPIC_BAD_ARGS",
                "createBatch: requests[" + str(i) + "].params must be a hash")
        }
        if !hasKey(p, "model") {
            // Clone before mutating so callers' hashes are untouched.
            let cloned = {}
            for k, v in p { cloned[k] = v }
            cloned["model"] = c.model
            p = cloned
        }
        out[i] = {"custom_id": r["custom_id"], "params": p}
    }
    let resp, httpErr = rest.postWith(c.baseUrl + "/messages/batches", {"requests": out}, _headers(c))
    if httpErr != null { return null, error("ANTHROPIC_NETWORK", httpErr) }
    if !rest.isOk(resp) { return null, _classifyError(resp) }
    return resp.data, null
}


// getBatch returns the current status of a batch as a hash.
fn getBatch(c, batchId) {
    let resp, httpErr = rest.getWith(c.baseUrl + "/messages/batches/" + batchId, _headers(c))
    if httpErr != null { return null, error("ANTHROPIC_NETWORK", httpErr) }
    if !rest.isOk(resp) { return null, _classifyError(resp) }
    return resp.data, null
}


// cancelBatch attempts to cancel a batch that's still processing. Returns
// the updated batch hash. Already-ended batches return an error from the API.
fn cancelBatch(c, batchId) {
    let resp, httpErr = rest.postWith(c.baseUrl + "/messages/batches/" + batchId + "/cancel",
                                  {}, _headers(c))
    if httpErr != null { return null, error("ANTHROPIC_NETWORK", httpErr) }
    if !rest.isOk(resp) { return null, _classifyError(resp) }
    return resp.data, null
}


// deleteBatch removes a batch. Only allowed after it has ended.
fn deleteBatch(c, batchId) {
    let resp, httpErr = rest.delWith(c.baseUrl + "/messages/batches/" + batchId, _headers(c))
    if httpErr != null { return null, error("ANTHROPIC_NETWORK", httpErr) }
    if !rest.isOk(resp) { return null, _classifyError(resp) }
    return null, null
}


// listBatches enumerates batches in this workspace. opts (optional) may
// contain pagination params:
//   limit     : int    — page size (1-100, default 20)
//   before_id : string — cursor for paging backward
//   after_id  : string — cursor for paging forward
// Returns (array_of_batch_hashes, err).
fn listBatches(c, opts = null) {
    let url = c.baseUrl + "/messages/batches"
    if opts != null {
        let qs = ""
        if hasKey(opts, "limit") {
            qs = qs + "limit=" + str(opts["limit"])
        }
        if hasKey(opts, "before_id") {
            if len(qs) > 0 { qs = qs + "&" }
            qs = qs + "before_id=" + opts["before_id"]
        }
        if hasKey(opts, "after_id") {
            if len(qs) > 0 { qs = qs + "&" }
            qs = qs + "after_id=" + opts["after_id"]
        }
        if len(qs) > 0 { url = url + "?" + qs }
    }
    let resp, httpErr = rest.getWith(url, _headers(c))
    if httpErr != null { return null, error("ANTHROPIC_NETWORK", httpErr) }
    if !rest.isOk(resp) { return null, _classifyError(resp) }
    if type(resp.data) == "HASH" && hasKey(resp.data, "data") {
        return resp.data["data"], null
    }
    return null, error("ANTHROPIC_PARSE", "no data array in list response")
}


// batchResults fetches the JSONL results stream for a completed batch and
// parses it into an array of {custom_id, result} hashes. Each entry's
// result.type is one of "succeeded", "errored", "canceled", "expired".
//
// On success: r["result"]["message"] is the full Message API response.
// On failure: r["result"]["error"]   is the error hash.
//
// Only works once the batch's processing_status has reached "ended" —
// call awaitBatch() first, or check getBatch() yourself.
fn batchResults(c, batchId) {
    let status, sErr = getBatch(c, batchId)
    if sErr != null { return null, sErr }
    if type(status) != "HASH" || !hasKey(status, "results_url") {
        return null, error("ANTHROPIC_BATCH_NOT_READY",
            "batch has no results_url — has processing_status reached 'ended'?")
    }
    let url = status["results_url"]
    if url == null {
        return null, error("ANTHROPIC_BATCH_NOT_READY",
            "results_url is null — batch may still be processing")
    }
    let resp, httpErr = rest.getWith(url, _headers(c))
    if httpErr != null { return null, error("ANTHROPIC_NETWORK", httpErr) }
    if !rest.isOk(resp) { return null, _classifyError(resp) }

    // Results endpoint returns application/x-jsonl — rest.lex leaves
    // non-JSON Content-Types as the raw body string in resp.data.
    let raw = resp.data
    if type(raw) != "STRING" {
        return null, error("ANTHROPIC_PARSE",
            "expected JSONL results body as string, got " + type(raw))
    }
    let lines = split(raw, "\n")

    // Two-pass: count non-empty lines, preallocate, fill.
    let total = 0
    for ln in lines {
        if len(ln) > 0 { total = total + 1 }
    }
    let out = makeArray(total, null)
    let idx = 0
    for ln in lines {
        if len(ln) == 0 { continue }
        let parsed, pErr = json.parse(ln)
        if pErr != null {
            return null, error("ANTHROPIC_PARSE",
                "results line " + str(idx) + ": " + str(pErr))
        }
        out[idx] = parsed
        idx = idx + 1
    }
    return out, null
}


// awaitBatch polls getBatch() until processing_status reaches a terminal
// state ("ended", "canceled", or "expired") or the deadline elapses.
// Returns the final batch hash.
//
// opts (optional):
//   pollIntervalMs : int — sleep between polls (default 30000 = 30s)
//   deadlineMs     : int — total wait budget (default 0 = no deadline)
//   onPoll         : fn(status) — called after each successful getBatch
//
// Anthropic recommends polling no more than once every ~30s.
fn awaitBatch(c, batchId, opts = null) {
    let pollMs   = 30000
    let deadline = 0
    let onPoll   = null
    if opts != null {
        if hasKey(opts, "pollIntervalMs") { pollMs   = opts["pollIntervalMs"] }
        if hasKey(opts, "deadlineMs")     { deadline = opts["deadlineMs"] }
        if hasKey(opts, "onPoll")         { onPoll   = opts["onPoll"] }
    }
    let startNs = _timeNanos()
    while true {
        let status, err = getBatch(c, batchId)
        if err != null { return null, err }
        if onPoll != null { onPoll(status) }
        let ps = ""
        if hasKey(status, "processing_status") {
            ps = status["processing_status"]
        }
        if ps == "ended" || ps == "canceled" || ps == "expired" {
            return status, null
        }
        if deadline > 0 {
            let elapsedMs = (_timeNanos() - startNs) / 1000000
            if elapsedMs >= deadline {
                return null, error("ANTHROPIC_BATCH_DEADLINE",
                    "batch did not reach a terminal state within " +
                    str(deadline) + "ms — last processing_status: " + ps)
            }
        }
        sleep(pollMs)
    }
    return null, error("ANTHROPIC_BATCH_DEADLINE", "unreachable")
}
