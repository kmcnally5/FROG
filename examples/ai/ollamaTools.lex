// ollamaTools.lex — tool-use agent demo for stdlib/ai/ollama.lex.
//
// Mirrors anthropicTools.lex: register kLex functions as tools and let
// the model call them in a multi-turn loop. The library handles the
// dispatch + result feedback transparently.
//
// IMPORTANT: tool calling requires a model trained for it.  Good
// defaults on Ollama:
//   llama3.2   (8B, fast, decent tool support)
//   llama3.1   (8B/70B, mature)
//   qwen2.5    (excellent at tools)
//   mistral-nemo
// Smaller / older models (orca-mini, phi-2, etc.) often don't emit
// tool_calls at all — they'll just answer in prose. Choose accordingly.
//
// Run:
//   ollama serve
//   ollama pull llama3.2
//   ./klex tests/examples/ai/ollamaTools.lex

import "stdlib/ai/ollama.lex"    as ollama
import "stdlib/ai/ai_common.lex" as ai
import "stdlib/datetime.lex"     as dt

let modelName = env("OLLAMA_MODEL")
if modelName == null || len(modelName) == 0 { modelName = "gemma4:local" }

let c = ollama.newClient(env("OLLAMA_HOST"), modelName)

let _, lerr = ollama.listModels(c)
if lerr != null {
    println("ollamaTools: cannot reach Ollama — " + lerr.message)
    _osExit(1)
}


// ── Tool handlers — plain kLex functions ──────────────────────────────────

fn rollDice(input) {
    let sides = input["sides"]
    let count = input["count"]
    let total = 0
    let rolls = makeArray(count, 0)
    let i = 0
    while i < count {
        let r = randInt(1, sides)
        rolls[i] = r
        total = total + r
        i = i + 1
    }
    return {"rolls": rolls, "total": total, "sides": sides}
}

fn getCurrentTime(input) {
    _ = input
    let now = dt.now()
    return {
        "year": now.year,  "month": now.month,  "day": now.day,
        "hour": now.hour,  "minute": now.minute,
        "weekday": now.weekday,
    }
}


// ── Tool registrations — Anthropic-shape; ollama.lex translates ───────────

let tools = [
    {
        "name":         "roll_dice",
        "description":  "Roll one or more dice. Returns each individual roll plus the sum.",
        "input_schema": {
            "type": "object",
            "properties": {
                "sides": { "type": "integer", "description": "Sides per die (e.g. 6 for d6)" },
                "count": { "type": "integer", "description": "How many dice to roll" },
            },
            "required": ["sides", "count"],
        },
        "handler": rollDice,
    },
    {
        "name":         "get_current_time",
        "description":  "Return the current time.",
        "input_schema": {
            "type":       "object",
            "properties": {},
        },
        "handler": getCurrentTime,
    },
]

println("Asking " + modelName + " to roll dice + check the time…")
println("")

let finalResp, history, agentErr = ollama.runAgent(c, {
    "system":   "You are helpful and use tools when asked.",
    "messages": [ai.userMsg(
        "Roll three 20-sided dice and tell me the total. " +
        "Then tell me what time it is."
        )],
    "tools":   tools,
    "think":   false,
    "options": {"temperature": 0.3},
}, 8)

if agentErr != null {
    println("agent error: " + agentErr.message + " (code: " + agentErr.code + ")")
    _osExit(1)
}

println("Final answer:")
println("  " + ollama.textOf(finalResp))
println("")
println("Conversation length: " + str(len(history)) + " messages")
println("done_reason: " + ollama.doneReasonOf(finalResp))
