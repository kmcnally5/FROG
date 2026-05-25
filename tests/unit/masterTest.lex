import "stdlib/fs.lex"      as fs

// Resolve the interpreter path from our own argv[0] so sub-processes
// use the exact same binary (works with both `go run .` and `./klex`).
let allArgs = _osArgs()
let interpreter = allArgs[0]

// Discover test files — listDir returns names sorted alphabetically.
let files, listErr = fs.listDir("tests/unit")
if listErr != null {
    println("ERROR: cannot list tests/: " + listErr)
    return null
}

// Collect .lex files, excluding ourselves.
let tests = []
let i = 0
while i < len(files) {
    let name = files[i]
    let parts = split(name, ".")
    if parts[len(parts) - 1] == "lex" && name != "masterTest.lex" {
        tests = push(tests, name)
    }
    i = i + 1
}

let total = len(tests)

// Layout constants — tweak if test names grow longer.
let NAME_WIDTH = 22
let DOTS_WIDTH = 28

println("")
println("  kLex Master Test Suite")
println("  ========================")
println("")

let passed = []
let failed = []

i = 0
while i < total {
    let name = tests[i]

    // Strip ".lex" for display.
    let nameParts = split(name, ".")
    let displayName = nameParts[0]

    // Pad name to fixed width.
    let label = displayName
    while len(label) < NAME_WIDTH {
        label = label + " "
    }

    // Build dot separator.
    let dots = ""
    while len(dots) < DOTS_WIDTH {
        dots = dots + "."
    }

    // Index badge — right-align the numerator within its field.
    let numStr = str(i + 1)
    let totStr = str(total)
    while len(numStr) < len(totStr) {
        numStr = " " + numStr
    }
    let badge = "  [" + numStr + "/" + totStr + "]  "

    // Print the row prefix with no newline — result appended after exec.
    print(badge + label + " " + dots + " ")

    // Run the sub-script, capturing all output so nothing leaks to the terminal.
    let stdout, stderr, exitCode, runErr = _processExec(interpreter, ["tests/unit/" + name])

    // Determine pass/fail.
    // exitCode != 0 → parse error (main.go calls os.Exit(1) on parse failures).
    // stderr non-empty → same source.
    // runErr non-null → couldn't even launch the interpreter.
    if runErr != null {
        println("ERROR  (could not launch interpreter)")
        failed = push(failed, name)
    } else if exitCode != 0 || len(stderr) > 0 {
        println("FAIL")
        failed = push(failed, name)
    } else {
        println("PASS")
        passed = push(passed, name)
    }

    i = i + 1
}

// Summary.
println("")
println("  ========================")
println("  Passed : " + str(len(passed)) + " / " + str(total))
println("  Failed : " + str(len(failed)) + " / " + str(total))
println("")

if len(failed) > 0 {
    println("  Failed tests:")
    i = 0
    while i < len(failed) {
        println("    x  " + failed[i])
        i = i + 1
    }
    println("")
}
