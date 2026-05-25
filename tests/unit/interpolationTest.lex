// interpolationTest.lex — locks the Stage D rules for string interpolation.
//
// Design (FROG-pure):
//   - "..." strings: BOTH braces are special everywhere.
//       * Bare `{` starts an interpolation expression (must have matching `}`).
//       * Bare `}` outside an interp expression is a PARSE ERROR — no
//         silent "literal close-brace" fallback.
//   - Two ways to write a literal brace inside "..." text:
//       * brace-doubling (Python f-string style)
//       * backslash-escape (kLex escape style)
//   - Inside an interp expression, you write real kLex code. Quotes
//     don't need to be escaped — the lexer recursively lexes them.
//   - For JSON / regex / template text with raw braces, use backtick raw
//     strings — no escape processing at all.
//
// Architecture: the LEXER builds segments and pre-tokenizes each
// expression, so the parser just walks them. There is no raw-byte
// re-scanning anywhere downstream.

let failures = 0
let lb = chr(123)
let rb = chr(125)

fn check(name, cond) {
    if cond {
        println("ok: " + name)
    } else {
        println("FAIL: " + name)
        failures = failures + 1
    }
}

// ── 1. Plain interpolation still works ────────────────────────────────

let name = "world"
check("simple variable",                "<{name}>" == "<world>")
check("arithmetic expression",          "<{1 + 2 * 3}>" == "<7>")
let arr = [10, 20, 30]
check("array indexing",                 "<{arr[1]}>" == "<20>")
let h = {"a": 1, "b": 2}
check("hash indexing with inner quote", "<{h["a"]}>" == "<1>")

// ── 2. Inner strings inside expressions — no escape needed ────────────
//
// The outer lexer recursively lexes inner strings, so a bare `"` inside
// `{…}` opens a normal kLex string token. This is the natural f-string-
// like syntax and the only way to use string literals inside expressions
// (the old `\"` form is illegal in expression position because, inside
// `{…}`, you are writing real kLex code — and `\` outside a string
// literal is not a valid kLex token).

fn loud(s) { return s + "!" }
check("inner string in fn call arg",    "<{loud("yo")}>" == "<yo!>")
check("inner string literal in expr",   "<{ "hello" }>" == "<hello>")

fn pair(a, b) { return a + ":" + b }
check("two inner-string args",          "<{pair("x", "y")}>" == "<x:y>")

// ── 3. Nested hash literal inside expression ──────────────────────────

let h2 = "<{ {"a": 1} }>"
check("nested hash literal",            h2 == "<" + lb + "a: 1" + rb + ">")

// ── 4. Brace-doubling escapes ─────────────────────────────────────────

check("doubled open → one open",         "{{x" == lb + "x")
check("doubled close → one close",       "y}}" == "y" + rb)
check("doubled open + doubled close",    "{{x}}" == lb + "x" + rb)
check("doubled wrapping interp",         "{{{name}}}" == lb + "world" + rb)
check("four braces → two of each",       "{{{{}}}}" == lb + lb + rb + rb)

// ── 5. Backslash escapes still work as the alternative ────────────────

check("backslash-open",                  "\{" == lb)
check("backslash-close",                 "\}" == rb)
check("backslash pair",                  "\{\}" == lb + rb)
check("backslash around interp",         "\{{name}\}" == lb + "world" + rb)

// ── 6. Hex / control escapes in interp strings ────────────────────────

let x = 7
check("hex escape in interp",            "{x}\x41" == "7A")
check("newline escape in interp",        "a\n{x}" == "a\n7")
check("tab escape in interp",            "a\t{x}" == "a\t7")

// ── 7. Multi-line expression ──────────────────────────────────────────

let total = "<{
    1 + 2 + 3
}>"
check("multi-line expression",           total == "<6>")

// ── 8. Backtick raw strings — no interp, no escapes ───────────────────

let raw = `{"users": [], "count": 0}`
check("backtick raw JSON",               raw == lb + "\"users\": [], \"count\": 0" + rb)
check("raw string has no interp",        `{name}` == lb + "name" + rb)

// ── 9. Empty interp is a parse error — verify via subprocess ──────────

let bin = _osArgs()[0]
let helper = "/tmp/__klex_interp_empty.lex"
// Use backtick raw string for the helper source so the literal {} inside
// it isn't itself parsed as an interpolation by THIS test file.
let _, werr = _fsWrite(helper, `println("x={}")` + "\n")
if werr == null {
    let _, stderr, code, _ = _processExec(bin, [helper])
    check("empty interp is a parse error", code != 0 && indexOf(stderr, "empty expression") >= 0)
    _fsRemove(helper)
}

// ── 10. Bare close-brace in text is a parse error — verify via subprocess

_, werr = _fsWrite(helper, `println("x}")` + "\n")
if werr == null {
    let _, stderr, code, _ = _processExec(bin, [helper])
    check("bare close-brace is reserved",  code != 0 && indexOf(stderr, "bare") >= 0)
    _fsRemove(helper)
}

if failures > 0 {
    println("FAILURES: " + str(failures))
    _osExit(1)
}
println("PASS — strict braces, recursive inner strings, both escape styles")
