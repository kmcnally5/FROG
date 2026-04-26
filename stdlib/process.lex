// process.lex
// Subprocess execution for kLex.
//
// run and shell are the common case: they capture stdout and fold stderr into
// the error on failure. Use exec when you need stdout and stderr separately,
// or when you need the exit code regardless of success or failure.
//
// args must always be an array of strings — pass [] for no arguments.
//
// Usage:
//   import "process.lex" as process
//   out, err = process.run("ls", ["-la", "/tmp"])
//   if err != null { println(err)  return null }
//   println(out)
//
//   out, err = process.shell("echo hello | tr a-z A-Z")
//   if err != null { println(err)  return null }
//   println(out)

// run executes cmd with the given args array and returns (stdout, err).
// stderr is folded into err when the process exits non-zero.
fn run(cmd, args) {
    return _processRun(cmd, args)
}

// exec executes cmd with the given args array and returns
// (stdout, stderr, exitCode, err).
// err is non-null only when the process could not be started.
// A non-zero exit code is not itself an error — check exitCode directly.
fn exec(cmd, args) {
    return _processExec(cmd, args)
}

// shell runs cmdStr through /bin/sh -c and returns (stdout, err).
// stderr is folded into err when the shell exits non-zero.
fn shell(cmdStr) {
    return _processShell(cmdStr)
}
