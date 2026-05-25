// quickDemo.lex — tour of stdlib/ai/quick.lex one-liners.
//
// Five entry points, each in fewer lines than it took to describe them:
//   ai.ask, ai.executeQuery, ai.extract, ai.classify, ai.askCached
//
// Run from the kLex project root with your Anthropic API key in the env:
//   ANTHROPIC_API_KEY=sk-ant-… ./klex tests/examples/ai/quickDemo.lex
//
// Skips cleanly (exit 0) without the env var so this is safe to include in
// a broader test sweep.

import "stdlib/ai/quick.lex"  as ai
import "stdlib/ai/schema.lex" as schema


let apiKey = env("ANTHROPIC_API_KEY")
if apiKey == null || len(apiKey) == 0 {
    println("quickDemo: ANTHROPIC_API_KEY not set — skipping.")
    println("           export ANTHROPIC_API_KEY=sk-ant-… to run.")
    _osExit(0)
}


// ── 1. ai.ask — plain Q&A ─────────────────────────────────────────────────

println("── ai.ask ──")
let ans = ai.ask("In one short sentence, what is kLex?", {"max_tokens": 200})?
println("  " + ans)
println("")


// ── 2. ai.executeQuery — natural-language tool dispatch ───────────────────
//
// Register a couple of kLex functions; the model decides which (if any)
// to call. This is the "ai.executeQuery('How wide is the earth?', _fn)"
// pattern in action.

fn _earthDiameterKm() { return 12742 }
fn _moonDiameterKm()  { return 3474 }

println("── ai.executeQuery (single tool) ──")
ans = ai.executeQuery("How wide is the earth, in kilometres?", _earthDiameterKm)?
println("  " + ans)
println("")

println("── ai.executeQuery (multi-tool dispatch) ──")
ans = ai.executeQuery("How wide is the moon?", {
    "earthDiameter": _earthDiameterKm,
    "moonDiameter":  _moonDiameterKm,
})?
println("  " + ans)
println("")


// ── 3. ai.extract — text → typed hash ─────────────────────────────────────

println("── ai.extract ──")
let PersonSchema = schema.object({
    "name":      schema.string("Full name"),
    "birthYear": schema.integer("Year of birth", 1000, 2100),
    "field":     schema.string("Primary field of work"),
})
let person = ai.extract(
    "Marie Curie was a Polish-French physicist and chemist born in 1867. " +
    "She conducted pioneering research on radioactivity.",
    PersonSchema,
)?
println("  name:      " + person["name"])
println("  birthYear: " + str(person["birthYear"]))
println("  field:     " + person["field"])
println("")


// ── 4. ai.classify — pick one of N labels ────────────────────────────────

println("── ai.classify ──")
let samples = [
    "The release went perfectly — everyone is thrilled!",
    "I keep hitting a 500 from the staging server.",
    "Asking for clarification on the spec.",
]
for s in samples {
    let label = ai.classify(s, ["positive", "negative", "question"])?
    println("  [" + label + "]  " + s)
}
println("")


// ── 5. ai.askCached — same call twice, second hit is free ────────────────

println("── ai.askCached ──")
let q = "Say the word 'hello' exactly once and nothing else."

let start1 = _timeNanos()
let a1 = ai.askCached(q)?
let ms1 = (_timeNanos() - start1) / 1000000
println("  1st call (network): " + str(ms1) + " ms — '" + a1 + "'")

let start2 = _timeNanos()
let a2 = ai.askCached(q)?
let ms2 = (_timeNanos() - start2) / 1000000
println("  2nd call (cached):  " + str(ms2) + " ms — '" + a2 + "'")
println("  cache size: " + str(ai.cacheSize()))
println("")

println("done.")
