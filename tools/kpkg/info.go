package main

// info.go — `kpkg info <module>` command.
//
// Prints the full picture of one module: metadata, file path, content
// hash, imports, exports. The view is intentionally verbose — this is
// the command you reach for when `kpkg list` has piqued your interest
// and you want everything kpkg knows about a single file.

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func runInfo(args []string) {
	fs := flag.NewFlagSet("info", flag.ExitOnError)
	stdlibRel := fs.String("stdlib", "stdlib", "directory to scan (relative to repo root)")
	_ = fs.Parse(args)

	rest := fs.Args()
	if len(rest) != 1 {
		fmt.Fprintln(os.Stderr, "usage: kpkg info <module>")
		os.Exit(2)
	}
	wanted := rest[0]

	root := findRoot()
	mods := scanStdlib(filepath.Join(root, *stdlibRel))

	for _, m := range mods {
		if m.Name == wanted {
			emitInfo(m)
			return
		}
	}
	// Fallback: try filename-stem match for users who type "rest.lex".
	stripped := strings.TrimSuffix(wanted, ".lex")
	for _, m := range mods {
		if m.Name == stripped {
			emitInfo(m)
			return
		}
	}
	fatal("no module %q under %s/", wanted, *stdlibRel)
}

// emitInfo prints one module's full record. The format is deliberately
// human-readable rather than JSON — `kpkg list --format json` already
// exists for machine consumption, and `info` is the inspection command
// a human runs interactively.
func emitInfo(m moduleInfo) {
	fmt.Printf("Module:  %s\n", m.Name)
	fmt.Printf("Path:    %s\n", m.Path)
	fmt.Printf("Size:    %d bytes\n", m.Size)
	fmt.Printf("Hash:    sha256:%s\n", m.Hash)

	if m.HasMetadata() {
		fmt.Println()
		fmt.Println("Metadata:")
		if m.Version != "" {
			fmt.Printf("  version  %s\n", m.Version)
		}
		if m.Since != "" {
			fmt.Printf("  since    %s\n", m.Since)
		}
		if m.Author != "" {
			fmt.Printf("  author   %s\n", m.Author)
		}
		if m.Summary != "" {
			fmt.Printf("  summary  %s\n", m.Summary)
		}
	} else {
		fmt.Println()
		fmt.Println("Metadata: (none — no @tags in header)")
	}

	fmt.Println()
	if len(m.Imports) == 0 {
		fmt.Println("Imports: (none)")
	} else {
		fmt.Printf("Imports (%d):\n", len(m.Imports))
		for _, imp := range m.Imports {
			fmt.Printf("  %s\n", imp)
		}
	}

	fmt.Println()
	if len(m.Exports) == 0 {
		fmt.Println("Exports: (none — no public top-level fns found)")
	} else {
		fmt.Printf("Exports (%d):\n", len(m.Exports))
		for _, fn := range m.Exports {
			fmt.Printf("  %s\n", fn)
		}
	}
}
