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

	names, err := scanBuiltinNames(evalDir)
	if err != nil {
		fatalf("scan %s: %v", evalDir, err)
	}
	if len(names) == 0 {
		fatalf("no Builtins[\"…\"] = … registrations found in %s — broken scanner?", evalDir)
	}

	// Stable alphabetical ordering. Critical: every regen against the
	// same source set must produce the same indices, so a compiled
	// bytecode artifact stays valid across compiler runs.
	sort.Strings(names)

	src, err := renderBuiltinsGen(names)
	if err != nil {
		fatalf("render: %v", err)
	}
	if err := writeAtomic(outPath, src); err != nil {
		fatalf("write %s: %v", outPath, err)
	}
	fmt.Printf("✓ vmbuiltins — wrote %d builtins to %s\n", len(names), outPath)
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
func scanBuiltinNames(dir string) ([]string, error) {
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
func renderBuiltinsGen(names []string) ([]byte, error) {
	var b bytes.Buffer

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
