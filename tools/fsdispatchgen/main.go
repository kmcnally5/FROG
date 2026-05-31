// fsdispatchgen — generates the URL-scheme dispatcher file for kLex's
// _fs* builtins.
//
// Output: eval/builtins_fs_dispatch_gen.go (build-tag-free, all targets).
// Each generated dispatcher type-checks the path arg, calls
// ParseIOPath, and routes to a per-scheme native helper:
//
//   file://  -> native<Op>File   (platform-specific impl)
//   opfs://  -> native<Op>OPFS   (wasm-only impl + desktop stub)
//
// The dispatcher's job is path classification + arity check; the
// per-scheme helpers do the actual work. This means every _fs* builtin
// is registered exactly once (in the generated file) instead of N times
// across the platform-specific source files.
//
// The schema lives in `ops` below — that's the single source of truth
// for which fs builtins exist, their arity, and which schemes they
// support. Adding a new fs builtin = adding one entry there + writing
// the per-platform helpers.
//
// Usage:
//
//   go run ./tools/fsdispatchgen                  # write eval/builtins_fs_dispatch_gen.go
//   go run ./tools/fsdispatchgen -out path.go     # custom output path
//   go run ./tools/fsdispatchgen -stdout          # print to stdout (don't write file)
package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/format"
	"os"
	"strings"
)

type scheme string

const (
	schFile scheme = "file"
	schOPFS scheme = "opfs"
)

// op describes one _fs* builtin. PathArgs is the count of leading path
// arguments (1 for most; 2 for src/dst ops like copy, rename, symlink).
// PassthroughArgs are extra arg names appended after the path args —
// stored only for the arity-error message; their values are passed
// through to native helpers as raw Object so each helper does its own
// type-checking (matches the existing builtin convention).
type op struct {
	Name            string
	PathArgs        []string // names of leading path-typed args
	PassthroughArgs []string // names of remaining args (not classified, just forwarded)
	Schemes         []scheme
}

// helperBase returns the camel-case stem used for per-scheme helper
// names. e.g. "_fsRead" -> "FsRead". Helpers are then named
// `nativeFsRead` (file scheme) and `opfsFsRead` (opfs scheme).
func (o op) helperBase() string {
	if !strings.HasPrefix(o.Name, "_") {
		return o.Name
	}
	rest := o.Name[1:]
	if rest == "" {
		return ""
	}
	return strings.ToUpper(rest[:1]) + rest[1:]
}

func (o op) totalArgs() int { return len(o.PathArgs) + len(o.PassthroughArgs) }

func (o op) allArgNames() []string {
	out := make([]string, 0, o.totalArgs())
	out = append(out, o.PathArgs...)
	out = append(out, o.PassthroughArgs...)
	return out
}

// ops — the single source of truth for fs builtin dispatch.
//
// All 25 _fs* builtins. File scheme is universal; OPFS is supported on
// every operation that has sensible semantics in a sandboxed FS (almost
// everything except symlinks, which the OPFS API does not expose).
//
// HTTP scheme support is deferred to v2 of the dispatcher — see the
// Phase 5 plan. The audit's gap analysis (`./bin/ioaudit`) listed 23
// fs builtins as wasm-missing; two more (_fsCountLines, _fsTruncate)
// compile on wasm but fail at runtime because they hit the host FS, so
// they're routed through the dispatcher too. Total: 25.
var ops = []op{
	// READ-shaped (single path)
	{"_fsRead", []string{"path"}, nil, []scheme{schFile, schOPFS}},
	{"_fsReadBytes", []string{"path"}, nil, []scheme{schFile, schOPFS}},
	{"_fsReadChunk", []string{"path"}, []string{"offset", "byteCount"}, []scheme{schFile, schOPFS}},
	{"_fsMap", []string{"path"}, nil, []scheme{schFile, schOPFS}},
	{"_fsCountLines", []string{"path"}, nil, []scheme{schFile, schOPFS}},
	{"_fsExists", []string{"path"}, nil, []scheme{schFile, schOPFS}},
	{"_fsStat", []string{"path"}, nil, []scheme{schFile, schOPFS}},
	{"_fsLstat", []string{"path"}, nil, []scheme{schFile, schOPFS}},
	{"_fsListDir", []string{"path"}, nil, []scheme{schFile, schOPFS}},
	{"_fsReadDir", []string{"path"}, nil, []scheme{schFile, schOPFS}},
	{"_fsReadlink", []string{"path"}, nil, []scheme{schFile}}, // OPFS has no symlinks

	// WRITE-shaped (single path)
	{"_fsWrite", []string{"path"}, []string{"content"}, []scheme{schFile, schOPFS}},
	{"_fsAppend", []string{"path"}, []string{"content"}, []scheme{schFile, schOPFS}},
	{"_fsAppendBytesSync", []string{"path"}, []string{"bytes"}, []scheme{schFile, schOPFS}},
	{"_fsTruncate", []string{"path"}, []string{"newSize"}, []scheme{schFile, schOPFS}},
	{"_fsRemove", []string{"path"}, nil, []scheme{schFile, schOPFS}},
	{"_fsRemoveAll", []string{"path"}, nil, []scheme{schFile, schOPFS}},
	{"_fsMkdir", []string{"path"}, nil, []scheme{schFile, schOPFS}},
	{"_fsMkdirAll", []string{"path"}, nil, []scheme{schFile, schOPFS}},
	{"_fsChmod", []string{"path"}, []string{"mode"}, []scheme{schFile}}, // OPFS has no perm bits
	{"_fsTmpFile", []string{"dir"}, []string{"pattern"}, []scheme{schFile, schOPFS}},
	{"_fsTmpDir", []string{"dir"}, []string{"pattern"}, []scheme{schFile, schOPFS}},

	// TWO-path
	{"_fsCopy", []string{"src", "dst"}, nil, []scheme{schFile, schOPFS}},
	{"_fsRename", []string{"src", "dst"}, nil, []scheme{schFile, schOPFS}},
	{"_fsSymlink", []string{"target", "link"}, nil, []scheme{schFile}}, // OPFS has no symlinks
}

func main() {
	var (
		outPath = flag.String("out", "eval/builtins_fs_dispatch_gen.go", "output file path")
		stdout  = flag.Bool("stdout", false, "print to stdout instead of writing the file")
	)
	flag.Parse()

	src := generate()
	formatted, err := format.Source(src)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fsdispatchgen: gofmt failed: %v\n", err)
		fmt.Fprintln(os.Stderr, "--- raw output ---")
		os.Stderr.Write(src)
		os.Exit(1)
	}

	if *stdout {
		os.Stdout.Write(formatted)
		return
	}
	if err := os.WriteFile(*outPath, formatted, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "fsdispatchgen: write %s: %v\n", *outPath, err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "fsdispatchgen: %d dispatchers written to %s\n", len(ops), *outPath)
}

func generate() []byte {
	var b bytes.Buffer
	b.WriteString(`// Code generated by tools/fsdispatchgen. DO NOT EDIT.
//
// To regenerate: go run ./tools/fsdispatchgen
// The schema lives in tools/fsdispatchgen/main.go.

package eval

import "klex/ast"

func init() {
`)
	for _, o := range ops {
		emitDispatcher(&b, o)
	}
	b.WriteString("}\n")
	return b.Bytes()
}

func emitDispatcher(b *bytes.Buffer, o op) {
	argNames := o.allArgNames()
	total := o.totalArgs()

	arityMsg := fmt.Sprintf("%s expects %d argument", o.Name, total)
	if total != 1 {
		arityMsg += "s"
	}
	if total > 0 {
		arityMsg += " (" + strings.Join(argNames, ", ") + ")"
	}

	supportedList := make([]string, len(o.Schemes))
	for i, s := range o.Schemes {
		supportedList[i] = string(s)
	}
	supportedStr := strings.Join(supportedList, ", ")

	fmt.Fprintf(b, "\n\tBuiltins[%q] = &Builtin{Fn: func(args []Object) Object {\n", o.Name)
	fmt.Fprintf(b, "\t\tif len(args) != %d {\n", total)
	fmt.Fprintf(b, "\t\t\treturn runtimeError(%q, ast.Pos{})\n", arityMsg)
	b.WriteString("\t\t}\n")

	// Type-check each path arg and parse its scheme.
	parsedVars := make([]string, 0, len(o.PathArgs))
	for i, name := range o.PathArgs {
		objVar := name + "Obj"
		parsedVar := name + "Parsed"
		parsedVars = append(parsedVars, parsedVar)
		fmt.Fprintf(b, "\t\t%s, ok := args[%d].(*String)\n", objVar, i)
		b.WriteString("\t\tif !ok {\n")
		fmt.Fprintf(b, "\t\t\treturn typeError(%q+string(args[%d].Type()), ast.Pos{})\n",
			fmt.Sprintf("%s: %s must be string, got ", o.Name, name), i)
		b.WriteString("\t\t}\n")
		fmt.Fprintf(b, "\t\t%s := ParseIOPath(%s.Value)\n", parsedVar, objVar)
	}

	// For multi-path ops, enforce all paths share a scheme.
	if len(o.PathArgs) > 1 {
		first := parsedVars[0]
		for i := 1; i < len(parsedVars); i++ {
			other := parsedVars[i]
			fmt.Fprintf(b, "\t\tif %s.Scheme != %s.Scheme {\n", first, other)
			fmt.Fprintf(b, "\t\t\treturn runtimeError(%q+%s.Scheme.String()+%q+%s.Scheme.String()+\")\", ast.Pos{})\n",
				fmt.Sprintf("%s: %s and %s schemes differ (", o.Name, o.PathArgs[0], o.PathArgs[i]),
				first, " vs ", other)
			b.WriteString("\t\t}\n")
		}
	}

	primary := parsedVars[0]
	fmt.Fprintf(b, "\t\tswitch %s.Scheme {\n", primary)
	for _, s := range o.Schemes {
		fmt.Fprintf(b, "\t\tcase %s:\n", schemeConst(s))
		fmt.Fprintf(b, "\t\t\treturn %s(%s)\n", helperName(o, s), helperArgs(o, parsedVars))
	}
	b.WriteString("\t\t}\n")
	fmt.Fprintf(b, "\t\treturn runtimeError(%q+%s.Scheme.String()+%q, ast.Pos{})\n",
		fmt.Sprintf("%s: scheme ", o.Name),
		primary,
		fmt.Sprintf(" not supported (%s)", supportedStr))
	b.WriteString("\t}}\n")
}

func schemeConst(s scheme) string {
	switch s {
	case schFile:
		return "SchemeFile"
	case schOPFS:
		return "SchemeOPFS"
	}
	return "SchemeFile"
}

// helperName returns e.g. "nativeFsRead" or "opfsFsRead".
func helperName(o op, s scheme) string {
	base := o.helperBase()
	switch s {
	case schFile:
		return "native" + base
	case schOPFS:
		return "opfs" + base
	}
	return "native" + base
}

// helperArgs renders the helper's call-site argument list. Path args are
// passed as `parsed.Remainder` (the post-scheme payload); passthrough
// args are passed by raw Object index.
func helperArgs(o op, parsedVars []string) string {
	parts := make([]string, 0, o.totalArgs())
	for _, pv := range parsedVars {
		parts = append(parts, pv+".Remainder")
	}
	for i := range o.PassthroughArgs {
		parts = append(parts, fmt.Sprintf("args[%d]", len(o.PathArgs)+i))
	}
	return strings.Join(parts, ", ")
}
