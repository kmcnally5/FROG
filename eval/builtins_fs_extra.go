//go:build !js

package eval

// builtins_fs_extra.go — small, cross-platform fs helpers (Unix/macOS
// AND Windows) that don't fit the platform-specific fs files (which are
// split between Unix/macOS and Windows for mmap/fadvise reasons).
// Everything here uses only the stdlib `os` and `bufio` packages so it
// works identically on every desktop platform without further build tags.
//
// Browser/WASM provides its own definitions in builtins_fs_native_wasm.go
// (returning browserNoFsTuple* — no host FS). Build-tagged `!js` so the
// wasm build doesn't see these and conflict.
//
// As of Phase 5, these are plain `nativeFs*` helpers — the dispatcher
// in builtins_fs_dispatch_gen.go registers the public builtin names.

import (
	"bufio"
	"klex/ast"
	"os"
)

// nativeFsCountLines streams the file and counts '\n' bytes. Returns
// the count including a trailing line WITHOUT a final newline (so a
// file containing exactly "a\nb" counts as 2 lines, matching wc -l for
// JSONL row counting). Errors are returned as the second tuple element
// rather than as a kLex runtime error so the caller can treat "missing"
// as "no data yet" instead of crashing.
//
// Used by store.openStoreLite to detect library.f32 / library.json
// row-count desync and trigger crash-recovery truncation. Streaming so
// it works on multi-GB files without holding the whole content in memory.
func nativeFsCountLines(path string) Object {
	f, err := os.Open(path)
	if err != nil {
		return &Tuple{Elements: []Object{&Integer{Value: 0}, &String{Value: err.Error()}}}
	}
	defer f.Close()

	// Hand-rolled fast counter — bufio.Scanner allocates per line; reading
	// raw bytes and counting newlines avoids that. ~1.5 GB/s on a modern SSD.
	reader := bufio.NewReaderSize(f, 1<<20) // 1 MB buffer
	count := 0
	hasContent := false
	lastByte := byte(0)
	buf := make([]byte, 64*1024)
	for {
		n, rerr := reader.Read(buf)
		if n > 0 {
			hasContent = true
			for _, b := range buf[:n] {
				if b == '\n' {
					count++
				}
			}
			lastByte = buf[n-1]
		}
		if rerr != nil {
			break
		}
	}
	if hasContent && lastByte != '\n' {
		count++
	}
	return &Tuple{Elements: []Object{&Integer{Value: count}, NULL}}
}

// nativeFsTruncate trims the file at path to exactly newSize bytes.
// If the file was smaller, it grows (zero-padded) — matches POSIX
// truncate(2). Returns null on success, an error string on failure.
//
// Used by store.openStoreLite to repair a desynced library.f32 by
// trimming orphan trailing vectors back to the row count library.json
// can actually describe. Always back up before calling — no undo.
func nativeFsTruncate(path string, raw Object) Object {
	sz, ok := raw.(*Integer)
	if !ok {
		return typeError("_fsTruncate: newSize must be integer, got "+string(raw.Type()), ast.Pos{})
	}
	if sz.Value < 0 {
		return runtimeError("_fsTruncate: newSize must be non-negative", ast.Pos{})
	}
	if err := os.Truncate(path, int64(sz.Value)); err != nil {
		return &String{Value: err.Error()}
	}
	return NULL
}
