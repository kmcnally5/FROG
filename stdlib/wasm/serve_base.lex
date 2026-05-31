// serve_base.lex — shared HTTP file-serving helpers for kLex WASM hosts.
//
// Every WASM application served by a kLex HTTP server (examples/playground,
// tests/wasm_*) imports this module and uses its three exports:
//
//   mime(path)       — map a file extension to the correct Content-Type
//   serveFile(path)  — read a file from disk and return an HTTP response
//   logged(path)     — handler factory: logs the request and calls serveFile
//
// Callers obtain the canonical shared asset paths via _scriptDir():
//
//   let ROOT = _scriptDir() + "/../.."
//   s.get("/wasm_exec.js", logged(ROOT + "/stdlib/wasm/wasm_exec.js"))
//   s.get("/klex.wasm",    logged(ROOT + "/bin/klex.wasm"))
//
// _scriptDir() is the directory containing the calling serve.lex, so ROOT
// is always the repo root regardless of which subdirectory the caller lives in.

import "stdlib/server.lex" as _srv
import "stdlib/fs.lex"     as _fs

fn mime(path) {
    if endsWith(path, ".wasm")  { return "application/wasm" }
    if endsWith(path, ".js")    { return "application/javascript" }
    if endsWith(path, ".html")  { return "text/html; charset=utf-8" }
    if endsWith(path, ".lex")   { return "text/plain; charset=utf-8" }
    if endsWith(path, ".css")   { return "text/css; charset=utf-8" }
    if endsWith(path, ".json")  { return "application/json" }
    if endsWith(path, ".woff2") { return "font/woff2" }
    if endsWith(path, ".ttf")   { return "font/ttf" }
    if endsWith(path, ".png")   { return "image/png" }
    return "application/octet-stream"
}

fn serveFile(path) {
    let body, err = _fs.read(path)
    if err != null {
        println("  404 " + path + " — " + err)
        return _srv.status(404, "not found: " + path)
    }
    return _srv.respond(200, body, {
        "Content-Type":                mime(path),
        "Cache-Control":               "no-store",
        "Access-Control-Allow-Origin": "*",
    })
}

fn logged(path) {
    return fn(req) {
        println(req["method"] + " " + req["path"])
        return serveFile(path)
    }
}
