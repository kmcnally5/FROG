// serve.lex — HTTP server for the kLex Playground.
//
// Run via serve.sh which handles the build step first. Direct invocation:
//
//   KLEX_PATH=/path/to/kLex ./klex examples/playground/serve.lex
//
// Browser: http://localhost:8768/

import "stdlib/wasm/serve_base.lex" as base
import "stdlib/server.lex"          as srv

let PORT = 8768
let ROOT = _scriptDir() + "/../.."

let s = srv.new()
s.get("/",                            base.logged(_scriptDir() + "/index.html"))
s.get("/index.html",                  base.logged(_scriptDir() + "/index.html"))
s.get("/klex.wasm",                   base.logged(ROOT + "/bin/klex.wasm"))
s.get("/wasm_exec.js",                base.logged(ROOT + "/stdlib/wasm/wasm_exec.js"))
s.get("/style.css",                   base.logged(_scriptDir() + "/style.css"))
s.get("/playground.lex",              base.logged(_scriptDir() + "/playground.lex"))
s.get("/docs-index.json",             base.logged(_scriptDir() + "/docs-index.json"))
s.get("/JetBrainsMono-Regular.woff2", base.logged(_scriptDir() + "/JetBrainsMono-Regular.woff2"))

println("kLex Playground — http://localhost:" + str(PORT) + "/")
println("Ctrl-C to stop.")
s.start(PORT)
