// anthropicStructured.lex — structured-output demo for anthropic.lex.
//
// Shows the Tier-A schema-builder pattern: define a JSON Schema with
// stdlib/ai/schema.lex, pass it to claude.complete(), get back a
// validated kLex hash matching the shape. No string-juggling, no
// regex parsing of free-text LLM output.
//
// This is the differentiator: every other language treats LLM output as
// a string and bolts on a third-party library (Pydantic, Zod, …) to
// validate the shape. kLex makes structured output a first-class
// stdlib feature — schema in, typed data out.
//
// Run:
//   ANTHROPIC_API_KEY=sk-ant-… ./klex tests/examples/ai/anthropicStructured.lex

import "stdlib/ai/anthropic.lex" as claude
import "stdlib/ai/ai_common.lex" as ai
import "stdlib/ai/schema.lex"    as schema

let apiKey = env("ANTHROPIC_API_KEY")
if apiKey == null || len(apiKey) == 0 {
    println("anthropicStructured: ANTHROPIC_API_KEY not set — skipping.")
    _osExit(0)
}

let c = claude.newClient(apiKey, null)


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

let person, err = claude.complete(c, PersonSchema,
    "Extract a profile from this prose:\n\n" + prose)
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


// ── Demo 2: classify with an enum + nested array of objects ───────────────

println("")
println("─── Demo 2: security audit with a nested schema ───")

let VulnSchema = schema.object({
    "severity": schema.enumOf(["critical", "high", "medium", "low", "info"],
                              "Severity level"),
    "category": schema.string("OWASP-style category, e.g. 'injection'"),
    "summary":  schema.string("One-line description of the issue"),
    "line":     schema.integer("Approximate line number in the snippet"),
})

let ReportSchema = schema.object({
    "overall_risk":     schema.enumOf(["high", "medium", "low"],
                                       "Overall risk level for this snippet"),
    "vulnerabilities":  schema.array(VulnSchema, "List of vulnerabilities found"),
    "recommendation":   schema.string("Top recommendation in one sentence"),
})

let snippet = "" +
    "def login(user, password):\n" +
    "    query = \"SELECT * FROM users WHERE name='\" + user + \"' AND pwd='\" + password + \"'\"\n" +
    "    return db.exec(query)"

let report, err2 = claude.complete(c, ReportSchema,
    "Audit this code snippet for security issues:\n\n" + snippet)
if err2 != null {
    println("error: " + err2.message)
    _osExit(1)
}

println("Overall risk: " + report["overall_risk"])
println("Recommendation: " + report["recommendation"])
println("Vulnerabilities (" + str(len(report["vulnerabilities"])) + "):")
for v in report["vulnerabilities"] {
    println("  [" + v["severity"] + "] line " + str(v["line"]) +
            " — " + v["category"] + ": " + v["summary"])
}
