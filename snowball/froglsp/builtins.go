package main

type BuiltinInfo struct {
	Signature     string
	Documentation string
	Params        []string
}

var builtinSignatures = map[string]BuiltinInfo{
	// Core / Output
	"println": {
		Signature:     "println(...vals: any) -> null",
		Documentation: "Print values to stdout with newline.",
		Params:        []string{"...vals"},
	},
	"print": {
		Signature:     "print(...vals: any) -> null",
		Documentation: "Print values to stdout without newline.",
		Params:        []string{"...vals"},
	},

	// Type / Introspection
	"type": {
		Signature:     "type(val: any) -> string",
		Documentation: "Return the type name of a value (e.g., 'INTEGER', 'STRING', 'ARRAY').",
		Params:        []string{"val"},
	},
	"str": {
		Signature:     "str(val: any) -> string",
		Documentation: "str converts any value to its string representation.\nThis is how you turn an integer into a string for building output.\nstr(42) → \"42\", str(true) → \"true\", str(null) → \"null\"",
		Params:        []string{"val"},
	},
	"int": {
		Signature:     "int(val: string | float | int) -> int",
		Documentation: "Convert a number or string to integer. Crashes on bad input — use parseInt() for untrusted strings.",
		Params:        []string{"val"},
	},
	"float": {
		Signature:     "float(val: string | int | float) -> float",
		Documentation: "Convert a number or string to float. Crashes on bad input — use parseFloat() for untrusted strings.",
		Params:        []string{"val"},
	},
	"parseInt": {
		Signature:     "parseInt(str: string) -> (int, error)",
		Documentation: "Safely parse a string as an integer. Trims whitespace. Returns (value, null) on success or (null, error) on failure — use this instead of int() for untrusted input (CSV, HTTP, databases).\n\nExample:\n  n, err = parseInt(row[\"count\"])\n  if err != null { println(\"bad value: {err.message}\")  return }",
		Params:        []string{"str"},
	},
	"parseFloat": {
		Signature:     "parseFloat(str: string) -> (float, error)",
		Documentation: "Safely parse a string as a float. Trims whitespace. Returns (value, null) on success or (null, error) on failure — use this instead of float() for untrusted input (CSV, HTTP, databases).\n\nExample:\n  f, err = parseFloat(row[\"price\"])\n  if err != null { println(\"bad value: {err.message}\")  return }",
		Params:        []string{"str"},
	},

	// Arrays
	"len": {
		Signature:     "len(val: array | string | hash) -> int",
		Documentation: "Return the length of an array, string, or hash.",
		Params:        []string{"val"},
	},
	"push": {
		Signature:     "push(arr: array, val: any) -> array",
		Documentation: "push returns a NEW array with the element appended — it does not mutate.\nThis is intentional: immutable array operations are safer and more predictable.",
		Params:        []string{"arr", "val"},
	},
	"pop": {
		Signature:     "pop(arr: array) -> array",
		Documentation: "pop returns a NEW array with the last element removed — it does not mutate.\nConsistent with push: both operations return new arrays rather than modifying in place.\nCalling pop on an empty array returns an empty array.",
		Params:        []string{"arr"},
	},
	"concat": {
		Signature:     "concat(a: array, b: array) -> array",
		Documentation: "concat merges two arrays into a new array in a single allocation.\nFaster than looping push when combining two existing arrays.\nUsage: concat(arr1, arr2) -> new array containing all elements of arr1 followed by arr2.\n\nANTIPATTERN ALERT (OFI #10): `acc = concat(acc, batch)` in a loop\nis O(n²) — each call copies the growing accumulator. If you're\nmerging more than two arrays, collect them into a single outer\narray first and call concatAll() — that's O(total) in one pass.",
		Params:        []string{"a", "b"},
	},
	"slice": {
		Signature:     "slice(arr: array | bytes, start: int, end?: int) -> array | bytes",
		Documentation: "Extract a slice. Accepts arrays and bytes; returns the same kind. Indices are 0-based; end is exclusive and defaults to the source length. Out-of-bounds indices produce a RuntimeError.",
		Params:        []string{"arr", "start", "end"},
	},
	"saveImage": {
		Signature:     "saveImage(img: image, path: string) -> null",
		Documentation: "Write an image (from loadImage()) to disk. Format is chosen from the path extension:\n  .png         lossless (default for any other extension)\n  .jpg, .jpeg  quality 92\n  .gif         palette-quantised\n\nEncoding happens directly from the image's in-memory RGBA pixels — no separate readback is needed. Returns null on success, raises a RuntimeError on file or encoding failure.\n\nExample:\n  img = loadImage(bytes)\n  saveImage(img, \"/tmp/generated.png\")\n  saveImage(img, \"/tmp/generated.jpg\")",
		Params:        []string{"img", "path"},
	},
	"bytes": {
		Signature:     "bytes(x: string | array | int | bytes) -> bytes",
		Documentation: "Construct a bytes value.\n\nForms:\n  bytes(\"hello\")         — utf-8 encode the string\n  bytes([72, 101, 108])  — pack an integer array; each element must be in [0, 255]\n  bytes(5)               — N zero bytes\n  bytes(b\"...\")          — defensive copy of an existing bytes value\n\nThe bytes type is kLex's representation of arbitrary binary data — not utf-8 validated. Use bytesToStr() to decode utf-8 (returns an error tuple for invalid sequences), or bytesToHex() / bytesToBase64() for text-safe representations.",
		Params:        []string{"x"},
	},
	"strToBytes": {
		Signature:     "strToBytes(s: string) -> bytes",
		Documentation: "utf-8 encode a string. Strings in kLex are already utf-8 internally, so this is effectively a typed copy — but having it as an explicit builtin keeps the conversion direction obvious at the call site.",
		Params:        []string{"s"},
	},
	"bytesToStr": {
		Signature:     "bytesToStr(b: bytes) -> (string, error)",
		Documentation: "utf-8 decode bytes. Returns (str, null) on success, (null, err) when the bytes are not valid utf-8 — err.code is \"BYTES_INVALID_UTF8\". Use this when binary may be text; unwrap with `?` if the bytes are guaranteed utf-8 (e.g. JSON response bodies).",
		Params:        []string{"b"},
	},
	"bytesToBase64": {
		Signature:     "bytesToBase64(b: bytes) -> string",
		Documentation: "Standard base64 encoding (RFC 4648, with padding). Reverse of base64ToBytes. Returns a single string with no internal line breaks.",
		Params:        []string{"b"},
	},
	"base64ToBytes": {
		Signature:     "base64ToBytes(s: string) -> (bytes, error)",
		Documentation: "Decode standard base64. Returns (bytes, null) on success, (null, err) with code \"BASE64_INVALID\" on bad input.",
		Params:        []string{"s"},
	},
	"bytesToHex": {
		Signature:     "bytesToHex(b: bytes) -> string",
		Documentation: "Lowercase hex encoding, two characters per byte, no separator. Reverse of hexToBytes.",
		Params:        []string{"b"},
	},
	"hexToBytes": {
		Signature:     "hexToBytes(s: string) -> (bytes, error)",
		Documentation: "Decode a hex string. Accepts both upper and lower case. Returns (bytes, null) on success, (null, err) with code \"HEX_INVALID\" on bad input (odd length, non-hex character).",
		Params:        []string{"s"},
	},
	"bytesConcat": {
		Signature:     "bytesConcat(arr: array of bytes) -> bytes",
		Documentation: "Merge an array of bytes values into one bytes value via Go-level memcpy. Empty input → empty bytes. Single-element input → a defensive copy. Mixed-type input → typeError naming the first non-bytes index.\n\nMuch faster than the historical workaround of byte-indexing each into an int array and calling `bytes(intArray)` — that was ~40ms per 393kB merge in the interpreter; this is microsecond-scale.\n\nExample:\n  payload = bytesConcat([header, body1, body2])\n  _fsAppendBytesSync(path, payload)   // one syscall instead of N",
		Params:        []string{"arr"},
	},
	"makeArray": {
		Signature:     "makeArray(n: int, default?: any) -> array",
		Documentation: "makeArray allocates an array of n elements in a single allocation, all set to defaultVal.\nUse this instead of building with push() in a loop — push is O(n) per call (O(n²) total),\nmakeArray is O(n) once. Fill elements with arr[i] = val afterwards.\nUsage: arr = makeArray(1000000, 0)   — 1M zeros, O(n)",
		Params:        []string{"n", "default"},
	},

	// Strings
	"split": {
		Signature:     "split(str: string, sep: string) -> array",
		Documentation: "Split a string by separator.",
		Params:        []string{"str", "sep"},
	},
	"join": {
		Signature:     "join(arr: array, sep: string) -> string",
		Documentation: "join concatenates an array of strings into a single string with a separator.\njoin([\"a\", \"b\", \"c\"], \",\") → \"a,b,c\"\nAll array elements must be strings — mixing types is a TypeError.",
		Params:        []string{"arr", "sep"},
	},
	"upper": {
		Signature:     "upper(str: string) -> string",
		Documentation: "Convert string to uppercase.",
		Params:        []string{"str"},
	},
	"lower": {
		Signature:     "lower(str: string) -> string",
		Documentation: "Convert string to lowercase.",
		Params:        []string{"str"},
	},
	"trim": {
		Signature:     "trim(str: string) -> string",
		Documentation: "Remove leading and trailing whitespace.",
		Params:        []string{"str"},
	},
	"replace": {
		Signature:     "replace(str: string, old: string, new: string) -> string",
		Documentation: "Replace EVERY occurrence of `old` with `new` (thin wrapper over Go's strings.ReplaceAll). Despite the JS-style name, this is replace-all, not single-replace. Use `replaceAll` if you prefer the more explicit name — same behaviour.",
		Params:        []string{"str", "old", "new"},
	},
	"concatAll": {
		Signature:     "concatAll(arrs: array of array) -> array",
		Documentation: "Flatten an array of arrays into one array in a single pass — O(total) instead of the O(n²) you'd get from looping `acc = concat(acc, batch)`.\n\nExample:\n  // BAD — O(n²) for large N\n  acc = []\n  for batch in batches { acc = concat(acc, batch) }\n\n  // GOOD — O(total) one allocation\n  acc = concatAll(batches)\n\nEmpty outer → empty result. Non-array elements surface as a typed error naming the index.",
		Params:        []string{"arrs"},
	},
	"replaceAll": {
		Signature:     "replaceAll(str: string, old: string, new: string) -> string",
		Documentation: "JS/Python-friendly alias of `replace` — replaces EVERY occurrence of `old` with `new`. Both names exist so coming from a language where replace() is single-replace doesn't trip you up; pick whichever reads better in context.",
		Params:        []string{"str", "old", "new"},
	},
	"substr": {
		Signature:     "substr(str: string, start: int, end?: int) -> string",
		Documentation: "returns the substring from start to the end of str.\nsubstr(str, start, end) — returns the substring from start up to (not including) end.\nIndices are 0-based. A RuntimeError is raised if start or end is out of bounds.\n\n  substr(\"hello world\", 6)     → \"world\"\n  substr(\"hello world\", 0, 5)  → \"hello\"",
		Params:        []string{"str", "start", "end"},
	},
	"indexOf": {
		Signature:     "indexOf(str: string, substr: string) -> int",
		Documentation: "indexOf returns the index of the first occurrence of substr in str, or -1 if not found.\nOperates on Unicode code points, consistent with string indexing.\nindexOf(\"hello\", \"ll\") → 2,  indexOf(\"hello\", \"x\") → -1",
		Params:        []string{"str", "substr"},
	},
	"startsWith": {
		Signature:     "startsWith(str: string, prefix: string) -> bool",
		Documentation: "Check if string starts with prefix.",
		Params:        []string{"str", "prefix"},
	},
	"endsWith": {
		Signature:     "endsWith(str: string, suffix: string) -> bool",
		Documentation: "Check if string ends with suffix.",
		Params:        []string{"str", "suffix"},
	},
	"ord": {
		Signature:     "ord(c: string) -> int",
		Documentation: "Return the Unicode code point of the first character of c. Pairs with chr(). Errors on empty string or invalid UTF-8. Multi-char inputs use only the first rune.\n\nExamples: `ord(\"A\")` → 65, `ord(\"☃\")` → 9731.",
		Params:        []string{"c"},
	},
	"chr": {
		Signature:     "chr(n: int) -> string",
		Documentation: "Return a single-character string for code point n. Pairs with ord(). Errors if n is negative, a surrogate (0xD800–0xDFFF), or >= 0x110000.\n\nExamples: `chr(65)` → \"A\", `chr(9731)` → \"☃\".",
		Params:        []string{"n"},
	},

	// Hash / Object
	"keys": {
		Signature:     "keys(hash: hash) -> array",
		Documentation: "keys returns an array of all keys in a hash.\nNote: Go maps have no guaranteed order, so the key order is non-deterministic.",
		Params:        []string{"hash"},
	},
	"values": {
		Signature:     "values(hash: hash) -> array",
		Documentation: "values returns an array of all values in a hash.\nMirrors keys() — accepts both *Hash and *ConcurrentHash so the two\nbuiltins stay symmetric (calling code can iterate values(ch) the same\nway it iterates keys(ch)). Go map iteration is non-deterministic so\ndo not rely on order across calls; ConcurrentHash uses sync.Map.Range\nwhose ordering is also non-deterministic and may briefly include or\nexclude entries added/removed during iteration.",
		Params:        []string{"hash"},
	},
	"hasKey": {
		Signature:     "hasKey(hash: hash, key: any) -> bool",
		Documentation: "hasKey returns true if the hash contains the given key, false otherwise.\nhasKey(h, \"name\") — avoids the null-check pattern of h[\"name\"] == null.",
		Params:        []string{"hash", "key"},
	},
	"delete": {
		Signature:     "delete(hash: hash, key: any) -> null",
		Documentation: "Delete a key from a hash.",
		Params:        []string{"hash", "key"},
	},

	// Range
	"range": {
		Signature:     "range(stop: int) | range(start: int, stop: int) | range(start: int, stop: int, step: int) -> array",
		Documentation: "range generates an array of integers.\nrange(stop)             → [0, 1, ..., stop-1]\nrange(start, stop)      → [start, start+1, ..., stop-1]\nrange(start, stop, step)→ [start, start+step, ...] up to but not including stop\nA negative step counts down. Returns an empty array if the range is empty.",
		Params:        []string{"start", "stop", "step"},
	},

	// Utility
	"env": {
		Signature:     "env(name: string) -> string | null",
		Documentation: "Get an environment variable value.",
		Params:        []string{"name"},
	},

	// File I/O
	"readFile": {
		Signature:     "readFile(path: string) -> string",
		Documentation: "readFile reads the entire contents of a file and returns it as a string.\nOn failure (file not found, permission denied, etc.) it returns a runtime error.\nUse safe(readFile, path) to handle the error without crashing.",
		Params:        []string{"path"},
	},
	"writeFile": {
		Signature:     "writeFile(path: string, content: string) -> null",
		Documentation: "writeFile writes a string to a file, creating it if it does not exist and\ntruncating it if it does. On failure returns a runtime error.",
		Params:        []string{"path", "content"},
	},
	"appendFile": {
		Signature:     "appendFile(path: string, content: string) -> null",
		Documentation: "appendFile appends a string to a file, creating it if it does not exist.\nOn failure returns a runtime error.",
		Params:        []string{"path", "content"},
	},

	// Process
	"exec": {
		Signature:     "exec(cmd: string, args: array) -> string",
		Documentation: "exec runs an external binary and returns its stdout as a string.\nThe first argument is the command name or path; the second is an array\nof string arguments. On non-zero exit or any OS error, returns a runtime\nerror — use safe(exec, cmd, args) to handle failures without crashing.",
		Params:        []string{"cmd", "args"},
	},

	// Channels
	"channel": {
		Signature:     "channel(capacity?: int) -> channel",
		Documentation: "channel creates a new channel for passing values between async tasks.\nchannel()    — unbuffered: send blocks until a receiver is ready.\nchannel(n)   — buffered with capacity n: send blocks only when the buffer is full.",
		Params:        []string{"capacity"},
	},
	"send": {
		Signature:     "send(ch: channel, val: any) -> null | false",
		Documentation: "Send a value to a channel. Returns false if channel is closed.",
		Params:        []string{"ch", "val"},
	},
	"recv": {
		Signature:     "recv(ch: channel) -> (any, bool)",
		Documentation: "recv receives the next value from a channel.\nReturns (value, true) when a value is available.\nReturns (null, false) when the channel is closed and empty.\nBlocks until a value is available or the channel is closed.",
		Params:        []string{"ch"},
	},
	"recvNonBlock": {
		Signature:     "recvNonBlock(ch: channel) -> any | null",
		Documentation: "recvNonBlock attempts to receive from a channel without blocking.\nReturns the value if one is immediately available.\nReturns null if the channel is empty (no value available yet).\nReturns null if the channel is closed with no buffered values.\nUsed for cooperative cancellation signaling in parallel workers.",
		Params:        []string{"ch"},
	},
	"close": {
		Signature:     "close(ch: channel) -> null",
		Documentation: "close signals that no more values will be sent on the channel.\nReceivers will drain any buffered values then get (null, false) from recv.\nReturns null on success. RuntimeError if the channel is already closed.",
		Params:        []string{"ch"},
	},
	"cancel": {
		Signature:     "cancel(ch: channel) -> null",
		Documentation: "cancel signals that the consumer of a channel is done and no more values\nshould be sent. Any blocked or future send() call on this channel returns\nfalse instead of blocking. cancel() is idempotent — calling it twice is safe.\nThis is the explicit form; breaking out of a for-in loop over a channel\nalso cancels it automatically.",
		Params:        []string{"ch"},
	},

	// Error handling
	"isError": {
		Signature:     "isError(val: any) -> bool",
		Documentation: "isError returns true if val is a RuntimeError or TypeError produced by the\nevaluator. Use this inside stream pipeline stages to detect errors returned\nby callback functions via safe().",
		Params:        []string{"val"},
	},
	"error": {
		Signature:     "error(code: string, message: string) -> error",
		Documentation: "error creates a first-class Error value with a user-defined code and message.\nerror(\"NOT_FOUND\", \"key was missing\")\nThe returned Error is NOT a propagation signal — it stays in the environment\nand can be inspected via .code, .message, and .is(code).",
		Params:        []string{"code", "message"},
	},
	"safe": {
		Signature:     "safe(fn: function, ...args: any) -> (any, error | null)",
		Documentation: "Call a function and catch any errors. Returns (result, error).",
		Params:        []string{"fn", "...args"},
	},
	"assert": {
		Signature:     "assert(condition: bool, message?: string) -> null",
		Documentation: "assert checks that condition is true.\nassert(condition)          — fails with \"assert: condition is false\"\nassert(condition, message) — fails with the given message\ncondition must be bool; a non-bool condition is a TypeError.\nOn success returns null. On failure raises a RuntimeError that propagates\nnormally — catchable with safe() like any other error.",
		Params:        []string{"condition", "message"},
	},

	// Higher-order
	"apply": {
		Signature:     "apply(fn: function, args: array) -> any",
		Documentation: "call fn with the elements of args as positional arguments.\n\nThis is the spread/variadic-call operator: where `fn(a, b, c)` is fixed at\nparse time, `apply(fn, [a, b, c])` lets you build the argument list at\nruntime. Indispensable for higher-order utilities like partial, flip,\ncurry, and pipelines that hand off an arbitrary-arity call.\n\n  apply(fn(a, b) { a + b }, [3, 4])   → 7\n  apply(println, [\"hello\", \"world\"])  → prints \"hello world\"\n\nErrors:\n  - fn must be a *Function or *Builtin\n  - args must be an *Array\n  - any error raised by fn itself is returned unchanged",
		Params:        []string{"fn", "args"},
	},
	"map": {
		Signature:     "map(arr: array, fn: function) -> array",
		Documentation: "Apply a function to each element.",
		Params:        []string{"arr", "fn"},
	},
	"filter": {
		Signature:     "filter(arr: array, fn: function) -> array",
		Documentation: "filter returns a new array containing only the elements for which\nthe function returns true. Example: filter([1,2,3,4], fn(x) { x > 2 }) → [3, 4]",
		Params:        []string{"arr", "fn"},
	},
	"reduce": {
		Signature:     "reduce(arr: array, fn: function, init: any) -> any",
		Documentation: "reduce folds an array into a single value by repeatedly applying a function.\nThe function receives (accumulator, currentElement) and returns the new accumulator.\nExample: reduce([1,2,3], fn(acc, x) { acc + x }, 0) → 6",
		Params:        []string{"arr", "fn", "init"},
	},

	// Parallel array primitives
	"parallelArrayUpdate": {
		Signature:     "parallelArrayUpdate(arr: array, fn: function) -> array",
		Documentation: "Mutates each element in-place using fn(value, index). Splits work across runtime.NumCPU() goroutines. Workers run lock-free in env snapshot. Returns the same array.",
		Params:        []string{"arr", "fn"},
	},
	"parallelArrayMap": {
		Signature:     "parallelArrayMap(arr: array, fn: function) -> array",
		Documentation: "Returns a new array where each element is fn(value, index) of the input. Parallel across runtime.NumCPU() goroutines. Source array unchanged.",
		Params:        []string{"arr", "fn"},
	},
	"parallelArrayReduce": {
		Signature:     "parallelArrayReduce(arr: array, fn: function, init: any) -> any",
		Documentation: "parallelArrayReduce performs a parallel reduction over the array.\n\nCONSTRAINTS (both required for correct results):\n  1. fn(accumulator, element) MUST be ASSOCIATIVE\n     (e.g. +, *, max — chunk order is not guaranteed).\n  2. `initial` MUST be the IDENTITY for fn\n     (0 for +, 1 for *, \"\" for string concat, [] for array concat).\n\nWhy identity matters: each worker starts its own chunk from `initial`,\nand the final serial pass also folds `initial` in once more. If you pass\na non-identity init like 100 for +, you get 100 applied (numWorkers + 1)\ntimes instead of once. The result will not equal a serial reduce.\n\nFor non-identity inits, use stdlib/parallel.lex's parallel_reduce, which\ntakes a separate mergeFn and applies init exactly once.\n\n  total = parallelArrayReduce(nums, fn(a, b) { a + b }, 0)",
		Params:        []string{"arr", "fn", "init"},
	},
	"parallelArrayForEach": {
		Signature:     "parallelArrayForEach(arr: array, fn: function) -> null",
		Documentation: "Like parallelArrayMap but discards return values. Use when callbacks have side effects (e.g. atomic updates) instead of producing a transformed array.",
		Params:        []string{"arr", "fn"},
	},

	// Atomic array primitives (lock-free)
	"atomicIntArray": {
		Signature:     "atomicIntArray(size: int, [initial: int]) -> AtomicIntArray",
		Documentation: "Creates a fixed-size lock-free integer array. Multiple goroutines can safely call atomicAdd/Load/Store/CAS concurrently without mutexes.",
		Params:        []string{"size", "initial"},
	},
	"atomicFloatArray": {
		Signature:     "atomicFloatArray(size: int, [initial: float]) -> AtomicFloatArray",
		Documentation: "Creates a fixed-size lock-free float64 array. Floats stored as int64 bits; atomicAdd uses CAS retry loop. Lock-free under concurrent access.",
		Params:        []string{"size", "initial"},
	},
	"atomicLoad": {
		Signature:     "atomicLoad(arr: AtomicIntArray|AtomicFloatArray, idx: int) -> int|float",
		Documentation: "Atomically reads the value at idx. Safe for concurrent access from multiple goroutines.",
		Params:        []string{"arr", "idx"},
	},
	"atomicStore": {
		Signature:     "atomicStore(arr: AtomicIntArray|AtomicFloatArray, idx: int, value: int|float) -> null",
		Documentation: "Atomically writes value at idx. Safe for concurrent access.",
		Params:        []string{"arr", "idx", "value"},
	},
	"atomicAdd": {
		Signature:     "atomicAdd(arr: AtomicIntArray|AtomicFloatArray, idx: int, delta: int|float) -> int|float",
		Documentation: "Atomically adds delta to arr[idx], returns new value. Lock-free (CAS-loop for floats). Use for shared counters and accumulators across goroutines.",
		Params:        []string{"arr", "idx", "delta"},
	},
	"atomicCAS": {
		Signature:     "atomicCAS(arr: AtomicIntArray|AtomicFloatArray, idx: int, old: int|float, new: int|float) -> bool",
		Documentation: "Compare-and-swap. If arr[idx] == old, replaces with new and returns true; otherwise returns false. Building block for custom lock-free algorithms.",
		Params:        []string{"arr", "idx", "old", "new"},
	},

	// Concurrent hash map (lock-free shared key/value store)
	"concurrentHash": {
		Signature:     "concurrentHash() -> ConcurrentHash",
		Documentation: "Creates a thread-safe hash map for shared mutable state across goroutines. Read with ch[key], write with ch[key] = val (atomic). Use atomicHashIncr/Add for lock-free arithmetic, atomicHashCAS for compare-and-swap.\n\nlen(ch) is O(1) but APPROXIMATE under concurrent mutation — it can briefly diverge from the actual entry count by the number of in-flight writes. The map data itself is always consistent; only the reported count is best-effort during contention. For an exact size, use **quiesceLen(ch)** (O(n) walk) after a known quiescent point.",
		Params:        []string{},
	},
	"quiesceLen": {
		Signature:     "quiesceLen(ch: ConcurrentHash) -> int",
		Documentation: "Returns the exact entry count of a ConcurrentHash by walking every live entry. O(n) vs len(ch)'s O(1) but always accurate.\n\nUse after writer tasks have been awaited / quiesced when you need a guaranteed-correct count (invariants, test assertions, 'exactly N items' checks). Prefer the O(1) len(ch) for progress indicators where ±1 doesn't matter.\n\nConcurrent writes during the walk are not blocked; entries inserted or deleted mid-walk may or may not be counted (sync.Map.Range semantics). For a fully consistent snapshot, ensure no writers are active.",
		Params:        []string{"ch"},
	},
	"atomicHashIncr": {
		Signature:     "atomicHashIncr(ch: ConcurrentHash, key: string|int|bool, delta: int) -> int",
		Documentation: "Atomically increments the integer at key by delta. Treats missing key as 0. Returns new value. Lock-free CAS-loop; safe under concurrent access from any goroutines.",
		Params:        []string{"ch", "key", "delta"},
	},
	"atomicHashAdd": {
		Signature:     "atomicHashAdd(ch: ConcurrentHash, key: string|int|bool, delta: float) -> float",
		Documentation: "Atomically adds delta (float) to the value at key. Treats missing key as 0.0. Returns new value. Lock-free.",
		Params:        []string{"ch", "key", "delta"},
	},
	"atomicHashCAS": {
		Signature:     "atomicHashCAS(ch: ConcurrentHash, key: string|int|bool, old: any, new: any) -> bool",
		Documentation: "Compare-and-swap on hash entry. Returns true if key exists with value structurally equal to old (and was swapped to new); false otherwise. Building block for lock-free state machines.",
		Params:        []string{"ch", "key", "old", "new"},
	},

	// Async
	"async": {
		Signature:     "async(fn: function, ...args: any) -> task",
		Documentation: "async launches a function in a background goroutine and returns a Task\nimmediately. Accepts both user-defined functions and builtins.\nUsage: task = async(fn, arg1, arg2, ...)\nThe function runs in a snapshotted environment: it can read globals from the\ntime the task was launched, but mutations are task-local and not visible to\nthe caller. This eliminates mutex contention and prevents shared mutable state bugs.\nNote: do not call input() from async — it shares a global stdin reader.",
		Params:        []string{"fn", "...args"},
	},
	"await": {
		Signature:     "await(task: task) -> any",
		Documentation: "await blocks until the given task completes and returns its result.\nIf the task's function produced an error, await propagates it.\nUsage: result = await(task)",
		Params:        []string{"task"},
	},

	// Native Bridge (cross-language FFI via subprocess JSON-RPC)
	"bridgeOpen": {
		Signature:     "bridgeOpen(transport: hash) -> (bridge, error)",
		Documentation: "Transport-polymorphic bridge constructor. Dispatches on transport[\"kind\"]:\n  \"subprocess\" — spawn a subprocess and speak the kLex bridge protocol over stdin/stdout (only kind implemented today)\n  \"worker\"     — Web Worker (browser/WASM target; not yet implemented — returns BRIDGE_TRANSPORT_UNAVAILABLE)\n  \"remote\"     — TCP/QUIC remote bridge (future — returns BRIDGE_TRANSPORT_UNAVAILABLE)\n\nNamed `bridgeOpen` (not `bridge`) to match the bridgeXxx family (bridgeClose / bridgeCall / …) and to avoid shadowing the natural variable name. `let bridge, err = bridgeOpen({...})` is the idiomatic call shape.\n\nSubprocess transport hash schema:\n  kind:             \"subprocess\"   (required)\n  cmd:              string          (required — executable name or path)\n  args:             [string, ...]   (optional — argv tail)\n  timeout_seconds:  number          (optional — default per-call timeout)\n  max_response_mb:  int             (optional — wire-frame cap, 1..256)\n  stderr_log:       string          (optional — mirror bridge stderr here)\n\nKey validation is STRICT — any key not in the schema yields BRIDGE_TRANSPORT_MISCONFIGURED naming the bad key, so typos like \"tiemout_seconds\" fail loud instead of silently disabling the option.\n\nError codes returned in the second tuple element:\n  BRIDGE_TRANSPORT_MISCONFIGURED — missing required key, wrong type, or unknown key\n  BRIDGE_TRANSPORT_UNKNOWN       — `kind` value not recognised\n  BRIDGE_TRANSPORT_UNAVAILABLE   — `kind` recognised but not implemented in this build\n  BRIDGE_ERROR                   — failed to start subprocess\n\nExample:\n  let bridge, err = bridgeOpen({\n      \"kind\": \"subprocess\",\n      \"cmd\":  \"python3\",\n      \"args\": [\"bridge.py\"],\n      \"timeout_seconds\": 30,\n      \"max_response_mb\": 16\n  })\n  if err != null { println(err.message)  return }\n\nSee docs/BRIDGE_API_DESIGN.MD for the full design rationale.",
		Params:        []string{"transport"},
	},
	"bridgeCall": {
		Signature:     "bridgeCall(bridge, fn: string, args: array, timeoutSec?: number) -> (result, error)",
		Documentation: "Call a function in the bridge subprocess. Blocks until the subprocess responds or the timeout fires. Calls are serialised per bridge — safe from async tasks.\n\nAll kLex types marshal automatically: integers, floats, strings, booleans, arrays, hashes.\n\nOptional 4th argument overrides the bridge's default timeout for this call only. Pass 0 for no timeout.\n\nError codes returned in the second tuple element:\n  BRIDGE_ERROR    — protocol/logic error returned by the bridge\n  BRIDGE_CLOSED   — subprocess closed unexpectedly\n  BRIDGE_TIMEOUT  — call exceeded the timeout (bridge becomes tainted)\n  BRIDGE_TAINTED  — bridge is unusable after a prior fatal error; call bridgeClose() and start a new bridge\n\nOn BRIDGE_CLOSED, BRIDGE_ERROR and BRIDGE_TIMEOUT, the error message includes the last lines of stderr captured from the subprocess.\n\nExample:\n  result, err = bridgeCall(bridge, \"slow_fn\", [arg], 120)\n  if err != null { println(err.code + \": \" + err.message)  return }",
		Params:        []string{"bridge", "fn", "args", "timeoutSec"},
	},
	"bridgeClose": {
		Signature:     "bridgeClose(bridge) -> null",
		Documentation: "Close the bridge subprocess cleanly. Sends EOF on stdin so a well-written bridge loop exits naturally; if it does not exit within 2 seconds, the bridge process group is force-killed. Safe to call multiple times.",
		Params:        []string{"bridge"},
	},
	"bridgeStderr": {
		Signature:     "bridgeStderr(bridge) -> array",
		Documentation: "Return the captured tail of the bridge subprocess's stderr output as an array of strings, one element per line. Useful for surfacing Python tracebacks and other diagnostic output that would otherwise be invisible.\n\nReturns an empty array if no stderr has been captured. When the bridge was created with a stderr_log option, output may also be in the log file.\n\nExample:\n  result, err = bridgeCall(bridge, \"crash\", [])\n  if err != null {\n      lines = bridgeStderr(bridge)\n      for line in lines { println(\"  \" + line) }\n  }",
		Params:        []string{"bridge"},
	},
	"bridgeNotifications": {
		Signature:     "bridgeNotifications(bridge) -> channel",
		Documentation: "Return the notification channel for this bridge. The channel receives every {\"notif\": ...} message the subprocess emits as a kLex value. The channel is closed when the bridge closes, making for-in loops exit cleanly.\n\nThe notification channel is buffered (256 items, drop-newest). Call this before starting a long-running bridgeCall so early notifications are not lost.\n\nThe subprocess emits notifications with no id field:\n  {\"notif\": {\"phase\": \"progress\", \"done\": 42}}\n\nPython helper:\n  def notify(data):\n      print(json.dumps({\"notif\": data}), flush=True)\n\nExample:\n  notifCh = bridgeNotifications(bridge)\n  async(fn() {\n      msg, ok = recv(notifCh)\n      while ok {\n          println(msg[\"done\"])\n          msg, ok = recv(notifCh)\n      }\n  })\n  result, err = bridgeCall(bridge, \"long_job\", [arg])",
		Params:        []string{"bridge"},
	},
	"bridgeSchema": {
		Signature:     "bridgeSchema(bridge, fn?: string) -> hash | null",
		Documentation: "Return the handler schemas declared by the bridge.\n\nSchemas are fetched automatically during nativeBridge() via a __schema__ handshake. Bridges that don't implement __schema__ return null from this builtin — those bridges still work, just without argument validation.\n\nWith one argument, returns a hash of every handler keyed by name:\n  {\n    \"add\":   { \"args\": [[\"a\",\"int\"], [\"b\",\"int\"]], \"returns\": \"int\" },\n    \"greet\": { \"args\": [[\"name\",\"string\"]],         \"returns\": \"string\" }\n  }\n\nWith two arguments, returns the single handler's schema (or null if it isn't declared):\n  bridgeSchema(bridge, \"add\")\n  // { \"args\": [[\"a\",\"int\"], [\"b\",\"int\"]], \"returns\": \"int\" }\n\nWhen schemas are present, bridgeCall validates positional arguments against them before marshalling and returns a BRIDGE_SCHEMA_ARG error on mismatch. Return values are validated too — Python-side via klex_bridge's serve(), kLex-side via bridgeCall as a safety net (BRIDGE_SCHEMA_RETURN).\n\nSchema mini-language:\n  int, float, string, bool, array, hash, null, any, bytes\n  Trailing \"?\" makes the type nullable: \"string?\" accepts string or null.\n\nNote: `bytes` declares the slot as binary. Until the wire format gains a native binary capability (negotiated via __hello__), bytes are carried as base64-encoded strings on the wire — helper authors should decode explicitly in handlers that receive them.\n\nPython side: declare schemas via the klex_bridge helper module — either with the @handler decorator or the imperative register() function. Use @stream_handler for streaming functions consumed by bridgeStream.",
		Params:        []string{"bridge", "fn"},
	},
	"bridgeInfo": {
		Signature:     "bridgeInfo(bridge) -> hash",
		Documentation: "Return the protocol metadata negotiated with the bridge during the __hello__ handshake.\n\nThe handshake runs automatically at nativeBridge() time and reports the protocol version both sides agreed on plus the capability set both sides support. Capabilities gate optional behaviour — features that have always been on the wire (stream, cancel, backpressure, notif, timeout) are implicit and never appear.\n\nCurrently advertised capability names:\n  schema  — bridge speaks the schema mini-language; bridgeSchema() returns a non-null map\n  binary  — bridge accepts/produces binary payloads (reserved; not yet wired)\n\nReturn shape:\n  {\n    \"protocol\":         1,\n    \"capabilities\":     [\"binary\", \"schema\"],\n    \"helper\":           \"klex_bridge.py/0.7.0\",\n    \"language\":         \"python\",\n    \"language_version\": \"3.12.4\"\n  }\n\nBridges that don't implement __hello__ are treated as protocol 0 with an empty capabilities array and empty helper/language fields — those bridges still work, exactly as they did before negotiation existed.\n\nExample:\n  info = bridgeInfo(bridge)\n  if info[\"protocol\"] == 0 {\n      println(\"legacy bridge — no negotiated capabilities\")\n  }\n  for cap in info[\"capabilities\"] {\n      println(\"supports: \" + cap)\n  }",
		Params:        []string{"bridge"},
	},
	"bridgeMetrics": {
		Signature:     "bridgeMetrics(bridge) -> hash",
		Documentation: "Snapshot observability counters for the bridge. Tracking is automatic — every bridgeCall and bridgeStream is recorded with no setup needed. Cheap to call repeatedly (one mutex acquisition + a small sort per function for percentiles), so a dashboard polling every second adds no measurable overhead.\n\nReturn shape:\n  {\n    \"calls_total\":     N,    // bridgeCall completions (success + error)\n    \"calls_inflight\":  M,    // calls currently waiting for a response\n    \"calls_failed\":    K,    // bridgeCall results carrying an error\n    \"streams_total\":   S,    // bridgeStream submissions\n    \"bytes_sent\":      X,    // total request bytes written to the bridge\n    \"bytes_received\":  Y,    // total response bytes read (call replies only)\n    \"errors_by_code\":  {\"BRIDGE_TIMEOUT\": 4, \"BRIDGE_SCHEMA_ARG\": 8, ...},\n    \"per_function\":    {\n      \"add\":   {\"count\": 230, \"errors\": 2, \"p50_ms\": 12.0, \"p95_ms\": 84.0, \"p99_ms\": 230.0},\n      \"scan\":  {\"count\":  47, \"errors\": 0, \"p50_ms\":  2.1, \"p95_ms\":  8.3, \"p99_ms\":  41.0}\n    }\n  }\n\nPercentiles are computed from a 256-sample rolling window per function. Older samples are overwritten so the percentiles reflect recent behaviour; a brief slow patch fades from p99 once 256 normal calls follow.\n\nExample — show pool latency in a kLex UI:\n  m = bridgeMetrics(bridge)\n  pf = m[\"per_function\"]\n  for fn in keys(pf) {\n      println(fn + \" p95: \" + str(pf[fn][\"p95_ms\"]) + \" ms\")\n  }",
		Params:        []string{"bridge"},
	},
	"bridgeStream": {
		Signature:     "bridgeStream(bridge, fn: string, args: array, timeout?: number | hash) -> (channel, error)",
		Documentation: "Call a STREAMING handler on a bridge subprocess. Returns immediately with a kLex channel; the bridge produces items in the background and the reader goroutine delivers each to the channel.\n\nConsumers drain via `for item in ch { ... }`. The channel closes on clean end-of-stream. Mid-stream errors are delivered as the final item — an *Error value the consumer can detect with `type(item) == \"ERROR\"`.\n\nThe handler on the bridge side must be registered with @stream_handler (klex_bridge) or register(..., stream=True). Calling a non-streaming handler via bridgeStream surfaces an error.\n\nArgument validation runs before the call hits the wire (same BRIDGE_SCHEMA_ARG semantics as bridgeCall). Each yielded item is also validated against the declared per-item type — both Python-side in serve() and (defensively) kLex-side.\n\nCancellation: breaking out of the for-in loop closes the channel's done signal, which kLex translates into a {\"cancel\": N} message sent on the bridge's stdin. The Python helper picks that up between yields and closes the running generator, so the bridge stops producing within one iteration instead of running to natural end. No wasted work after the consumer walks away.\n\nOptions (4th argument):\n  bridgeStream(b, fn, args, idleSec)              — number means idle timeout in seconds\n  bridgeStream(b, fn, args, {\"idle\": N, \"total\": M, \"window\": W})  — hash for fine control\n  bridgeStream(b, fn, args)                       — absent means no timeout, default window\n\nTimeout keys:\n  idle   — fail if no item arrives for N seconds\n  total  — fail if total stream duration exceeds N seconds\n\nWhen a timeout fires, the consumer receives a final *Error item with code \"BRIDGE_TIMEOUT\" on the channel and then the channel closes — same shape as a mid-stream error. The bridge is sent a cancel under the hood so it stops yielding.\n\nBackpressure (window key):\n  window — max items the bridge may have in flight (yielded but not acked).\n           Defaults to 32. Setting 0 disables backpressure entirely (the\n           bridge produces as fast as the OS pipe / kLex channel buffer\n           allows — today's pre-window behaviour, kept for back-compat).\n\nkLex acks the bridge every window/2 items delivered to the consumer channel,\nso the bridge never blocks waiting on an ack we're still batching. Hand-rolled\nbridges that don't speak ack ignore acks harmlessly and inherit the no-flow-\ncontrol path (window in the request is just an integer they may safely drop).\n\nError codes returned in the second tuple element (before any items arrive):\n  BRIDGE_SCHEMA_ARG  — argument validation failed\n  BRIDGE_CLOSED      — bridge already closed or stdin write failed\n  BRIDGE_TAINTED     — bridge is unusable after a prior fatal error\n  BRIDGE_ERROR       — marshal failed\n\nError code delivered as a final channel item:\n  BRIDGE_TIMEOUT     — idle or total timeout fired before the stream ended naturally\n\nExample:\n  ch, err = bridgeStream(b, \"generate\", [prompt], {\"idle\": 30, \"total\": 300, \"window\": 16})\n  if err != null { println(err.message)  return }\n  for item in ch {\n      if type(item) == \"ERROR\" {\n          if item.code == \"BRIDGE_TIMEOUT\" { println(\"stalled: \" + item.message) }\n          else                              { println(\"stream failed: \" + item.message) }\n          break\n      }\n      println(item)\n  }",
		Params:        []string{"bridge", "fn", "args", "timeout"},
	},
	"bridgePool": {
		Signature:     "bridgePool(n: int, transport: hash, opts?: hash) -> (pool, error)",
		Documentation: "Start `n` identical bridges from a transport hash and return them as a round-robin pool. Use this when you want to fan out the same workload across multiple workers without manually spawning and tracking each one.\n\nThe transport hash takes the same shape as bridgeOpen() — kind, cmd, args, plus optional timeout_seconds / max_response_mb / stderr_log. Strict key validation; only kind=\"subprocess\" is implemented today.\n\nOptional opts hash holds pool-only configuration:\n  init  — callable run once on each newly-started bridge. Use for per-bridge setup such as loading rules, opening a connection, or warming a cache. Receives the bridge as its single argument. Returning an Error (or a (result, err) tuple whose err is non-null, the bridgeCall shape) marks that bridge dead in the pool; callers can inspect bridgePoolHealth() to find out.\n\nOn partial spawn failure (e.g. cmd not found halfway through) every bridge already started is closed before the error is returned — no orphaned subprocesses.\n\nExample:\n  pool, err = bridgePool(16, {\n      \"kind\": \"subprocess\",\n      \"cmd\":  \"python3\",\n      \"args\": [\"yara_bridge.py\"],\n      \"timeout_seconds\": 30\n  }, {\n      \"init\": fn(b) {\n          _, e = bridgeCall(b, \"load\", [\"secrets.yar\"])\n          return e\n      }\n  })\n  if err != null { println(err.message)  return }",
		Params:        []string{"n", "transport", "opts"},
	},
	"bridgePoolCall": {
		Signature:     "bridgePoolCall(pool, fn: string, args: array, timeoutSec?: number) -> (result, error)",
		Documentation: "Pick the next alive bridge in the pool (round-robin) and forward the call to bridgeCall(). Returns BRIDGE_POOL_EMPTY when every member is dead; otherwise behaves identically to bridgeCall on the picked bridge — same timeouts, schema validation, error codes.\n\nRouting is round-robin via an atomic counter. The pool serialises calls within each bridge (today's contract) but parallelises across them — N concurrent async tasks hit N different bridges with no contention.\n\nExample:\n  result, err = bridgePoolCall(pool, \"scan_batch\", [files])",
		Params:        []string{"pool", "fn", "args", "timeoutSec"},
	},
	"bridgePoolStream": {
		Signature:     "bridgePoolStream(pool, fn: string, args: array, timeout?: number | hash) -> (channel, error)",
		Documentation: "Pick the next alive bridge in the pool (round-robin) and forward to bridgeStream(). Same cancellation and timeout semantics as bridgeStream — breaking out of the for-in stops the picked bridge from producing more items; an optional 4th argument specifies idle and/or total stream timeouts (see bridgeStream for the timeout shape).\n\nExample:\n  ch, err = bridgePoolStream(pool, \"scan_batch_stream\", [files], {\"idle\": 30})\n  for item in ch {\n      if type(item) == \"ERROR\" && item.code == \"BRIDGE_TIMEOUT\" { break }\n      if item.kind == \"finding\" { ... }\n  }",
		Params:        []string{"pool", "fn", "args", "timeout"},
	},
	"bridgePoolClose": {
		Signature:     "bridgePoolClose(pool) -> null",
		Documentation: "Close every bridge in the pool. Idempotent — calling twice is a no-op. After close, further bridgePoolCall / bridgePoolStream return BRIDGE_CLOSED.\n\nDefer this immediately after a successful bridgePool() to guarantee cleanup on every exit path.",
		Params:        []string{"pool"},
	},
	"bridgePoolHealth": {
		Signature:     "bridgePoolHealth(pool) -> hash",
		Documentation: "Return a snapshot of pool liveness: {size: N, alive: A, dead: D}. Useful for the diagnostic panel in long-running apps that need to surface worker failures.\n\nA member counts as dead when either:\n  - its init callable returned an error during bridgePool(), or\n  - the bridge subprocess crashed or was tainted mid-session.\n\nDead membership is latched — once observed, a slot stays dead for the lifetime of the pool. The round-robin picker (bridgePoolCall / bridgePoolStream) automatically routes around dead slots, so callers never see BRIDGE_TAINTED from a tainted pool member.\n\nExample:\n  h = bridgePoolHealth(pool)\n  println(str(h[\"alive\"]) + \" / \" + str(h[\"size\"]) + \" workers alive\")",
		Params:        []string{"pool"},
	},
	"bridgePoolStderr": {
		Signature:     "bridgePoolStderr(pool) -> array",
		Documentation: "Concatenated stderr tail across every pool member, with each line prefixed \"[N] \" where N is the member index. Otherwise behaves like bridgeStderr(bridge) — same ring-buffer source.\n\nExample:\n  for line in bridgePoolStderr(pool) { println(line) }",
		Params:        []string{"pool"},
	},

	// Database
	"dbOpen": {
		Signature:     "dbOpen(driver: string, dsn: string) -> (conn, err)",
		Documentation: "Open a database connection pool and verify connectivity with a ping.\n\nSupported drivers:\n  \"mssql\"      — Microsoft SQL Server (also accepts \"sqlserver\")\n  \"postgres\"   — PostgreSQL via pgx\n\nMS SQL connection string:\n  \"server=host;database=mydb;user id=sa;password=Pass1!\"\n  \"sqlserver://sa:Pass1!@host:1433?database=mydb\"\n\nPostgres connection string:\n  \"host=host user=user password=pass dbname=mydb sslmode=disable\"\n  \"postgres://user:pass@host:5432/mydb\"\n\nExample:\n  conn, err = dbOpen(\"mssql\", \"server=myserver;database=Sales;user id=sa;password=Pass1!\")\n  if err != null { println(err.message)  return }",
		Params:        []string{"driver", "dsn"},
	},
	"dbOpenWithPool": {
		Signature:     "dbOpenWithPool(driver: string, dsn: string, options?: hash) -> (conn, err)",
		Documentation: "Like dbOpen but with explicit connection pool settings. The options hash is itself optional — call with just (driver, dsn) to get the same defaults as dbOpen.\n\noptions keys (all optional):\n  \"maxIdle\"     — max idle connections (default 2)\n  \"maxOpen\"     — max open connections; 0 = unlimited (default 0)\n  \"idleTimeout\" — seconds before an idle conn is closed\n  \"lifetime\"    — max seconds a conn may be reused\n\nExample:\n  conn, err = dbOpenWithPool(\"mssql\", \"server=...\", {\n      \"maxIdle\": 5, \"maxOpen\": 20, \"idleTimeout\": 300, \"lifetime\": 3600\n  })\n  if err != null { println(err.message)  return }",
		Params:        []string{"driver", "dsn", "options?"},
	},
	"dbClose": {
		Signature:     "dbClose(conn: DB_CONN) -> null",
		Documentation: "Close the connection pool. Call when done with the connection.",
		Params:        []string{"conn"},
	},
	"dbPing": {
		Signature:     "dbPing(conn: DB_CONN) -> (null, err)",
		Documentation: "Verify the connection is still alive. Useful for health checks.",
		Params:        []string{"conn"},
	},
	"dbQuery": {
		Signature:     "dbQuery(conn: DB_CONN | DB_TX, sql: string, args?: array) -> (rows, err)",
		Documentation: "Execute a SELECT and return all rows as an array of hashes. Column names are hash keys. SQL NULLs become kLex null. Works on connections and transactions.\n\nAlways pass parameters as an array — never interpolate values into the SQL string.\n\nExample:\n  rows, err = dbQuery(conn, \"SELECT id, name FROM users WHERE active = ?\", [true])\n  if err != null { println(err.message)  return }\n  for row in rows {\n      println(\"{row[\\\"id\\\"]}  {row[\\\"name\\\"]}\")\n  }",
		Params:        []string{"conn", "sql", "args"},
	},
	"dbQueryStream": {
		Signature:     "dbQueryStream(conn: DB_CONN | DB_TX, sql: string, args?: array) -> (channel, err)",
		Documentation: "Execute a SELECT and return a channel that yields rows one at a time. Each value is a hash (same format as dbQuery). Suitable for large result sets. Works on connections and transactions. Break out of the for-in loop to cancel early.\n\nExample:\n  stream, err = dbQueryStream(conn, \"SELECT id, name FROM big_table\", [])\n  if err != null { println(err.message)  return }\n  for row in stream {\n      println(\"{row[\\\"id\\\"]}  {row[\\\"name\\\"]}\")\n  }",
		Params:        []string{"conn", "sql", "args"},
	},
	"dbQueryOne": {
		Signature:     "dbQueryOne(conn: DB_CONN | DB_TX, sql: string, args?: array) -> (row, err)",
		Documentation: "Execute a SELECT and return the first row as a hash, or null if no rows match. Use for primary-key lookups. Works on connections and transactions.\n\nExample:\n  row, err = dbQueryOne(conn, \"SELECT * FROM users WHERE id = ?\", [42])\n  if err != null { return }\n  if row == null { println(\"not found\")  return }\n  println(row[\"name\"])",
		Params:        []string{"conn", "sql", "args"},
	},
	"dbBulkInsert": {
		Signature:     "dbBulkInsert(conn: DB_CONN | DB_TX, table: string, columns: array, rows: array) -> (n: int, err)",
		Documentation: "Insert multiple rows in a single SQL statement — one round trip per batch, far faster than dbExec in a loop.\n\ncolumns is an array of column name strings.\nrows is an array of arrays, each sub-array is one row's values.\nAuto-batches to stay within driver parameter limits (MSSQL: 2000, Postgres: 60000).\nReturns total rows affected across all batches.\n\nWARNING: table and column names are interpolated directly into SQL. Never pass user-controlled input as table or column names.\n\nExample:\n  n, err = dbBulkInsert(conn, \"users\", [\"id\", \"name\", \"age\"], [\n      [1, \"Alice\", 28],\n      [2, \"Bob\",   35],\n  ])\n  if err != null { println(err.message)  return }\n  println(\"{n} rows inserted\")",
		Params:        []string{"conn", "table", "columns", "rows"},
	},
	"dbExec": {
		Signature:     "dbExec(conn: DB_CONN | DB_TX, sql: string, args?: array) -> (rowsAffected, err)",
		Documentation: "Execute an INSERT, UPDATE, DELETE, or DDL statement. Returns the number of rows affected (-1 if unavailable). Works on connections and transactions.\n\nExample:\n  n, err = dbExec(conn, \"UPDATE accounts SET balance = ? WHERE id = ?\", [1500, 42])\n  if err != null { println(err.message)  return }\n  println(\"{n} row(s) updated\")",
		Params:        []string{"conn", "sql", "args"},
	},
	"dbSetTimeout": {
		Signature:     "dbSetTimeout(conn: DB_CONN | DB_TX, ms: int) -> null",
		Documentation: "Set a per-operation timeout on a connection or transaction. All subsequent dbQuery, dbQueryOne, dbExec, dbBulkInsert calls will fail with DB_TIMEOUT_ERROR if they exceed ms milliseconds. Pass 0 to remove the timeout.\n\ndbBegin propagates the conn's timeout to the new transaction automatically.\n\nExample:\n  dbSetTimeout(conn, 5000)  // 5 second limit\n  rows, err = dbQuery(conn, \"SELECT * FROM large_table\", [])\n  // → DB_TIMEOUT_ERROR if query takes more than 5s\n  dbSetTimeout(conn, 0)     // remove timeout",
		Params:        []string{"conn", "ms"},
	},
	"dbExecReturning": {
		Signature:     "dbExecReturning(conn: DB_CONN | DB_TX, sql: string, args?: array) -> (rows: array, err)",
		Documentation: "Execute a DML statement that returns rows — for INSERT/UPDATE/DELETE with RETURNING (PostgreSQL) or OUTPUT (SQL Server) clauses. Returns an array of hashes, same format as dbQuery. Works on connections and transactions.\n\nPostgres example:\n  rows, err = dbExecReturning(conn, \"INSERT INTO users (name) VALUES (?) RETURNING id, name\", [\"Alice\"])\n  id = rows[0][\"id\"]\n\nSQL Server example:\n  rows, err = dbExecReturning(conn, \"INSERT INTO users (name) OUTPUT INSERTED.id, INSERTED.name VALUES (?)\", [\"Alice\"])\n  id = rows[0][\"id\"]",
		Params:        []string{"conn", "sql", "args"},
	},
	"dbBegin": {
		Signature:     "dbBegin(conn: DB_CONN) -> (tx, err)",
		Documentation: "Start a database transaction. Pass the returned tx to dbQuery, dbExec, dbCommit, and dbRollback.\n\nExample:\n  tx, err = dbBegin(conn)\n  if err != null { return }\n  _, err = dbExec(tx, \"INSERT INTO log VALUES (?, ?)\", [42, \"updated\"])\n  if err != null { dbRollback(tx)  return }\n  dbCommit(tx)",
		Params:        []string{"conn"},
	},
	"dbCommit": {
		Signature:     "dbCommit(tx: DB_TX) -> (null, err)",
		Documentation: "Commit a transaction started with dbBegin.",
		Params:        []string{"tx"},
	},
	"dbRollback": {
		Signature:     "dbRollback(tx: DB_TX) -> null",
		Documentation: "Roll back a transaction started with dbBegin. Safe to call even if the transaction is already finished.",
		Params:        []string{"tx"},
	},

	// Math
	"floor": {
		Signature:     "floor(n: int | float) -> int",
		Documentation: "floor returns the greatest integer less than or equal to x.\nAccepts integer or float. Always returns Integer.\nfloor(3.7) → 3    floor(-2.3) → -3    floor(4) → 4",
		Params:        []string{"n"},
	},
	"ceil": {
		Signature:     "ceil(n: int | float) -> int",
		Documentation: "ceil returns the smallest integer greater than or equal to x.\nAccepts integer or float. Always returns Integer.\nceil(3.2) → 4    ceil(-2.7) → -2    ceil(4) → 4",
		Params:        []string{"n"},
	},
	"round": {
		Signature:     "round(n: int | float) -> int",
		Documentation: "round returns the nearest integer to x, rounding half away from zero.\nAccepts integer or float. Always returns Integer.\nround(3.5) → 4    round(3.4) → 3    round(-2.5) → -3",
		Params:        []string{"n"},
	},
	"sqrt": {
		Signature:     "sqrt(n: int | float) -> float",
		Documentation: "sqrt returns the square root of x. Always returns Float.\nAccepts integer or float. x must be non-negative.\nsqrt(4) → 2.0    sqrt(2.0) → 1.4142135623730951",
		Params:        []string{"n"},
	},
	"sin": {
		Signature:     "sin(n: int | float) -> float",
		Documentation: "Sine (radians).",
		Params:        []string{"n"},
	},
	"cos": {
		Signature:     "cos(n: int | float) -> float",
		Documentation: "Cosine (radians).",
		Params:        []string{"n"},
	},
	"tan": {
		Signature:     "tan(n: int | float) -> float",
		Documentation: "Tangent (radians).",
		Params:        []string{"n"},
	},
	"min": {
		Signature:     "min(a: int | float, b: int | float) -> int | float",
		Documentation: "min returns the smaller of two values. Accepts integers and floats.\nMixed types are compared as floats; the original value (and its type) is returned.\nmin(3, 7) → 3    min(1.5, 2.5) → 1.5    min(3, 2.5) → 2.5",
		Params:        []string{"a", "b"},
	},
	"max": {
		Signature:     "max(a: int | float, b: int | float) -> int | float",
		Documentation: "max returns the larger of two values. Accepts integers and floats.\nMixed types are compared as floats; the original value (and its type) is returned.\nmax(3, 7) → 7    max(1.5, 2.5) → 2.5    max(3, 2.5) → 3",
		Params:        []string{"a", "b"},
	},
	"abs": {
		Signature:     "abs(n: int | float) -> int | float",
		Documentation: "Return the absolute value of n. Returns the same type as input.",
		Params:        []string{"n"},
	},
	"pow": {
		Signature:     "pow(base: int | float, exp: int | float) -> float",
		Documentation: "Return base raised to the power exp. Always returns float.",
		Params:        []string{"base", "exp"},
	},
	"log": {
		Signature:     "log(n: int | float) -> float",
		Documentation: "log returns the natural logarithm of x. Always returns Float.\nx must be positive.\nlog(1) → 0.0    log(2.718281828) ≈ 1.0",
		Params:        []string{"n"},
	},
	"log2": {
		Signature:     "log2(n: int | float) -> float",
		Documentation: "Base-2 logarithm of n. n must be positive.",
		Params:        []string{"n"},
	},
	"log10": {
		Signature:     "log10(n: int | float) -> float",
		Documentation: "Base-10 logarithm of n. n must be positive.",
		Params:        []string{"n"},
	},
	"exp": {
		Signature:     "exp(n: int | float) -> float",
		Documentation: "Return e raised to the power n. Always returns float.",
		Params:        []string{"n"},
	},
	"asin": {
		Signature:     "asin(n: int | float) -> float",
		Documentation: "Arc sine of n in radians. n must be in [-1, 1].",
		Params:        []string{"n"},
	},
	"acos": {
		Signature:     "acos(n: int | float) -> float",
		Documentation: "acos returns the arc cosine of x in radians. Always returns Float.\nx must be in [-1, 1].\nacos(1) → 0.0    acos(0) → 1.5707963267948966 (π/2)",
		Params:        []string{"n"},
	},
	"atan": {
		Signature:     "atan(n: int | float) -> float",
		Documentation: "Arc tangent of n in radians.",
		Params:        []string{"n"},
	},
	"atan2": {
		Signature:     "atan2(y: int | float, x: int | float) -> float",
		Documentation: "Arc tangent of y/x in radians, using sign of both arguments to determine the correct quadrant.",
		Params:        []string{"y", "x"},
	},
	"pi": {
		Signature:     "pi() -> float",
		Documentation: "Returns the mathematical constant π (3.141592653589793).",
		Params:        []string{},
	},
	"e": {
		Signature:     "e() -> float",
		Documentation: "Returns Euler's number (2.718281828459045).",
		Params:        []string{},
	},
	"mod": {
		Signature:     "mod(a: int, b: int) -> int",
		Documentation: "Integer remainder of a divided by b. Both arguments must be integers.",
		Params:        []string{"a", "b"},
	},
	"fmod": {
		Signature:     "fmod(a: float, b: float) -> float",
		Documentation: "Floating-point remainder of a divided by b. Both arguments must be floats.",
		Params:        []string{"a", "b"},
	},
	// Bitwise
	"bitAnd": {
		Signature:     "bitAnd(a: int, b: int) -> int",
		Documentation: "Bitwise AND of two integers. Both operands must be integer.\n\nExample:\n  bitAnd(0b1100, 0b1010) → 8   // 0b1000\n  bitAnd(0xFF, 0x0F)     → 15  // 0x0F",
		Params:        []string{"a", "b"},
	},
	"bitOr": {
		Signature:     "bitOr(a: int, b: int) -> int",
		Documentation: "Bitwise OR of two integers. Both operands must be integer.\n\nExample:\n  bitOr(0b1100, 0b0011) → 15  // 0b1111",
		Params:        []string{"a", "b"},
	},
	"bitXor": {
		Signature:     "bitXor(a: int, b: int) -> int",
		Documentation: "Bitwise XOR of two integers. Both operands must be integer.\n\nExample:\n  bitXor(0b1100, 0b1010) → 6  // 0b0110",
		Params:        []string{"a", "b"},
	},
	"bitNot": {
		Signature:     "bitNot(x: int) -> int",
		Documentation: "Bitwise NOT (ones' complement) of an integer.\n\nExample:\n  bitNot(0)  → -1\n  bitNot(-1) → 0",
		Params:        []string{"x"},
	},
	"bitShiftLeft": {
		Signature:     "bitShiftLeft(x: int, n: int) -> int",
		Documentation: "Shift x left by n bits. n must be non-negative. Equivalent to x * 2^n for non-negative x.\n\nExample:\n  bitShiftLeft(1, 4)  → 16\n  bitShiftLeft(3, 8)  → 768",
		Params:        []string{"x", "n"},
	},
	"bitShiftRight": {
		Signature:     "bitShiftRight(x: int, n: int) -> int",
		Documentation: "Arithmetic right shift of x by n bits. n must be non-negative. Sign bit is preserved.\n\nExample:\n  bitShiftRight(16, 4)  → 1\n  bitShiftRight(256, 3) → 32",
		Params:        []string{"x", "n"},
	},

	"remap": {
		Signature:     "remap(val, inLow, inHigh, outLow, outHigh: float) -> float",
		Documentation: "Re-map val from input range [inLow, inHigh] to output range [outLow, outHigh]. Not clamped — use constrain() afterward if needed. Named remap to avoid collision with the higher-order map(arr, fn).\n\nExample:\n  // Mouse X → red intensity\n  r = remap(mouseX(), 0, winWidth(), 0.0, 1.0)\n  fill(r, 0.3, 0.6, 1.0)\n\n  // Data value → bar height\n  h = remap(value, 0, maxVal, 0, barMaxH)",
		Params:        []string{"val", "inLow", "inHigh", "outLow", "outHigh"},
	},
	"constrain": {
		Signature:     "constrain(val, lo, hi: float) -> float | int",
		Documentation: "Clamp val to the range [lo, hi]. Returns lo if val < lo, hi if val > hi, otherwise val unchanged. Returns integer if val is integer, float otherwise.\n\nExample:\n  x = constrain(x + dx, 0, winWidth())\n  opacity = constrain(opacity - 0.02, 0.0, 1.0)",
		Params:        []string{"val", "lo", "hi"},
	},
	"lerp": {
		Signature:     "lerp(a, b, t: float) -> float",
		Documentation: "Linear interpolation: returns a + (b-a)*t. Returns a at t=0, b at t=1. Not clamped — values outside [0,1] extrapolate.\n\nExample:\n  // Smooth camera follow (10% per frame)\n  camX = lerp(camX, targetX, 0.1)\n\n  // Animate opacity\n  alpha = lerp(0.0, 1.0, progress)",
		Params:        []string{"a", "b", "t"},
	},
	"hsl": {
		Signature:     "hsl(h, s, l: float) -> array\nhsl(h, s, l, a: float) -> array",
		Documentation: "Convert HSL colour to a [r, g, b, a] float array compatible with fill(), gradient(), and theme slots. All values in [0.0, 1.0]. Alpha defaults to 1.0.\n\nh=0/1.0=red, h=0.33=green, h=0.67=blue. s=0 is greyscale. l=0=black, l=0.5=pure colour, l=1=white.\n\nExample:\n  // Cycle through rainbow\n  t = elapsedTime() * 0.1\n  c = hsl(fmod(t, 1.0), 0.8, 0.5)\n  fill(c[0], c[1], c[2], c[3])\n\n  // Generate gradient from theme hue\n  top = hsl(0.6, 0.5, 0.20)\n  bot = hsl(0.6, 0.5, 0.12)\n  gradient(0, 0, w, h, top, bot, \"v\")",
		Params:        []string{"h", "s", "l", "a"},
	},

	// Random
	"rand": {
		Signature:     "rand() -> float",
		Documentation: "rand returns a random float in [0.0, 1.0).\nThe global source is automatically seeded — no setup required.\nUsage: rand()  →  0.7341...",
		Params:        []string{},
	},
	"randInt": {
		Signature:     "randInt(min: int, max: int) -> int",
		Documentation: "randInt returns a random integer in the closed range [min, max].\nBoth endpoints are inclusive: randInt(1, 6) simulates a die roll.\nmin must be <= max.\nUsage: randInt(1, 10)  →  7",
		Params:        []string{"min", "max"},
	},
	"shuffle": {
		Signature:     "shuffle(arr: array) -> array",
		Documentation: "shuffle returns a new array with the elements in random order.\nThe original array is not mutated (consistent with push, pop, concat).\nUsage: shuffle([1, 2, 3, 4, 5])  →  [3, 1, 5, 2, 4]",
		Params:        []string{"arr"},
	},

	// Sort
	"sort": {
		Signature:     "sort(arr: array) -> array",
		Documentation: "sort returns a new array sorted in ascending order.\nWorks for arrays of integers, floats, or strings (not mixed element types).\nUses a stable sort — equal elements keep their original relative order.\nUsage: sort([3, 1, 4, 1, 5]) → [1, 1, 3, 4, 5]",
		Params:        []string{"arr"},
	},
	"sortBy": {
		Signature:     "sortBy(arr: array, fn: function) -> array",
		Documentation: "sortBy returns a new array sorted using a comparator function.\ncompareFn(a, b) must return true when a should appear before b.\nUses a stable sort — equal elements keep their original relative order.\nUsage: sortBy(people, fn(a, b) { return a[\"age\"] < b[\"age\"] })",
		Params:        []string{"arr", "fn"},
	},

	// Time
	"sleep": {
		Signature:     "sleep(ms: int) -> null",
		Documentation: "Sleep for the given milliseconds.",
		Params:        []string{"ms"},
	},

	// Input
	"input": {
		Signature:     "input(prompt?: string) -> string",
		Documentation: "input prints an optional prompt and reads one line from stdin.\nThe trailing newline is stripped. Returns the line as a string.",
		Params:        []string{"prompt"},
	},

	// Format
	"format": {
		Signature:     "format(fmt: string, ...args: any) -> string",
		Documentation: "printf-style string formatting.\n\nFormat verbs:\n  %d         integer decimal\n  %f %e %g   float (decimal / scientific / shortest); accepts INTEGER too\n  %s         string (strict — use %v for any type)\n  %t         boolean\n  %v         any value (calls Inspect())\n  %x %X      integer hex (lower / upper)\n  %o         integer octal\n  %b         integer binary\n  %%         literal percent sign\n\nWidth, precision, and flags (-+0 space) follow standard printf conventions.\nToo few or too many arguments is a RuntimeError.\nWrong type for a verb is a RuntimeError.",
		Params:        []string{"fmt", "...args"},
	},

	// Version
	"__version__": {
		Signature:     "__version__() -> string",
		Documentation: "Get the kLex version.",
		Params:        []string{},
	},

	// Network
	"tcpDial": {
		Signature:     "tcpDial(host: string, port: int) -> (conn, error)",
		Documentation: "Open a TCP connection to host:port. Returns (connection, null) on success or (null, error) on failure.",
		Params:        []string{"host", "port"},
	},
	"tcpListen": {
		Signature:     "tcpListen(host: string, port: int) -> channel",
		Documentation: "Start a TCP server on host:port. Returns a channel of accepted connections. Break from for-in loop to stop listening.",
		Params:        []string{"host", "port"},
	},
	"netRead": {
		Signature:     "netRead(conn: connection, maxBytes: int) -> (string, error)",
		Documentation: "Read up to maxBytes from a connection. Returns (data, null) on success or (null, error) on failure.",
		Params:        []string{"conn", "maxBytes"},
	},
	"netWrite": {
		Signature:     "netWrite(conn: connection, data: string) -> (null, error)",
		Documentation: "Write data to a connection. Returns (null, null) on success or (null, error) on failure.",
		Params:        []string{"conn", "data"},
	},
	"netClose": {
		Signature:     "netClose(conn: connection) -> null",
		Documentation: "Close a network connection.",
		Params:        []string{"conn"},
	},
	"dnsLookup": {
		Signature:     "dnsLookup(hostname: string) -> (addresses: array, error)",
		Documentation: "Resolve a hostname to an array of IP addresses. Returns (ips, null) on success or (null, error) on failure.",
		Params:        []string{"hostname"},
	},
	// Undocumented builtins (added for LSP hover support)
	"_base64Decode": {
		Signature:     "_base64Decode(s) -> (decoded_string, err)",
		Documentation: "standard base64",
		Params:        []string{"s"},
	},
	"_base64Encode": {
		Signature:     "_base64Encode(s) -> string",
		Documentation: "standard base64 with padding",
		Params:        []string{"s"},
	},
	"_aesDecrypt": {Signature: "_aesDecrypt(ciphertext_hex, key) -> (string, error)", Documentation: "Decrypt AES-256-GCM ciphertext produced by _aesEncrypt. Returns (plaintext, null) on success or (null, error) if key is wrong or data was tampered with.", Params: []string{"ciphertext_hex", "key"}},
	"_aesEncrypt": {Signature: "_aesEncrypt(plaintext, key) -> (string, error)", Documentation: "Encrypt with AES-256-GCM (authenticated encryption). Key shorter than 32 bytes is PBKDF2-SHA256 derived. Returns (ciphertext_hex, null) on success — each call produces a unique ciphertext via random nonce.", Params: []string{"plaintext", "key"}},
	"_base64UrlDecode": {
		Signature:     "_base64UrlDecode(s) -> (decoded_string, err)",
		Documentation: "URL-safe base64, no padding",
		Params:        []string{"s"},
	},
	"_base64UrlEncode": {
		Signature:     "_base64UrlEncode(s) -> string",
		Documentation: "URL-safe base64, no padding",
		Params:        []string{"s"},
	},
	"_bcryptHash": {
		Signature:     "_bcryptHash(password) -> (hash, err)",
		Documentation: "Generates bcrypt hash of password (cost=12, default)",
		Params:        []string{"password"},
	},
	"_bcryptVerify": {
		Signature:     "_bcryptVerify(password, hash) -> (matches, err)",
		Documentation: "Verifies password against bcrypt hash\nReturns (true, null) if password matches\nReturns (false, null) if password doesn't match\nReturns (false, error) on error",
		Params:        []string{"password", "hash"},
	},
	"_constantTimeEquals": {
		Signature:     "_constantTimeEquals(a, b) -> boolean",
		Documentation: "Compares two strings in constant time (resistant to timing attacks)\nUse this for comparing passwords, tokens, signatures",
		Params:        []string{"a", "b"},
	},
	"_csvFirstRowCols": {
		Signature:     "_csvFirstRowCols(data, delim) -> (column_count, error)",
		Documentation: "Optimized: Parses only first row and returns column count (early exit)\nUsed by columnCount() to avoid parsing entire CSV",
		Params:        []string{"data", "delim"},
	},
	"_csvFormat": {
		Signature:     "_csvFormat(rows) -> (string, error)",
		Documentation: "Formats array of arrays to CSV (comma-delimited)",
		Params:        []string{"rows"},
	},
	"_csvFormatDelim": {
		Signature:     "_csvFormatDelim(rows, delim) -> (string, error)",
		Documentation: "Formats array of arrays with custom delimiter",
		Params:        []string{"rows", "delim"},
	},
	"_csvHasRows": {
		Signature:     "_csvHasRows(data, delim) -> (bool, error)",
		Documentation: "Optimized: Checks if CSV has at least one row (early exit after first read)\nUsed by isEmpty() to avoid parsing entire CSV",
		Params:        []string{"data", "delim"},
	},
	"_csvParse": {
		Signature:     "_csvParse(data) -> (array_of_arrays, error)",
		Documentation: "Parses CSV with comma delimiter (RFC 4180)",
		Params:        []string{"data"},
	},
	"_csvParseDelim": {
		Signature:     "_csvParseDelim(data, delim) -> (array_of_arrays, error)",
		Documentation: "Parses CSV with custom delimiter (first rune of delim string)",
		Params:        []string{"data", "delim"},
	},
	"_csvParseHeaders": {
		Signature:     "_csvParseHeaders(data) -> (array_of_hashes, error)",
		Documentation: "Parses CSV treating first row as column headers\nReturns array of hashes where each hash maps column name to value\nUses lenient parsing (FieldsPerRecord=-1) to handle ragged rows",
		Params:        []string{"data"},
	},
	"_csvStream": {
		Signature:     "_csvStream(data, delim) -> Channel",
		Documentation: "Streams CSV rows as they are parsed. Each element sent to channel is an Array of Strings.\nChannel closes automatically when parsing completes or on error.\nAllows overlapping parsing and processing for better parallelization.\nOptimized: Large buffer + async parser prevent lock contention with multiple workers.",
		Params:        []string{"data", "delim"},
	},
	"_deflateCompress": {
		Signature:     "_deflateCompress(data) -> (compressed, err)",
		Documentation: "Compresses a string using deflate (no gzip header)",
		Params:        []string{"data"},
	},
	"_deflateDecompress": {
		Signature:     "_deflateDecompress(data) -> (decompressed, err)",
		Documentation: "Decompresses deflate-compressed data",
		Params:        []string{"data"},
	},
	"_fsAppend":  {Signature: "_fsAppend(path, content) -> (null, error)", Documentation: "Append to file.", Params: []string{"path", "content"}},
	"_fsChmod":   {Signature: "_fsChmod(path, mode) -> (null, error)", Documentation: "Change file permissions.", Params: []string{"path", "mode"}},
	"_fsCopy":    {Signature: "_fsCopy(src, dst) -> (null, error)", Documentation: "Copy file.", Params: []string{"src", "dst"}},
	"_fsExists":  {Signature: "_fsExists(path) -> bool", Documentation: "Check if file exists.", Params: []string{"path"}},
	"_fsListDir": {Signature: "_fsListDir(path) -> (array, error)", Documentation: "List directory names.", Params: []string{"path"}},
	"_fsLstat":   {Signature: "_fsLstat(path) -> (hash, error)", Documentation: "Get file metadata (no symlink follow).", Params: []string{"path"}},
	"_fsMap": {
		Signature:     "_fsMap(path) -> (content, err)",
		Documentation: "Memory-maps a file and returns its content as a string.\nThe string is backed by the mmap'd region—no copying.\nPerfect for analyzing large files in parallel (each worker accesses different ranges).",
		Params:        []string{"path"},
	},
	"_fsMkdir":    {Signature: "_fsMkdir(path) -> (null, error)", Documentation: "Create directory.", Params: []string{"path"}},
	"_fsMkdirAll": {Signature: "_fsMkdirAll(path) -> (null, error)", Documentation: "Create directory tree.", Params: []string{"path"}},
	"_fsRead":     {Signature: "_fsRead(path) -> (string, error)", Documentation: "Read entire file.", Params: []string{"path"}},
	"_fsReadChunk": {
		Signature:     "_fsReadChunk(path, offset, byteCount) -> (content, isEOF, err)",
		Documentation: "Reads up to byteCount bytes from file starting at offset.\nReturns tuple: (content_string, isEOF_bool, error_or_null)\nisEOF is true if the returned chunk reaches the end of the file.\nUseful for streaming large files without loading entirely into memory.",
		Params:        []string{"path", "offset", "byteCount"},
	},
	"_fsReadDir": {
		Signature:     "_fsReadDir(path) -> (array_of_info_hashes, err)",
		Documentation: "Each hash has the same keys as _fsStat. Entries whose Info() fails are skipped.",
		Params:        []string{"path"},
	},
	"_fsReadlink":  {Signature: "_fsReadlink(path) -> (string, error)", Documentation: "Read symlink target.", Params: []string{"path"}},
	"_fsRemove":    {Signature: "_fsRemove(path) -> (null, error)", Documentation: "Remove file.", Params: []string{"path"}},
	"_fsRemoveAll": {Signature: "_fsRemoveAll(path) -> (null, error)", Documentation: "Remove recursively.", Params: []string{"path"}},
	"_fsRename":    {Signature: "_fsRename(src, dst) -> (null, error)", Documentation: "Rename file.", Params: []string{"src", "dst"}},
	"_fsStat": {
		Signature:     "_fsStat(path) -> (info_hash, err)",
		Documentation: "info_hash keys: \"name\",\"size\",\"isDir\",\"isSymlink\",\"modTime\",\"mode\"\nStat follows symlinks — isSymlink is always false on the resolved target.",
		Params:        []string{"path"},
	},
	"_fsSymlink": {Signature: "_fsSymlink(target, link) -> (null, error)", Documentation: "Create symlink.", Params: []string{"target", "link"}},
	"_fsTmpDir": {
		Signature:     "_fsTmpDir(dir, pattern) -> (path, err)",
		Documentation: "Creates a new temp directory in dir matching pattern, returns its path.\nPass \"\" for dir to use the system temp directory.",
		Params:        []string{"dir", "pattern"},
	},
	"_fsTmpFile": {
		Signature:     "_fsTmpFile(dir, pattern) -> (path, err)",
		Documentation: "Creates a new temp file in dir matching pattern, closes it, returns its path.\nPass \"\" for dir to use the system temp directory.",
		Params:        []string{"dir", "pattern"},
	},
	"_fsWrite": {Signature: "_fsWrite(path, content) -> (null, error)", Documentation: "Write file.", Params: []string{"path", "content"}},
	"_gzipCompress": {
		Signature:     "_gzipCompress(data) -> (compressed, err)",
		Documentation: "Compresses a string using gzip (default level 6)",
		Params:        []string{"data"},
	},
	"_gzipDecompress": {
		Signature:     "_gzipDecompress(data) -> (decompressed, err)",
		Documentation: "Decompresses gzip-compressed data",
		Params:        []string{"data"},
	},
	"_hmacSha256": {
		Signature:     "_hmacSha256(key, data) -> hex_string",
		Documentation: "Returns HMAC-SHA256 of data with key as hex string",
		Params:        []string{"key", "data"},
	},
	"_hmacSha512": {
		Signature:     "_hmacSha512(key, data) -> hex_string",
		Documentation: "Returns HMAC-SHA512 of data with key as hex string",
		Params:        []string{"key", "data"},
	},
	"_httpDo":        {Signature: "_httpDo(method, url, headers, body [, timeoutSec]) -> (status: int, body: string, headers: hash, err: string?)", Documentation: "Make a one-shot HTTP request and return the full response. method is the HTTP verb; url the full URL; headers is a hash of string→string (or null); body is a string payload (or null).\n\ntimeoutSec is an optional number of seconds overriding the shared 30s client default — required for LLM cold-starts (Ollama, llama.cpp), large batch embeddings, or any endpoint that may legitimately take >30s. Pass null or 0 to keep the default. The override uses request context (no leaked connections; cancels cleanly).\n\nOn failure err is a descriptive string and status=0.", Params: []string{"method", "url", "headers", "body", "timeoutSec"}},
	"cosineSim":      {Signature: "cosineSim(vec1: array, vec2: array) -> float", Documentation: "Compute the cosine similarity of two equal-length numeric arrays. Range: -1.0 (opposite) … 0.0 (orthogonal) … 1.0 (identical direction). Standard ranking metric for sentence/document embeddings; used by stdlib/ai/vector_store.lex for k-NN search. Zero-magnitude vectors return 0. Both arrays must have the same length and contain only numbers (Integer or Float).", Params: []string{"vec1", "vec2"}},
	"dotProduct":     {Signature: "dotProduct(vec1: array, vec2: array) -> float", Documentation: "Inner product of two equal-length numeric arrays. Faster than cosineSim (no normalisation) — use when vectors are already unit-length, or when raw similarity is sufficient.", Params: []string{"vec1", "vec2"}},
	"vecNorm":        {Signature: "vecNorm(vec: array) -> float", Documentation: "L2 (Euclidean) norm of a numeric array — the geometric length of the vector. Use to normalise: `unit = v / vecNorm(v)` (after which dotProduct equals cosineSim).", Params: []string{"vec"}},
	"_httpStream":    {Signature: "_httpStream(method, url, headers, body [, mode]) -> (channel, err: string?)", Documentation: "Open an HTTP request and return a kLex channel that yields parsed streaming frames as hashes: {\"event\": <event-name>, \"data\": <raw-data-payload>}.\n\nmode selects the wire format (default \"sse\"):\n  \"sse\"   — Server-Sent Events. Each frame is one blank-line-delimited group of event:/data: lines (Anthropic, OpenAI, etc.). Default Accept: text/event-stream.\n  \"lines\" — Newline-delimited JSON (NDJSON / JSON Lines). Each non-empty response line becomes one frame with event=\"\" and data = the raw line text (Ollama, llama.cpp server, etc.). Default Accept: application/x-ndjson.\n\nCancellation: breaking out of a `for evt in ch { ... }` loop closes the response body and stops the request — no leaked connections.\n\nOn connection-level failure returns (null, error_string). Mid-stream errors are delivered as a final frame {\"event\": \"error\", \"data\": <message>} before channel close.\n\nBacks streaming chat completions in stdlib/ai/anthropic.lex (sse mode) and stdlib/ai/ollama.lex (lines mode). Accept header is set automatically based on mode if not supplied. No request timeout — rely on consumer cancellation.", Params: []string{"method", "url", "headers", "body", "mode"}},
	"_httpServe":     {Signature: "_httpServe() -> any", Documentation: "Start HTTP server.", Params: []string{}},
	"_fsCountLines":  {Signature: "_fsCountLines(path: string) -> (int, error)", Documentation: "Stream the file at `path` and return its newline count, matching `wc -l` semantics (a file ending without a trailing newline still has its last line counted). Empty file → 0 lines. Missing file → (0, error) — does NOT runtime-crash, so callers can treat the absence as \"no data yet\".\n\nStreaming-based (1 MB buffer + raw byte scan), so it handles multi-GB JSONL files without holding the content in memory.\n\nUsed by frogLight's openStoreLite to detect library.f32 / library.json row-count desync and trigger crash-recovery truncation (OFI #15).", Params: []string{"path"}},
	"_jsonParse":     {Signature: "_jsonParse(s: string) -> (value, errStr: string?)", Documentation: "Go-side JSON decoder backing stdlib/json.lex's `parse`. Returns (value, null) on success or (null, error_string) on parse failure. Numbers are decoded as integer when the literal has no '.' or 'e'/'E' AND fits in int64; otherwise float. Use the stdlib wrapper unless you specifically need to avoid the module import.\n\nReplaces the historical kLex-interpreted parser; ~100× faster on large documents.", Params: []string{"s"}},
	"_jsonStringify": {Signature: "_jsonStringify(v: any) -> (string, errStr: string?)", Documentation: "Go-side JSON encoder backing stdlib/json.lex's `stringify`. Hash keys are emitted in sorted order so output is deterministic. Bytes / NaN / Inf cannot be encoded and surface as the error string (rather than a runtime error) — call this directly when you want the safe (string, err) tuple; the stdlib wrapper turns the err into a runtime error.", Params: []string{"v"}},
	"_fsTruncate":    {Signature: "_fsTruncate(path: string, newSize: int) -> error", Documentation: "Truncate `path` to exactly `newSize` bytes (wraps POSIX truncate(2)). Growing is supported (zero-padded). Negative newSize is rejected as a runtime error. Returns null on success, an error string on failure.\n\nDestructive — make a defensive backup of the trailing bytes before calling if the data could be valuable. Used by frogLight's openStoreLite to repair desynced library.f32 by trimming orphan trailing vectors (OFI #15).", Params: []string{"path", "newSize"}},
	"_md5": {
		Signature:     "_md5(data) -> hex_string",
		Documentation: "Returns MD5 hash of data as hex string (DEPRECATED: use SHA256)\nMD5 is cryptographically broken; use SHA256 for security",
		Params:        []string{"data"},
	},
	"_osArgs": {
		Signature:     "_osArgs() -> array of strings",
		Documentation: "Returns command-line arguments AFTER kLex's flag parser has\nconsumed its own options (--cpuprofile, --help, --version, etc.).\n\nShape (stable across flag-or-no-flag invocations):\n  args[0]    — the kLex binary path (os.Args[0])\n  args[1]    — the script path\n  args[2..N] — script arguments\n\nPre-fix behaviour returned raw os.Args, which meant any script using\n`args[2]` etc. broke if a kLex flag was present — the flag pair\nshifted every positional. OFI #13. The prepended binary path keeps\n`args[0]` meaningful for scripts that re-spawn klex on themselves\n(e.g. frogLight UI spawning the cataloger via _processSpawnDetached).",
		Params:        []string{},
	},
	"_scriptDir":        {Signature: "_scriptDir() -> string", Documentation: "Return the directory of the .lex file whose code is currently running. Walks the enclosing function/module env chain — inside an imported module it returns THAT module's directory, not the entry script's. Empty string if no script context (REPL).\n\nUse to locate sibling resources independent of caller CWD:\n  bridge = nativeBridge(\"python3\", [_scriptDir() + \"/helper.py\"], opts)\n  font   = loadFont(_scriptDir() + \"/fonts/Outfit.ttf\", 32)", Params: []string{}},
	"_osCwd":            {Signature: "_osCwd() -> (string, error)", Documentation: "Get working directory.", Params: []string{}},
	"_osExit":           {Signature: "_osExit(code) -> never", Documentation: "Exit program.", Params: []string{"code"}},
	"_osGetenv":         {Signature: "_osGetenv(name) -> string | null", Documentation: "Return the value of the named environment variable, or null if the variable is unset. Single-value return — NOT a (value, err) tuple, despite what older docs may suggest. Unpacking into two variables fails at runtime with 'cannot unpack NULL into 2 variables'.\n\nExample:\n  val = _osGetenv(\"HOME\")\n  if val == null { val = \"/tmp\" }", Params: []string{"name"}},
	"_osHostname":       {Signature: "_osHostname() -> (string, error)", Documentation: "Get hostname.", Params: []string{}},
	"_osName":           {Signature: "_osName() -> string", Documentation: "Return Go's runtime.GOOS verbatim — common values: \"darwin\", \"linux\", \"windows\", \"freebsd\", \"openbsd\". Use for cross-platform dispatch where path-probing would be brittle.\n\nExample:\n  name = _osName()\n  if name == \"darwin\"  { cmd = \"open\"     }\n  if name == \"linux\"   { cmd = \"xdg-open\" }\n  if name == \"windows\" { cmd = \"cmd\"      }", Params: []string{}},
	"_osPid":            {Signature: "_osPid() -> int", Documentation: "Get process ID.", Params: []string{}},
	"_osSetenv":         {Signature: "_osSetenv(name, value) -> (null, error)", Documentation: "Set environment variable.", Params: []string{"name", "value"}},
	"_pdfExtractByPage": {Signature: "_pdfExtractByPage(path) -> (pages, err)", Documentation: "Extract PDF text as an array of strings, one per page (1-indexed in source PDF, 0-indexed in returned array). Use stdlib/pdf.lex extractByPage() instead — it converts the raw error string into typed PDF_OPEN / PDF_DECODE errors.", Params: []string{"path"}},
	"_pdfExtractText":   {Signature: "_pdfExtractText(path) -> (text, err)", Documentation: "Extract all text from a PDF, with pages concatenated back-to-back (no page-break markers). Use stdlib/pdf.lex extractText() instead — it converts the raw error string into typed PDF_OPEN / PDF_DECODE errors. Works on text PDFs; scanned/image-only PDFs return empty (no OCR).", Params: []string{"path"}},
	"_pdfPageCount":     {Signature: "_pdfPageCount(path) -> (n, err)", Documentation: "Return the number of pages in a PDF. Cheap — does not extract text. Use stdlib/pdf.lex pageCount() instead — it converts the raw error string into typed PDF_OPEN / PDF_DECODE errors.", Params: []string{"path"}},
	"_processExec": {
		Signature:     "_processExec(cmd, args) -> (stdout, stderr, exitCode, err)",
		Documentation: "Runs cmd with args, captures stdout and stderr separately.\nexitCode is the integer exit code (-1 if the process could not be started).\nerr is non-null only when the process could not be started at all.",
		Params:        []string{"cmd", "args"},
	},
	"_processRun": {
		Signature:     "_processRun(cmd, args) -> (stdout, err)",
		Documentation: "Runs cmd with args, captures stdout. stderr is folded into err on failure.\nargs must be an Array of strings (may be empty).",
		Params:        []string{"cmd", "args"},
	},
	"_processShell": {
		Signature:     "_processShell(cmd) -> (stdout, err)",
		Documentation: "Runs cmd as a shell command via /bin/sh -c. stderr is folded into err.",
		Params:        []string{"cmd"},
	},
	"_randomBytes":    {Signature: "_randomBytes(n: int) -> bytes", Documentation: "Return n cryptographically-strong random bytes (uses Go's crypto/rand). Use for tokens, salts, nonces, ephemeral keys.\n\nExample:\n  salt = _randomBytes(16)\n  hex  = bytesToHex(salt)", Params: []string{"n"}},
	"_replaySeenFile": {Signature: "_replaySeenFile(path: string) -> hash", Documentation: "Fast JSONL → {path: int mtime} replay implemented in Go. Reads every line of `path`, extracts the `path` and `mtime` fields, and returns a hash keyed by path with the latest mtime as value. Records with `\"deleted\": true` or missing `path` are skipped. Malformed lines are silently skipped. Missing file → empty hash (no error).\n\nBuilt for the frogLight cataloger's resume-index path, where streaming bufio + encoding/json is dramatically faster than parsing 1M+ lines through stdlib/json.lex. Result shape feeds isFileFresh(s, path, mtime) directly via a scalar lookup.\n\nExample:\n  seen = _replaySeenFile(\"/store/catalog.jsonl\")\n  if seen[\"/a/b.txt\"] == fileMtime { /* skip — already cataloged */ }", Params: []string{"path"}},
	"_regexFind": {
		Signature:     "_regexFind(pattern, str) -> (string|null, err)",
		Documentation: "Returns the first match, or null if none.",
		Params:        []string{"pattern", "str"},
	},
	"_regexFindAll": {
		Signature:     "_regexFindAll(pattern, str) -> (array, err)",
		Documentation: "Returns all non-overlapping matches.",
		Params:        []string{"pattern", "str"},
	},
	"_regexGroups": {
		Signature:     "_regexGroups(pattern, str) -> (array|null, err)",
		Documentation: "Returns capture groups of the first match. Index 0 is the full match,\nindexes 1+ are the capture groups. Returns null if no match.",
		Params:        []string{"pattern", "str"},
	},
	"_regexGroupsAll": {
		Signature:     "_regexGroupsAll(pattern, str) -> (array_of_arrays, err)",
		Documentation: "Returns capture groups for every match. Each element is an array where\nindex 0 is the full match and indexes 1+ are the capture groups.",
		Params:        []string{"pattern", "str"},
	},
	"_regexMatch": {
		Signature:     "_regexMatch(pattern, str) -> (bool, err)",
		Documentation: "True if pattern matches anywhere in str.",
		Params:        []string{"pattern", "str"},
	},
	"_regexReplace": {
		Signature:     "_regexReplace(pattern, str, repl) -> (string, err)",
		Documentation: "Replaces the first match with repl.",
		Params:        []string{"pattern", "str", "repl"},
	},
	"_regexReplaceAll": {
		Signature:     "_regexReplaceAll(pattern, str, repl) -> (string, err)",
		Documentation: "Replaces all matches with repl.",
		Params:        []string{"pattern", "str", "repl"},
	},
	"_regexSplit": {
		Signature:     "_regexSplit(pattern, str) -> (array, err)",
		Documentation: "Splits str on every match of pattern.",
		Params:        []string{"pattern", "str"},
	},
	"_sha256": {Signature: "_sha256(data: string | bytes) -> string", Documentation: "SHA-256 hash → 64-char lowercase hex string. Accepts string or bytes input.", Params: []string{"data"}},
	"_sha512": {Signature: "_sha512(data: string | bytes) -> string", Documentation: "SHA-512 hash → 128-char lowercase hex string. Accepts string or bytes input.", Params: []string{"data"}},
	"_timeFields": {
		Signature:     "_timeFields(unix) -> (year, month, day, hour, minute, second, weekday)",
		Documentation: "Converts a unix timestamp (integer seconds) into local time fields.",
		Params:        []string{"unix"},
	},
	"_timeFormat": {
		Signature:     "_timeFormat(unix, layout) -> string",
		Documentation: "Formats a unix timestamp using Go's reference-time layout string.\nNamed layout constants are provided in datetime.lex for convenience.",
		Params:        []string{"unix", "layout"},
	},
	"_timeNanos": {
		Signature:     "_timeNanos() -> integer",
		Documentation: "Returns the current time in nanoseconds since an arbitrary epoch.\nSuitable for high-resolution timing and benchmarking.",
		Params:        []string{},
	},
	"_timeNow": {
		Signature:     "_timeNow() -> (year, month, day, hour, minute, second, unix, weekday)",
		Documentation: "Returns the current local time as an 8-element tuple.\nunix is integer seconds since 1970-01-01 00:00:00 UTC.\nweekday is the full English name: \"Monday\", \"Tuesday\", etc.",
		Params:        []string{},
	},
	"_timeParse": {
		Signature:     "_timeParse(str, layout) -> (unix, err)",
		Documentation: "Parses a time string using Go's reference-time layout. Returns the unix\ntimestamp on success, or (0, err_message) on failure.",
		Params:        []string{"str", "layout"},
	},
	"_tsvFormat": {Signature: "_tsvFormat(rows: array) -> string", Documentation: "Serialize an array of string arrays back into TSV (tab-separated) text. Counterpart to _tsvParse.", Params: []string{"rows"}},
	"_tsvParse":  {Signature: "_tsvParse(s: string) -> array", Documentation: "Parse TSV (tab-separated) text into an array of arrays of strings. Use for TSV files or tab-delimited data.\n\nExample:\n  rows = _tsvParse(_fsRead(\"data.tsv\"))\n  for r in rows { println(r[0]) }", Params: []string{"s"}},
	"_urlDecode": {
		Signature:     "_urlDecode(s) -> (decoded_string, err)",
		Documentation: "decodes a percent-encoded string.",
		Params:        []string{"s"},
	},
	"_urlEncode": {
		Signature:     "_urlEncode(s) -> string",
		Documentation: "percent-encodes a query string component.\nSpaces become +, special characters become %XX.",
		Params:        []string{"s"},
	},
	"_uuid": {
		Signature:     "_uuid() -> string",
		Documentation: "generates a random UUID v4.\nUses crypto/rand for proper randomness, not math/rand.\nFormat: xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx",
		Params:        []string{},
	},
	"color_bg_black":   {Signature: "color_bg_black() -> string", Documentation: "ANSI black background.", Params: []string{}},
	"color_bg_blue":    {Signature: "color_bg_blue() -> string", Documentation: "ANSI blue background.", Params: []string{}},
	"color_bg_cyan":    {Signature: "color_bg_cyan() -> string", Documentation: "ANSI cyan background.", Params: []string{}},
	"color_bg_green":   {Signature: "color_bg_green() -> string", Documentation: "ANSI green background.", Params: []string{}},
	"color_bg_magenta": {Signature: "color_bg_magenta() -> string", Documentation: "ANSI magenta background.", Params: []string{}},
	"color_bg_red":     {Signature: "color_bg_red() -> string", Documentation: "ANSI red background.", Params: []string{}},
	"color_bg_white":   {Signature: "color_bg_white() -> string", Documentation: "ANSI white background.", Params: []string{}},
	"color_bg_yellow":  {Signature: "color_bg_yellow() -> string", Documentation: "ANSI yellow background.", Params: []string{}},
	"color_black":      {Signature: "color_black() -> string", Documentation: "ANSI black foreground.", Params: []string{}},
	"color_blue":       {Signature: "color_blue() -> string", Documentation: "ANSI blue foreground.", Params: []string{}},
	"color_bold":       {Signature: "color_bold() -> string", Documentation: "ANSI bold text.", Params: []string{}},
	"color_cyan":       {Signature: "color_cyan() -> string", Documentation: "ANSI cyan foreground.", Params: []string{}},
	"color_dim":        {Signature: "color_dim() -> string", Documentation: "ANSI dim text.", Params: []string{}},
	"color_green":      {Signature: "color_green() -> string", Documentation: "ANSI green foreground.", Params: []string{}},
	"color_magenta":    {Signature: "color_magenta() -> string", Documentation: "ANSI magenta foreground.", Params: []string{}},
	"color_red":        {Signature: "color_red() -> string", Documentation: "ANSI red foreground.", Params: []string{}},
	"color_reset":      {Signature: "color_reset() -> string", Documentation: "ANSI reset formatting.", Params: []string{}},
	"color_underline":  {Signature: "color_underline() -> string", Documentation: "ANSI underline text.", Params: []string{}},
	"color_white":      {Signature: "color_white() -> string", Documentation: "ANSI white foreground.", Params: []string{}},
	"color_yellow":     {Signature: "color_yellow() -> string", Documentation: "ANSI yellow foreground.", Params: []string{}},
	"colorize":         {Signature: "colorize(text, code) -> string", Documentation: "Wrap text with color code.", Params: []string{"text", "code"}},

	// Graphics
	"window": {
		Signature:     "window(width: int, height: int, title: string, drawFn: fn(frame: int)) -> null",
		Documentation: "Open an OpenGL window and run the draw loop. Calls `drawFn(frameCount)` every frame at vsync. Blocks until the window is closed.\n\nCoordinate system: pixel origin at top-left, x right, y down.",
		Params:        []string{"width", "height", "title", "drawFn"},
	},
	"background": {
		Signature:     "background(gray: float) | background(r, g, b: float) | background(r, g, b, a: float) -> null",
		Documentation: "Clear the screen with a colour. Values are in [0.0, 1.0]. Single argument sets a grey level.",
		Params:        []string{"r", "g", "b"},
	},
	"fill": {
		Signature:     "fill(gray: float) | fill(r, g, b: float) | fill(r, g, b, a: float) -> null",
		Documentation: "Set the fill colour for subsequent shapes. Values in [0.0, 1.0].",
		Params:        []string{"r", "g", "b"},
	},
	"noFill": {
		Signature:     "noFill() -> null",
		Documentation: "Disable fill so shapes are drawn outline-only.",
		Params:        []string{},
	},
	"stroke": {
		Signature:     "stroke(gray: float) | stroke(r, g, b: float) | stroke(r, g, b, a: float) -> null",
		Documentation: "Set the stroke (outline) colour for subsequent shapes. Values in [0.0, 1.0].",
		Params:        []string{"r", "g", "b"},
	},
	"noStroke": {
		Signature:     "noStroke() -> null",
		Documentation: "Disable stroke so shapes are drawn fill-only.",
		Params:        []string{},
	},
	"strokeWeight": {
		Signature:     "strokeWeight(w: float) -> null",
		Documentation: "Set the stroke line width in pixels.",
		Params:        []string{"w"},
	},
	"blendMode": {
		Signature:     "blendMode(mode: string) -> null",
		Documentation: "Set the OpenGL blend mode for all subsequent draw calls.\n\nModes:\n  \"normal\"   — standard alpha blending (default)\n  \"add\"      — additive: src*alpha + dst; fire, glow, light, particles\n  \"multiply\" — dst * src; shadows, darkening\n  \"screen\"   — 1-(1-src)*(1-dst); brightening, light\n\nCall blendMode(\"normal\") to reset after drawing effects.\n\nExample:\n  blendMode(\"add\")\n  drawParticles(xs, ys, rs, gs, bs, alphas, n, 4.0)  // glowing particles\n  blendMode(\"normal\")",
		Params:        []string{"mode"},
	},
	"rect": {
		Signature:     "rect(x: float, y: float, w: float, h: float) -> null",
		Documentation: "Draw a rectangle. Origin is the top-left corner. Respects fill and stroke state.",
		Params:        []string{"x", "y", "w", "h"},
	},
	"circle": {
		Signature:     "circle(x: float, y: float, radius: float) -> null",
		Documentation: "Draw a circle centred at (x, y). Approximated with 64 segments. Respects fill and stroke state.",
		Params:        []string{"x", "y", "radius"},
	},
	"line": {
		Signature:     "line(x1: float, y1: float, x2: float, y2: float) -> null",
		Documentation: "Draw a line from (x1, y1) to (x2, y2) using the current stroke colour.",
		Params:        []string{"x1", "y1", "x2", "y2"},
	},
	"triangle": {
		Signature:     "triangle(x1, y1, x2, y2, x3, y3: float) -> null",
		Documentation: "Draw a triangle. Respects fill and stroke state.",
		Params:        []string{"x1", "y1", "x2", "y2", "x3", "y3"},
	},
	"frameCount": {
		Signature:     "frameCount() -> int",
		Documentation: "Return the number of frames rendered since the window opened.",
		Params:        []string{},
	},
	"mouseX": {
		Signature:     "mouseX() -> float",
		Documentation: "Return the current mouse X position in window pixels.",
		Params:        []string{},
	},
	"mouseY": {
		Signature:     "mouseY() -> float",
		Documentation: "Return the current mouse Y position in window pixels.",
		Params:        []string{},
	},
	"mouseDown": {
		Signature:     "mouseDown() -> bool",
		Documentation: "Return true if the left mouse button is currently pressed.",
		Params:        []string{},
	},
	"winWidth": {
		Signature:     "winWidth() -> int",
		Documentation: "Return the window width in pixels.",
		Params:        []string{},
	},
	"winHeight": {
		Signature:     "winHeight() -> int",
		Documentation: "Return the window height in pixels.",
		Params:        []string{},
	},
	"translate": {
		Signature:     "translate(x: float, y: float) -> null",
		Documentation: "Move the drawing origin by (x, y). Affects all subsequent draw calls until popMatrix().",
		Params:        []string{"x", "y"},
	},
	"rotate": {
		Signature:     "rotate(angle: float) -> null",
		Documentation: "Rotate the drawing axes by angle radians (clockwise, y-down). Affects all subsequent draw calls until popMatrix().",
		Params:        []string{"angle"},
	},
	"scale": {
		Signature:     "scale(sx: float, sy: float) -> null",
		Documentation: "Scale the drawing axes by (sx, sy). Affects all subsequent draw calls until popMatrix().",
		Params:        []string{"sx", "sy"},
	},
	"pushMatrix": {
		Signature:     "pushMatrix() -> null",
		Documentation: "Save the current transform matrix onto the stack. Pair with popMatrix() to restore it.",
		Params:        []string{},
	},
	"popMatrix": {
		Signature:     "popMatrix() -> null",
		Documentation: "Restore the transform matrix saved by the last pushMatrix() call.",
		Params:        []string{},
	},
	"keyDown": {
		Signature:     "keyDown(key: string) -> bool",
		Documentation: "Return true while the named key is physically held down.\n\nKey names: \"A\"–\"Z\", \"0\"–\"9\", \"SPACE\", \"ENTER\", \"ESC\", \"LEFT\", \"RIGHT\", \"UP\", \"DOWN\", \"BACKSPACE\", \"SHIFT\", \"CTRL\", \"TAB\".",
		Params:        []string{"key"},
	},
	"keyPressed": {
		Signature:     "keyPressed(key: string) -> bool",
		Documentation: "Return true only on the single frame the key was first pressed (one-shot). Resets to false the next frame.",
		Params:        []string{"key"},
	},
	"elapsedTime": {
		Signature:     "elapsedTime() -> float",
		Documentation: "Return seconds elapsed since the window opened as a float. Preferable to frameCount() for physics and animation — speed is independent of frame rate.",
		Params:        []string{},
	},
	// Phase 3
	"ellipse": {
		Signature:     "ellipse(x: float, y: float, rx: float, ry: float) -> null",
		Documentation: "Draw an ellipse centred at (x, y) with horizontal radius rx and vertical radius ry. Respects fill and stroke state.",
		Params:        []string{"x", "y", "rx", "ry"},
	},
	"polygon": {
		Signature:     "polygon(points: array) -> null",
		Documentation: "Draw an arbitrary polygon from a flat array of x,y pairs: [x1,y1, x2,y2, ...]. Must have at least 3 points (6 values). Respects fill and stroke state.",
		Params:        []string{"points"},
	},
	"lineChart": {
		Signature:     "lineChart(data: array, x, y, w, h: float) | lineChart(data: array, x, y, w, h, min, max: float) -> null",
		Documentation: "Draw a line chart of numeric data in the rect (x, y, w×h).\n\nUses the current fill() colour for the line and area fill (25% alpha). If min/max are omitted they are derived from the data.\n\nDraws: dark background, subtle axis lines, filled area, line, dot per point, accent border.\n\nExample:\n  fill(0.3, 0.7, 1.0, 1.0)\n  lineChart(durations, 10, 10, 400, 150)\n  lineChart(durations, 10, 10, 400, 150, 0.0, 10.0)",
		Params:        []string{"data", "x", "y", "w", "h", "min", "max"},
	},
	"barChart": {
		Signature:     "barChart(data: array, x, y, w, h: float) | barChart(data: array, x, y, w, h, min, max: float) -> null",
		Documentation: "Draw a vertical bar chart of numeric data in the rect (x, y, w×h).\n\nUses the current fill() colour for bars. Baseline is 0 unless min is provided. If max is omitted it is derived from the data.\n\nEach bar has a 15% gap. A dim track bar shows the full column height for context.\n\nExample:\n  fill(0.9, 0.4, 0.2, 1.0)\n  barChart(findingCounts, 10, 170, 400, 120)\n  barChart(findingCounts, 10, 170, 400, 120, 0.0, 50.0)",
		Params:        []string{"data", "x", "y", "w", "h", "min", "max"},
	},
	"pieChart": {
		Signature:     "pieChart(data: array, colors: array, cx, cy, radius: float, innerRadius?: float) -> null",
		Documentation: "Draw a pie chart centred at (cx, cy) with the given radius.\n\ndata is an array of numbers; colors is a matching array of [r,g,b,a] colour arrays.\nOptional innerRadius > 0 draws a donut chart instead.\nSlices start at the top (12 o'clock) and go clockwise.\nSlices with value 0 are skipped.\n\nExample:\n  counts  = [crit, high, med, low]\n  colours = [[0.9,0.2,0.2,1], [0.9,0.55,0.1,1], [0.9,0.8,0.1,1], [0.3,0.7,0.3,1]]\n  pieChart(counts, colours, 400, 300, 80)\n  pieChart(counts, colours, 400, 300, 80, 36)  // donut",
		Params:        []string{"data", "colors", "cx", "cy", "radius", "innerRadius"},
	},

	"sparkline": {
		Signature:     "sparkline(data: array, x, y, w, h: float) -> null",
		Documentation: "Draw a minimal inline line chart — no background, no axes, just the line.\n\nAuto-scales to the data range. Uses the current fill() colour. Needs at least 2 data points.\n\nIdeal for compact dashboard tiles or sidebar metrics.\n\nExample:\n  fill(0.4, 0.8, 0.4, 1.0)\n  sparkline(history, 10, 10, 120, 32)",
		Params:        []string{"data", "x", "y", "w", "h"},
	},

	"loadImage": {
		Signature:     "loadImage(path: string) -> image",
		Documentation: "Load a PNG or JPEG from disk and return an image handle. Safe to call before window() — texture upload is deferred to the first drawImage() call.",
		Params:        []string{"path"},
	},
	"drawImage": {
		Signature:     "drawImage(img: image, x: float, y: float) | drawImage(img: image, x, y, w, h: float) -> null",
		Documentation: "Draw an image at (x, y). Optional w and h scale the image; defaults to the image's natural size.",
		Params:        []string{"img", "x", "y", "w", "h"},
	},
	"text": {
		Signature:     "text(str: string, x: float, y: float, scale?: float) -> null",
		Documentation: "Draw a string using the embedded 8×8 monospace bitmap font. Optional scale multiplies character size (default 1 = 8px). Uses current fill colour as tint.",
		Params:        []string{"str", "x", "y", "scale"},
	},
	// Phase 4
	"point": {
		Signature:     "point(x: float, y: float) -> null",
		Documentation: "Draw a single pixel at (x, y) using the current stroke colour. Point size is controlled by strokeWeight().",
		Params:        []string{"x", "y"},
	},
	"frameRate": {
		Signature:     "frameRate(fps: float) -> null",
		Documentation: "Cap the render loop to fps frames per second. Pass 0 to revert to vsync. Safe to call before window() — the cap is applied when the window opens.\n\nExample: frameRate(30) limits to 30fps.",
		Params:        []string{"fps"},
	},
	"mouseClicked": {
		Signature:     "mouseClicked() -> bool",
		Documentation: "Return true only on the single frame the left mouse button was first pressed (one-shot). Resets to false the next frame.\n\nUse this for button clicks — mouseDown() fires every frame while held.",
		Params:        []string{},
	},
	"mouseRightClicked": {
		Signature:     "mouseRightClicked() -> bool",
		Documentation: "Return true only on the single frame the right mouse button was first pressed (one-shot). Resets to false the next frame.",
		Params:        []string{},
	},
	"mouseRightDown": {
		Signature:     "mouseRightDown() -> bool",
		Documentation: "Return true if the right mouse button is currently held down.",
		Params:        []string{},
	},
	"mouseScrollY": {
		Signature:     "mouseScrollY() -> float",
		Documentation: "Return the vertical mouse wheel delta for this frame. Positive = scrolled up/forward, negative = scrolled down/backward. Resets to 0 each frame.\n\nExample: scroll offset += mouseScrollY() * speed",
		Params:        []string{},
	},
	"mouseScrollX": {
		Signature:     "mouseScrollX() -> float",
		Documentation: "Return the horizontal mouse wheel delta for this frame (trackpad two-finger swipe or horizontal scroll wheel). Resets to 0 each frame.",
		Params:        []string{},
	},
	"droppedFiles": {
		Signature:     "droppedFiles() -> array",
		Documentation: "Return an array of file/folder paths dropped onto the window since the last call, then clear the buffer. Returns an empty array if nothing was dropped.\n\nThe drop callback fires when the user releases dragged files over the window. Call once per frame and act on any paths returned.\n\nNote: there is no \"drag hover\" event — you only learn about a drop after it lands.\n\nExample:\n  dropped = droppedFiles()\n  if len(dropped) > 0 {\n      scanPath = dropped[0]\n  }\n\n  // Handle multiple files:\n  i = 0\n  while i < len(dropped) {\n      processFile(dropped[i])\n      i = i + 1\n  }",
		Params:        []string{},
	},
	"arc": {
		Signature:     "arc(x, y, r, startAngle, endAngle: float) -> null",
		Documentation: "Draw an arc centred at (x, y) with radius r sweeping from startAngle to endAngle (radians). Angles follow screen-space convention: 0 = right, π/2 = down.\n\nWith fill active: draws a filled sector (pie slice from centre to arc).\nWith stroke active: draws the arc line only.\nBoth can be active simultaneously.\n\nExample:\n  // Circular progress bar (75% = 0 to 1.5π)\n  fill(0.3, 0.7, 1.0, 1.0)\n  noStroke()\n  arc(200, 200, 60, -1.5708, 3.1416)\n\n  // Gauge ring\n  noFill()\n  stroke(0.8, 0.8, 0.9, 0.4)\n  strokeWeight(8)\n  arc(200, 200, 50, 2.356, 0.785)",
		Params:        []string{"x", "y", "r", "startAngle", "endAngle"},
	},
	"roundedRect": {
		Signature:     "roundedRect(x, y, w, h, r: float) -> null",
		Documentation: "Draw a rounded rectangle with corner radius r. Rendered using a signed distance field (SDF) shader — mathematically perfect anti-aliased edges at any size. Respects fill and stroke state.",
		Params:        []string{"x", "y", "w", "h", "r"},
	},
	"shadow": {
		Signature:     "shadow(offsetX, offsetY, blur: float) -> null\nshadow(offsetX, offsetY, blur, r, g, b, a: float) -> null",
		Documentation: "Enable analytical Gaussian drop shadows on rect(), circle(), and roundedRect().\n\noffsetX/offsetY: shadow displacement in pixels (positive = right/down).\nblur: Gaussian sigma — larger values produce a softer, wider shadow.\nColour defaults to black at 50% alpha; pass r,g,b,a for an explicit colour.\n\nCall noShadow() to disable.\n\nExample:\n  shadow(4, 6, 12)\n  fill(0.95, 0.95, 0.98, 1.0)\n  roundedRect(50, 50, 300, 180, 12)\n  noShadow()",
		Params:        []string{"offsetX", "offsetY", "blur"},
	},
	"noShadow": {
		Signature:     "noShadow() -> null",
		Documentation: "Disable drop shadows set by shadow(). Subsequent rect(), circle(), and roundedRect() calls will not draw a shadow.",
		Params:        []string{},
	},
	"beginPath": {
		Signature:     "beginPath() -> null",
		Documentation: "Clear the current vector path and reset the pen. Must be called before building a new path with moveTo / lineTo / bezierTo / quadTo.",
		Params:        []string{},
	},
	"moveTo": {
		Signature:     "moveTo(x, y: float) -> null",
		Documentation: "Move the pen to (x, y) without drawing. Starts a new contour. Must be called before lineTo / bezierTo / quadTo.",
		Params:        []string{"x", "y"},
	},
	"lineTo": {
		Signature:     "lineTo(x, y: float) -> null",
		Documentation: "Add a straight line from the current pen position to (x, y). Requires a prior moveTo.",
		Params:        []string{"x", "y"},
	},
	"bezierTo": {
		Signature:     "bezierTo(cp1x, cp1y, cp2x, cp2y, x, y: float) -> null",
		Documentation: "Add a cubic Bézier curve from the current pen to (x, y). cp1 and cp2 are the two control points. The curve is tessellated to line segments at 0.25px tolerance using De Casteljau subdivision.\n\nExample:\n  beginPath()\n  moveTo(100, 200)\n  bezierTo(100, 100,  300, 100,  300, 200)\n  strokePath()",
		Params:        []string{"cp1x", "cp1y", "cp2x", "cp2y", "x", "y"},
	},
	"quadTo": {
		Signature:     "quadTo(cpx, cpy, x, y: float) -> null",
		Documentation: "Add a quadratic Bézier curve from the current pen to (x, y) via control point (cpx, cpy). Internally elevated to a cubic for uniform tessellation.",
		Params:        []string{"cpx", "cpy", "x", "y"},
	},
	"closePath": {
		Signature:     "closePath() -> null",
		Documentation: "Close the current contour by adding a line from the current pen back to the last moveTo point.",
		Params:        []string{},
	},
	"fillPath": {
		Signature:     "fillPath() -> null",
		Documentation: "Fill the current path with the current fill colour. Uses GPU stencil even-odd rule — handles concave and complex shapes correctly. Does not clear the path; call beginPath() to start fresh.",
		Params:        []string{},
	},
	"strokePath": {
		Signature:     "strokePath() -> null",
		Documentation: "Stroke the current path outline using the current stroke colour and strokeWeight. Renders each segment as an expanded quad. Does not clear the path.",
		Params:        []string{},
	},
	"gradient": {
		Signature:     "gradient(x, y, w, h: float, color1, color2: array, dir: string) -> null",
		Documentation: "Fill a rectangle with a two-color linear gradient rendered on the GPU.\n\ncolor1 and color2 are [r, g, b, a] float arrays (values 0.0–1.0).\ndir: \"h\" = horizontal (color1 left → color2 right); \"v\" = vertical (color1 top → color2 bottom).\n\nCan be called inside or outside uiBegin()/uiEnd(). Layer with pushClip/popClip to confine gradients to panels.\n\nExample:\n  // Dark panel header\n  gradient(0, 0, winWidth(), 48, [0.15, 0.15, 0.22, 1.0], [0.10, 0.10, 0.16, 1.0], \"v\")\n\n  // Sidebar fade\n  gradient(0, 0, splitX, winHeight(), [0.14, 0.16, 0.22, 1.0], [0.09, 0.11, 0.16, 1.0], \"v\")\n\n  // Accent button fill\n  gradient(btnX, btnY, btnW, btnH, [0.35, 0.55, 1.0, 1.0], [0.20, 0.40, 0.90, 1.0], \"v\")",
		Params:        []string{"x", "y", "w", "h", "color1", "color2", "dir"},
	},
	"drawParticles": {
		Signature:     "drawParticles(xs, ys, rs, gs, bs, alphas: array, count: int, pointSize: float) -> null",
		Documentation: "Render up to count particles in a single GPU draw call using per-vertex colour. xs/ys are position arrays, rs/gs/bs/alphas are colour component arrays (SoA layout). Particles with alpha < 0.01 are skipped automatically.",
		Params:        []string{"xs", "ys", "rs", "gs", "bs", "alphas", "count", "pointSize"},
	},
	"fontCharWidth": {
		Signature:     "fontCharWidth() -> int",
		Documentation: "Return the pixel width of one character in the embedded monospace font at scale 1. Multiply by scale to get the display width.",
		Params:        []string{},
	},
	"fontCharHeight": {
		Signature:     "fontCharHeight() -> int",
		Documentation: "Return the pixel height of one character in the embedded monospace font at scale 1. Multiply by scale to get the display height.",
		Params:        []string{},
	},
	"loadFont": {
		Signature:     "loadFont(path: string) -> Font\nloadFont(path: string, ptSize: float) -> Font",
		Documentation: "Load a TrueType or OpenType font file from disk and build a proportional SDF texture atlas. ptSize defaults to 16.\n\nGPU upload is deferred to the first textFont() call — safe to call before window() opens.\n\nExample:\n  myFont = loadFont(\"/path/to/font.ttf\", 24)\n  textFont(myFont, \"Hello!\", 100, 100)",
		Params:        []string{"path", "ptSize"},
	},
	"textFont": {
		Signature:     "textFont(font: Font, str: string, x: float, y: float) -> null\ntextFont(font: Font, str: string, x: float, y: float, scale: float) -> null",
		Documentation: "Draw a string using a font returned by loadFont(). Respects the current fill colour. scale defaults to 1.\n\nUse textWidth() to measure the string before drawing for alignment.\n\nExample:\n  fill(1, 1, 1, 1)\n  textFont(myFont, \"Score: 42\", 10, 10, 1.5)",
		Params:        []string{"font", "str", "x", "y", "scale"},
	},
	"textWidth": {
		Signature:     "textWidth(font: Font, str: string) -> float\ntextWidth(font: Font, str: string, scale: float) -> float",
		Documentation: "Return the pixel width of str when rendered with font at the given scale (default 1).\n\nUse this to right-align or centre text:\n  w = textWidth(myFont, label, 1.0)\n  textFont(myFont, label, centreX - w/2, y)",
		Params:        []string{"font", "str", "scale"},
	},

	// UI Widgets
	"uiBegin": {
		Signature:     "uiBegin() -> null",
		Documentation: "Reset UI state at the start of each draw frame. Must be called before any UI widget calls.",
		Params:        []string{},
	},
	"uiEnd": {
		Signature:     "uiEnd() -> null",
		Documentation: "Finalise the UI frame — updates hover state for the next frame. Must be called after all UI widget calls.",
		Params:        []string{},
	},
	"uiNextFieldID": {
		Signature:     "uiNextFieldID() -> string",
		Documentation: "Returns the ID that the next textInput() call will claim. Call immediately before textInput() to capture its ID for Tab-key focus management.\n\nExample:\n  fid = uiNextFieldID()\n  val = textInput(\"\", val, x, y, w, h)\n  if keyPressed(\"TAB\") { uiSetFocus(fid) }",
		Params:        []string{},
	},
	"uiGetFocus": {
		Signature:     "uiGetFocus() -> string",
		Documentation: "Returns the ID of the currently focused widget, or \"\" if nothing is focused. Use with uiSetFocus() to implement Tab key navigation between text fields.",
		Params:        []string{},
	},
	"uiSetFocus": {
		Signature:     "uiSetFocus(id: string) -> null",
		Documentation: "Programmatically focus the widget with the given ID. Use in combination with uiNextFieldID() and keyPressed(\"TAB\") to cycle focus between text input fields.",
		Params:        []string{"id"},
	},
	"button": {
		Signature:     "button(label: string, x, y, w, h: int, size?: float) -> bool",
		Documentation: "Draw a clickable button at (x, y) with dimensions w×h. Returns true on the frame it is clicked. Optional size controls text scale (default 0.5).\n\nMust be called between uiBegin() and uiEnd().",
		Params:        []string{"label", "x", "y", "w", "h", "size"},
	},
	"label": {
		Signature:     "label(text: string, x, y: int, size?: float) -> null",
		Documentation: "Draw a text label at (x, y). Optional size controls text scale (default 0.5).\n\nMust be called between uiBegin() and uiEnd().",
		Params:        []string{"text", "x", "y", "size"},
	},
	"textInput": {
		Signature:     "textInput(label: string, text: string, x, y, w, h: int, size?: float) -> string",
		Documentation: "Immediate-mode text input widget. Draws a labelled box at (x, y) with size w×h. Returns the updated string each frame. Click to focus; click elsewhere to unfocus. Optional size controls text scale (default 0.5).\n\nFull cursor and selection support:\n  Click              — position cursor\n  Shift+click        — extend selection\n  Left/Right         — move cursor one character\n  Ctrl/Cmd+Left/Right — jump by word\n  Home/End           — jump to start/end of text\n  Shift+arrow/Home/End — extend selection\n  Ctrl/Cmd+A         — select all\n\nEditing shortcuts (when focused):\n  Backspace          — delete character before cursor (or selection)\n  Delete             — delete character after cursor (or selection)\n  Ctrl/Cmd+Z         — undo (word-boundary granularity, up to 50 steps)\n  Ctrl/Cmd+Shift+Z   — redo\n  Ctrl/Cmd+Y         — redo (alternative)\n  Ctrl/Cmd+V         — paste from clipboard at cursor\n  Ctrl/Cmd+C         — copy selection (or full text)\n  Ctrl/Cmd+X         — cut selection (or full text)\n\nText longer than the field scrolls horizontally; the view tracks the cursor automatically.\n\nMust be called between uiBegin() and uiEnd().",
		Params:        []string{"label", "text", "x", "y", "w", "h", "size"},
	},
	"list": {
		Signature:     "list(label: string, items: array, x, y, w, h: int, size?: float) -> string",
		Documentation: "Scrollable selection list. Draws items inside a box at (x, y) with size w×h. Returns the currently selected item as a string. Scroll with the mouse wheel. Optional size controls text scale (default 0.5).\n\nMust be called between uiBegin() and uiEnd().",
		Params:        []string{"label", "items", "x", "y", "w", "h", "size"},
	},
	"listMulti": {
		Signature:     "listMulti(label: string, items: array, selected: array, x, y, w, h: int, size?: float) -> array",
		Documentation: "Scrollable multi-selection list. selected is a bool array (one entry per item) — pass it in each frame and reassign the return value to keep state.\n\nClick any row to toggle its selection. Selected rows are highlighted with an accent strip on the left. Scroll with the mouse wheel.\n\nExample:\n  selected = makeArray(len(items), false)\n\n  // in draw loop:\n  selected = listMulti(\"\", items, selected, x, y, w, h)\n\n  // read selections:\n  i = 0\n  while i < len(selected) {\n      if selected[i] { doSomething(items[i]) }\n      i = i + 1\n  }",
		Params:        []string{"label", "items", "selected", "x", "y", "w", "h", "size"},
	},
	"checkbox": {
		Signature:     "checkbox(label: string, x, y: int, checked: bool, size?: float) -> bool",
		Documentation: "Draw a checkbox at (x, y). Returns the new checked state (toggled when clicked). Optional size controls text scale (default 0.5).\n\nMust be called between uiBegin() and uiEnd().",
		Params:        []string{"label", "x", "y", "checked", "size"},
	},
	"slider": {
		Signature:     "slider(label: string, x, y, w: int, value, min, max: float, size?: float) -> float",
		Documentation: "Horizontal drag slider spanning w pixels. Returns the updated value clamped to [min, max]. Displays label and current value above the track. Click or drag the handle to set. Optional size controls text scale (default 0.5).\n\nMust be called between uiBegin() and uiEnd().",
		Params:        []string{"label", "x", "y", "w", "value", "min", "max", "size"},
	},
	"progressBar": {
		Signature:     "progressBar(x, y, w, h: int, value, min, max: float) -> null",
		Documentation: "Display-only filled progress bar at (x, y) with size w×h. Fill fraction = (value - min) / (max - min), clamped to [0, 1]. No interaction.\n\nMust be called between uiBegin() and uiEnd().",
		Params:        []string{"x", "y", "w", "h", "value", "min", "max"},
	},
	"dropdown": {
		Signature:     "dropdown(label: string, items: array, x, y, w: int, size?: float) -> string",
		Documentation: "Compact single-selection dropdown. Shows the selected item in a header bar; click to open a menu below. Returns the selected item string. Optional size controls text scale (default 0.5).\n\nCall after other widgets so the open menu renders on top. Must be called between uiBegin() and uiEnd().",
		Params:        []string{"label", "items", "x", "y", "w", "size"},
	},

	"tooltip": {
		Signature:     "tooltip(text: string) -> null",
		Documentation: "Attach hover text to the widget drawn immediately before this call. The tooltip appears after the cursor rests on that widget for 0.5 seconds, positioned near the cursor and clamped to the window bounds.\n\nMust be called between uiBegin() and uiEnd(), directly after the target widget.\n\nExample:\n  slider(\"\", x, y, w, val, 0.0, 1.0)\n  tooltip(\"Drag to adjust the master volume\")\n\n  checkbox(\"Include git history\", x, y, on)\n  tooltip(\"Also scans all commits for leaked credentials\")",
		Params:        []string{"text"},
	},

	"image": {
		Signature:     "image(img: image, x, y, w, h: int, mode?: string) -> null",
		Documentation: "Draw an image loaded with loadImage() inside the UI at rect (x, y, w, h).\n\nmode controls how the image fills the box:\n  \"fit\"     — scale to fit, preserve aspect ratio, centre with letterbox (default)\n  \"fill\"    — scale to fill, preserve aspect ratio, crop edges\n  \"stretch\" — stretch to exactly w×h\n\nMust be called between uiBegin() and uiEnd().\n\nExample:\n  logo = loadImage(\"logo.png\")\n  image(logo, 10, 10, 200, 100)\n  image(logo, 10, 10, 200, 100, \"fill\")",
		Params:        []string{"img", "x", "y", "w", "h", "mode"},
	},

	"toast": {
		Signature:     "toast(message: string, style?: string, duration?: float) -> null",
		Documentation: "Show an ephemeral notification in the bottom-right corner of the window. The toast fades out after duration seconds (default 3.0).\n\nstyle controls the left-accent colour:\n  \"info\"    — blue (default)\n  \"success\" — green\n  \"warn\"    — orange\n  \"error\"   — red\n\nToasts stack upward when multiple are active. Must be called between uiBegin() and uiEnd().\n\nExample:\n  toast(\"Scan complete — 3 secrets found\", \"warn\")\n  toast(\"File saved\", \"success\", 2.0)",
		Params:        []string{"message", "style", "duration"},
	},

	"uiSetFont": {
		Signature:     "uiSetFont(font: Font) -> null",
		Documentation: "Set the active font for all widget text (button labels, tabs, etc.). Call once per frame after loading a font with loadFont(). Reverts to the embedded monospace font when uiResetFont() is called.\n\nMust be called between uiBegin() and uiEnd().",
		Params:        []string{"font"},
	},
	"uiResetFont": {
		Signature:     "uiResetFont() -> null",
		Documentation: "Revert widget text back to the embedded monospace font. Call after uiSetFont() when you want to stop using a custom font for subsequent widgets.",
		Params:        []string{},
	},
	"makeTheme": {
		Signature:     "makeTheme() -> array",
		Documentation: "Return the default 14-slot UI palette as an array of [r,g,b,a] arrays. Modify slots then pass to uiTheme() to apply globally.\n\nSlot order:\n  0=widgetBg, 1=widgetBgHover, 2=widgetBgActive\n  3=widgetText, 4=labelText, 5=dimText\n  6=accent, 7=accentBg\n  8=track, 9=trackFill, 10=handle\n  11=inputBg, 12=inputFocusBg\n  13=shadow\n\nExample:\n  t = makeTheme()\n  t[0] = [0.1, 0.2, 0.4, 1.0]   // widgetBg → deep blue\n  uiTheme(t)",
		Params:        []string{},
	},
	"uiTheme": {
		Signature:     "uiTheme(palette: array) -> null",
		Documentation: "Install a 14-slot palette from makeTheme() as the global widget color theme. All widgets (button, label, slider, list, tabs, contextMenu, etc.) read colors from this palette each frame.\n\nCall once at startup or whenever you want to switch themes. Must be called after window() opens.",
		Params:        []string{"palette"},
	},

	"pushClip": {
		Signature:     "pushClip(x, y, w, h: float) -> null",
		Documentation: "Push a scissor clipping rectangle onto the clip stack. All subsequent draw calls are clipped to this region. Nesting is fully supported — nested calls automatically intersect with the parent rect so content can never escape an outer clip. Pair every pushClip with a matching popClip.\n\nExample:\n  pushClip(0, 0, panelW, winHeight())\n  // sidebar content\n  pushClip(4, scrollY, panelW-8, listH)  // clips inside the panel\n  // scrollable list\n  popClip()\n  popClip()",
		Params:        []string{"x", "y", "w", "h"},
	},
	"popClip": {
		Signature:     "popClip() -> null",
		Documentation: "Pop the top clip rect from the stack and restore the one below it. If the stack is now empty, clipping is disabled entirely. Every pushClip must have exactly one matching popClip.",
		Params:        []string{},
	},

	"pushDisabled": {
		Signature:     "pushDisabled(disabled: bool) -> null",
		Documentation: "Push a disabled-state frame. All interactive widgets drawn between pushDisabled(true) and popDisabled() render at half opacity and ignore hover/click. Stack-based so nested forms can selectively re-enable sections.\n\nMust be called between uiBegin() and uiEnd().",
		Params:        []string{"disabled"},
	},
	"popDisabled": {
		Signature:     "popDisabled() -> null",
		Documentation: "Pop the top disabled-state frame pushed by pushDisabled(). Every pushDisabled must have exactly one matching popDisabled.\n\nMust be called between uiBegin() and uiEnd().",
		Params:        []string{},
	},

	"setTheme": {
		Signature:     "setTheme(name: string) -> null",
		Documentation: "Install a preset theme by name. Valid names:\n  \"nebula\"       — deep violet + ice cyan (the default)\n  \"light\"        — clean light theme, blue accent\n  \"dark\"         — modern dark theme, blue accent\n  \"highContrast\" — accessibility: pure black/white + yellow accent\nSets both colour palette AND style tokens (radii, spacing, font base). Use uiTheme(palette) for fine-grained colour overrides.\n\nCall once at startup or on theme-switch. Must be called after window() opens.",
		Params:        []string{"name"},
	},

	"lineHeight": {
		Signature:     "lineHeight(scale?: float) -> int",
		Documentation: "Pixel height of one line of widget text at scale (default 0.5). Useful for vertically centring text manually:\n  label(s, x, y + (h - lineHeight()) / 2)\n\nMust be called between uiBegin() and uiEnd().",
		Params:        []string{"scale"},
	},

	"tabs": {
		Signature:     "tabs(x, y, w: int, items: array, activeIdx: int, size?: float) -> int",
		Documentation: "Draw a horizontal tab bar at (x, y) spanning w pixels. items is the array of tab label strings. Returns the active tab index. Use if-blocks on the returned index to render tab content.\n\nMust be called between uiBegin() and uiEnd().",
		Params:        []string{"x", "y", "w", "items", "activeIdx", "size"},
	},
	"textArea": {
		Signature:     "textArea(label: string, text: string, x, y, w, h: int, size?: float) -> string",
		Documentation: "Multi-line text editing widget. Draws a labelled box at (x, y) with size w×h. Returns the updated string after applying typed characters and backspaces from the current frame. Click to focus; click elsewhere to unfocus. Supports newlines.\n\nMust be called between uiBegin() and uiEnd().",
		Params:        []string{"label", "text", "x", "y", "w", "h", "size"},
	},
	"toggle": {
		Signature:     "toggle(label: string, x, y: int, on: bool, size?: float) -> bool",
		Documentation: "Draw a toggle switch at (x, y) with a label. Returns the new on/off state (toggled when clicked).\n\nMust be called between uiBegin() and uiEnd().",
		Params:        []string{"label", "x", "y", "on", "size"},
	},
	"radio": {
		Signature:     "radio(label: string, x, y: int, value: string, groupValue: string, size?: float) -> string",
		Documentation: "Draw a radio button at (x, y). Returns value if this option was clicked, otherwise returns groupValue unchanged. Chain calls through a group — the returned groupValue carries the selected option forward.\n\nExample:\n  sel = radio(\"Option A\", x, y,   \"a\", sel)\n  sel = radio(\"Option B\", x, y2,  \"b\", sel)\n\nMust be called between uiBegin() and uiEnd().",
		Params:        []string{"label", "x", "y", "value", "groupValue", "size"},
	},
	"numericStepper": {
		Signature:     "numericStepper(label: string, x, y, w: int, value, min, max: int, size?: float) -> int",
		Documentation: "Integer stepper with − and + buttons spanning w pixels. Returns the updated integer value clamped to [min, max]. Displays label and current value above the controls.\n\nMust be called between uiBegin() and uiEnd().",
		Params:        []string{"label", "x", "y", "w", "value", "min", "max", "size"},
	},
	"getTypedChars": {
		Signature:     "getTypedChars() -> string",
		Documentation: "Return a string containing all printable ASCII characters typed this frame. Resets each frame. Use for custom keyboard input handling outside of textInput().",
		Params:        []string{},
	},
	"table": {
		Signature:     "table(headers: array, rows: array, x, y, w, h: int, size?: float) -> int",
		Documentation: "Scrollable data grid with fixed column headers. headers is an array of column name strings; rows is an array of arrays (each sub-array is one row). Returns the selected row index, or -1 if no row is selected. Click a row to select it.\n\nMust be called between uiBegin() and uiEnd().",
		Params:        []string{"headers", "rows", "x", "y", "w", "h", "size"},
	},
	"accordion": {
		Signature:     "accordion(x, y, w: int, sections: array, openIdx: int, size?: float) -> int",
		Documentation: "Draw stacked collapsible section headers at (x, y) spanning w pixels. sections is an array of label strings. Returns the open section index (-1 = all closed). Render section content at y + (openIdx+1)*sectionH using the returned index.\n\nMust be called between uiBegin() and uiEnd().",
		Params:        []string{"x", "y", "w", "sections", "openIdx", "size"},
	},
	"contextMenu": {
		Signature:     "contextMenu(x, y: int, items: array, visible: bool, size?: float) -> int",
		Documentation: "Draw a floating context menu at (x, y) when visible is true. items is an array of label strings. Returns the selected item index, -1 if nothing was clicked, or -2 if the user clicked outside the menu (use this to dismiss). Render after other widgets so the menu appears on top.\n\nMust be called between uiBegin() and uiEnd().",
		Params:        []string{"x", "y", "items", "visible", "size"},
	},
	"colorPicker": {
		Signature:     "colorPicker(x, y, w: int, r, g, b, a: float) -> array",
		Documentation: "Four RGBA drag sliders with a live preview swatch. Values are in [0.0, 1.0]. Returns [r, g, b, a] with the updated colour components.\n\nExample:\n  result = colorPicker(10, 10, 200, r, g, b, a)\n  r = result[0]; g = result[1]; b = result[2]; a = result[3]\n\nMust be called between uiBegin() and uiEnd().",
		Params:        []string{"x", "y", "w", "r", "g", "b", "a"},
	},
	"treeView": {
		Signature:     "treeView(x, y, w, h: int, labels: array, levels: array, expanded: array, size?: float) -> array",
		Documentation: "Hierarchical tree view at (x, y) with size w×h. labels[i] is the display text for node i; levels[i] is its indent depth (0 = root); expanded[i] is a bool for whether the node is expanded. Returns [selectedIdx, newExpanded]. Reassign both:\n  result = treeView(...)\n  sel = result[0]\n  exp = result[1]\n\nMust be called between uiBegin() and uiEnd().",
		Params:        []string{"x", "y", "w", "h", "labels", "levels", "expanded", "size"},
	},
	"scrollArea": {
		Signature:     "scrollArea(x, y, w, h: int, contentH: float) -> float",
		Documentation: "Draw a panel at (x, y) with size w×h and a vertical scrollbar sized to contentH (total scrollable content height). Returns the current scroll offset in pixels. Use with pushClip/popClip to clip content:\n\n  offset = scrollArea(x, y, w, h, contentH)\n  pushClip(x, y, w, h)\n  // draw content shifted up by offset\n  popClip()",
		Params:        []string{"x", "y", "w", "h", "contentH"},
	},
	"splitter": {
		Signature:     "splitter(pos, x, y, length, orient: string, min, max: int, thickness?: int) -> int",
		Documentation: "Draw an interactive resize handle between two panels. Pass the current bar position as pos and reassign the return value each frame to keep state.\n\norient: \"v\" = vertical bar (drags left/right, changes X); \"h\" = horizontal bar (drags up/down, changes Y).\nx, y: top-left origin of the region; length: bar extent in the perpendicular direction.\nmin/max: clamping bounds for pos. thickness: hit area width/height in pixels (default 6).\n\nThe bar is drawn with the track colour normally and the accent colour on hover/drag. The mouse cursor changes to a resize arrow automatically.\n\nMust be called between uiBegin() and uiEnd().\n\nExample:\n  let splitX = 200\n\n  // in draw loop:\n  splitX = splitter(splitX, 0, 0, winHeight(), \"v\", 100, 600)\n  pushClip(0, 0, splitX, winHeight())\n  // sidebar content\n  popClip()\n  pushClip(splitX + 1, 0, winWidth() - splitX - 1, winHeight())\n  // main content\n  popClip()",
		Params:        []string{"pos", "x", "y", "length", "orient", "min", "max", "thickness"},
	},
	"modal": {
		Signature:     "modal(title: string, message: string, buttons: array) -> string",
		Documentation: "Draw a full-screen dimmed overlay with a centred dialog containing title, a word-wrapped message, and a row of buttons. Returns the label of the clicked button, or \"\" each frame until one is clicked.\n\nCall AFTER all other widgets (before uiEnd()) so it renders on top. While visible it blocks background widget interaction via a full-screen hit element.\n\nExample:\n  if showConfirm {\n    result = modal(\"Delete?\", \"This cannot be undone.\", [\"Cancel\", \"Delete\"])\n    if result != \"\" {\n      showConfirm = false\n      if result == \"Delete\" { doDelete() }\n    }\n  }",
		Params:        []string{"title", "message", "buttons"},
	},

	// ── Layout cursors ────────────────────────────────────────────────────────

	"uiBeginRow": {
		Signature:     "uiBeginRow(x, y, h, gap: float) -> null",
		Documentation: "Initialise a horizontal layout cursor at (x, y) with item height h and gap pixels between items.\n\nAfter calling uiBeginRow(), use uiRowX() and uiRowY() to position each widget, then call uiRowAdvance(w) to move the cursor right by w + gap.\n\nMust be called between uiBegin() and uiEnd().",
		Params:        []string{"x", "y", "h", "gap"},
	},

	"uiRowX": {
		Signature:     "uiRowX() -> int",
		Documentation: "Returns the current X position of the row cursor — where the next widget should be placed. Advances after each uiRowAdvance() call.",
		Params:        []string{},
	},

	"uiRowY": {
		Signature:     "uiRowY() -> int",
		Documentation: "Returns the Y position of the active row. This value is constant for the lifetime of the row (set by uiBeginRow).",
		Params:        []string{},
	},

	"uiRowH": {
		Signature:     "uiRowH() -> int",
		Documentation: "Returns the height of the active row. This value is constant for the lifetime of the row (set by uiBeginRow). Use it as the h argument for widgets placed in the row.",
		Params:        []string{},
	},

	"uiRowAdvance": {
		Signature:     "uiRowAdvance(w: float) -> null",
		Documentation: "Advance the row cursor right by w + gap (the gap set in uiBeginRow). Call once after each widget placed at uiRowX(), uiRowY().",
		Params:        []string{"w"},
	},

	"uiBeginCol": {
		Signature:     "uiBeginCol(x, y, w, gap: float) -> null",
		Documentation: "Initialise a vertical layout cursor at (x, y) with column width w and gap pixels between items.\n\nAfter calling uiBeginCol(), use uiColX() and uiColY() to position each widget, then call uiColAdvance(h) to move the cursor down by h + gap.\n\nMust be called between uiBegin() and uiEnd().",
		Params:        []string{"x", "y", "w", "gap"},
	},

	"uiColX": {
		Signature:     "uiColX() -> int",
		Documentation: "Returns the X position of the active column. This value is constant for the lifetime of the column (set by uiBeginCol).",
		Params:        []string{},
	},

	"uiColY": {
		Signature:     "uiColY() -> int",
		Documentation: "Returns the current Y position of the column cursor — where the next widget should be placed. Advances after each uiColAdvance() call.",
		Params:        []string{},
	},

	"uiColW": {
		Signature:     "uiColW() -> int",
		Documentation: "Returns the width of the active column. This value is constant for the lifetime of the column (set by uiBeginCol). Use it as the w argument for widgets that should span the full column.",
		Params:        []string{},
	},

	"uiColAdvance": {
		Signature:     "uiColAdvance(h: float) -> null",
		Documentation: "Advance the column cursor down by h + gap (the gap set in uiBeginCol). Call once after each widget placed at uiColX(), uiColY().",
		Params:        []string{"h"},
	},

	// ── Model Context Protocol (MCP) client ─────────────────────────────────
	// Stdio-based JSON-RPC 2.0 client for talking to any MCP server (frogMcp,
	// filesystem-mcp, github-mcp, etc.). Most user code should prefer the
	// idiomatic stdlib/mcp.lex wrapper (mcp.newClient / mcp.listTools /
	// mcp.callTool / mcp.callToolText) rather than these underscore primitives.
	"_mcpSpawn": {
		Signature:     "_mcpSpawn(cmd: string, args: array, opts?: hash) -> (mcp_client, error)",
		Documentation: "Spawn an MCP server subprocess and complete the JSON-RPC initialize handshake.\n\nopts (hash, optional):\n  env          : hash string→string of additional env vars\n  timeout_sec  : initialize-handshake timeout in seconds (default 30)\n  notif_buffer : server-notification channel size (default 32)\n\nReturns the MCP client object ready for _mcpCall, or a typed error:\n  MCP_SPAWN_FAILED — exec.Start error (cmd not found, pipe failure)\n  MCP_INIT_FAILED  — initialize RPC errored or timed out\n\nPrefer mcp.newClient(...) from stdlib/mcp.lex over calling this directly.",
		Params:        []string{"cmd", "args", "opts"},
	},

	"_mcpCall": {
		Signature:     "_mcpCall(client: mcp_client, method: string, params: any, timeoutSec?: number) -> (result, error)",
		Documentation: "Issue a JSON-RPC request to the MCP server and wait for the response.\n\nparams may be a hash, array, or null. timeoutSec defaults to 60.\n\nResult is the parsed MCP \"result\" value mapped to a kLex object (typically a hash).\n\nError codes:\n  MCP_CALL_RPC    — server returned an error response (err.message has details)\n  MCP_CALL_CLOSED — connection was closed before/during the call. Timeouts\n                    also surface under this code (no distinct MCP_CALL_TIMEOUT\n                    exists today); inspect err.message if you need to\n                    distinguish a timeout from a hard close.\n\nFor standard MCP calls (tools/list, tools/call), prefer the typed helpers in stdlib/mcp.lex.",
		Params:        []string{"client", "method", "params", "timeoutSec"},
	},

	"_mcpNotify": {
		Signature:     "_mcpNotify(client: mcp_client, method: string, params: any) -> (null, error)",
		Documentation: "Fire a JSON-RPC notification (no id, no response expected). The server's reply (if any) goes to the notification channel rather than back to the caller.",
		Params:        []string{"client", "method", "params"},
	},

	"_mcpClose": {
		Signature:     "_mcpClose(client: mcp_client) -> null",
		Documentation: "Gracefully terminate the MCP server: close stdin so the server sees EOF, wait up to 2s, then SIGKILL. Idempotent — safe to call multiple times.",
		Params:        []string{"client"},
	},

	"_mcpInfo": {
		Signature:     "_mcpInfo(client: mcp_client) -> hash",
		Documentation: "Return server identity from the initialize handshake:\n  {name: string, version: string, protocol: string}\nAll fields default to \"\" if the server didn't supply them.",
		Params:        []string{"client"},
	},

	"_mcpNotifications": {
		Signature:     "_mcpNotifications(client: mcp_client) -> channel",
		Documentation: "Returns the kLex channel of server-initiated notifications. Each value is a hash {method: string, params: any} matching the JSON-RPC envelope. The channel is closed automatically when the server exits. Drain via recv() or recvNonBlock(); ignore if you don't need pushed events.",
		Params:        []string{"client"},
	},

	// ── MCP SERVER (the other side of the protocol) ─────────────────────────

	"_mcpServeHTTP": {
		Signature:     "_mcpServeHTTP(spec: hash) -> (server, error)",
		Documentation: "Start an MCP server (HTTP+SSE transport) so external agents (Claude Code, Claude Desktop, MCP Inspector) can drive this kLex program. Returns immediately; server runs on background goroutines.\n\nspec hash:\n  name    : string (required) — advertised via initialize\n  version : string (required) — advertised via initialize\n  port    : int (required) — TCP port on 127.0.0.1\n  tools   : hash (optional) — name → {description, schema, handler}\n\nPrefer mcp_server.serveHTTP(...) from stdlib/mcp_server.lex over calling this directly.",
		Params:        []string{"spec"},
	},

	"_mcpStopServer": {
		Signature:     "_mcpStopServer(server: mcp_server) -> null",
		Documentation: "Graceful shutdown: closes the listener, drops every SSE session, and waits up to 5s for in-flight tool calls to finish before forcing shutdown. Idempotent — safe to call multiple times.",
		Params:        []string{"server"},
	},

	"_mcpServerInfo": {
		Signature:     "_mcpServerInfo(server: mcp_server) -> hash",
		Documentation: "Snapshot of the server's current state: {name, version, port, tools (array of tool names), stopped (bool)}. Useful for logging, status pages, and tests.",
		Params:        []string{"server"},
	},

	// ── Binary I/O (additions for vector indexes, embeddings, etc.) ─────────

	"_fsReadBytes": {
		Signature:     "_fsReadBytes(path: string) -> (bytes, error)",
		Documentation: "Read entire file as raw bytes. Required for binary data (float32 vectors, images, encrypted blobs) that would not survive UTF-8 string coercion through _fsRead. Returns (Bytes, null) on success or (null, error) on failure.",
		Params:        []string{"path"},
	},
	"_fsAppendBytesSync": {
		Signature:     "_fsAppendBytesSync(path: string, bytes: bytes) -> (null, error)",
		Documentation: "Append bytes to a file and fsync(2) before close. Bytes are durably on disk when this function returns — safe to lose at most one call's worth of work on a power-loss crash. Use for append-only logs, vector indexes, audit trails. Adds ~1-5 ms per call; batch small writes into one call where possible.",
		Params:        []string{"path", "bytes"},
	},
	"floatsToBytes": {
		Signature:     "floatsToBytes(arr: array) -> bytes",
		Documentation: "Pack a kLex number array as little-endian IEEE-754 float32 bytes — the standard wire format for embeddings, scientific data, audio samples. Output length is 4 × len(arr). Integer elements are accepted and converted (so a literal [1, 2, 3] works). Round-trips with bytesToFloats. Same layout as Metal MTLBuffer<float>, so the same bytes can be passed straight to _mtlBuffer without repacking.",
		Params:        []string{"arr"},
	},
	"bytesToFloats": {
		Signature:     "bytesToFloats(b: bytes) -> (array, error)",
		Documentation: "Unpack little-endian IEEE-754 float32 bytes into a kLex array. Counterpart to floatsToBytes. len(b) must be a multiple of 4 — otherwise returns (null, error) with err.code = \"BYTES_LEN_INVALID\".",
		Params:        []string{"b"},
	},

	// ── Image (low-level pixel access) ──────────────────────────────────────

	"imageFromRgba": {
		Signature:     "imageFromRgba(bytes: bytes, width: int, height: int) -> image",
		Documentation: "Wrap raw RGBA8 pixel bytes (row-major, exactly width*height*4 bytes) into a kLex Image handle without going through PNG/JPEG decode. The counterpart to imageToRgba for sources that already have raw pixels — Metal surface readbacks, procedural generators, video frames, screen captures.\n\nPixels are copied into the Image's own buffer so the caller can mutate the source bytes afterwards without affecting the image.",
		Params:        []string{"bytes", "width", "height"},
	},
	"imageSize": {
		Signature:     "imageSize(img: image) -> (int, int)",
		Documentation: "Return the pixel dimensions of an Image as a two-element tuple (width, height). Cheap (struct field reads); safe to call before the image's first drawImage (no GPU upload required).",
		Params:        []string{"img"},
	},
	"imageToRgba": {
		Signature:     "imageToRgba(img: image) -> bytes",
		Documentation: "Return the Image's raw RGBA8 pixels (row-major, width*height*4 bytes). The natural counterpart to imageFromRgba — round-tripping imageFromRgba(imageToRgba(img), w, h) produces an equivalent image.\n\nSource priority:\n  1. img.pixels  — pre-GPU-upload CPU bytes (present after loadImage until the first drawImage uploads to a texture).\n  2. img.TextureID — once uploaded, reads back from the GPU via glGetTexImage. MUST be called from the GL thread (i.e. inside a draw frame).",
		Params:        []string{"img"},
	},

	// ── Image FX (cross-platform CPU pixel filters) ─────────────────────────
	// These are the cross-platform fallback path for kLex's image effects.
	// On macOS the `mtl_fx` stdlib prefers Metal compute kernels; on
	// Linux/Windows it calls these Go builtins. All take (bytes, w, h, …)
	// and return (Bytes, err). Alpha is preserved verbatim. Filters operate
	// on sRGB-encoded byte values (matching Photoshop/Lightroom defaults),
	// not on linear light.

	"_imgExposure": {
		Signature:     "_imgExposure(bytes: bytes, w: int, h: int, stops: float) -> (bytes, error)",
		Documentation: "Multiply each RGB channel by 2^stops. stops in roughly [-4, +4] — one stop doubles or halves linear-light intensity.",
		Params:        []string{"bytes", "w", "h", "stops"},
	},
	"_imgBrightness": {
		Signature:     "_imgBrightness(bytes: bytes, w: int, h: int, amount: float) -> (bytes, error)",
		Documentation: "Add `amount` to each RGB channel in [0,1] space. amount in [-1, +1]; 0 is no change. Additive, unlike exposure (multiplicative).",
		Params:        []string{"bytes", "w", "h", "amount"},
	},
	"_imgContrast": {
		Signature:     "_imgContrast(bytes: bytes, w: int, h: int, amount: float) -> (bytes, error)",
		Documentation: "Scale each channel around 0.5 mid-gray: out = (in - 0.5)*(1+amount) + 0.5. amount in [-1, +1]; 0 = no change, +1 doubles contrast, -1 flattens to mid-gray.",
		Params:        []string{"bytes", "w", "h", "amount"},
	},
	"_imgSaturation": {
		Signature:     "_imgSaturation(bytes: bytes, w: int, h: int, amount: float) -> (bytes, error)",
		Documentation: "Pull each pixel toward (amount < 0) or away from (amount > 0) its Rec.709 luma. amount in [-1, +1]: -1 fully desaturates, 0 no change, +1 doubles saturation (may clip on already-saturated pixels).",
		Params:        []string{"bytes", "w", "h", "amount"},
	},
	"_imgHueShift": {
		Signature:     "_imgHueShift(bytes: bytes, w: int, h: int, degrees: float) -> (bytes, error)",
		Documentation: "Rotate hue by `degrees` (any real number; wraps modulo 360). Routes through HSV — saturation and value are unchanged.",
		Params:        []string{"bytes", "w", "h", "degrees"},
	},
	"_imgGamma": {
		Signature:     "_imgGamma(bytes: bytes, w: int, h: int, gamma: float) -> (bytes, error)",
		Documentation: "Raise each channel to the power 1/gamma. gamma > 1 brightens midtones; gamma < 1 darkens them. Must be > 0.",
		Params:        []string{"bytes", "w", "h", "gamma"},
	},
	"_imgLevels": {
		Signature:     "_imgLevels(bytes: bytes, w: int, h: int, inBlack: float, inWhite: float, gamma: float, outBlack: float, outWhite: float) -> (bytes, error)",
		Documentation: "Classic Photoshop-style levels: remap [inBlack, inWhite] of the input to [outBlack, outWhite] of the output with an intermediate gamma adjustment. All five sliders are floats in [0, 1] (gamma is the usual \"midtone\" knob, > 0).",
		Params:        []string{"bytes", "w", "h", "inBlack", "inWhite", "gamma", "outBlack", "outWhite"},
	},
	"_imgChannelMixer": {
		Signature:     "_imgChannelMixer(bytes: bytes, w: int, h: int, matrix9: array) -> (bytes, error)",
		Documentation: "matrix9 is a flat 9-element array of floats representing a 3×3 row-major matrix applied to each pixel's RGB column: [rr,rg,rb, gr,gg,gb, br,bg,bb]. Identity matrix [1,0,0, 0,1,0, 0,0,1] = no change.",
		Params:        []string{"bytes", "w", "h", "matrix9"},
	},
	"_imgInvert": {
		Signature:     "_imgInvert(bytes: bytes, w: int, h: int) -> (bytes, error)",
		Documentation: "Negative — 1 - c on each RGB channel.",
		Params:        []string{"bytes", "w", "h"},
	},
	"_imgDesaturate": {
		Signature:     "_imgDesaturate(bytes: bytes, w: int, h: int) -> (bytes, error)",
		Documentation: "Convenience equivalent to saturation(-1) but cheaper — every channel becomes Rec.709 luma.",
		Params:        []string{"bytes", "w", "h"},
	},
	"_imgSepia": {
		Signature:     "_imgSepia(bytes: bytes, w: int, h: int, strength: float) -> (bytes, error)",
		Documentation: "Mix the original pixel with its sepia-toned counterpart by `strength` (0 = no change, 1 = full sepia). Sepia transform is the standard Photoshop matrix.",
		Params:        []string{"bytes", "w", "h", "strength"},
	},
	"_imgVignette": {
		Signature:     "_imgVignette(bytes: bytes, w: int, h: int, strength: float, radius: float) -> (bytes, error)",
		Documentation: "Darken pixels away from the image centre. strength in [0,1] is how dark the corners get (0 = no change, 1 = corners → black). radius in (0, ∞) controls how quickly the falloff starts; 0.5 is a tight vignette, 1.0 gentle, 1.5 only kisses the corners.",
		Params:        []string{"bytes", "w", "h", "strength", "radius"},
	},

	// ── Metal / GPU (macOS only — error tuple elsewhere) ────────────────────
	// Low-level Metal bindings. Most users want the higher-level wrappers in
	// stdlib/mtl.lex which call these. On non-macOS each builtin returns a
	// clear "Metal is macOS-only" error tuple.

	"_mtlInfo": {
		Signature:     "_mtlInfo() -> (hash, error)",
		Documentation: "Default-device capability hash:\n  {name, registry_id, has_unified_memory,\n   supports_raytracing, supports_apple7,\n   max_threads_per_group, max_bind_slots}\nUsed as both the \"is Metal available?\" probe and the canonical place to read capability flags before code that depends on them runs.",
		Params:        []string{},
	},

	"_mtlSurface": {
		Signature:     "_mtlSurface(width: int, height: int) -> (handle: int, error)",
		Documentation: "Allocate a fresh offscreen RGBA8 Metal texture (\"surface\"). Returns an integer handle the rest of the _mtl* API uses to refer to it. Pair every successful surface with _mtlSurfaceRelease.",
		Params:        []string{"width", "height"},
	},
	"_mtlSurfaceFromBytes": {
		Signature:     "_mtlSurfaceFromBytes(bytes: bytes, width: int, height: int) -> (handle: int, error)",
		Documentation: "Allocate a fresh RGBA8 surface and upload `bytes` (length must equal width*height*4) into it. Use to bring an in-memory image (e.g. a generated SD image) onto a Metal surface so a compute kernel can post-process it.",
		Params:        []string{"bytes", "width", "height"},
	},
	"_mtlSurfaceReadRgba": {
		Signature:     "_mtlSurfaceReadRgba(handle: int) -> (bytes, error)",
		Documentation: "Read the surface's pixels back into a kLex bytes value (RGBA8 row-major, width*height*4 bytes). Counterpart to _mtlSurfaceFromBytes — completes the round-trip for image post-processing pipelines. On Apple Silicon (unified memory) the read is zero-copy (memcpy from shared storage).",
		Params:        []string{"handle"},
	},
	"_mtlSurfaceSavePng": {
		Signature:     "_mtlSurfaceSavePng(handle: int, path: string) -> channel",
		Documentation: "Async — read the surface pixels, PNG-encode, write to disk. Returns a single-shot channel that emits (null, null) on success or (null, err) on failure.",
		Params:        []string{"handle", "path"},
	},
	"_mtlClear": {
		Signature:     "_mtlClear(handle: int, color: array) -> channel",
		Documentation: "Async clear-to-color. color is a 4-element [r, g, b, a] array of floats in [0, 1]. Returns a single-shot channel that emits (null, null) on success or (null, err) on failure.",
		Params:        []string{"handle", "color"},
	},
	"_mtlSurfaceRelease": {
		Signature:     "_mtlSurfaceRelease(handle: int) -> null",
		Documentation: "Drop the bridge's retain on the surface so the GPU memory can be freed. Idempotent — safe to call with an unknown handle.",
		Params:        []string{"handle"},
	},

	"_mtlKernel": {
		Signature:     "_mtlKernel(mslSource: string, fnName: string) -> (handle: int, error)",
		Documentation: "Compile MSL source and create an MTLComputePipelineState for the named entry-point function. The returned handle is reusable across many dispatches.",
		Params:        []string{"mslSource", "fnName"},
	},
	"_mtlKernelRelease": {
		Signature:     "_mtlKernelRelease(kernel) -> null",
		Documentation: "Drops the bridge's retain on the compiled pipeline state.\nIdempotent — unknown handles are no-ops.",
		Params:        []string{"kernel"},
	},

	"_mtlBuffer": {
		Signature:     "_mtlBuffer(arr: array) -> (handle: int, error)",
		Documentation: "Create an MTLBuffer of float32 elements from a kLex number array. Bytes are uploaded once at create time; the buffer is then bindable to compute dispatches.",
		Params:        []string{"arr"},
	},
	"_mtlBufferFromTensor": {
		Signature:     "_mtlBufferFromTensor(t: tensor) -> (handle: int, error)",
		Documentation: "Zero-marshalling MTLBuffer upload from a FrogPy f32 tensor's underlying []float32 slice. Skips the per-element Float-to-float32 conversion that _mtlBuffer does for kLex Arrays — ~5x faster at 1024^2. Tensor must be DType f32 and contiguous; other dtypes / strided views error cleanly.",
		Params:        []string{"t"},
	},
	"_mtlBufferU32": {
		Signature:     "_mtlBufferU32(arr: array) -> (handle: int, error)",
		Documentation: "Same as _mtlBuffer but the buffer holds uint32 elements — for index buffers, atomic counters, packed flags, etc.",
		Params:        []string{"arr"},
	},
	"_mtlBufferAllocF32": {
		Signature:     "_mtlBufferAllocF32(count: int) -> (handle: int, error)",
		Documentation: "Allocate an MTLBuffer of `count` zeroed float32 slots without an initial upload. Use for output buffers a kernel will write into.",
		Params:        []string{"count"},
	},
	"_mtlBufferAllocU32": {
		Signature:     "_mtlBufferAllocU32(count: int) -> (handle: int, error)",
		Documentation: "Allocate an MTLBuffer of `count` zeroed uint32 slots without an initial upload. Use for output index / counter buffers.",
		Params:        []string{"count"},
	},
	"_mtlReadBuffer": {
		Signature:     "_mtlReadBuffer(handle: int) -> (array, error)",
		Documentation: "Read a float32 MTLBuffer's contents back into a kLex float array. Pair with _mtlBufferAllocF32 + _mtlDispatch for compute results.",
		Params:        []string{"handle"},
	},
	"_mtlReadBufferU32": {
		Signature:     "_mtlReadBufferU32(handle: int) -> (array, error)",
		Documentation: "Read a uint32 MTLBuffer's contents back into a kLex int array.",
		Params:        []string{"handle"},
	},
	"_mtlReadBufferIntoTensor": {
		Signature:     "_mtlReadBufferIntoTensor(handle: int, t: tensor) -> (null, error)",
		Documentation: "Zero-allocation readback: copies a float32 MTLBuffer directly into a pre-allocated f32 FrogPy tensor's underlying []float32 slice. Skips the per-element Float construction _mtlReadBuffer pays — ~7ms saved per 1M cells. Tensor must be f32, contiguous, and sized exactly to the buffer's element count.",
		Params:        []string{"handle", "t"},
	},
	"_mtlBufferRelease": {
		Signature:     "_mtlBufferRelease(handle: int) -> null",
		Documentation: "Free an MTLBuffer. Idempotent — safe with unknown handles. Phase-F buffer pool recycles same-size buffers internally; release pushes the buffer back into the pool rather than freeing it outright.",
		Params:        []string{"handle"},
	},

	"_mtlDispatch": {
		Signature:     "_mtlDispatch(kernel: int, bindings: hash, grid: array) -> channel",
		Documentation: "Async compute dispatch. bindings is a hash with optional keys:\n  textures: [tex_handle, ...]  → bound to MSL [[texture(i)]]\n  buffers:  [buf_handle, ...]  → bound to MSL [[buffer(i)]]\n  accels:   [accel_handle, ...] → bound past the last buffer index\ngrid is [gx, gy, gz] thread count (Metal does the threadgroup math). Returns a single-shot channel that emits (null, null) or (null, err).",
		Params:        []string{"kernel", "bindings", "grid"},
	},

	"_mtlBatchBegin": {
		Signature:     "_mtlBatchBegin() -> (batch: int, error)",
		Documentation: "Open a fresh batch command buffer. Subsequent _mtlBatchDispatch calls encode compute passes onto it WITHOUT committing. Pair every successful begin with either _mtlBatchCommit (run them) or _mtlBatchRelease (abandon them) — leaking a batch leaks an MTLCommandBuffer.",
		Params:        []string{},
	},
	"_mtlBatchDispatch": {
		Signature:     "_mtlBatchDispatch(batch: int, kernel: int, bindings: hash, grid: array) -> (null, error)",
		Documentation: "Encode one compute dispatch onto an open batch's command buffer. Same bindings/grid shape as _mtlDispatch. Synchronous — the GPU work doesn't run until _mtlBatchCommit.",
		Params:        []string{"batch", "kernel", "bindings", "grid"},
	},
	"_mtlBatchCommit": {
		Signature:     "_mtlBatchCommit(batch: int) -> channel",
		Documentation: "Async — close the batch, submit its command buffer to the GPU, and wait. Returns a single-shot channel that emits (null, null) or (null, err). Releases the batch handle either way.",
		Params:        []string{"batch"},
	},
	"_mtlBatchRelease": {
		Signature:     "_mtlBatchRelease(batch: int) -> null",
		Documentation: "Abandon a batch without committing. Discards any encoded dispatches and frees the underlying MTLCommandBuffer. Idempotent — releasing an unknown handle is a no-op. Pair with _mtlBatchBegin in error paths so a failed mid-batch dispatch doesn't leak the cb.",
		Params:        []string{"batch"},
	},

	"_mtlMatmulMPS": {
		Signature:     "_mtlMatmulMPS(a: int, b: int, c: int, m: int, k: int, n: int) -> channel",
		Documentation: "Async MPSMatrixMultiplication: C := A · B. a, b, c are MTLBuffer handles (float32). A is [m × k] row-major, B is [k × n] row-major, C is overwritten in place. Apple's MPS kernels use simdgroup matrix instructions and beat naive matmul by 10-20× for n ≥ 256.",
		Params:        []string{"a", "b", "c", "m", "k", "n"},
	},
	"_mtlFFT": {
		Signature:     "_mtlFFT(in_handle, out_handle, n, inverse) -> Channel of (null, err)",
		Documentation: "1D complex-to-complex Fast Fourier Transform via MPSGraph.\nBoth buffers hold 2n floats in interleaved (re, im) layout —\nsame byte format as NumPy / SciPy complex64 arrays.\n\n  inverse=0  →  forward FFT\n  inverse=1  →  inverse FFT (caller divides by n for a true inverse)\n\nAsync via runAsyncSingle.",
		Params:        []string{"in_handle", "out_handle", "n", "inverse"},
	},

	"_mtlAccel": {
		Signature:     "_mtlAccel(vertex_buffer: int, vertex_count: int) -> channel",
		Documentation: "Build a primitive MTLAccelerationStructure from a flat triangle list. vertex_count must be a positive multiple of 3. Required for ray-tracing kernels. Note: sharing exact-equal vertex positions across triangles in an UNINDEXED AS breaks the BVH on some chips — use _mtlAccelIndexed instead if you have shared verts.",
		Params:        []string{"vertex_buffer", "vertex_count"},
	},
	"_mtlAccelIndexed": {
		Signature:     "_mtlAccelIndexed(vertex_buffer: int, vertex_count: int, index_buffer: int, index_count: int) -> channel",
		Documentation: "Indexed-triangle variant of _mtlAccel. index_buffer is a uint32 MTLBuffer; index_count must be a positive multiple of 3. Required when triangles share vertex positions.",
		Params:        []string{"vertex_buffer", "vertex_count", "index_buffer", "index_count"},
	},
	"_mtlAccelRelease": {
		Signature:     "_mtlAccelRelease(handle: int) -> null",
		Documentation: "Free an MTLAccelerationStructure. Note: the underlying vertex/index buffers are NOT auto-released — release them separately via _mtlBufferRelease.",
		Params:        []string{"handle"},
	},

	// FrogPy tensor — in-place element-wise ops.
	// Each variant mutates its first argument and returns the same tensor reference,
	// skipping the fresh allocation + zero-init of the non-inplace path.
	// Measured on M4 (2026-05-23): ~2× throughput vs allocating variant; the
	// 100K f64 add hits 95 GB/s, near M4's ~100 GB/s memory ceiling.
	// Stdlib wrapper: add_inplace(a, b) etc. in stdlib/tensor.lex.

	"_tensor_add_inplace": {
		Signature:     "_tensor_add_inplace(a: tensor, b: tensor) -> tensor",
		Documentation: "In-place element-wise sum: a[i] = a[i] + b[i]. Returns a. Same dtype + shape rules as _tensor_add. Skips the fresh-allocation tax of the non-inplace variant — ~2× faster on M4.",
		Params:        []string{"a", "b"},
	},
	"_tensor_sub_inplace": {
		Signature:     "_tensor_sub_inplace(a: tensor, b: tensor) -> tensor",
		Documentation: "In-place element-wise difference: a[i] = a[i] - b[i]. Returns a. Same dtype + shape rules as _tensor_sub.",
		Params:        []string{"a", "b"},
	},
	"_tensor_mul_inplace": {
		Signature:     "_tensor_mul_inplace(a: tensor, b: tensor) -> tensor",
		Documentation: "In-place element-wise product: a[i] = a[i] * b[i]. Returns a. Integer overflow wraps.",
		Params:        []string{"a", "b"},
	},
	"_tensor_div_inplace": {
		Signature:     "_tensor_div_inplace(a: tensor, b: tensor) -> tensor",
		Documentation: "In-place element-wise quotient: a[i] = a[i] / b[i]. Returns a. Float: IEEE 754 (n/0 = ±Inf, 0/0 = NaN). Integer: division-by-zero is rejected with a clean error reporting the offending index.",
		Params:        []string{"a", "b"},
	},
	"_tensor_pow_inplace": {
		Signature:     "_tensor_pow_inplace(a: tensor, b: tensor) -> tensor",
		Documentation: "In-place element-wise exponentiation: a[i] = a[i] ** b[i]. Returns a. Float uses libm pow/powf. Integer: exponentiation by squaring; negative exponents are rejected (result <1 isn't representable in i64).",
		Params:        []string{"a", "b"},
	},
	"_tensor_neg_inplace": {
		Signature:     "_tensor_neg_inplace(a: tensor) -> tensor",
		Documentation: "In-place element-wise negation: a[i] = -a[i]. Returns a. Works on f32/f64/i64.",
		Params:        []string{"a"},
	},
	"_tensor_abs_inplace": {
		Signature:     "_tensor_abs_inplace(a: tensor) -> tensor",
		Documentation: "In-place element-wise absolute value. Returns a. Works on f32/f64/i64. For i64, INT64_MIN wraps to itself (matches NumPy).",
		Params:        []string{"a"},
	},
	"_tensor_exp_inplace": {
		Signature:     "_tensor_exp_inplace(a: tensor) -> tensor",
		Documentation: "In-place element-wise exp(a). Returns a. Float-only (f32/f64); rejects i64 with a clean error.",
		Params:        []string{"a"},
	},
	"_tensor_log_inplace": {
		Signature:     "_tensor_log_inplace(a: tensor) -> tensor",
		Documentation: "In-place element-wise log(a). Returns a. Float-only. Negative inputs produce NaN silently (IEEE 754).",
		Params:        []string{"a"},
	},
	"_tensor_sqrt_inplace": {
		Signature:     "_tensor_sqrt_inplace(a: tensor) -> tensor",
		Documentation: "In-place element-wise sqrt(a). Returns a. Float-only. Negative inputs produce NaN silently (IEEE 754).",
		Params:        []string{"a"},
	},
	"_tensor_sin_inplace": {
		Signature:     "_tensor_sin_inplace(a: tensor) -> tensor",
		Documentation: "In-place element-wise sin(a). Returns a. Float-only.",
		Params:        []string{"a"},
	},
	"_tensor_cos_inplace": {
		Signature:     "_tensor_cos_inplace(a: tensor) -> tensor",
		Documentation: "In-place element-wise cos(a). Returns a. Float-only.",
		Params:        []string{"a"},
	},

	// ── Agentic runtime hooks (Phase 1 + 2) ─────────────────────────────
	// Prefer the friendlier wrappers in stdlib/agent.lex
	// (agent.onErrorBubble / onAsyncSpawn / onAsyncDone +
	// clearXxx variants) rather than calling these primitives directly.

	"_setErrorHook": {
		Signature:     "_setErrorHook(hookFn: fn | null) -> null",
		Documentation: "Phase 1 agentic hook primitive. Registers `hookFn` (or null to clear) as the on_error_bubble handler. The hook fires once per internal *Error at the moment it begins propagating up the call stack — BEFORE any safe() / `?` / top-level handler sees it. Observer-only — returning a value from the hook or producing an error inside it does not alter the original error.\n\nEvent hash shape passed to the hook:\n  {\n    \"kind\":    \"TypeError\" | \"RuntimeError\",\n    \"message\": \"<the error message>\",\n    \"code\":    \"<user code if any, else ''>\",\n    \"line\":    <1-based line where the error originated>,\n    \"stack\":   [{\"line\": N}, ...]   // innermost first, VM only; tree-walker leaves empty\n  }\n\nRe-entry: if the hook itself errors, the runtime skips the recursive call — no infinite loop.\n\nUser errors from error() do NOT fire the hook (they're first-class values, not propagation signals).\n\nPrefer agent.onErrorBubble(hookFn) instead of calling this directly.",
		Params:        []string{"hookFn"},
	},

	"_setAsyncSpawnHook": {
		Signature:     "_setAsyncSpawnHook(hookFn: fn | null) -> null",
		Documentation: "Phase 2 agentic hook primitive. Registers `hookFn` (or null to clear) as the on_async_spawn handler. Invoked SYNCHRONOUSLY in the spawning goroutine BEFORE the task's goroutine starts running. Each spawn carries a monotonic task_id that pairs with the on_async_done event for the same task.\n\nEvent hash shape:\n  {\n    \"task_id\":    42,            // monotonic uint64, unique per process\n    \"fn\":         \"enhance\",     // closure's name, \"<anon>\" / \"<closure>\" / \"<builtin>\" / \"<external>\"\n    \"argc\":       1,             // arg count (not the args themselves)\n    \"spawned_at\": 1700000000000000000   // unix nanoseconds at spawn\n  }\n\nPrefer agent.onAsyncSpawn(hookFn) instead of calling this directly.",
		Params:        []string{"hookFn"},
	},

	"_setAsyncDoneHook": {
		Signature:     "_setAsyncDoneHook(hookFn: fn | null) -> null",
		Documentation: "Phase 2 agentic hook primitive. Registers `hookFn` (or null to clear) as the on_async_done handler. Invoked from the TASK'S OWN goroutine immediately after the body returns. Use task_id to pair with the earlier on_async_spawn event.\n\nEvent hash shape:\n  {\n    \"task_id\":     42,\n    \"duration_ms\": 1342,        // wall time the goroutine ran\n    \"ok\":          true,        // false when the task threw an internal error\n    \"error\":       null         // or {\"kind\": ..., \"message\": ...} when ok=false\n  }\n\nUser errors from error() do NOT mark the task as failed (ok stays true).\n\nPrefer agent.onAsyncDone(hookFn) instead of calling this directly.",
		Params:        []string{"hookFn"},
	},

	"_setUiEventHook": {
		Signature:     "_setUiEventHook(hookFn: fn | null) -> null",
		Documentation: "Phase 3 agentic hook primitive. Registers `hookFn` (or null to clear) as the on_ui_event handler. Fired SYNCHRONOUSLY by interactive widgets in stdlib/ui.lex (button / slider / checkbox / dropdown / textField / toggle / radio) when the user causes a state change. Pure-display widgets do NOT fire. Hover events are not emitted.\n\nEvent hash shape:\n  {\n    \"kind\":   \"click\" | \"drag\" | \"toggle\" | \"select\" | \"text\",\n    \"widget\": \"button\" | \"slider\" | \"checkbox\" | \"dropdown\" | \"textField\" | \"toggle\" | \"radio\",\n    \"label\":  \"<the widget's label, or '' if none>\",\n    \"value\":  <the new value the interaction produced, or null for plain clicks>,\n    \"x\":      <int mouse x>, \"y\": <int mouse y>\n  }\n\nPrefer agent.onUiEvent(hookFn) instead of calling this directly.",
		Params:        []string{"hookFn"},
	},

	"_setBridgeCallHook": {
		Signature:     "_setBridgeCallHook(hookFn: fn | null) -> null",
		Documentation: "Phase 3 agentic hook primitive. Registers `hookFn` (or null to clear) as the on_bridge_call handler. Fires once per `_bridgeCall` AFTER the round-trip completes (bridge calls are synchronous from kLex's view).\n\nEvent hash shape:\n  {\n    \"fn\":          \"enhance_prompt\",      // remote function name\n    \"argc\":        2,                     // arg count\n    \"duration_ms\": 1342,                  // wall-clock round-trip\n    \"ok\":          true,                  // false if the bridge transport failed (crash, timeout, etc.)\n    \"error\":       null                   // or {\"kind\": ..., \"message\": ..., \"code\": ...} when ok=false\n    \"user_error\":  null                   // or {\"kind\": ..., \"message\": ..., \"code\": ...} when the\n                                          // remote handler threw / returned a (null, err) tuple;\n                                          // ok stays true — the round-trip itself succeeded\n  }\n\nok=false = bridge transport failure. ok=true + user_error != null = handler returned an error. ok=true + user_error == null = clean success.\n\nPrefer agent.onBridgeCall(hookFn) instead of calling this directly.",
		Params:        []string{"hookFn"},
	},

	"_uiEventActive": {
		Signature:     "_uiEventActive() -> bool",
		Documentation: "Returns true when an on_ui_event hook is currently registered. Used by stdlib/ui.lex widget instrumentation as a fast gate: when no hook is registered, widgets skip event-hash construction entirely (single atomic pointer load returns false). Cheap to call from hot UI code.",
		Params:        []string{},
	},

	"_uiEvent": {
		Signature:     "_uiEvent(kind: string, widget: string, label: string, value: any, x: int, y: int) -> null",
		Documentation: "Producer-side primitive for emitting on_ui_event events. Called by instrumented widgets in stdlib/ui.lex when the user causes a state change; the runtime dispatches the event to the registered handler (if any). When no hook is registered, returns immediately without building the event hash.\n\nDirect-use is fine for custom widgets — pass the documented `kind` and `widget` vocabulary so subscribers can pattern-match consistently. See agent.onUiEvent for the event shape.",
		Params:        []string{"kind", "widget", "label", "value", "x", "y"},
	},

	// ── Iteration prep helper ───────────────────────────────────────────
	"_iterPrep": {
		Signature:     "_iterPrep(coll: array | hash | string | bytes | tuple | channel | concurrentHash, twoVar: bool) -> (iterArray, isPairs)",
		Documentation: "Internal helper used by the VM's compileForIn lowering. Normalises an iterable into a (iterArray, isPairs) tuple the VM's index-based loop can consume.\n\n  - For *Array / *String / *Bytes / *Tuple / *Channel / *ConcurrentHash: pass-through; isPairs is FALSE; the VM's existing index-based loop handles iteration.\n  - For *Hash with twoVar=true: returns ([[k, v], ...], TRUE). The VM sees isPairs=true and unpacks each iter element as a 2-tuple into the loop's two binding slots.\n  - For *Hash with twoVar=false: returns an Error (matches the tree-walker's single-var hash-iteration rejection).\n  - For any other type: returns an Error.\n\nNot intended for direct kLex use — emitted by the compiler.",
		Params:        []string{"coll", "twoVar"},
	},

	// ── Process spawning ────────────────────────────────────────────────
	"_processSpawnDetached": {
		Signature:     "_processSpawnDetached(cmd: string, args: array, opts?: hash) -> (pid: int, err)",
		Documentation: "Start a child process detached from the parent. Returns the child's PID on success. The child survives the parent — typical use is \"spawn a background daemon and forget it.\" Unlike _processExec / _processRun, this returns IMMEDIATELY after the OS reports the process as started; the parent does NOT block on the child's exit.\n\nopts (hash, all optional):\n  logFile : string             — append child stdout AND stderr to this path. Strongly recommended for daemons: without it, writes block once the OS pipe buffer fills (typically ~64KB).\n  env     : hash string→string — extra env vars added to the inherited env\n  dir     : string             — child's working directory (default: cwd)\n\nReturns (pid, null) on success or (null, errMsg) if the OS could not start the process. The child is reaped in a background goroutine so callers don't leak zombies — there is no API to wait on the child; callers should detect liveness via the daemon's own heartbeat file.",
		Params:        []string{"cmd", "args", "opts?"},
	},

	// ── Tensor: pure (non-inplace) element-wise + reductions ──────────
	// Pure variants allocate a fresh result tensor (matching NumPy
	// semantics). Pair with *_inplace variants when the caller owns the
	// LHS and wants to avoid allocation.

	"_tensor_abs": {
		Signature:     "_tensor_abs(a: tensor) -> tensor",
		Documentation: "Element-wise abs(a). Returns a fresh tensor with the same shape and dtype as a.",
		Params:        []string{"a"},
	},
	"_tensor_neg": {
		Signature:     "_tensor_neg(a: tensor) -> tensor",
		Documentation: "Element-wise -a. Returns a fresh tensor with the same shape and dtype as a.",
		Params:        []string{"a"},
	},
	"_tensor_exp": {
		Signature:     "_tensor_exp(a: tensor) -> tensor",
		Documentation: "Element-wise exp(a). Returns a fresh tensor with the same shape as a. Float-only.",
		Params:        []string{"a"},
	},
	"_tensor_log": {
		Signature:     "_tensor_log(a: tensor) -> tensor",
		Documentation: "Element-wise log(a) (natural log). Returns a fresh tensor with the same shape as a. Float-only. Negative inputs produce NaN silently (IEEE 754).",
		Params:        []string{"a"},
	},
	"_tensor_sqrt": {
		Signature:     "_tensor_sqrt(a: tensor) -> tensor",
		Documentation: "Element-wise sqrt(a). Returns a fresh tensor with the same shape as a. Float-only. Negative inputs produce NaN silently (IEEE 754).",
		Params:        []string{"a"},
	},
	"_tensor_sin": {
		Signature:     "_tensor_sin(a: tensor) -> tensor",
		Documentation: "Element-wise sin(a). Returns a fresh tensor with the same shape as a. Float-only.",
		Params:        []string{"a"},
	},
	"_tensor_cos": {
		Signature:     "_tensor_cos(a: tensor) -> tensor",
		Documentation: "Element-wise cos(a). Returns a fresh tensor with the same shape as a. Float-only.",
		Params:        []string{"a"},
	},
	"_tensor_pow": {
		Signature:     "_tensor_pow(a: tensor | number, b: tensor | number) -> tensor",
		Documentation: "Element-wise pow(a, b). NumPy-style broadcasting: either operand may be a scalar (Integer or Float) or a tensor with a broadcast-compatible shape. At least one operand must be a tensor. Dtypes must match after scalar promotion. Float: libm pow / NaN on negative^non-integer. Integer: negative exponents rejected (not representable in i64).",
		Params:        []string{"a", "b"},
	},
	"_tensor_add": {
		Signature:     "_tensor_add(a: tensor | number, b: tensor | number) -> tensor",
		Documentation: "Element-wise a + b. NumPy-style broadcasting: either operand may be a scalar (Integer or Float) or a tensor with a broadcast-compatible shape (e.g. (m,n) + (n,) or (m,1) + (1,n)). At least one operand must be a tensor. Dtypes must match after scalar promotion.",
		Params:        []string{"a", "b"},
	},
	"_tensor_sub": {
		Signature:     "_tensor_sub(a: tensor | number, b: tensor | number) -> tensor",
		Documentation: "Element-wise a - b. NumPy-style broadcasting; see _tensor_add for rules.",
		Params:        []string{"a", "b"},
	},
	"_tensor_mul": {
		Signature:     "_tensor_mul(a: tensor | number, b: tensor | number) -> tensor",
		Documentation: "Element-wise a * b. NumPy-style broadcasting; see _tensor_add for rules. Integer overflow wraps.",
		Params:        []string{"a", "b"},
	},
	"_tensor_div": {
		Signature:     "_tensor_div(a: tensor | number, b: tensor | number) -> tensor",
		Documentation: "Element-wise a / b. NumPy-style broadcasting; see _tensor_add for rules. Float: IEEE 754 (n/0 = +/-Inf, 0/0 = NaN). Integer: division by zero is rejected pre-kernel with the offending index.",
		Params:        []string{"a", "b"},
	},

	// ── Tensor: reductions ──────────────────────────────────────────────
	"_tensor_sum": {
		Signature:     "_tensor_sum(t: tensor) -> number",
		Documentation: "Sum of all elements across the tensor. Accumulator type matches t's dtype (f32 → f32 sum, f64 → f64 sum, i64 → i64 sum with wrap-around at overflow).",
		Params:        []string{"t"},
	},
	"_tensor_mean": {
		Signature:     "_tensor_mean(t: tensor) -> float",
		Documentation: "Mean of all elements. Computed in Go as sum/n. f32 stays in float, f64 stays in double, i64 widens to float64 for the divide (matches NumPy's np.mean on integer arrays).",
		Params:        []string{"t"},
	},
	"_tensor_max": {
		Signature:     "_tensor_max(t: tensor) -> number",
		Documentation: "Maximum element across the tensor. Return type matches t's dtype. Returns 0 (or +0.0) on an empty tensor.",
		Params:        []string{"t"},
	},
	"_tensor_min": {
		Signature:     "_tensor_min(t: tensor) -> number",
		Documentation: "Minimum element across the tensor. Return type matches t's dtype. Returns 0 (or +0.0) on an empty tensor.",
		Params:        []string{"t"},
	},
	"_tensor_argmax": {
		Signature:     "_tensor_argmax(t: tensor) -> int",
		Documentation: "Index of the maximum element treating t as a flat 1-D view of its storage. First occurrence wins on ties.",
		Params:        []string{"t"},
	},
	"_tensor_argmin": {
		Signature:     "_tensor_argmin(t: tensor) -> int",
		Documentation: "Index of the minimum element treating t as a flat 1-D view of its storage. First occurrence wins on ties.",
		Params:        []string{"t"},
	},

	// ── Tensor: introspection + construction ────────────────────────────
	"_tensor_dtype": {
		Signature:     "_tensor_dtype(t: tensor) -> string",
		Documentation: "Returns the dtype name (\"f32\", \"f64\", \"i64\").",
		Params:        []string{"t"},
	},
	"_tensor_shape": {
		Signature:     "_tensor_shape(t: tensor) -> array",
		Documentation: "Returns a fresh kLex array of integers describing t's shape.",
		Params:        []string{"t"},
	},
	"_tensor_numel": {
		Signature:     "_tensor_numel(t: tensor) -> int",
		Documentation: "Total element count (product of shape).",
		Params:        []string{"t"},
	},
	"_tensor_get": {
		Signature:     "_tensor_get(t: tensor, idx: int) -> number",
		Documentation: "Linear element access — treats the tensor as a flat 1-D view of its logical data. Used by stdlib/tensor.lex's preview/print helpers and by users who want a single scalar out without computing the multi-index. Bounds-checked. Works on strided views (from _tensor_slice) — the flat index is translated through Strides to the physical backing position.",
		Params:        []string{"t", "idx"},
	},
	"_tensor_slice": {
		Signature:     "_tensor_slice(t: tensor, specs: array) -> tensor",
		Documentation: "Returns a VIEW into t — a new tensor with its own Shape/Strides but sharing t's backing data slice. specs is an array with one entry per axis: each entry is null (full axis) or [start, stop, step]. Negative start/stop count from the end; any of start/stop/step may itself be null for the default (0/dim/1). step must be positive in v1 (no reversed slicing yet).\n\nViews share storage — mutations propagate. Kernel ops (add, matmul, reductions) require contiguous input — call _tensor_contiguous(view) before passing to a kernel. Inspection ops (get, to_array, shape, dtype, numel, clone) work on views directly.\n\nSource must be contiguous in v1 — slicing-a-slice requires t.contiguous() in between.",
		Params:        []string{"t", "specs"},
	},
	"_tensor_contiguous": {
		Signature:     "_tensor_contiguous(t: tensor) -> tensor",
		Documentation: "If t is already contiguous returns t unchanged (no copy — matches NumPy ascontiguousarray fast path). If t is a strided view (from _tensor_slice) walks the multi-index and copies the logical data into a fresh contiguous tensor. Required before passing a slice view to any kernel-based op.",
		Params:        []string{"t"},
	},
	"_tensor_from_array": {
		Signature:     "_tensor_from_array(data: array, dtype: string) -> tensor",
		Documentation: "Converts a flat 1-D kLex array (of numbers) to a 1-D tensor of the given dtype. Element conversion is strict: Float64 elements convert losslessly to f64, are truncated to f32, and rejected for i64; Integer elements convert losslessly to i64 and as floats for f32/f64. Mixed-type arrays where some element doesn't fit the target dtype error cleanly.\n\nFor multi-dim construction in v1, build a 1-D tensor here then reshape (TBD). Keeps the v1 surface tight.",
		Params:        []string{"data", "dtype"},
	},
	"_tensor_zeros": {
		Signature:     "_tensor_zeros(shape: array, dtype: string) -> tensor",
		Documentation: "shape is a kLex array of integers; dtype is one of \"f32\"/\"float32\", \"f64\"/\"float64\", \"i64\"/\"int64\". Allocates a fresh contiguous tensor with all elements set to zero (Go's slice-of-numeric-type default).",
		Params:        []string{"shape", "dtype"},
	},
	"_tensor_full": {
		Signature:     "_tensor_full(shape: array, value: number, dtype: string) -> tensor",
		Documentation: "Allocates a fresh contiguous tensor with every element set to value. NumPy equivalent: np.full. Value conversion: f32 / f64 accept Integer or Float; i64 accepts Integer only (Float rejected without explicit conversion).",
		Params:        []string{"shape", "value", "dtype"},
	},
	"_tensor_random": {
		Signature:     "_tensor_random(shape: array, dtype: string, seed: int) -> tensor",
		Documentation: "Allocates a tensor filled with pseudo-random values. Float dtypes get uniform [0, 1) (NumPy np.random.rand semantics); i64 gets the full int63 range. seed > 0 → deterministic; seed == 0 → time-seeded. Uses Go math/rand; not cryptographic.",
		Params:        []string{"shape", "dtype", "seed"},
	},
	"_tensor_matmul": {
		Signature:     "_tensor_matmul(a: tensor, b: tensor) -> tensor",
		Documentation: "Matrix multiply C := A · B. Shapes: a is [m, k], b is [k, n], result is [m, n]. Both inputs must be 2-D, contiguous, same dtype (no promotion in v1). Dispatch: f32 routes through Apple MPSMatrixMultiplication on macOS (GPU; wins decisively above ~512³), naive CPU autovec kernel on Linux. f64 / i64 always use the CPU kernel — MPS is f32-only at the bridge surface.",
		Params:        []string{"a", "b"},
	},
	"_tensor_reshape": {
		Signature:     "_tensor_reshape(t: tensor, newShape: array) -> tensor",
		Documentation: "Returns a new tensor with the requested shape, sharing the backing data slice with t (NumPy-style view). product(newShape) must equal numel(t). v1 only handles contiguous inputs; non-contiguous tensors error cleanly. Because storage is shared, in-place ops on either alias mutate both.",
		Params:        []string{"t", "newShape"},
	},
	"_tensor_transpose": {
		Signature:     "_tensor_transpose(t: tensor) -> tensor",
		Documentation: "2-D matrix transpose: shape [m, n] -> [n, m], out[i, j] = in[j, i]. Materialising — returns a fresh contiguous tensor (does NOT share storage). Use directly as input to matmul for the common A·Bᵀ pattern. v1 is 2-D only; N-D axis-permutation form deferred.",
		Params:        []string{"t"},
	},
	"_tensor_squeeze": {
		Signature:     "_tensor_squeeze(t: tensor) -> tensor",
		Documentation: "Returns a view of t with all size-1 dimensions removed. Shares backing data with t. NumPy parallel: np.squeeze (without axis= argument).",
		Params:        []string{"t"},
	},
	"_tensor_expand_dims": {
		Signature:     "_tensor_expand_dims(t: tensor, axis: int) -> tensor",
		Documentation: "Returns a view of t with a new size-1 dimension inserted at axis. axis may be negative (counts from end). Valid range: [-len(shape)-1, len(shape)]. Shares backing data with t. NumPy parallel: np.expand_dims.",
		Params:        []string{"t", "axis"},
	},
	"_tensor_dot": {
		Signature:     "_tensor_dot(a: tensor, b: tensor) -> number",
		Documentation: "1-D inner product: sum(a[i] * b[i]). Both inputs must be 1-D, same dtype, same length, contiguous. Returns a scalar (Float for f32/f64, Integer for i64). Fused mul+sum in a single C pass; no temporary tensor allocated. For 2-D matrix multiplication use _tensor_matmul. NumPy parallel: np.dot for the 1-D × 1-D case.",
		Params:        []string{"a", "b"},
	},
	"_tensor_sum_axis": {
		Signature:     "_tensor_sum_axis(t: tensor, axis: int) -> tensor",
		Documentation: "Sum along one axis of any-rank tensor. Output has rank len(t.shape)-1 with the reduced axis dropped. axis may be negative (-1 = last). Output is same dtype as input. Empty axis returns zeros (sum identity). NumPy parallel: np.sum(t, axis=N).",
		Params:        []string{"t", "axis"},
	},
	"_tensor_sum_axis_keepdims": {
		Signature:     "_tensor_sum_axis_keepdims(t: tensor, axis: int) -> tensor",
		Documentation: "Same as _tensor_sum_axis but the output keeps the reduced axis at size 1 instead of dropping it. Composes cleanly with broadcasting. NumPy parallel: np.sum(t, axis=N, keepdims=True).",
		Params:        []string{"t", "axis"},
	},
	"_tensor_mean_axis": {
		Signature:     "_tensor_mean_axis(t: tensor, axis: int) -> tensor",
		Documentation: "Mean along one axis of any-rank tensor. Output rank is len(t.shape)-1 with the reduced axis dropped. Output is always f64 dtype (matches scalar mean's policy and NumPy's np.mean on integer arrays). Empty axis errors cleanly. axis may be negative.",
		Params:        []string{"t", "axis"},
	},
	"_tensor_mean_axis_keepdims": {
		Signature:     "_tensor_mean_axis_keepdims(t: tensor, axis: int) -> tensor",
		Documentation: "keepdims variant of _tensor_mean_axis — the reduced axis stays at size 1 in the output. Output is always f64. Useful for centring (x - mean(x, -1, keepdims=true)).",
		Params:        []string{"t", "axis"},
	},
	"_tensor_min_axis": {
		Signature:     "_tensor_min_axis(t: tensor, axis: int) -> tensor",
		Documentation: "Minimum along one axis of any-rank tensor. Output has rank len(t.shape)-1. Same dtype as input. Empty axis errors. axis may be negative.",
		Params:        []string{"t", "axis"},
	},
	"_tensor_min_axis_keepdims": {
		Signature:     "_tensor_min_axis_keepdims(t: tensor, axis: int) -> tensor",
		Documentation: "keepdims variant of _tensor_min_axis — reduced axis kept at size 1 in the output.",
		Params:        []string{"t", "axis"},
	},
	"_tensor_max_axis": {
		Signature:     "_tensor_max_axis(t: tensor, axis: int) -> tensor",
		Documentation: "Maximum along one axis of any-rank tensor. Output has rank len(t.shape)-1. Same dtype as input. Empty axis errors. axis may be negative.",
		Params:        []string{"t", "axis"},
	},
	"_tensor_max_axis_keepdims": {
		Signature:     "_tensor_max_axis_keepdims(t: tensor, axis: int) -> tensor",
		Documentation: "keepdims variant of _tensor_max_axis — reduced axis kept at size 1 in the output.",
		Params:        []string{"t", "axis"},
	},
	"_tensor_argmin_axis": {
		Signature:     "_tensor_argmin_axis(t: tensor, axis: int) -> tensor",
		Documentation: "Index of the minimum element along one axis of any-rank tensor. Indices refer to positions within the reduced axis. Output is i64 regardless of input dtype. First-occurrence on ties. Empty axis errors. axis may be negative.",
		Params:        []string{"t", "axis"},
	},
	"_tensor_argmin_axis_keepdims": {
		Signature:     "_tensor_argmin_axis_keepdims(t: tensor, axis: int) -> tensor",
		Documentation: "keepdims variant of _tensor_argmin_axis — reduced axis kept at size 1 in the output. Output is i64.",
		Params:        []string{"t", "axis"},
	},
	"_tensor_argmax_axis": {
		Signature:     "_tensor_argmax_axis(t: tensor, axis: int) -> tensor",
		Documentation: "Index of the maximum element along one axis of any-rank tensor. Indices refer to positions within the reduced axis. Output is i64 regardless of input dtype. First-occurrence on ties. Empty axis errors. axis may be negative.",
		Params:        []string{"t", "axis"},
	},
	"_tensor_argmax_axis_keepdims": {
		Signature:     "_tensor_argmax_axis_keepdims(t: tensor, axis: int) -> tensor",
		Documentation: "keepdims variant of _tensor_argmax_axis — reduced axis kept at size 1 in the output. Output is i64.",
		Params:        []string{"t", "axis"},
	},
	"_tensor_arange": {
		Signature:     "_tensor_arange(start: number, stop: number, step: number, dtype: string) -> tensor",
		Documentation: "Returns a 1-D tensor of values from start up to (but not including) stop, separated by step. step must be non-zero. NumPy equivalent: np.arange(start, stop, step, dtype=dtype).",
		Params:        []string{"start", "stop", "step", "dtype"},
	},
	"_tensor_cast": {
		Signature:     "_tensor_cast(t: tensor, dtype: string) -> tensor",
		Documentation: "Returns a new tensor with every element converted to the target dtype. When src and target dtypes are identical, this is equivalent to clone. Conversion rules: f32→f64 lossless widening; f64→f32 truncating; i64→f32 best-effort (exact for |x|≤2^24); i64→f64 near-lossless (exact for |x|≤2^53); f32/f64→i64 truncate toward zero.",
		Params:        []string{"t", "dtype"},
	},
	"_tensor_clip": {
		Signature:     "_tensor_clip(t: tensor, lo: number, hi: number) -> tensor",
		Documentation: "Returns a new tensor with every element clamped to [lo, hi]. lo and hi must be numbers compatible with t's dtype (same rules as _tensor_full: Float rejects i64; Integer widens to f32/f64). lo <= hi is required.",
		Params:        []string{"t", "lo", "hi"},
	},
	"_tensor_clone": {
		Signature:     "_tensor_clone(t: tensor) -> tensor",
		Documentation: "Returns a fresh independent copy of t. The backing data is fully copied — mutations to the result do not affect t and vice versa. Use this when you need an independent snapshot of a tensor that may otherwise be aliased by reshape / flatten / expand_dims views, or to materialise a strided view from _tensor_slice into a fresh contiguous tensor.",
		Params:        []string{"t"},
	},
	"_tensor_concatenate": {
		Signature:     "_tensor_concatenate(tensors: array, axis: int) -> tensor",
		Documentation: "Joins an array of tensors along an existing axis. All tensors must have the same dtype, the same rank, and identical shapes on every axis except `axis`. NumPy equivalent: np.concatenate([t1, t2, ...], axis=N).",
		Params:        []string{"tensors", "axis"},
	},
	"_tensor_eq": {
		Signature:     "_tensor_eq(a, b) -> tensor",
		Documentation: "Element-wise equality: out[i] = 1 if a[i] == b[i] else 0. Output is always i64 regardless of input dtype. Operands may be tensor+tensor (broadcast-compatible) or tensor+scalar. NaN follows IEEE 754: any comparison with NaN is 0. Pair with _tensor_where for mask-based selection. NumPy parallel: np.equal(a, b) cast to int64.",
		Params:        []string{"a", "b"},
	},
	"_tensor_ne": {
		Signature:     "_tensor_ne(a, b) -> tensor",
		Documentation: "Element-wise inequality: out[i] = 1 if a[i] != b[i] else 0. Output is i64. NaN comparisons: NaN != NaN is 1 (matches IEEE 754 and NumPy). Same broadcasting + scalar rules as _tensor_eq.",
		Params:        []string{"a", "b"},
	},
	"_tensor_lt": {
		Signature:     "_tensor_lt(a, b) -> tensor",
		Documentation: "Element-wise less-than: out[i] = 1 if a[i] < b[i] else 0. Output is i64. NaN comparisons return 0. Same broadcasting + scalar rules as _tensor_eq.",
		Params:        []string{"a", "b"},
	},
	"_tensor_le": {
		Signature:     "_tensor_le(a, b) -> tensor",
		Documentation: "Element-wise less-than-or-equal: out[i] = 1 if a[i] <= b[i] else 0. Output is i64. NaN comparisons return 0. Same broadcasting + scalar rules as _tensor_eq.",
		Params:        []string{"a", "b"},
	},
	"_tensor_gt": {
		Signature:     "_tensor_gt(a, b) -> tensor",
		Documentation: "Element-wise greater-than: out[i] = 1 if a[i] > b[i] else 0. Output is i64. NaN comparisons return 0. Same broadcasting + scalar rules as _tensor_eq.",
		Params:        []string{"a", "b"},
	},
	"_tensor_ge": {
		Signature:     "_tensor_ge(a, b) -> tensor",
		Documentation: "Element-wise greater-than-or-equal: out[i] = 1 if a[i] >= b[i] else 0. Output is i64. NaN comparisons return 0. Same broadcasting + scalar rules as _tensor_eq.",
		Params:        []string{"a", "b"},
	},
	"_tensor_eye": {
		Signature:     "_tensor_eye(n: int, dtype: string) -> tensor",
		Documentation: "Returns an n×n identity matrix: 1 on the diagonal, 0 elsewhere. NumPy equivalent: np.eye(n, dtype=dtype).",
		Params:        []string{"n", "dtype"},
	},
	"_tensor_linspace": {
		Signature:     "_tensor_linspace(start: number, stop: number, n: int, dtype: string) -> tensor",
		Documentation: "Returns a 1-D tensor of n evenly spaced values from start to stop inclusive. n must be >= 1. For n == 1 the result is [start]. NumPy equivalent: np.linspace(start, stop, num=n).",
		Params:        []string{"start", "stop", "n", "dtype"},
	},
	"_tensor_stack": {
		Signature:     "_tensor_stack(tensors: array, axis: int) -> tensor",
		Documentation: "Joins an array of tensors along a NEW axis. All tensors must have identical shapes. The output has rank = input_rank + 1, with the new axis of size len(tensors) inserted at position `axis`. NumPy equivalent: np.stack([t1, t2, ...], axis=N).",
		Params:        []string{"tensors", "axis"},
	},
	"_tensor_to_array": {
		Signature:     "_tensor_to_array(t: tensor) -> array",
		Documentation: "Extracts all elements from t into a flat kLex array, in row-major logical order. f32/f64 elements come back as Float; i64 as Integer. Works on strided views (from _tensor_slice) — the multi-index is walked through Strides to fetch each element from the right physical backing position.",
		Params:        []string{"t"},
	},
	"_tensor_where": {
		Signature:     "_tensor_where(mask: tensor, x: tensor|number, y: tensor|number) -> tensor",
		Documentation: "Element-wise conditional selection: out[i] = x[i] if mask[i] != 0 else y[i]. mask must be an i64 tensor (typically the output of a comparison op). x and y must have the same dtype; either may be a scalar number (broadcast to mask's shape). All shapes must match mask's shape after scalar promotion. NumPy equivalent: np.where(condition, x, y).",
		Params:        []string{"mask", "x", "y"},
	},

	// ── WASM / Browser-Only (//go:build js && wasm) ─────────────────────
	// Registered only in the WebAssembly build that powers the kLex
	// Playground IDE. They have no desktop equivalent — calling them in a
	// desktop script is an undefined-builtin error. See docs/WASM.MD.

	"runScript": {
		Signature:     "runScript(src: string) -> hash",
		Documentation: "WASM-only (browser playground). Evaluate src as a complete, isolated kLex program in a fresh environment and return a hash with keys \"output\" (printed text), \"error\" (message, empty on success) and \"isError\" (bool). Definitions do not persist between calls. Scripts that call window() are rejected — the canvas belongs to the playground IDE. Not available on desktop.",
		Params:        []string{"src"},
	},
	"openURL": {
		Signature:     "openURL(url: string) -> null",
		Documentation: "WASM-only (browser playground). Open url in a new browser tab via window.open(url, \"_blank\"). Used by the playground's documentation links. Not available on desktop.",
		Params:        []string{"url"},
	},
}
