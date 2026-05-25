// ollamaStructured.lex — structured-output demo for ollama.lex.
//
// Same Tier-A schema-builder pattern as anthropicStructured.lex, but
// running entirely against a local Ollama daemon. Zero API key, zero
// per-call cost. Uses Ollama's native `format` parameter (accepting JSON
// Schema since Ollama 0.5+) to constrain output shape.
//
// Note: structured output on Ollama requires a model that's been trained
// for it. Confirmed working: llama3.2, qwen2.5, qwen3.x, mistral-nemo.
// Smaller models (orca-mini, phi-2) may emit malformed JSON.
//
// Run:
//   ollama serve
//   ollama pull qwen2.5      # or llama3.2 / qwen3.x
//   OLLAMA_MODEL=qwen2.5:latest ./klex tests/examples/ai/ollamaStructured.lex

import "stdlib/ai/ollama.lex"    as ollama
import "stdlib/ai/ai_common.lex" as ai
import "stdlib/ai/schema.lex"    as schema

let modelName = env("OLLAMA_MODEL")
if modelName == null || len(modelName) == 0 { modelName = "gemma4:latest" }

let c = ollama.newClient(env("OLLAMA_HOST"), modelName)

let _, lerr = ollama.listModels(c)
if lerr != null {
    println("ollamaStructured: cannot reach Ollama — " + lerr.message)
    _osExit(1)
}


// ── Demo 1: extract structured data from prose ────────────────────────────

println("─── Demo 1: extract structured data from prose ───")

let PersonSchema = schema.object({
    "name":       schema.string("Person's full name"),
    "age":        schema.integer("Age in years (estimate if not given)", 0, 120),
    "occupation": schema.string("Job title or role"),
    "city":       schema.string("City of residence"),
    "interests":  schema.array(schema.string(), "List of 2-5 interests / hobbies"),
})

let prose = "Karl is a 47-year-old software engineer from Christchurch who " +
        "enjoys building languages, retro computing, and the occasional " +
        "long walk. He's been writing code since the late 80s."

let person, err = ollama.completeWith(c, PersonSchema,
    "Extract a profile from this prose:\n\n" + prose,
    {"think": false})    // reasoning models — skip the thinking phase
if err != null {
    println("error: " + err.message)
    _osExit(1)
}

println("Extracted:")
println("  name:       " + person["name"])
println("  age:        " + str(person["age"]))
println("  occupation: " + person["occupation"])
println("  city:       " + person["city"])
println("  interests:")
for it in person["interests"] {
    println("    - " + it)
}


// ── Demo 2: classify with a nested schema ────────────────────────────────

println("")
println("─── Demo 2: TODO extraction with a nested schema ───")

let TaskSchema = schema.object({
    "title":    schema.string("Short title"),
    "priority": schema.enumOf(["high", "medium", "low"], "Priority level"),
    "tags":     schema.array(schema.string(), "0-3 relevant tags"),
})

let TodoListSchema = schema.object({
    "tasks": schema.array(TaskSchema, "All tasks found in the input"),
    "count": schema.integer("Total number of tasks"),
})

let prose2 = "" +
    "Remember to finish the auth refactor (high priority, tagged: backend, security). " +
    "Also need to update the docs and write tests — medium priority. " +
    "Eventually rebrand the landing page, but that's low priority for now."

let todos, err2 = ollama.completeWith(c, TodoListSchema,
    "Extract a task list from this text:\n\n" + prose2,
    {"think": false})
if err2 != null {
    println("error: " + err2.message)
    _osExit(1)
}

println("Found " + str(todos["count"]) + " tasks:")
for t in todos["tasks"] {
    let tagsStr = ""
    for tag in t["tags"] {
        if len(tagsStr) > 0 { tagsStr = tagsStr + ", " }
        tagsStr = tagsStr + tag
    }
    println("  [" + t["priority"] + "] " + t["title"] + "  (" + tagsStr + ")")
}
