import "server.lex" as srv

s = srv.new()

s.get("/hello", fn(req) {
    return srv.ok("Hello from kLex!")
})

s.get("/greet", fn(req) {
    name = req["query"]["name"]
    if name == null {
        name = "stranger"
    }
    return srv.ok("Hello, " + name + "!")
})

s.post("/echo", fn(req) {
    return srv.ok(req["body"])
})

s.get("/headers", fn(req) {
    return srv.ok("ua=" + req["headers"]["user-agent"])
})

println("listening on :8765")
s.start(8765)
