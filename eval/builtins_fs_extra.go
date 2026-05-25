package eval

// builtins_fs_extra.go — small, cross-platform fs builtins that don't fit
// the platform-specific fs files (which are split between Unix/macOS and
// Windows for mmap/fadvise reasons). Everything in here uses only the
// stdlib `os` and `bufio` packages so it works identically on every
// platform without build tags.

import (
	"bufio"
	"klex/ast"
	"os"
)

func init() {
	// ── _fsCountLines(path) → (int, error) ──────────────────────────────────
	//
	// Stream the file and count `\n` bytes. Returns the count including a
	// trailing line WITHOUT a final newline (so a file containing exactly
	// "a\nb" counts as 2 lines, matching `wc -l`'s `-l` behaviour for
	// our use-case of "how many JSON records are in this JSONL?").
	//
	// Empty file → 0 lines, no error. Missing file → (0, error) with
	// the raw OS error string in the second slot. Errors are NOT a kLex
	// runtime error so the caller can choose to treat "missing" as
	// "no data yet" instead of crashing.
	//
	// Used by store.openStoreLite to detect library.f32 / library.json
	// row-count desync and trigger crash-recovery truncation (OFI #15).
	// Streaming-based so it works on multi-GB files without holding the
	// whole content in memory.
	Builtins["_fsCountLines"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_fsCountLines expects 1 argument (path)", ast.Pos{})
		}
		p, ok := args[0].(*String)
		if !ok {
			return typeError("_fsCountLines: path must be a string, got "+string(args[0].Type()), ast.Pos{})
		}
		f, err := os.Open(p.Value)
		if err != nil {
			return &Tuple{Elements: []Object{&Integer{Value: 0}, &String{Value: err.Error()}}}
		}
		defer f.Close()

		// Hand-rolled fast counter — bufio.Scanner would also work but
		// allocates per line; reading raw bytes and counting newlines
		// avoids that entirely. ~1.5 GB/s on a modern SSD.
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
		// If the file has content but doesn't end with '\n', the trailing
		// line still counts (matches wc -l semantics for our use case of
		// "JSONL record count").
		if hasContent && lastByte != '\n' {
			count++
		}
		return &Tuple{Elements: []Object{&Integer{Value: count}, NULL}}
	}}

	// ── _fsTruncate(path, newSize) → error ──────────────────────────────────
	//
	// Truncate the file at `path` to exactly `newSize` bytes. If the file
	// was smaller than newSize it grows (zero-padded) — matches POSIX
	// truncate(2). Returns null on success, an error string on failure.
	//
	// Used by store.openStoreLite to repair a desynced library.f32 by
	// trimming orphan trailing vectors back to the row count library.json
	// can actually describe (OFI #15). Always make a backup before calling
	// — there's no undo.
	Builtins["_fsTruncate"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_fsTruncate expects 2 arguments (path, newSize)", ast.Pos{})
		}
		p, ok := args[0].(*String)
		if !ok {
			return typeError("_fsTruncate: path must be a string, got "+string(args[0].Type()), ast.Pos{})
		}
		sz, ok := args[1].(*Integer)
		if !ok {
			return typeError("_fsTruncate: newSize must be an integer, got "+string(args[1].Type()), ast.Pos{})
		}
		if sz.Value < 0 {
			return runtimeError("_fsTruncate: newSize must be non-negative", ast.Pos{})
		}
		if err := os.Truncate(p.Value, int64(sz.Value)); err != nil {
			return &String{Value: err.Error()}
		}
		return NULL
	}}
}
