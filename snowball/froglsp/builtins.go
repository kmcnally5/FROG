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
		Documentation: "Convert a value to its string representation.",
		Params:        []string{"val"},
	},
	"int": {
		Signature:     "int(val: string | float | int) -> int",
		Documentation: "Parse a string or convert a number to integer.",
		Params:        []string{"val"},
	},
	"float": {
		Signature:     "float(val: string | int | float) -> float",
		Documentation: "Parse a string or convert a number to float.",
		Params:        []string{"val"},
	},

	// Arrays
	"len": {
		Signature:     "len(val: array | string | hash) -> int",
		Documentation: "Return the length of an array, string, or hash.",
		Params:        []string{"val"},
	},
	"push": {
		Signature:     "push(arr: array, val: any) -> array",
		Documentation: "Return a new array with the value appended.",
		Params:        []string{"arr", "val"},
	},
	"pop": {
		Signature:     "pop(arr: array) -> array",
		Documentation: "Return a new array with the last element removed.",
		Params:        []string{"arr"},
	},
	"concat": {
		Signature:     "concat(a: array, b: array) -> array",
		Documentation: "Concatenate two arrays.",
		Params:        []string{"a", "b"},
	},
	"slice": {
		Signature:     "slice(arr: array, start: int, end?: int) -> array",
		Documentation: "Extract a slice of an array.",
		Params:        []string{"arr", "start", "end"},
	},
	"makeArray": {
		Signature:     "makeArray(n: int, default?: any) -> array",
		Documentation: "Create an array of size n with optional default value.",
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
		Documentation: "Join array elements with separator.",
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
		Documentation: "Replace all occurrences of old with new.",
		Params:        []string{"str", "old", "new"},
	},
	"substr": {
		Signature:     "substr(str: string, start: int, end?: int) -> string",
		Documentation: "Extract a substring.",
		Params:        []string{"str", "start", "end"},
	},
	"indexOf": {
		Signature:     "indexOf(str: string, substr: string) -> int",
		Documentation: "Find index of substring, or -1 if not found.",
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

	// Hash / Object
	"keys": {
		Signature:     "keys(hash: hash) -> array",
		Documentation: "Get all keys from a hash.",
		Params:        []string{"hash"},
	},
	"values": {
		Signature:     "values(hash: hash) -> array",
		Documentation: "Get all values from a hash.",
		Params:        []string{"hash"},
	},
	"hasKey": {
		Signature:     "hasKey(hash: hash, key: any) -> bool",
		Documentation: "Check if hash contains a key.",
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
		Documentation: "Create an array of integers in the given range.",
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
		Documentation: "Read entire file contents.",
		Params:        []string{"path"},
	},
	"writeFile": {
		Signature:     "writeFile(path: string, content: string) -> null",
		Documentation: "Write content to a file.",
		Params:        []string{"path", "content"},
	},
	"appendFile": {
		Signature:     "appendFile(path: string, content: string) -> null",
		Documentation: "Append content to a file.",
		Params:        []string{"path", "content"},
	},

	// Process
	"exec": {
		Signature:     "exec(cmd: string, args: array) -> string",
		Documentation: "Execute a command and return stdout.",
		Params:        []string{"cmd", "args"},
	},

	// Channels
	"channel": {
		Signature:     "channel(capacity?: int) -> channel",
		Documentation: "Create a new channel with optional capacity.",
		Params:        []string{"capacity"},
	},
	"send": {
		Signature:     "send(ch: channel, val: any) -> null | false",
		Documentation: "Send a value to a channel. Returns false if channel is closed.",
		Params:        []string{"ch", "val"},
	},
	"recv": {
		Signature:     "recv(ch: channel) -> (any, bool)",
		Documentation: "Receive a value from a channel. Returns (val, ok).",
		Params:        []string{"ch"},
	},
	"recvNonBlock": {
		Signature:     "recvNonBlock(ch: channel) -> any | null",
		Documentation: "Non-blocking receive from a channel.",
		Params:        []string{"ch"},
	},
	"close": {
		Signature:     "close(ch: channel) -> null",
		Documentation: "Close a channel.",
		Params:        []string{"ch"},
	},
	"cancel": {
		Signature:     "cancel(ch: channel) -> null",
		Documentation: "Cancel all pending receives on a channel.",
		Params:        []string{"ch"},
	},

	// Error handling
	"isError": {
		Signature:     "isError(val: any) -> bool",
		Documentation: "Check if a value is an error.",
		Params:        []string{"val"},
	},
	"error": {
		Signature:     "error(code: string, message: string) -> error",
		Documentation: "Create a user error with code and message.",
		Params:        []string{"code", "message"},
	},
	"safe": {
		Signature:     "safe(fn: function, ...args: any) -> (any, error | null)",
		Documentation: "Call a function and catch any errors. Returns (result, error).",
		Params:        []string{"fn", "...args"},
	},
	"assert": {
		Signature:     "assert(condition: bool, message?: string) -> null",
		Documentation: "Assert that a condition is true, or raise an error.",
		Params:        []string{"condition", "message"},
	},

	// Higher-order
	"map": {
		Signature:     "map(arr: array, fn: function) -> array",
		Documentation: "Apply a function to each element.",
		Params:        []string{"arr", "fn"},
	},
	"filter": {
		Signature:     "filter(arr: array, fn: function) -> array",
		Documentation: "Filter array elements by a predicate function.",
		Params:        []string{"arr", "fn"},
	},
	"reduce": {
		Signature:     "reduce(arr: array, fn: function, init: any) -> any",
		Documentation: "Reduce array to a single value.",
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
		Documentation: "Parallel reduction. fn(acc, element) MUST be associative. Each worker reduces its chunk from initial, then partials combine serially.",
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
		Documentation: "Creates a thread-safe hash map for shared mutable state across goroutines. Read with ch[key], write with ch[key] = val (atomic). Use atomicHashIncr/Add for lock-free arithmetic, atomicHashCAS for compare-and-swap.",
		Params:        []string{},
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
		Documentation: "Run a function asynchronously and return a task.",
		Params:        []string{"fn", "...args"},
	},
	"await": {
		Signature:     "await(task: task) -> any",
		Documentation: "Wait for a task to complete and return its result.",
		Params:        []string{"task"},
	},

	// Math
	"floor": {
		Signature:     "floor(n: int | float) -> int",
		Documentation: "Round down to nearest integer.",
		Params:        []string{"n"},
	},
	"ceil": {
		Signature:     "ceil(n: int | float) -> int",
		Documentation: "Round up to nearest integer.",
		Params:        []string{"n"},
	},
	"round": {
		Signature:     "round(n: int | float) -> int",
		Documentation: "Round to nearest integer.",
		Params:        []string{"n"},
	},
	"sqrt": {
		Signature:     "sqrt(n: int | float) -> float",
		Documentation: "Square root.",
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
		Documentation: "Return the minimum of two numbers.",
		Params:        []string{"a", "b"},
	},
	"max": {
		Signature:     "max(a: int | float, b: int | float) -> int | float",
		Documentation: "Return the maximum of two numbers.",
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
		Documentation: "Natural logarithm of n. n must be positive.",
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
		Documentation: "Arc cosine of n in radians. n must be in [-1, 1].",
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
		Documentation: "Random float in [0, 1).",
		Params:        []string{},
	},
	"randInt": {
		Signature:     "randInt(min: int, max: int) -> int",
		Documentation: "Random integer in [min, max] inclusive.",
		Params:        []string{"min", "max"},
	},
	"shuffle": {
		Signature:     "shuffle(arr: array) -> array",
		Documentation: "Return a randomly shuffled copy of the array.",
		Params:        []string{"arr"},
	},

	// Sort
	"sort": {
		Signature:     "sort(arr: array) -> array",
		Documentation: "Return a sorted copy of the array.",
		Params:        []string{"arr"},
	},
	"sortBy": {
		Signature:     "sortBy(arr: array, fn: function) -> array",
		Documentation: "Sort array by a comparison function.",
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
		Documentation: "Read a line from stdin.",
		Params:        []string{"prompt"},
	},

	// Format
	"format": {
		Signature:     "format(fmt: string, ...args: any) -> string",
		Documentation: "Format a string using {} placeholders.",
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
	"_base64Decode": {Signature: "_base64Decode() -> any", Documentation: "Decode base64 string.", Params: []string{}},
	"_base64Encode": {Signature: "_base64Encode() -> any", Documentation: "Encode string to base64.", Params: []string{}},
	"_aesDecrypt": {Signature: "_aesDecrypt(ciphertext_hex, key) -> (string, error)", Documentation: "Decrypt AES-256-GCM ciphertext produced by _aesEncrypt. Returns (plaintext, null) on success or (null, error) if key is wrong or data was tampered with.", Params: []string{"ciphertext_hex", "key"}},
	"_aesEncrypt": {Signature: "_aesEncrypt(plaintext, key) -> (string, error)", Documentation: "Encrypt with AES-256-GCM (authenticated encryption). Key shorter than 32 bytes is PBKDF2-SHA256 derived. Returns (ciphertext_hex, null) on success — each call produces a unique ciphertext via random nonce.", Params: []string{"plaintext", "key"}},
	"_base64UrlDecode": {Signature: "_base64UrlDecode() -> any", Documentation: "Decode URL-safe base64.", Params: []string{}},
	"_base64UrlEncode": {Signature: "_base64UrlEncode() -> any", Documentation: "Encode to URL-safe base64.", Params: []string{}},
	"_bcryptHash": {Signature: "_bcryptHash() -> any", Documentation: "Hash password with bcrypt.", Params: []string{}},
	"_bcryptVerify": {Signature: "_bcryptVerify() -> any", Documentation: "Verify password against bcrypt hash.", Params: []string{}},
	"_constantTimeEquals": {Signature: "_constantTimeEquals() -> any", Documentation: "Constant-time string comparison.", Params: []string{}},
	"_csvFirstRowCols": {Signature: "_csvFirstRowCols() -> any", Documentation: "Get column count from first CSV row.", Params: []string{}},
	"_csvFormat": {Signature: "_csvFormat() -> any", Documentation: "Format array as CSV.", Params: []string{}},
	"_csvFormatDelim": {Signature: "_csvFormatDelim() -> any", Documentation: "Format CSV with custom delimiter.", Params: []string{}},
	"_csvHasRows": {Signature: "_csvHasRows() -> any", Documentation: "Check if CSV has data rows.", Params: []string{}},
	"_csvParse": {Signature: "_csvParse() -> any", Documentation: "Parse CSV data.", Params: []string{}},
	"_csvParseDelim": {Signature: "_csvParseDelim() -> any", Documentation: "Parse CSV with custom delimiter.", Params: []string{}},
	"_csvParseHeaders": {Signature: "_csvParseHeaders() -> any", Documentation: "Parse CSV with headers.", Params: []string{}},
	"_csvStream": {Signature: "_csvStream() -> any", Documentation: "Stream CSV records.", Params: []string{}},
	"_deflateCompress": {Signature: "_deflateCompress() -> any", Documentation: "Compress data with deflate.", Params: []string{}},
	"_deflateDecompress": {Signature: "_deflateDecompress() -> any", Documentation: "Decompress deflate data.", Params: []string{}},
	"_fsAppend": {Signature: "_fsAppend(path, content) -> (null, error)", Documentation: "Append to file.", Params: []string{"path", "content"}},
	"_fsChmod": {Signature: "_fsChmod(path, mode) -> (null, error)", Documentation: "Change file permissions.", Params: []string{"path", "mode"}},
	"_fsCopy": {Signature: "_fsCopy(src, dst) -> (null, error)", Documentation: "Copy file.", Params: []string{"src", "dst"}},
	"_fsExists": {Signature: "_fsExists(path) -> bool", Documentation: "Check if file exists.", Params: []string{"path"}},
	"_fsListDir": {Signature: "_fsListDir(path) -> (array, error)", Documentation: "List directory names.", Params: []string{"path"}},
	"_fsLstat": {Signature: "_fsLstat(path) -> (hash, error)", Documentation: "Get file metadata (no symlink follow).", Params: []string{"path"}},
	"_fsMap": {Signature: "_fsMap(path) -> (string, error)", Documentation: "Memory-map file.", Params: []string{"path"}},
	"_fsMkdir": {Signature: "_fsMkdir(path) -> (null, error)", Documentation: "Create directory.", Params: []string{"path"}},
	"_fsMkdirAll": {Signature: "_fsMkdirAll(path) -> (null, error)", Documentation: "Create directory tree.", Params: []string{"path"}},
	"_fsRead": {Signature: "_fsRead(path) -> (string, error)", Documentation: "Read entire file.", Params: []string{"path"}},
	"_fsReadChunk": {Signature: "_fsReadChunk(path, offset, len) -> (string, bool, error)", Documentation: "Read file chunk.", Params: []string{"path", "offset", "len"}},
	"_fsReadDir": {Signature: "_fsReadDir(path) -> (array, error)", Documentation: "List directory with metadata.", Params: []string{"path"}},
	"_fsReadlink": {Signature: "_fsReadlink(path) -> (string, error)", Documentation: "Read symlink target.", Params: []string{"path"}},
	"_fsRemove": {Signature: "_fsRemove(path) -> (null, error)", Documentation: "Remove file.", Params: []string{"path"}},
	"_fsRemoveAll": {Signature: "_fsRemoveAll(path) -> (null, error)", Documentation: "Remove recursively.", Params: []string{"path"}},
	"_fsRename": {Signature: "_fsRename(src, dst) -> (null, error)", Documentation: "Rename file.", Params: []string{"src", "dst"}},
	"_fsStat": {Signature: "_fsStat(path) -> (hash, error)", Documentation: "Get file metadata.", Params: []string{"path"}},
	"_fsSymlink": {Signature: "_fsSymlink(target, link) -> (null, error)", Documentation: "Create symlink.", Params: []string{"target", "link"}},
	"_fsTmpDir": {Signature: "_fsTmpDir(dir, pattern) -> (string, error)", Documentation: "Create temp directory.", Params: []string{"dir", "pattern"}},
	"_fsTmpFile": {Signature: "_fsTmpFile(dir, pattern) -> (string, error)", Documentation: "Create temp file.", Params: []string{"dir", "pattern"}},
	"_fsWrite": {Signature: "_fsWrite(path, content) -> (null, error)", Documentation: "Write file.", Params: []string{"path", "content"}},
	"_gzipCompress": {Signature: "_gzipCompress() -> any", Documentation: "Compress with gzip.", Params: []string{}},
	"_gzipDecompress": {Signature: "_gzipDecompress() -> any", Documentation: "Decompress gzip.", Params: []string{}},
	"_hmacSha256": {Signature: "_hmacSha256() -> any", Documentation: "HMAC-SHA256 hash.", Params: []string{}},
	"_hmacSha512": {Signature: "_hmacSha512() -> any", Documentation: "HMAC-SHA512 hash.", Params: []string{}},
	"_httpDo": {Signature: "_httpDo() -> any", Documentation: "Make HTTP request.", Params: []string{}},
	"_httpServe": {Signature: "_httpServe() -> any", Documentation: "Start HTTP server.", Params: []string{}},
	"_md5": {Signature: "_md5(data) -> string", Documentation: "MD5 hash.", Params: []string{"data"}},
	"_osArgs": {Signature: "_osArgs() -> array", Documentation: "Command-line arguments.", Params: []string{}},
	"_osCwd": {Signature: "_osCwd() -> (string, error)", Documentation: "Get working directory.", Params: []string{}},
	"_osExit": {Signature: "_osExit(code) -> never", Documentation: "Exit program.", Params: []string{"code"}},
	"_osGetenv": {Signature: "_osGetenv(name) -> (string, error)", Documentation: "Get environment variable.", Params: []string{"name"}},
	"_osHostname": {Signature: "_osHostname() -> (string, error)", Documentation: "Get hostname.", Params: []string{}},
	"_osPid": {Signature: "_osPid() -> int", Documentation: "Get process ID.", Params: []string{}},
	"_osSetenv": {Signature: "_osSetenv(name, value) -> (null, error)", Documentation: "Set environment variable.", Params: []string{"name", "value"}},
	"_processExec": {Signature: "_processExec() -> any", Documentation: "Execute command.", Params: []string{}},
	"_processRun": {Signature: "_processRun() -> any", Documentation: "Run command with output.", Params: []string{}},
	"_processShell": {Signature: "_processShell() -> any", Documentation: "Run shell script.", Params: []string{}},
	"_randomBytes": {Signature: "_randomBytes() -> any", Documentation: "Random bytes.", Params: []string{}},
	"_regexFind": {Signature: "_regexFind() -> any", Documentation: "Regex find.", Params: []string{}},
	"_regexFindAll": {Signature: "_regexFindAll() -> any", Documentation: "Regex find all.", Params: []string{}},
	"_regexGroups": {Signature: "_regexGroups() -> any", Documentation: "Regex capture groups.", Params: []string{}},
	"_regexGroupsAll": {Signature: "_regexGroupsAll() -> any", Documentation: "Regex all groups.", Params: []string{}},
	"_regexMatch": {Signature: "_regexMatch() -> any", Documentation: "Regex match.", Params: []string{}},
	"_regexReplace": {Signature: "_regexReplace() -> any", Documentation: "Regex replace.", Params: []string{}},
	"_regexReplaceAll": {Signature: "_regexReplaceAll() -> any", Documentation: "Regex replace all.", Params: []string{}},
	"_regexSplit": {Signature: "_regexSplit() -> any", Documentation: "Regex split.", Params: []string{}},
	"_sha256": {Signature: "_sha256(data) -> string", Documentation: "SHA-256 hash.", Params: []string{"data"}},
	"_sha512": {Signature: "_sha512(data) -> string", Documentation: "SHA-512 hash.", Params: []string{"data"}},
	"_timeFields": {Signature: "_timeFields() -> any", Documentation: "Time fields.", Params: []string{}},
	"_timeFormat": {Signature: "_timeFormat() -> any", Documentation: "Format time.", Params: []string{}},
	"_timeNanos": {Signature: "_timeNanos() -> int", Documentation: "Time in nanoseconds.", Params: []string{}},
	"_timeNow": {Signature: "_timeNow() -> int", Documentation: "Current timestamp.", Params: []string{}},
	"_timeParse": {Signature: "_timeParse() -> any", Documentation: "Parse time string.", Params: []string{}},
	"_tsvFormat": {Signature: "_tsvFormat() -> any", Documentation: "Format as TSV.", Params: []string{}},
	"_tsvParse": {Signature: "_tsvParse() -> any", Documentation: "Parse TSV.", Params: []string{}},
	"_urlDecode": {Signature: "_urlDecode() -> any", Documentation: "URL decode.", Params: []string{}},
	"_urlEncode": {Signature: "_urlEncode() -> any", Documentation: "URL encode.", Params: []string{}},
	"_uuid": {Signature: "_uuid() -> string", Documentation: "Generate UUIDv4.", Params: []string{}},
	"color_bg_black": {Signature: "color_bg_black() -> string", Documentation: "ANSI black background.", Params: []string{}},
	"color_bg_blue": {Signature: "color_bg_blue() -> string", Documentation: "ANSI blue background.", Params: []string{}},
	"color_bg_cyan": {Signature: "color_bg_cyan() -> string", Documentation: "ANSI cyan background.", Params: []string{}},
	"color_bg_green": {Signature: "color_bg_green() -> string", Documentation: "ANSI green background.", Params: []string{}},
	"color_bg_magenta": {Signature: "color_bg_magenta() -> string", Documentation: "ANSI magenta background.", Params: []string{}},
	"color_bg_red": {Signature: "color_bg_red() -> string", Documentation: "ANSI red background.", Params: []string{}},
	"color_bg_white": {Signature: "color_bg_white() -> string", Documentation: "ANSI white background.", Params: []string{}},
	"color_bg_yellow": {Signature: "color_bg_yellow() -> string", Documentation: "ANSI yellow background.", Params: []string{}},
	"color_black": {Signature: "color_black() -> string", Documentation: "ANSI black foreground.", Params: []string{}},
	"color_blue": {Signature: "color_blue() -> string", Documentation: "ANSI blue foreground.", Params: []string{}},
	"color_bold": {Signature: "color_bold() -> string", Documentation: "ANSI bold text.", Params: []string{}},
	"color_cyan": {Signature: "color_cyan() -> string", Documentation: "ANSI cyan foreground.", Params: []string{}},
	"color_dim": {Signature: "color_dim() -> string", Documentation: "ANSI dim text.", Params: []string{}},
	"color_green": {Signature: "color_green() -> string", Documentation: "ANSI green foreground.", Params: []string{}},
	"color_magenta": {Signature: "color_magenta() -> string", Documentation: "ANSI magenta foreground.", Params: []string{}},
	"color_red": {Signature: "color_red() -> string", Documentation: "ANSI red foreground.", Params: []string{}},
	"color_reset": {Signature: "color_reset() -> string", Documentation: "ANSI reset formatting.", Params: []string{}},
	"color_underline": {Signature: "color_underline() -> string", Documentation: "ANSI underline text.", Params: []string{}},
	"color_white": {Signature: "color_white() -> string", Documentation: "ANSI white foreground.", Params: []string{}},
	"color_yellow": {Signature: "color_yellow() -> string", Documentation: "ANSI yellow foreground.", Params: []string{}},
	"colorize": {Signature: "colorize(text, code) -> string", Documentation: "Wrap text with color code.", Params: []string{"text", "code"}},

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
		Signature:     "lineChart(data: array, x, y, w, h: float, min?: float, max?: float) -> null",
		Documentation: "Draw a line chart of numeric data in the rect (x, y, w×h).\n\nUses the current fill() colour for the line and area fill (25% alpha). If min/max are omitted they are derived from the data.\n\nDraws: dark background, subtle axis lines, filled area, line, dot per point, accent border.\n\nExample:\n  fill(0.3, 0.7, 1.0, 1.0)\n  lineChart(durations, 10, 10, 400, 150)\n  lineChart(durations, 10, 10, 400, 150, 0.0, 10.0)",
		Params:        []string{"data", "x", "y", "w", "h", "min", "max"},
	},
	"barChart": {
		Signature:     "barChart(data: array, x, y, w, h: float, min?: float, max?: float) -> null",
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
		Signature:     "progressBar(x, y, w, h: int, value, max: float) -> null",
		Documentation: "Display-only filled progress bar at (x, y) with size w×h. Fill fraction = value / max, clamped to [0, 1]. No interaction.\n\nMust be called between uiBegin() and uiEnd().",
		Params:        []string{"x", "y", "w", "h", "value", "max"},
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
}
