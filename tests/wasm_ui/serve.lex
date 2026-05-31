// serve.lex — HTTP server for the wasm_ui widget smoke test.
//
// Run via serve.sh. Direct invocation:
//
//   KLEX_PATH=/path/to/kLex ./klex tests/wasm_ui/serve.lex
//
// Browser: http://localhost:8767/

import "stdlib/wasm/serve_base.lex" as base
import "stdlib/server.lex"          as srv

let PORT = 8767
let ROOT = _scriptDir() + "/../.."

let s = srv.new()
s.get("/",             base.logged(_scriptDir() + "/index.html"))
s.get("/index.html",   base.logged(_scriptDir() + "/index.html"))
s.get("/klex.wasm",    base.logged(ROOT + "/bin/klex.wasm"))
s.get("/wasm_exec.js", base.logged(ROOT + "/stdlib/wasm/wasm_exec.js"))
s.get("/test.lex",     base.logged(_scriptDir() + "/test.lex"))
s.get("/serve.lex",    base.logged(_scriptDir() + "/serve.lex"))

println("kLex UI test — http://localhost:" + str(PORT) + "/")
println("Ctrl-C to stop.")
s.start(PORT)
