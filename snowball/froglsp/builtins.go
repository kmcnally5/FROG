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
}
