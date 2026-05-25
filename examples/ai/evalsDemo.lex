// evalsDemo.lex — running a Q&A eval suite against a local Ollama model.
//
// Demonstrates the full eval workflow:
//   - define a set of test cases (input + expected answer)
//   - run each case in parallel across N workers
//   - score the model's output against the expected answer
//   - print a pass/fail/score summary with failing-case details
//
// Tests qwen3.5:latest by default — override with OLLAMA_MODEL.
//
// Run:
//   ollama serve
//   OLLAMA_MODEL=qwen3.5:latest ./klex tests/examples/ai/evalsDemo.lex

import "stdlib/ai/ollama.lex"    as ollama
import "stdlib/ai/ai_common.lex" as ai
import "stdlib/ai/evals.lex"     as evals

let modelName = env("OLLAMA_MODEL")
if modelName == null || len(modelName) == 0 { modelName = "qwen3.5:latest" }

let c = ollama.newClient(env("OLLAMA_HOST"), modelName)

// Probe reachability before burning time generating.
let _, lerr = ollama.listModels(c)
if lerr != null {
    println("evalsDemo: cannot reach Ollama — " + lerr.message)
    _osExit(1)
}


// ── Test cases ───────────────────────────────────────────────────────────
//
// Each case: input prompt + expected answer (string, or array of strings
// where any/all must appear).

let cases = [
    // Arithmetic — exact answers
    {"input": "What is 2 plus 2? Reply with only the digit.",       "expected": "4"},
    {"input": "What is 9 times 7? Reply with only the digit(s).",   "expected": "63"},
    {"input": "What is 100 divided by 4? Reply with only the digit(s).", "expected": "25"},

    // Geography — well-known facts
    {"input": "Capital of France? One word.",                       "expected": "Paris"},
    {"input": "Capital of Japan? One word.",                        "expected": "Tokyo"},
    {"input": "On which continent is Egypt? One word.",             "expected": "Africa"},

    // Acronyms — multiple keywords must appear (use containsAll)
    {"input": "What does HTTP stand for? Just expand the letters.",
     "expected": ["hypertext", "transfer", "protocol"]},
    {"input": "What does JSON stand for? Just expand the letters.",
     "expected": ["javascript", "object", "notation"]},

    // History — short answers
    {"input": "In what year did World War 2 end? Just the year.",   "expected": "1945"},

    // Programming — multiple acceptable phrasings (use containsAny)
    {"input": "What's the file extension for Python source files? Just the extension.",
     "expected": [".py", "py"]},
]


// ── Run the eval ─────────────────────────────────────────────────────────

println("Evaluating " + modelName + " on " + str(len(cases)) + " cases " +
        "(4 parallel workers)…")

fn runOne(tc) {
    let resp, err = ollama.chat(c, {
        "messages": [ai.userMsg(tc["input"])],
        "think":    false,
        "options":  {"temperature": 0.0, "num_predict": 60},
    })
    if err != null { return error("RUN_FAIL", err.message) }
    return ollama.textOf(resp)
}

fn scoreOne(actual, tc) {
    let expected = tc["expected"]
    // Array of expected keywords — use containsAny for "any acceptable phrasing"
    // semantics when the array has fewer than 3 items; containsAll when
    // it's an acronym expansion where every keyword must appear.
    if type(expected) == "ARRAY" {
        if len(expected) >= 3 {
            return evals.containsAll(actual, tc)
        }
        return evals.containsAny(actual, tc)
    }
    // Plain string — case-insensitive contains.
    return evals.contains(actual, tc)
}

let results = evals.runSerial(cases, runOne, scoreOne, null)
evals.report(results)
