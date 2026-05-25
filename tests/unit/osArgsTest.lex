// osArgsTest.lex — locks the _osArgs contract after OFI #13.
//
// The fix:
//   args[0]    — kLex binary path
//   args[1]    — script path
//   args[2..N] — script-level positional args (FLAGS STRIPPED)
//
// The pre-fix behaviour returned raw os.Args, so passing --cpuprofile
// would shift positional args and break every script that read
// args[2]+. This test invokes klex twice from a child subprocess:
// once plain, once with --cpuprofile — and asserts the visible
// positional args are identical.
//
// Run with: ./klex tests/unit/osArgsTest.lex
// Exit 0 on all-pass.

let failures = 0

// 1. Self-shape check: this script was invoked as
//    `./klex tests/unit/osArgsTest.lex` (no flags, no args). So
//    _osArgs() must show binary + script + zero user args.
let args = _osArgs()
if len(args) < 2 {
    println("FAIL: _osArgs() returned only " + str(len(args)) + " entries, expected at least 2")
    failures = failures + 1
} else {
    let bin    = args[0]
    let script = args[1]
    if indexOf(bin, "klex") < 0 {
        println("FAIL: args[0] = '" + bin + "' doesn't look like the klex binary path")
        failures = failures + 1
    } else {
        println("ok: args[0] = klex binary ('" + bin + "')")
    }
    if indexOf(script, "osArgsTest.lex") < 0 {
        println("FAIL: args[1] = '" + script + "' is not this script path")
        failures = failures + 1
    } else {
        println("ok: args[1] = this script path")
    }
}

// 2. Sub-invocation: launch a child klex that just prints its own
//    args and exits. Do it twice — once with no flags, once with
//    --cpuprofile — and confirm the positional args returned by
//    _osArgs() in the child are byte-identical.

// kLex string-literal `{…}` is interpolation; literal braces would
// need `\{` / `\}` escaping. Easier to assemble the helper-script
// body via chr() for the curly braces so the source stays readable.
let helper = "/tmp/__klex_osargs_helper.lex"
let lb = chr(123)
let rb = chr(125)
let helperBody = "args = _osArgs()\n" +
             "i = 0\n" +
             "while i < len(args) " + lb + "\n" +
             "    println(str(i) + \": \" + args[i])\n" +
             "    i = i + 1\n" +
             rb + "\n"
let _, werr = _fsWrite(helper, helperBody)
if werr != null {
    println("FAIL: could not write helper: " + werr)
    failures = failures + 1
}

let bin = args[0]

// (a) plain invocation: 3 user args. _processExec returns 4 elements:
//     (stdout, stderr, exitCode, err)
let sa, _, _, _ = _processExec(bin, [helper, "alpha", "beta", "gamma"])

// (b) flag invocation — pass --cpuprofile in front of the script.
let profPath = "/tmp/__klex_osargs_helper.prof"
let sb, _, _, _ = _processExec(bin, ["--cpuprofile", profPath, helper, "alpha", "beta", "gamma"])

// Strip the [0] = .../klex line from both — that legitimately differs
// because in (b) we passed extra flags between the binary and the
// script. Everything from index 1 onwards must match.
fn stripBinaryLine(s) {
    let lines = split(s, "\n")
    let out = makeArray(len(lines), "")
    let n = 0
    let i = 0
    while i < len(lines) {
        let line = lines[i]
        // Keep lines that start with "1:", "2:", "3:", ... — skip "0:"
        // and any trailing blank line.
        if len(line) > 2 {
            if line[0] != "0" {
                out[n] = line
                n = n + 1
            }
        }
        i = i + 1
    }
    return slice(out, 0, n)
}

let posA = stripBinaryLine(sa)
let posB = stripBinaryLine(sb)

if len(posA) != len(posB) {
    println("FAIL: plain run printed " + str(len(posA)) + " positional lines, " +
            "flag run printed " + str(len(posB)))
    println("--- plain run ---")
    println(sa)
    println("--- flag run ---")
    println(sb)
    failures = failures + 1
} else {
    let mismatch = -1
    let i = 0
    while i < len(posA) {
        if posA[i] != posB[i] {
            mismatch = i
            i = len(posA)
        }
        i = i + 1
    }
    if mismatch >= 0 {
        println("FAIL: positional mismatch at index " + str(mismatch))
        println("plain: " + posA[mismatch])
        println("flag : " + posB[mismatch])
        failures = failures + 1
    } else {
        println("ok: positional args identical with/without --cpuprofile (" + str(len(posA)) + " lines)")
    }
}

// Verify the profile file was actually written by the flag run (proves
// kLex's flag parser DID consume --cpuprofile, not just ignored it).
if !_fsExists(profPath) {
    println("FAIL: --cpuprofile run did not produce profile file at " + profPath)
    failures = failures + 1
} else {
    println("ok: --cpuprofile was honoured (file exists at " + profPath + ")")
}

// 3. Cleanup.
safe(fn() { _fsRemove(helper)   return null })
safe(fn() { _fsRemove(profPath) return null })

// 4. Arity check.
let _, e = safe(fn() { return _osArgs("extra") })
if e == null {
    println("FAIL: _osArgs('extra') did not error")
    failures = failures + 1
} else {
    println("ok: extra-arg rejected — " + e.message)
}

if failures > 0 {
    println("FAILURES: " + str(failures))
    _osExit(1)
}
println("PASS — _osArgs strips flags; positional shape is stable")
