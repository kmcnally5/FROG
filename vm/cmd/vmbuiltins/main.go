// vmbuiltins — generator for the VM's builtin lookup table.
//
// The tree-walking eval/ runtime dispatches builtins by string lookup
// in a Go map[string]*Builtin. That's fine when the interpreter pays
// the cost once per call, but a stack VM emits OP_CALL_BUILTIN with a
// numeric immediate operand and resolves through an indexed table
// inside the dispatch loop. To keep the compiler and the VM agreed on
// "builtin index 47 means cosineSim, always," we generate both the
// table and the inverse name→index map from one scan.
//
// What this tool does
//
//  1. Walks every eval/*.go file via go/parser.
//  2. Finds every `Builtins["name"] = …` assignment AND every entry
//     in the `var Builtins = map[string]*Builtin{ "name": … }` literal
//     that lives in eval.go.
//  3. Sorts the discovered names alphabetically — this is the stable
//     ordering the rest of the project relies on. Stability matters:
//     a compiler binary built today against builtinIndex["cosineSim"]
//     = 47 must agree with a VM binary built tomorrow against the
//     same name. As long as the SAME generator is run on the same
//     source set, the indices are deterministic.
//  4. Emits vm/builtins_gen.go containing:
//
//       const NumBuiltins = N
//       var builtinNames [NumBuiltins]string                — index → name
//       var BuiltinIndex = map[string]uint16{ "name": idx } — name → index
//       var BuiltinTable [NumBuiltins]*eval.Builtin         — populated at init()
//       func init() { … completeness check … }
//
//     The init() function walks the eval.Builtins map and verifies
//     every name lands at the expected index, panicking with a
//     pinpoint message on any drift. So even if vmbuiltins emitted a
//     stale table because someone forgot to regenerate, startup fails
//     loudly with the offending name — never silent corruption.
//
// Why AST and not regex
//
// The existing tools/syncdocs uses line-by-line regex to find
// Builtins[…] = … and it works fine today. We chose go/ast here for
// two reasons:
//
//   - The map-literal form in eval.go has nested KeyValueExprs; the
//     regex form needs a separate pattern for it and an extra line of
//     filtering for nested struct field names (e.g. `Signature:` in
//     the LSP file accidentally matching). AST parsing eliminates
//     that whole class of false positive at no extra cost.
//   - The two scanners can drift independently. Reusing AST infra
//     across this tool, vmgen, and vmcoverage keeps maintenance to
//     one mental model.
package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func main() {
	root := findRoot()
	evalDir := filepath.Join(root, "eval")
	outPath := filepath.Join(root, "vm", "builtins_gen.go")
	outWasmPath := filepath.Join(root, "vm", "builtins_gen_wasm.go")

	// Base scan: exclude _wasm.go files — builtins that only exist in
	// the browser build must not appear in the desktop table.
	baseNames, err := scanBuiltinNames(evalDir, false)
	if err != nil {
		fatalf("scan %s (base): %v", evalDir, err)
	}
	if len(baseNames) == 0 {
		fatalf("no Builtins[\"…\"] = … registrations found in %s — broken scanner?", evalDir)
	}
	sort.Strings(baseNames)

	// WASM scan: include _wasm.go files to pick up WASM-only builtins
	// (currently runScript and openURL from builtins_eval_wasm.go).
	wasmNames, err := scanBuiltinNames(evalDir, true)
	if err != nil {
		fatalf("scan %s (wasm): %v", evalDir, err)
	}
	sort.Strings(wasmNames)

	// Emit the non-WASM table, gated //go:build !js.
	baseSrc, err := renderBuiltinsGen(baseNames, "!js")
	if err != nil {
		fatalf("render base: %v", err)
	}
	if err := writeAtomic(outPath, baseSrc); err != nil {
		fatalf("write %s: %v", outPath, err)
	}
	fmt.Printf("✓ vmbuiltins — wrote %d builtins to %s\n", len(baseNames), outPath)

	// Emit the WASM table, gated //go:build js && wasm.
	wasmSrc, err := renderBuiltinsGen(wasmNames, "js && wasm")
	if err != nil {
		fatalf("render wasm: %v", err)
	}
	if err := writeAtomic(outWasmPath, wasmSrc); err != nil {
		fatalf("write %s: %v", outWasmPath, err)
	}
	fmt.Printf("✓ vmbuiltins — wrote %d builtins to %s\n", len(wasmNames), outWasmPath)
}

// ── Scanning ──────────────────────────────────────────────────────────────────

// scanBuiltinNames walks every .go file in evalDir, finds every
// builtin name registered in any of three shapes:
//
//	Builtins["name"] = …            (the standard assignment form)
//	Builtins["name"] = &Builtin{Fn: …}
//	"name": {Fn: …}                 (inside `var Builtins = map[string]*Builtin{…}`)
//
// Duplicates are silently deduped — the same name might be re-declared
// across build tags (e.g. Windows vs POSIX fs builtins) and we want
// one entry, not three.
//
// When includeWasm is false, files ending in _wasm.go are skipped so
// the base (non-WASM) table excludes browser-only builtins. When true,
// only files that would be compiled under GOOS=js GOARCH=wasm are
// scanned — //go:build constraints are evaluated against the WASM tag
// set and filename-based GOOS conventions are honoured.
func scanBuiltinNames(dir string, includeWasm bool) ([]string, error) {
	seen := make(map[string]bool)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(dir, name)
		if includeWasm {
			ok, err := fileMatchesWasm(path)
			if err != nil {
				return nil, fmt.Errorf("%s: check wasm tags: %w", name, err)
			}
			if !ok {
				continue
			}
		} else {
			if strings.HasSuffix(name, "_wasm.go") {
				continue
			}
		}
		if err := scanOneFile(path, seen); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	return names, nil
}

// fileMatchesWasm reports whether the file at path would be compiled
// under GOOS=js GOARCH=wasm. It evaluates the //go:build constraint if
// present; for files with no constraint it falls back to the filename-
// based GOOS convention (foo_darwin.go → darwin only, etc.).
func fileMatchesWasm(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	for _, raw := range strings.SplitN(string(data), "\n", 30) {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "//") {
			break // reached non-comment source — no build constraint
		}
		if strings.HasPrefix(line, "//go:build ") {
			expr, err := constraint.Parse(line)
			if err != nil {
				return false, err
			}
			return expr.Eval(wasmTagMatch), nil
		}
	}
	// No //go:build line. Apply filename-based GOOS/GOARCH convention:
	// foo_GOOS.go or foo_GOARCH.go is included only for that target.
	stem := strings.TrimSuffix(filepath.Base(path), ".go")
	for _, seg := range strings.Split(stem, "_")[1:] {
		switch seg {
		case "unix", "darwin", "windows", "linux", "freebsd",
			"netbsd", "openbsd", "plan9", "solaris", "aix",
			"dragonfly", "android", "ios":
			return false, nil
		}
	}
	return true, nil
}

// wasmTagMatch reports whether tag is set under GOOS=js GOARCH=wasm.
// Used to evaluate //go:build constraints for the WASM builtin table.
func wasmTagMatch(tag string) bool {
	switch tag {
	case "js", "wasm":
		return true
	case "ignore":
		return false
	// OS tags that are not "js"
	case "unix", "aix", "android", "darwin", "dragonfly", "freebsd",
		"hurd", "illumos", "ios", "linux", "nacl", "netbsd", "openbsd",
		"plan9", "solaris", "wasip1", "windows", "zos":
		return false
	// Arch tags that are not "wasm"
	case "386", "amd64", "arm", "arm64", "loong64", "mips", "mips64",
		"mips64le", "mipsle", "ppc64", "ppc64le", "riscv64", "s390x", "sparc64":
		return false
	default:
		// Unknown tags (e.g. go version constraints like "go1.21"): treat as
		// satisfied so version-gated files are not silently excluded. This means
		// any novel build tag not listed above is assumed WASM-compatible. If a
		// new eval builtin file is ever gated with a non-WASM-specific tag that
		// is not in the lists above, re-run the generator and verify the count.
		return true
	}
}

// scanOneFile parses path and records every builtin name it finds
// into seen.
func scanOneFile(path string, seen map[string]bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, data, parser.SkipObjectResolution)
	if err != nil {
		return err
	}

	// Two passes through the AST: one to catch the var Builtins =
	// map[string]*Builtin{ … } literal (eval.go only), one to catch
	// every Builtins["name"] = … assignment in any function.
	ast.Inspect(file, func(n ast.Node) bool {
		switch v := n.(type) {

		// Shape 1: `var Builtins = map[string]*Builtin{ "name": { … }, … }`
		case *ast.ValueSpec:
			for i, name := range v.Names {
				if name.Name != "Builtins" {
					continue
				}
				if i >= len(v.Values) {
					return true
				}
				cl, ok := v.Values[i].(*ast.CompositeLit)
				if !ok {
					return true
				}
				for _, elt := range cl.Elts {
					kv, ok := elt.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					if bn, ok := stringKey(kv.Key); ok {
						seen[bn] = true
					}
				}
			}

		// Shape 2: `Builtins["name"] = …` (any statement, any function)
		case *ast.AssignStmt:
			if len(v.Lhs) != 1 {
				return true
			}
			idx, ok := v.Lhs[0].(*ast.IndexExpr)
			if !ok {
				return true
			}
			id, ok := idx.X.(*ast.Ident)
			if !ok || id.Name != "Builtins" {
				return true
			}
			if bn, ok := stringKey(idx.Index); ok {
				seen[bn] = true
			}
		}
		return true
	})
	return nil
}

// stringKey returns the unquoted value if expr is a string literal.
func stringKey(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return s, true
}

// ── Rendering ─────────────────────────────────────────────────────────────────

// renderBuiltinsGen emits the generated registry file. The generated
// file imports "klex/eval" so it can populate the table with the
// actual *eval.Builtin pointers at init() time. That way:
//
//   - The vm package keeps zero compile-time knowledge of which
//     builtins exist — only the GENERATED file knows the alphabetical
//     order.
//   - Drift is impossible at runtime: init() panics if any name is
//     missing OR if a new builtin was added without regenerating.
//
// buildTag is written as a //go:build constraint at the top of the
// file (e.g. "!js" or "js && wasm"). Empty string = no constraint.
func renderBuiltinsGen(names []string, buildTag string) ([]byte, error) {
	var b bytes.Buffer

	if buildTag != "" {
		fmt.Fprintf(&b, "//go:build %s\n\n", buildTag)
	}
	b.WriteString("// Code generated by vm/cmd/vmbuiltins. DO NOT EDIT.\n")
	b.WriteString("//\n")
	b.WriteString("// Source of truth: every Builtins[\"…\"] = … registration across\n")
	b.WriteString("// eval/*.go and the `var Builtins = map[string]*Builtin{…}` literal\n")
	b.WriteString("// in eval/eval.go. Regenerate with: go generate ./vm/...\n\n")
	b.WriteString("package vm\n\n")

	b.WriteString("import \"klex/eval\"\n\n")

	fmt.Fprintf(&b, "// NumBuiltins is the count of builtins discovered at codegen time.\n")
	fmt.Fprintf(&b, "// Compiled bytecode artifacts assume this exact count and\n")
	fmt.Fprintf(&b, "// alphabetical ordering — see vm/cmd/vmbuiltins for the contract.\n")
	fmt.Fprintf(&b, "const NumBuiltins = %d\n\n", len(names))

	// Name table: ordered, used by the disassembler ("CallBuiltin cosineSim").
	b.WriteString("// builtinNames maps index → canonical builtin name. Disassembly\n")
	b.WriteString("// and error messages read from here; the compiler uses BuiltinIndex.\n")
	b.WriteString("var builtinNames = [NumBuiltins]string{\n")
	for i, n := range names {
		fmt.Fprintf(&b, "\t%d: %q,\n", i, n)
	}
	b.WriteString("}\n\n")

	// Inverse: name → index. The compiler resolves a builtin call
	// site to its numeric operand via this map.
	b.WriteString("// BuiltinIndex maps canonical builtin name → numeric operand for\n")
	b.WriteString("// OP_CALL_BUILTIN. The compiler resolves call sites through here.\n")
	b.WriteString("var BuiltinIndex = map[string]uint16{\n")
	for i, n := range names {
		fmt.Fprintf(&b, "\t%q: %d,\n", n, i)
	}
	b.WriteString("}\n\n")

	// Actual table — populated at init() so we can verify the eval
	// package's runtime Builtins map agrees with what we generated.
	b.WriteString("// BuiltinTable holds direct *eval.Builtin pointers indexed by\n")
	b.WriteString("// the same numeric operand the compiler emits. Populated and\n")
	b.WriteString("// validated by init() — see below.\n")
	b.WriteString("var BuiltinTable [NumBuiltins]*eval.Builtin\n\n")

	// init() — completeness check. If anyone adds a builtin without
	// regenerating, OR removes one we still expect, this panics at
	// program startup with a pinpoint message.
	b.WriteString("// init wires BuiltinTable from eval.Builtins and verifies the\n")
	b.WriteString("// generated table is in sync with the runtime registry. Any drift\n")
	b.WriteString("// (missing builtin, extra builtin, or rename) panics at startup\n")
	b.WriteString("// with the offending name — never silent corruption.\n")
	b.WriteString("//\n")
	b.WriteString("// Regenerate with: go generate ./vm/...\n")
	b.WriteString("func init() {\n")
	b.WriteString("\tfor name, idx := range BuiltinIndex {\n")
	b.WriteString("\t\tb, ok := eval.Builtins[name]\n")
	b.WriteString("\t\tif !ok {\n")
	b.WriteString("\t\t\tpanic(\"vm: generated builtin table references missing eval.Builtins[\\\"\" + name + \"\\\"] — regenerate with `go generate ./vm/...`\")\n")
	b.WriteString("\t\t}\n")
	b.WriteString("\t\tBuiltinTable[idx] = b\n")
	b.WriteString("\t}\n")
	b.WriteString("\t// Reverse check: did eval/ gain builtins we don't know about?\n")
	b.WriteString("\tif got, want := len(eval.Builtins), NumBuiltins; got != want {\n")
	b.WriteString("\t\tpanic(\"vm: eval.Builtins has \" + itoa(got) + \" entries but generated table has \" + itoa(want) + \" — regenerate with `go generate ./vm/...`\")\n")
	b.WriteString("\t}\n")
	b.WriteString("}\n")

	formatted, err := format.Source(b.Bytes())
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: go/format failed on generated source: %v\n---\n%s\n---\n", err, b.String())
		return b.Bytes(), nil
	}
	return formatted, nil
}

// ── IO ────────────────────────────────────────────────────────────────────────

func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".vmbuiltins-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func findRoot() string {
	dir, _ := os.Getwd()
	for {
		data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil && strings.Contains(string(data), "module klex") {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "."
		}
		dir = parent
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "vmbuiltins: "+format+"\n", args...)
	os.Exit(1)
}
