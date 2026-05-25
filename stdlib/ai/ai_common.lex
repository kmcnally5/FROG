// ai_common.lex — shared vocabulary for AI provider library sets.
// @module    ai_common
// @version   1.0.0
// @since     klex 0.3.35
// @author    karl
// @summary   shared vocabulary for AI provider library sets.
//
// Every stdlib/ai/* library set imports this for the cross-provider types
// (Message, Usage) and content-block constructors (textBlock, imageBlock).
// Provider-specific shapes (response wire formats, tool result formatting,
// streaming events) belong in the per-provider library, NOT here — we
// deliberately resist the lowest-common-denominator trap that would limit
// the feature richness of any one provider library.
//
// See docs/AI_LIBRARY_SETS.md for the design pattern and authoring guide.
//
// Usage:
//   import "stdlib/ai/ai_common.lex" as ai
//
//   msgs = [
//       ai.userMsg("Summarise this paragraph:"),
//       ai.assistantMsg("Sure, paste it in."),
//       ai.userMsg("..."),
//   ]


// Message is the canonical chat-message shape every provider accepts as
// input. `content` is either:
//   - a plain string (most common — e.g. "What's the weather?")
//   - an array of content blocks (mixed text + image, tool I/O, etc.)
// Provider libraries translate this to their wire format internally.
struct Message {
    role, content
}


// Usage tracks token consumption from a model call. `cost_usd` is filled
// in by providers that report cost directly; null otherwise — callers can
// compute it from token counts + their own rate table.
struct Usage {
    input_tokens, output_tokens, cost_usd
}


// userMsg returns a Message with role="user". `content` may be a plain
// string or an array of content blocks (text + image, tool_result, …).
fn userMsg(content) {
    return Message { role: "user", content: content }
}


// assistantMsg returns a Message with role="assistant". Use this to feed
// prior assistant turns back into a multi-turn conversation.
fn assistantMsg(content) {
    return Message { role: "assistant", content: content }
}


// textBlock returns a single text content block — handy when building an
// array of mixed-content blocks (e.g. text alongside a tool_result).
fn textBlock(text) {
    return { "type": "text", "text": text }
}


// imageBlock returns an image content block from base64-encoded image
// bytes. `mediaType` is the MIME type, e.g. "image/png" or "image/jpeg".
// Works as-is on Anthropic; other providers may need translation.
fn imageBlock(base64Data, mediaType) {
    return {
        "type":   "image",
        "source": {
            "type":       "base64",
            "media_type": mediaType,
            "data":       base64Data,
        },
    }
}


// NOTE on tool definitions: there is intentionally NO `Tool` struct here.
// Tool schemas (input_schema field shape, supported JSON Schema dialect,
// constraints on names) vary subtly across providers. Each provider library
// documents its own tool-hash shape — typically {name, description,
// input_schema, handler?} — and accepts hashes directly. Trying to abstract
// over them would re-introduce the LCD problem that motivated this whole
// library-set architecture.


// ── Cost ledger ─────────────────────────────────────────────────────────
//
// Process-wide usage + spend tracking shared across every provider in
// stdlib/ai. Each provider library calls _record() after a successful
// API response; callers read spent() / usage() and can cap further
// spending with budget(usd).
//
// Enforcement is post-hoc: providers cannot predict output token counts,
// so we record actual usage after each response. A pre-flight check
// (budgetExceeded()) lets call sites bail early once the cap is hit.
//
// Pricing rates live in the provider libraries (anthropic.lex knows
// Claude rates; ollama.lex records at $0; etc.) — this file only owns
// the accumulator.

let _aiUsage  = {}      // model name → {in_tokens, out_tokens, in_cost, out_cost}
let _aiSpent  = 0.0     // running USD total across all providers
let _aiBudget = null    // USD cap; null = unlimited


// budget sets a USD spending cap for the rest of the process. Pass null
// (or a negative number) to clear it. After the cap is exceeded, calls
// that check budgetExceeded() will short-circuit with a BUDGET_EXCEEDED
// error before sending another request to the provider.
fn budget(usd) {
    if usd == null { _aiBudget = null   return }
    if type(usd) != "FLOAT" && type(usd) != "INTEGER" { _aiBudget = null   return }
    let f = float(usd)
    if f < 0.0 { _aiBudget = null   return }
    _aiBudget = f
}


// spent returns the cumulative USD spent across every provider call
// recorded in this process. Local providers (Ollama, Local SD) record
// at $0 so they show up in usage() but don't move spent().
fn spent() {
    return _aiSpent
}


// budgetExceeded is the cheap pre-flight check provider libraries call
// before sending a request. Returns true if a cap has been set and
// already-recorded spend has met or passed it.
fn budgetExceeded() {
    if _aiBudget == null { return false }
    return _aiSpent >= _aiBudget
}


// usage returns the per-model breakdown hash. Keys are model names;
// values are {in_tokens, out_tokens, in_cost, out_cost}. Useful for
// "where did the money go?" reporting at end-of-run.
fn usage() {
    return _aiUsage
}


// resetUsage zeroes the ledger. The budget cap is preserved — call
// budget(null) separately if you want to lift the cap too.
fn resetUsage() {
    _aiUsage = {}
    _aiSpent = 0.0
}


// _record is the internal hook every provider library calls after a
// successful API response. `inRate` and `outRate` are USD per 1M tokens
// (so $3/M = 3.0). Local providers pass 0.0 / 0.0 — they still register
// the token counts but don't move the spend total.
fn _record(model, inTok, outTok, inRate, outRate) {
    if model == null || type(model) != "STRING" { model = "unknown" }
    if inTok  == null { inTok  = 0 }
    if outTok == null { outTok = 0 }
    let inCost  = float(inTok)  * inRate  / 1000000.0
    let outCost = float(outTok) * outRate / 1000000.0

    if hasKey(_aiUsage, model) {
        let cur = _aiUsage[model]
        _aiUsage[model] = {
            "in_tokens":  cur["in_tokens"]  + inTok,
            "out_tokens": cur["out_tokens"] + outTok,
            "in_cost":    cur["in_cost"]    + inCost,
            "out_cost":   cur["out_cost"]   + outCost,
        }
    } else {
        _aiUsage[model] = {
            "in_tokens":  inTok,
            "out_tokens": outTok,
            "in_cost":    inCost,
            "out_cost":   outCost,
        }
    }
    _aiSpent = _aiSpent + inCost + outCost
}


// _budgetError builds the standard error tuple every provider returns
// when budgetExceeded() trips. Keeps the error shape identical across
// providers so callers can match on err.code.
fn _budgetError() {
    return error("BUDGET_EXCEEDED",
        "ai.budget cap of $" + str(_aiBudget) +
        " already reached (spent $" + str(_aiSpent) + "); " +
        "raise the cap with ai.budget() or call ai.resetUsage() to reset")
}
