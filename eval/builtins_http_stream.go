package eval

import (
	"bufio"
	"fmt"
	"io"
	"klex/ast"
	"net/http"
	"strings"
)

// streamErrTuple builds the (null, error_string) tuple returned on
// connection-level failures — same shape as _httpDo's httpErrTuple.
func streamErrTuple(msg string) Object {
	return &Tuple{Elements: []Object{NULL, &String{Value: msg}}}
}

func init() {
	// _httpStream(method, url, headers, body [, mode]) → (channel, err)
	//
	// Open an HTTP request and return a kLex channel that yields parsed
	// streaming frames as hashes:
	//
	//   { "event": <event-name>, "data": <raw-data-payload> }
	//
	// `mode` selects the wire format (default "sse"):
	//
	//   "sse"   — Server-Sent Events. Each frame is one blank-line-delimited
	//             group of event:/data: lines. event is the SSE event name;
	//             data is the joined data payload. Default Accept header:
	//             text/event-stream. Used by Anthropic, OpenAI, etc.
	//
	//   "lines" — Newline-delimited JSON (NDJSON / JSON Lines). Each non-
	//             empty response line becomes one frame with event="" and
	//             data = the raw line text. Default Accept header:
	//             application/x-ndjson. Used by Ollama, llama.cpp's server,
	//             and similar local-runtime APIs.
	//
	// In both modes kLex callers parse the JSON payload themselves — payload
	// shapes are provider-specific so the primitive stays neutral.
	//
	// Cancellation:
	//   When the consumer breaks out of a `for evt in ch { ... }` loop the
	//   evaluator closes ch.done; the producer goroutine sees that, aborts
	//   the in-flight read, and closes the response body. No leaked
	//   connections after a consumer walks away.
	//
	// Return shape:
	//   On connection-level error (DNS, refused, 4xx/5xx setup failure):
	//     (null, "error message string")
	//   On success:
	//     (channel, null)
	//   Mid-stream errors (connection dropped, scanner failure) are
	//   delivered as a final frame {"event": "error", "data": "<message>"}
	//   before the channel closes.
	//
	// Defaults:
	//   - Accept header set automatically based on mode if not supplied.
	//   - No request timeout (streams can run indefinitely; rely on
	//     consumer cancellation via channel.done for shutdown).
	Builtins["_httpStream"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 4 || len(args) > 5 {
			return streamErrTuple("_httpStream expects 4 or 5 arguments: method, url, headers, body [, mode]")
		}
		mode := "sse"
		if len(args) == 5 {
			m, ok := args[4].(*String)
			if !ok {
				return streamErrTuple(fmt.Sprintf("_httpStream: mode must be string, got %s", args[4].Type()))
			}
			mode = m.Value
			if mode != "sse" && mode != "lines" {
				return streamErrTuple(fmt.Sprintf("_httpStream: mode must be \"sse\" or \"lines\", got %q", mode))
			}
		}
		method, ok := args[0].(*String)
		if !ok {
			return streamErrTuple(fmt.Sprintf("_httpStream: method must be string, got %s", args[0].Type()))
		}
		rawURL, ok := args[1].(*String)
		if !ok {
			return streamErrTuple(fmt.Sprintf("_httpStream: url must be string, got %s", args[1].Type()))
		}

		var bodyReader io.Reader
		switch b := args[3].(type) {
		case *String:
			if b.Value != "" {
				bodyReader = strings.NewReader(b.Value)
			}
		case *Null:
			// no body
		default:
			return streamErrTuple(fmt.Sprintf("_httpStream: body must be string or null, got %s", args[3].Type()))
		}

		req, err := http.NewRequest(method.Value, rawURL.Value, bodyReader)
		if err != nil {
			return streamErrTuple(err.Error())
		}

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
			return streamErrTuple(fmt.Sprintf("_httpStream: headers must be hash or null, got %s", args[2].Type()))
		}
		if req.Header.Get("Accept") == "" {
			if mode == "lines" {
				req.Header.Set("Accept", "application/x-ndjson")
			} else {
				req.Header.Set("Accept", "text/event-stream")
			}
		}

		// Streaming responses can run indefinitely — use a no-timeout
		// client. Cancellation is driven by the consumer closing the
		// channel's done signal (which closes the response body and
		// stops the underlying read).
		streamClient := &http.Client{}
		resp, err := streamClient.Do(req)
		if err != nil {
			return streamErrTuple(err.Error())
		}
		if resp.StatusCode >= 400 {
			// Drain a bounded amount of the error body for the message.
			errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			return streamErrTuple(fmt.Sprintf("HTTP %d: %s",
				resp.StatusCode, strings.TrimSpace(string(errBody))))
		}

		outCh := &Channel{
			ch:   make(chan Object, 32),
			done: make(chan struct{}),
		}

		// Watcher: close the response body when the consumer cancels.
		// The producer goroutine sees the body close as an EOF and exits.
		go func() {
			<-outCh.done
			resp.Body.Close()
		}()

		// Producer: parse frames from the response body and deliver each
		// frame to the channel. The exact parse rules depend on `mode`
		// (set above from the optional 5th argument).
		go func() {
			defer close(outCh.ch)
			defer resp.Body.Close()

			scanner := bufio.NewScanner(resp.Body)
			// Default scanner buffer is 64 KB; some frames carry large
			// JSON payloads. Raise the max to 4 MB.
			scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

			// emitFrame sends one {event, data} hash to the channel.
			// Returns false when the consumer has cancelled.
			emitFrame := func(eventName, dataPayload string) bool {
				frame := &Hash{Pairs: make(map[HashKey]HashPair, 2)}
				frame.Pairs[HashKey{Type: STRING_OBJ, Value: "event"}] = HashPair{
					Key:   &String{Value: "event"},
					Value: &String{Value: eventName},
				}
				frame.Pairs[HashKey{Type: STRING_OBJ, Value: "data"}] = HashPair{
					Key:   &String{Value: "data"},
					Value: &String{Value: dataPayload},
				}
				select {
				case outCh.ch <- frame:
					return true
				case <-outCh.done:
					return false
				}
			}

			if mode == "lines" {
				// NDJSON: one frame per non-empty line; event field is "".
				for scanner.Scan() {
					select {
					case <-outCh.done:
						return
					default:
					}
					line := scanner.Text()
					if line == "" {
						continue
					}
					if !emitFrame("", line) {
						return
					}
				}
			} else {
				// SSE: blank-line-delimited frames built up from
				// event:/data: header lines (per the WHATWG SSE spec).
				var (
					curEvent strings.Builder
					curData  strings.Builder
					gotEvent bool
					gotData  bool
				)

				emit := func() bool {
					if !gotEvent && !gotData {
						return true
					}
					ok := emitFrame(curEvent.String(), curData.String())
					curEvent.Reset()
					curData.Reset()
					gotEvent = false
					gotData = false
					return ok
				}

				for scanner.Scan() {
					select {
					case <-outCh.done:
						return
					default:
					}
					line := scanner.Text()
					if line == "" {
						if !emit() {
							return
						}
						continue
					}
					if strings.HasPrefix(line, ":") {
						// SSE comment line (often used as keepalive). Skip.
						continue
					}
					if strings.HasPrefix(line, "event:") {
						curEvent.WriteString(strings.TrimSpace(line[6:]))
						gotEvent = true
						continue
					}
					if strings.HasPrefix(line, "data:") {
						if gotData {
							curData.WriteByte('\n')
						}
						curData.WriteString(strings.TrimSpace(line[5:]))
						gotData = true
						continue
					}
					// Other field types (id:, retry:) are intentionally
					// ignored — most LLM SSE streams don't use them.
				}
				_ = emit()
			}

			// Surface scanner errors as a final error frame (both modes).
			if err := scanner.Err(); err != nil && err != io.EOF {
				errFrame := &Hash{Pairs: make(map[HashKey]HashPair, 2)}
				errFrame.Pairs[HashKey{Type: STRING_OBJ, Value: "event"}] = HashPair{
					Key:   &String{Value: "event"},
					Value: &String{Value: "error"},
				}
				errFrame.Pairs[HashKey{Type: STRING_OBJ, Value: "data"}] = HashPair{
					Key:   &String{Value: "data"},
					Value: &String{Value: err.Error()},
				}
				select {
				case outCh.ch <- errFrame:
				case <-outCh.done:
				}
			}
		}()

		return &Tuple{Elements: []Object{outCh, NULL}}
	}}

	// silence "imported and not used" if ast becomes unused in future edits
	_ = ast.Pos{}
}
