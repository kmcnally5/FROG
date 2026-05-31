// serve.lex — HTTP server for the wasm_worker bridge test.
//
// Run via serve.sh. Direct invocation:
//
//   KLEX_PATH=/path/to/kLex ./klex tests/wasm_worker/serve.lex
//
// Browser: http://localhost:8765/

import "stdlib/wasm/serve_base.lex" as base
import "stdlib/server.lex"          as srv

let PORT = 8765
let ROOT = _scriptDir() + "/../.."

let s = srv.new()
s.get("/",                      base.logged(_scriptDir() + "/index.html"))
s.get("/index.html",            base.logged(_scriptDir() + "/index.html"))
s.get("/klex.wasm",             base.logged(ROOT + "/bin/klex.wasm"))
s.get("/wasm_exec.js",          base.logged(ROOT + "/stdlib/wasm/wasm_exec.js"))
s.get("/klex_bridge_worker.js", base.logged(ROOT + "/stdlib/worker/klex_bridge_worker.js"))
s.get("/test_worker.js",        base.logged(_scriptDir() + "/test_worker.js"))

println("kLex worker bridge test — http://localhost:" + str(PORT) + "/")
println("Ctrl-C to stop.")
s.start(PORT)
