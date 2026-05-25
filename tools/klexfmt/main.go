// klexfmt — kLex source code formatter CLI.
//
// Thin wrapper around the formatter/ package. All the logic lives there;
// this file just handles arg parsing, file I/O, and atomic in-place writes.
//
// ── Relationship to froglsp ────────────────────────────────────────────────
//
// There are TWO front-ends for kLex source formatting; both call into
// the same library (`klex/formatter`, at the repo root):
//
//   • klexfmt  (this file)            — CLI / pre-commit / CI usage
//   • froglsp  (snowball/froglsp)     — editor format-on-save via LSP
//
// They produce byte-identical output for any given input, by
// construction. A `klexfmt -w foo.lex` from the terminal and an
// editor "Format Document" command in VS Code / Neovim / Zed both
// route to the same Format() function — there is no second
// implementation to drift out of sync with.
//
// See formatter/format.go for the spec of what Format() does.
//
// Usage (run from kLex repo root):
//
//	go run ./tools/klexfmt path.lex          — print formatted output to stdout
//	go run ./tools/klexfmt -w path.lex       — rewrite the file in place
//	cat src.lex | go run ./tools/klexfmt     — stdin → stdout (editor-pipe mode)
//	go run ./tools/klexfmt -w file1 file2 …  — batch in-place format
//	go run ./tools/klexfmt -l path.lex       — list-only mode: print paths whose
//	                                            formatted output differs from disk
//	                                            (exits 0 if all clean, 1 if any
//	                                             would change). For CI use.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"klex/formatter"
)

func main() {
	write := flag.Bool("w", false, "write result to source file instead of stdout")
	list := flag.Bool("l", false, "list paths that would be reformatted; exit 1 if any")
	flag.Parse()

	paths := flag.Args()

	// stdin → stdout when no paths are supplied.
	if len(paths) == 0 {
		src, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "klexfmt: stdin: %v\n", err)
			os.Exit(1)
		}
		os.Stdout.Write(formatter.Format(src))
		return
	}

	anyChanges := false
	for _, path := range paths {
		changed, err := processOne(path, *write, *list)
		if err != nil {
			fmt.Fprintf(os.Stderr, "klexfmt: %s: %v\n", path, err)
			os.Exit(1)
		}
		if changed {
			anyChanges = true
		}
	}
	if *list && anyChanges {
		os.Exit(1)
	}
}

// processOne formats a single file and returns whether the formatted
// output differs from the file's current contents.
func processOne(path string, write, list bool) (bool, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	out := formatter.Format(src)
	changed := !bytes.Equal(src, out)

	if list {
		if changed {
			fmt.Println(path)
		}
		return changed, nil
	}
	if write {
		if !changed {
			return false, nil
		}
		// Write atomically via a sibling temp file to avoid partial
		// writes on crash. os.Rename does the swap.
		dir := filepath.Dir(path)
		base := filepath.Base(path)
		tmp, err := os.CreateTemp(dir, "."+base+".klexfmt-*")
		if err != nil {
			return false, err
		}
		tmpPath := tmp.Name()
		_, werr := tmp.Write(out)
		cerr := tmp.Close()
		if werr != nil {
			os.Remove(tmpPath)
			return false, werr
		}
		if cerr != nil {
			os.Remove(tmpPath)
			return false, cerr
		}
		if err := os.Rename(tmpPath, path); err != nil {
			os.Remove(tmpPath)
			return false, err
		}
		return true, nil
	}
	// Default: stdout.
	os.Stdout.Write(out)
	return changed, nil
}
