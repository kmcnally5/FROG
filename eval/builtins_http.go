package eval

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"klex/ast"
)

// serveRequest carries an incoming HTTP request from a net/http goroutine
// to the kLex eval goroutine via reqCh.
type serveRequest struct {
	method  string
	path    string
	query   map[string]string
	headers map[string]string
	body    string
	respCh  chan serveResponse
}

// serveResponse carries the handler's reply back to the waiting net/http goroutine.
type serveResponse struct {
	status  int
	body    string
	headers map[string]string
}

// httpClient is shared across all _httpDo calls so connections are reused.
var httpClient = &http.Client{Timeout: 30 * time.Second}

func init() {
	// _httpDo(method, url, headers, body) → (status, body, headers, err)
	//
	// The single Go-level primitive that backs all kLex HTTP functions.
	// method  — string: "GET", "POST", "PUT", "PATCH", "DELETE", etc.
	// url     — string
	// headers — HASH of string → string, or null for no custom headers
	// body    — string payload, or null for no body
	//
	// Always returns a 4-element Tuple so the kLex caller can destructure:
	//   status, body, headers, err = _httpDo(...)
	// On failure: status=0, body="", headers={}, err=<message string>
	// On success: err=null
	Builtins["_httpDo"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 4 {
			return httpErrTuple("_httpDo expects 4 arguments")
		}

		method, ok := args[0].(*String)
		if !ok {
			return httpErrTuple(fmt.Sprintf("_httpDo: method must be string, got %s", args[0].Type()))
		}

		rawURL, ok := args[1].(*String)
		if !ok {
			return httpErrTuple(fmt.Sprintf("_httpDo: url must be string, got %s", args[1].Type()))
		}

		// Build request body reader from string or null.
		var bodyReader io.Reader
		switch b := args[3].(type) {
		case *String:
			if b.Value != "" {
				bodyReader = strings.NewReader(b.Value)
			}
		case *Null:
			// no body
		default:
			return httpErrTuple(fmt.Sprintf("_httpDo: body must be string or null, got %s", args[3].Type()))
		}

		req, err := http.NewRequest(method.Value, rawURL.Value, bodyReader)
		if err != nil {
			return httpErrTuple(err.Error())
		}

		// Apply caller-supplied headers.
		switch h := args[2].(type) {
		case *Hash:
			for _, pair := range h.Pairs {
				k, ok := pair.Key.(*String)
				if !ok {
					continue
				}
				v, ok := pair.Value.(*String)
				if !ok {
					continue
				}
				req.Header.Set(k.Value, v.Value)
			}
		case *Null:
			// no custom headers
		default:
			return httpErrTuple(fmt.Sprintf("_httpDo: headers must be hash or null, got %s", args[2].Type()))
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			return httpErrTuple(err.Error())
		}
		defer resp.Body.Close()

		respBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return httpErrTuple(err.Error())
		}

		// Normalise response headers to lowercase string keys.
		headersHash := &Hash{Pairs: make(map[HashKey]HashPair)}
		for k, vals := range resp.Header {
			lower := strings.ToLower(k)
			key := &String{Value: lower}
			val := &String{Value: strings.Join(vals, ", ")}
			hk := HashKey{Type: STRING_OBJ, Value: lower}
			headersHash.Pairs[hk] = HashPair{Key: key, Value: val}
		}

		return &Tuple{Elements: []Object{
			&Integer{Value: resp.StatusCode},
			&String{Value: string(respBytes)},
			headersHash,
			&Null{},
		}}
	}}
}

// _httpServe(port, handlerFn) — binds to port, blocks, and dispatches every
// incoming HTTP request to handlerFn(req).
//
// req is a Hash: {"method", "path", "query", "headers", "body"}.
// handlerFn must return a Hash: {"status": int, "body": string, "headers": hash}.
// Use server.lex for a route-based wrapper around this primitive.
//
// All requests are serialised through the kLex eval goroutine — one at a time,
// no shared mutable state across goroutine boundaries. Correct for tool servers.
func init() {
	Builtins["_httpServe"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_httpServe expects 2 arguments (port, handler)", ast.Pos{})
		}
		portObj, ok := args[0].(*Integer)
		if !ok {
			return runtimeError(fmt.Sprintf("_httpServe: port must be integer, got %s", args[0].Type()), ast.Pos{})
		}
		fn, ok := args[1].(*Function)
		if !ok {
			return runtimeError(fmt.Sprintf("_httpServe: handler must be a function, got %s", args[1].Type()), ast.Pos{})
		}

		reqCh := make(chan serveRequest)

		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			r.Body.Close()

			query := map[string]string{}
			for k, vs := range r.URL.Query() {
				if len(vs) > 0 {
					query[k] = vs[0]
				}
			}
			headers := map[string]string{}
			for k, vs := range r.Header {
				headers[strings.ToLower(k)] = strings.Join(vs, ", ")
			}

			respCh := make(chan serveResponse, 1)
			reqCh <- serveRequest{
				method:  r.Method,
				path:    r.URL.Path,
				query:   query,
				headers: headers,
				body:    string(body),
				respCh:  respCh,
			}
			resp := <-respCh

			for k, v := range resp.headers {
				w.Header().Set(k, v)
			}
			w.WriteHeader(resp.status)
			fmt.Fprint(w, resp.body)
		})

		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", portObj.Value))
		if err != nil {
			return runtimeError(fmt.Sprintf("server: cannot listen on port %d: %s", portObj.Value, err.Error()), ast.Pos{})
		}

		srv := &http.Server{
			Handler:      mux,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
		}
		go srv.Serve(ln) //nolint:errcheck

		// Block the kLex goroutine: process one request at a time.
		for req := range reqCh {
			result, errObj := applyFunction(fn, []Object{buildServeRequestHash(req)})
			if errObj != nil {
				req.respCh <- serveResponse{status: 500, body: errObj.(*Error).Message, headers: map[string]string{}}
				continue
			}
			if result == nil {
				req.respCh <- serveResponse{status: 500, body: "handler returned nil", headers: map[string]string{}}
				continue
			}
			req.respCh <- extractServeResponse(result)
		}

		return &Null{}
	}}
}

// buildServeRequestHash converts a serveRequest into the kLex Hash passed to route handlers.
func buildServeRequestHash(req serveRequest) *Hash {
	h := &Hash{Pairs: make(map[HashKey]HashPair)}

	setStr := func(key, val string) {
		k := &String{Value: key}
		v := &String{Value: val}
		h.Pairs[HashKey{Type: STRING_OBJ, Value: key}] = HashPair{Key: k, Value: v}
	}
	setStr("method", req.method)
	setStr("path", req.path)
	setStr("body", req.body)

	buildStrHash := func(m map[string]string) *Hash {
		out := &Hash{Pairs: make(map[HashKey]HashPair)}
		for k, v := range m {
			hk := &String{Value: k}
			hv := &String{Value: v}
			out.Pairs[HashKey{Type: STRING_OBJ, Value: k}] = HashPair{Key: hk, Value: hv}
		}
		return out
	}
	h.Pairs[HashKey{Type: STRING_OBJ, Value: "query"}] = HashPair{
		Key: &String{Value: "query"}, Value: buildStrHash(req.query),
	}
	h.Pairs[HashKey{Type: STRING_OBJ, Value: "headers"}] = HashPair{
		Key: &String{Value: "headers"}, Value: buildStrHash(req.headers),
	}
	return h
}

// extractServeResponse reads "status", "body", "headers" out of the Hash
// a route handler returns.  Missing fields fall back to sensible defaults.
func extractServeResponse(obj Object) serveResponse {
	resp := serveResponse{status: 200, body: "", headers: map[string]string{}}

	h, ok := obj.(*Hash)
	if !ok {
		resp.status = 500
		resp.body = fmt.Sprintf("handler must return a hash response, got %s", obj.Type())
		return resp
	}
	if pair, ok := h.Pairs[HashKey{Type: STRING_OBJ, Value: "status"}]; ok {
		if i, ok := pair.Value.(*Integer); ok {
			resp.status = int(i.Value)
		}
	}
	if pair, ok := h.Pairs[HashKey{Type: STRING_OBJ, Value: "body"}]; ok {
		if s, ok := pair.Value.(*String); ok {
			resp.body = s.Value
		}
	}
	if pair, ok := h.Pairs[HashKey{Type: STRING_OBJ, Value: "headers"}]; ok {
		if hh, ok := pair.Value.(*Hash); ok {
			for _, hp := range hh.Pairs {
				if k, ok := hp.Key.(*String); ok {
					if v, ok := hp.Value.(*String); ok {
						resp.headers[k.Value] = v.Value
					}
				}
			}
		}
	}
	return resp
}

// httpErrTuple returns a failure Tuple with the error message in position 3.
func httpErrTuple(msg string) *Tuple {
	return &Tuple{Elements: []Object{
		&Integer{Value: 0},
		&String{Value: ""},
		&Hash{Pairs: make(map[HashKey]HashPair)},
		&String{Value: msg},
	}}
}
