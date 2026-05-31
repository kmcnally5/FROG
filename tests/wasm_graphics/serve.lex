// serve.lex — HTTP server for the wasm_graphics Canvas2D test.
//
// Run via serve.sh. Direct invocation:
//
//   KLEX_PATH=/path/to/kLex ./klex tests/wasm_graphics/serve.lex
//
// Browser: http://localhost:8766/

import "stdlib/wasm/serve_base.lex" as base
import "stdlib/server.lex"          as srv

let PORT = 8766
let ROOT = _scriptDir() + "/../.."

let s = srv.new()
s.get("/",             base.logged(_scriptDir() + "/index.html"))
s.get("/index.html",   base.logged(_scriptDir() + "/index.html"))
s.get("/klex.wasm",    base.logged(ROOT + "/bin/klex.wasm"))
s.get("/wasm_exec.js", base.logged(ROOT + "/stdlib/wasm/wasm_exec.js"))
s.get("/test.lex",     base.logged(_scriptDir() + "/test.lex"))
s.get("/Outfit.ttf",   base.logged(_scriptDir() + "/Outfit.ttf"))
s.get("/demo.png",     base.logged(_scriptDir() + "/demo.png"))

println("kLex Canvas2D test — http://localhost:" + str(PORT) + "/")
println("Ctrl-C to stop.")
s.start(PORT)
