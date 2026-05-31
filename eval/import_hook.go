package eval

// EmbeddedImportLookup, when set, is consulted by the import resolver
// AFTER the disk-based search paths fail. WASM builds set this at init()
// time to provide stdlib without filesystem access; desktop builds leave
// it nil and stick to disk resolution.
//
// Set this exactly once at init(). Mutating it at runtime is not safe —
// the import resolver reads it without locking on the hot path.
//
// The argument is the verbatim import path from the kLex source
// (e.g. "stdlib/json.lex" or "stdlib/ai/anthropic.lex"). Return
// (source, true) on hit or ("", false) on miss; the resolver falls
// through to a "not found" error if the hook misses.
//
// See cmd/wasm/main.go for the WASM-side hookup. Same hook pattern as
// eval.ExternalCallable (wired by the VM) and eval.VMCompileAndRunModule
// (wired by main.go for --vm mode) — set at init, read on the hot path.
var EmbeddedImportLookup func(path string) (source string, ok bool)
