// sdTest.lex — smoke test for stdlib/ai/sd.lex.
//
// Verifies the module loads, the Client constructs correctly, and the
// validation gates fire before any network call. Network-requiring paths
// (generate, models, …) are tested via ping() which is allowed to fail —
// we only assert it doesn't raise.

import "stdlib/ai/sd.lex" as sd

let failed = 0
let passed = 0

fn check(name, cond) {
    if cond {
        println("  PASS  " + name)
        passed = passed + 1
    } else {
        println("  FAIL  " + name)
        failed = failed + 1
    }
}

println("── sd.lex smoke test ───────────────────────────────────────────")

// ── Client construction ──────────────────────────────────────────────────

let c = sd.newClient(null)
check("newClient(null) returns a Client", type(c) == "STRUCT")
check("default baseUrl is 127.0.0.1:7860", c.baseUrl == "http://127.0.0.1:7860")
check("default authHeader is null", c.authHeader == null)

let c2 = sd.newClient("http://192.168.1.10:7860")
check("newClient with custom baseUrl", c2.baseUrl == "http://192.168.1.10:7860")

let c3 = sd.newClientAuth("http://example.com", "Basic dXNlcjpwYXNz")
check("newClientAuth carries auth header", c3.authHeader == "Basic dXNlcjpwYXNz")

// ── Input validation (no network) ────────────────────────────────────────

let _, err = sd.generate(c, null)
check("generate rejects null opts", err != null && err.code == "SD_BAD_OPTS")

_, err = sd.generate(c, {"steps": 20})
check("generate requires prompt", err != null && err.code == "SD_BAD_OPTS")

_, err = sd.img2img(c, {"prompt": "x"})
check("img2img requires init_image(s)", err != null && err.code == "SD_BAD_OPTS")

_, err = sd.img2img(c, {"prompt": "x", "init_image": "not bytes"})
check("img2img init_image must be bytes", err != null && err.code == "SD_BAD_OPTS")

_, err = sd.setModel(c, "")
check("setModel requires non-empty title", err != null && err.code == "SD_BAD_OPTS")

_, err = sd.setOption(c, "", "v")
check("setOption requires non-empty key", err != null && err.code == "SD_BAD_OPTS")

// ── Network paths (allowed to fail gracefully) ───────────────────────────

// ping() against an unreachable daemon should return false without raising.
let alive, _ = sd.ping(c)
check("ping returns bool (false if no daemon)", type(alive) == "BOOLEAN")

// generate against an unreachable daemon should return a typed network error.
_, err = sd.generate(sd.newClient("http://127.0.0.1:1"), {"prompt": "x", "timeout_sec": 1})
check("generate returns SD_NETWORK on unreachable daemon",
    err != null && err.code == "SD_NETWORK")

// ── Summary ──────────────────────────────────────────────────────────────

println("")
println("── result ──────────────────────────────────────────────────────")
println("  passed: " + str(passed))
println("  failed: " + str(failed))
if failed > 0 { _osExit(1) }
