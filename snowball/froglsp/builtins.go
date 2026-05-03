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
}
