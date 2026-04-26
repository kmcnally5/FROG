// os.lex
// Operating system interface for kLex.
//
// getenv returns null when the variable is unset — absence is information,
// not an error. All other fallible operations return (value, err) tuples.
// exit terminates the process immediately and does not return.
//
// args() returns the full os.Args slice: index 0 is the binary, index 1 is
// the script path, index 2 onward are user-supplied arguments.
//
// Usage:
//   import "os.lex" as os
//   val = os.getenv("HOME")
//   if val == null { println("HOME not set")  return null }
//   println(val)

// getenv returns the value of the named environment variable, or null if unset.
fn getenv(key) {
    return _osGetenv(key)
}

// setenv sets the named environment variable. Returns (null, err).
fn setenv(key, val) {
    return _osSetenv(key, val)
}

// cwd returns the current working directory. Returns (path, err).
fn cwd() {
    return _osCwd()
}

// hostname returns the host name of the machine. Returns (hostname, err).
fn hostname() {
    return _osHostname()
}

// pid returns the process ID of the running interpreter.
fn pid() {
    return _osPid()
}

// args returns the full command-line argument list as an array of strings.
fn args() {
    return _osArgs()
}

// exit terminates the process with the given integer exit code.
fn exit(code) {
    return _osExit(code)
}
