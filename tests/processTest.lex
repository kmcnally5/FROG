import "process.lex" as process
import "strings.lex" as strings

// --- run ---
out, err = process.run("echo", ["hello world"])
if err != null { println(err)  return null }
if strings.trimSpace(out) != "hello world" {
    println("FAIL: run echo")
} else {
    println("run echo: ok")
}

// run with a bad command returns an error
_, err = process.run("__klex_no_such_cmd__", [])
if err == null {
    println("FAIL: run bad cmd should error")
} else {
    println("run bad cmd error: ok")
}

// --- exec ---
out, errOut, code, err = process.exec("echo", ["exec test"])
if err != null { println(err)  return null }
if code != 0 { println("FAIL: exec exit code") } else { println("exec exit code: ok") }
if strings.trimSpace(out) != "exec test" { println("FAIL: exec stdout") } else { println("exec stdout: ok") }

// exec a command that exits non-zero — err should be null, code should be non-zero
out, errOut, code, err = process.exec("sh", ["-c", "exit 2"])
if err != null { println(err)  return null }
if code != 2 { println("FAIL: exec non-zero exit code") } else { println("exec non-zero exit: ok") }

// --- shell ---
out, err = process.shell("echo shell works | tr a-z A-Z")
if err != null { println(err)  return null }
if strings.trimSpace(out) != "SHELL WORKS" {
    println("FAIL: shell pipe")
} else {
    println("shell pipe: ok")
}

// shell failure returns an error
_, err = process.shell("exit 1")
if err == null {
    println("FAIL: shell non-zero should error")
} else {
    println("shell non-zero error: ok")
}
