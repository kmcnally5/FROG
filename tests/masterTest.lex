import "fs.lex"      as fs
import "process.lex" as proc
import "os.lex"      as osmod

// Resolve the interpreter path from our own argv[0] so sub-processes
// use the exact same binary (works with both `go run .` and `./klex`).
allArgs = osmod.args()
interpreter = allArgs[0]

// Discover test files — listDir returns names sorted alphabetically.
files, listErr = fs.listDir("tests")
if listErr != null {
    println("ERROR: cannot list tests/: " + listErr)
    return null
}

// Collect .lex files, excluding ourselves.
tests = []
i = 0
while i < len(files) {
    name = files[i]
    parts = split(name, ".")
    if parts[len(parts) - 1] == "lex" && name != "masterTest.lex" {
        tests = push(tests, name)
    }
    i = i + 1
}

total = len(tests)

// Layout constants — tweak if test names grow longer.
NAME_WIDTH = 22
DOTS_WIDTH = 28

println("")
println("  kLex Master Test Suite")
println("  ========================")
println("")

passed = []
failed = []

i = 0
while i < total {
    name = tests[i]

    // Strip ".lex" for display.
    nameParts = split(name, ".")
    displayName = nameParts[0]

    // Pad name to fixed width.
    label = displayName
    while len(label) < NAME_WIDTH {
        label = label + " "
    }

    // Build dot separator.
    dots = ""
    while len(dots) < DOTS_WIDTH {
        dots = dots + "."
    }

    // Index badge — right-align the numerator within its field.
    numStr = str(i + 1)
    totStr = str(total)
    while len(numStr) < len(totStr) {
        numStr = " " + numStr
    }
    badge = "  [" + numStr + "/" + totStr + "]  "

    // Print the row prefix with no newline — result appended after exec.
    print(badge + label + " " + dots + " ")

    // Run the sub-script, capturing all output so nothing leaks to the terminal.
    stdout, stderr, exitCode, runErr = proc.exec(interpreter, ["tests/" + name])

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
