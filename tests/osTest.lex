import "os.lex" as os

// --- getenv ---
home = os.getenv("HOME")
if home == null {
    println("FAIL: HOME should be set")
} else {
    println("getenv HOME: ok")
}

missing = os.getenv("KLEX_NO_SUCH_VAR_XYZ")
if missing != null {
    println("FAIL: unset var should return null")
} else {
    println("getenv missing: ok")
}

// --- setenv / getenv round-trip ---
_, err = os.setenv("KLEX_TEST_VAR", "hello")
if err != null { println(err)  return null }
got = os.getenv("KLEX_TEST_VAR")
if got != "hello" {
    println("FAIL: setenv/getenv round-trip")
} else {
    println("setenv round-trip: ok")
}

// --- cwd ---
dir, err = os.cwd()
if err != null { println(err)  return null }
if dir == null {
    println("FAIL: cwd should return a string")
} else {
    println("cwd: ok")
}

// --- hostname ---
host, err = os.hostname()
if err != null { println(err)  return null }
if host == null {
    println("FAIL: hostname should return a string")
} else {
    println("hostname: ok")
}

// --- pid ---
p = os.pid()
if p < 1 {
    println("FAIL: pid should be a positive integer")
} else {
    println("pid: ok")
}

// --- args ---
a = os.args()
if len(a) < 1 {
    println("FAIL: args should return at least one element")
} else {
    println("args: ok")
}
