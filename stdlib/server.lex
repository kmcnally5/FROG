// server.lex — HTTP server stdlib for kLex
//
// Wraps the _httpServe primitive with a route-based API.
// Routes are matched in registration order; the first match wins.
// server.start(port) blocks until the process exits.
//
// Usage:
//   import "server.lex" as srv
//
//   s = srv.new()
//   s.get("/hello", fn(req) { return srv.ok("Hello!") })
//   s.post("/echo", fn(req) { return srv.ok(req["body"]) })
//   s.start(8080)
//
// Async handlers (non-blocking routes):
//   s.get_async("/work", fn(req) { ... expensive work ... })
//   s.post_async("/work", fn(req) { ... expensive work ... })
//   Returns 202 Accepted immediately; handler runs in background.
//   Handlers should use channels to send results to coordinator.
//
// Request fields (hash):
//   req["method"]   — HTTP verb, e.g. "GET"
//   req["path"]     — URL path, e.g. "/hello"
//   req["query"]    — hash of query-string params (all values are strings)
//   req["headers"]  — hash of lowercase header names to values
//   req["body"]     — request body as a string (empty string if none)
//
// Response helpers:
//   srv.ok(body)                   — 200 text response
//   srv.json(body)                 — 200 with Content-Type: application/json
//   srv.status(code, body)         — custom status, plain body
//   srv.respond(code, body, hdrs)  — custom status, body, and headers hash
//   srv.accepted()                 — 202 Accepted (async handler response)

struct Server {
    routes

    fn get(path, handler) {
        self.routes = push(self.routes, {
            "method": "GET",
            "path": path,
            "handler": handler,
            "async": false
        })
        return null
    }

    fn post(path, handler) {
        self.routes = push(self.routes, {
            "method": "POST",
            "path": path,
            "handler": handler,
            "async": false
        })
        return null
    }

    fn put(path, handler) {
        self.routes = push(self.routes, {
            "method": "PUT",
            "path": path,
            "handler": handler,
            "async": false
        })
        return null
    }

    fn del(path, handler) {
        self.routes = push(self.routes, {
            "method": "DELETE",
            "path": path,
            "handler": handler,
            "async": false
        })
        return null
    }

    fn get_async(path, handler) {
        self.routes = push(self.routes, {
            "method": "GET",
            "path": path,
            "handler": handler,
            "async": true
        })
        return null
    }

    fn post_async(path, handler) {
        self.routes = push(self.routes, {
            "method": "POST",
            "path": path,
            "handler": handler,
            "async": true
        })
        return null
    }

    fn put_async(path, handler) {
        self.routes = push(self.routes, {
            "method": "PUT",
            "path": path,
            "handler": handler,
            "async": true
        })
        return null
    }

    fn del_async(path, handler) {
        self.routes = push(self.routes, {
            "method": "DELETE",
            "path": path,
            "handler": handler,
            "async": true
        })
        return null
    }

    fn start(port) {
        _httpServe(port, fn(req) {
            i = 0
            while i < len(self.routes) {
                route = self.routes[i]
                if route["method"] == req["method"] && route["path"] == req["path"] {
                    if route["async"] {
                        async(route["handler"], req)
                        return {"status": 202, "body": "Accepted", "headers": {}}
                    } else {
                        return route["handler"](req)
                    }
                }
                i = i + 1
            }
            return {"status": 404, "body": "Not Found", "headers": {}}
        })
    }
}

fn new() {
    return Server { routes: [] }
}

fn ok(body) {
    return {"status": 200, "body": body, "headers": {}}
}

fn json(body) {
    return {"status": 200, "body": body, "headers": {"Content-Type": "application/json"}}
}

fn status(code, body) {
    return {"status": code, "body": body, "headers": {}}
}

fn respond(code, body, headers) {
    return {"status": code, "body": body, "headers": headers}
}

fn accepted() {
    return {"status": 202, "body": "Accepted", "headers": {}}
}
