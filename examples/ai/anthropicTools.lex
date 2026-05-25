// anthropicTools.lex — agent-loop demo for stdlib/ai/anthropic.lex.
//
// Demonstrates the tool-use pattern: register kLex functions as tools the
// model can call, pass them to claude.runAgent, and the library handles
// the multi-turn loop (model → tool_use → handler → tool_result → model …)
// until the model produces a final non-tool answer.
//
// Run from the kLex project root with your Anthropic API key set:
//   ANTHROPIC_API_KEY=sk-ant-… ./klex tests/examples/ai/anthropicTools.lex
//
// Skips cleanly if the env var is absent.

import "stdlib/ai/anthropic.lex" as claude
import "stdlib/ai/ai_common.lex" as ai
import "stdlib/datetime.lex"     as dt

let apiKey = env("ANTHROPIC_API_KEY")
if apiKey == null || len(apiKey) == 0 {
    println("anthropicTools: ANTHROPIC_API_KEY not set — skipping.")
    _osExit(0)
}


// ── Tool handlers — plain kLex functions ──────────────────────────────────
// Each takes the input hash the model sent and returns any JSON-serialisable
// value. The library dispatches automatically when the model emits tool_use.

fn rollDice(input) {
    let sides = input["sides"]
    let count = input["count"]
    let total = 0
    let rolls = makeArray(count, 0)
    let i = 0
    while i < count {
        // randInt(min, max) is inclusive on both ends in kLex.
        let r = randInt(1, sides)
        rolls[i] = r
        total = total + r
        i = i + 1
    }
    return {
        "rolls": rolls,
        "total": total,
        "sides": sides,
    }
}


fn getCurrentTime(input) {
    // _ is the conventional discard for the unused input.
    _ = input
    let now = dt.now()
    return {
        "unix_seconds": now.unix,
        "year":         now.year,
        "month":        now.month,
        "day":          now.day,
        "hour":         now.hour,
        "minute":       now.minute,
        "weekday":      now.weekday,
    }
}


// ── Tool registrations ────────────────────────────────────────────────────
// Each tool is a hash with name, description, input_schema, and a handler
// function reference. The runAgent loop strips `handler` before sending the
// tool list over the wire — Anthropic only sees name/description/schema.

let tools = [
    {
        "name":         "roll_dice",
        "description":  "Roll one or more dice. Returns each individual roll plus the sum.",
        "input_schema": {
            "type": "object",
            "properties": {
                "sides": { "type": "integer", "description": "Number of sides per die (e.g. 6 for d6)" },
                "count": { "type": "integer", "description": "How many dice to roll" },
            },
            "required": ["sides", "count"],
        },
        "handler": rollDice,
    },
    {
        "name":         "get_current_time",
        "description":  "Return the current time as a unix timestamp.",
        "input_schema": {
            "type":       "object",
            "properties": {},
        },
        "handler": getCurrentTime,
    },
]


// ── Run the agent ─────────────────────────────────────────────────────────
// The prompt is crafted to force the model to use BOTH tools, so the demo
// exercises a real multi-step agent loop.

let c = claude.newClient(apiKey, null)

println("Asking Claude to roll dice + check the time…")
println("")

let finalResp, history, agentErr = claude.runAgent(c, {
    "system":     "You are a helpful assistant with access to tools. Use them when asked.",
    "max_tokens": 1024,
    "messages":   [ai.userMsg(
        "Roll three 20-sided dice for me, tell me the sum, " +
        "and then tell me what the current time is."
    )],
    "tools":      tools,
}, 8)

if agentErr != null {
    println("agent error: " + agentErr.message + " (code: " + agentErr.code + ")")
    _osExit(1)
}

println("Final answer:")
println("  " + claude.textOf(finalResp))
println("")
println("Conversation length: " + str(len(history)) + " messages")
println("Stop reason: " + claude.stopReasonOf(finalResp))
