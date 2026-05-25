package main

// list.go — `kpkg list` command. The inventory view.
//
// Walks the configured stdlib directory and prints one line per .lex
// module with name, version (or "—" when no header), short hash
// prefix, export count, and a one-line summary. The shape is
// optimised for skim-reading at a terminal: name on the left,
// metadata in tabular columns, summary trailing on the right.

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"
)

func runList(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	stdlibRel := fs.String("stdlib", "stdlib", "directory to scan (relative to repo root)")
	format := fs.String("format", "table", "output format: table | json")
	_ = fs.Parse(args)

	root := findRoot()
	mods := scanStdlib(filepath.Join(root, *stdlibRel))
	if len(mods) == 0 {
		fatal("no .lex files found under %s/", *stdlibRel)
	}

	switch *format {
	case "json":
		emitListJSON(mods)
	default:
		emitListTable(mods)
	}
}

// emitListTable writes a single-tab-stop aligned table to stdout.
// `tabwriter` handles the column alignment; we just provide one tab
// per cell.
func emitListTable(mods []moduleInfo) {
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "MODULE\tVERSION\tHASH\tEXPORTS\tSUMMARY")
	for _, m := range mods {
		ver := m.Version
		if ver == "" {
			ver = "—"
		}
		short := m.Hash
		if len(short) > 8 {
			short = short[:8]
		}
		summary := m.Summary
		if summary == "" {
			summary = ""
		}
		// Hard-cap summary so the table stays readable. Anything
		// longer goes to `kpkg info <module>`.
		if len(summary) > 64 {
			summary = summary[:61] + "…"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\n",
			m.Name, ver, short, len(m.Exports), summary)
	}
	_ = tw.Flush()
	fmt.Printf("\n%d module(s) scanned, %d with metadata headers\n",
		len(mods), countWithMetadata(mods))
}

// emitListJSON writes the full moduleInfo slice as JSON. Useful for
// programmatic consumers — IDE pickers, CI scripts, etc.
func emitListJSON(mods []moduleInfo) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(mods); err != nil {
		fatal("encode: %v", err)
	}
}

// countWithMetadata returns how many modules have at least one @tag
// set. Used for the footer line of the table view.
func countWithMetadata(mods []moduleInfo) int {
	n := 0
	for i := range mods {
		if mods[i].HasMetadata() {
			n++
		}
	}
	return n
}

