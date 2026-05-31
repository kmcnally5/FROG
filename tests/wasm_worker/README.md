# kLex worker bridge test

End-to-end manual test of `bridgeOpen({"kind": "worker", ...})` — kLex
WASM in a browser tab spawns a Web Worker and round-trips through the
bridge protocol over `postMessage` instead of stdio.

## How to run

The browser refuses to spawn workers from `file://` URLs, so the
artifacts must be served over HTTP. From this directory:

```
# Build the WASM (with embedded stdlib) + copy the helper, then serve.
./serve.sh
```

Or do it manually:

```
# From the repo root:
go run ./tools/stdlibgen
GOOS=js GOARCH=wasm CGO_ENABLED=0 go build -o tests/wasm_worker/klex.wasm ./cmd/wasm
cp docs/playground/wasm_exec.js                tests/wasm_worker/
cp stdlib/worker/klex_bridge_worker.js         tests/wasm_worker/

# From this directory:
python3 -m http.server 8765
# then open http://localhost:8765/
```

## What the test demonstrates

The `index.html` script in this directory shows the canonical worker
bridge pattern:

```lex
let bridge, err = bridgeOpen({
    "kind":   "worker",
    "script": "test_worker.js"
})
let sum, e = bridgeCall(bridge, "add", [2, 3])
bridgeClose(bridge)
```

`test_worker.js` is the worker side — it `importScripts` the helper
(`klex_bridge_worker.js`), declares its handlers via `handler(...)`,
and calls `serve()`. Identical shape to the Python/Node bridge helpers
in `stdlib/python/klex_bridge.py` and `stdlib/node/klex_bridge.js`.

## Expected output

```
✓ worker bridge opened
  protocol:     1
  language:     javascript-worker
  helper:       klex_bridge_worker.js/0.7.0
  capabilities: [schema]
✓ add(2,3) = 5
✓ greet = Hello from the worker, kLex!
✓ strlen = 19
✓ stats.count = 4
✓ stats.sum   = 100
✓ stats.mean  = 25

✓ bridge closed cleanly — worker terminated
```
