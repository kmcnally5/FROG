// vmgen — generator for VM opcode boilerplate.
//
// Reads the hand-edited source of truth at vm/opcodes_def.go, parses
// its `opcodeDefs` slice literal via go/parser (so formatting changes
// don't break us), and emits vm/opcodes_gen.go containing:
//
//   - the numeric const block (Op + Name = iota, …)
//   - the opcode count constant (numOpcodes)
//   - a name lookup table (opcodeName(op) → string, for disassembly)
//   - a stack-effect table (opcodeStackEffect(op) → (in, out))
//   - a structural operand layout table (opcodeOperands(op) →
//     []operandDef) so the disassembler and stack-balance verifier
//     can both walk the instruction stream from one source.
//
// Run via `go generate ./vm/...` from project root. The output file
// carries a generated-code banner; do not edit it by hand.
//
// Design notes
//
//   - We use go/parser, NOT regex, on the def file: editors that
//     reformat the source must not break us, and field access has to
//     be reliable in the face of trailing commas, line breaks, or
//     unicode comments.
//   - Output goes through go/format.Source so it's gofmt-clean and
//     a `go vet` pass on the package keeps working.
//   - Writes are atomic (write-temp + rename) so a crash mid-write
//     can never leave an inconsistent generated file behind.
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
	"strconv"
	"strings"
)

// opEntry is the in-memory representation of one entry parsed out of
// the def file's slice literal. Kept independent of the package-level
// `opcodeDef` type so the generator has no compile-time dependency on
// the package it's generating into.
type opEntry struct {
	Name        string
	Operands    []operandEntry
	StackIn     int
	StackOut    int
	Description string
}

type operandEntry struct {
	Name string
	Kind string // verbatim Ident name from the def file, e.g. "opKindInt32"
}

func main() {
	root := findRoot()
	defPath := filepath.Join(root, "vm", "opcodes_def.go")
	outPath := filepath.Join(root, "vm", "opcodes_gen.go")

	entries, err := parseDefFile(defPath)
	if err != nil {
		fatalf("parse %s: %v", defPath, err)
	}
	if len(entries) == 0 {
		fatalf("no opcodeDefs entries found in %s — was the var renamed?", defPath)
	}

	// Sanity check: no duplicate opcode names. Catches copy-paste
	// errors at generate time rather than after the const block
	// quietly assigns two opcodes the same numeric value (it can't,
	// but the dispatcher would never know which Name() to return for
	// a given value).
	seen := make(map[string]bool, len(entries))
	for _, e := range entries {
		if seen[e.Name] {
			fatalf("duplicate opcode name %q in %s", e.Name, defPath)
		}
		seen[e.Name] = true
	}

	src, err := renderOpcodesGen(entries)
	if err != nil {
		fatalf("render: %v", err)
	}

	if err := writeAtomic(outPath, src); err != nil {
		fatalf("write %s: %v", outPath, err)
	}

	fmt.Printf("✓ vmgen — wrote %d opcodes to %s\n", len(entries), outPath)
}

// ── Parsing ───────────────────────────────────────────────────────────────────

// parseDefFile reads opcodes_def.go and extracts every element of the
// `opcodeDefs` var declaration as an opEntry. Returns an error if the
// var is missing, the value isn't a slice composite literal, or any
// entry is malformed (e.g. StackIn not a numeric literal).
func parseDefFile(path string) ([]opEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, data, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				if name.Name != "opcodeDefs" {
					continue
				}
				if i >= len(vs.Values) {
					return nil, fmt.Errorf("opcodeDefs has no value expression")
				}
				cl, ok := vs.Values[i].(*ast.CompositeLit)
				if !ok {
					return nil, fmt.Errorf("opcodeDefs is not a composite literal")
				}
				out := make([]opEntry, 0, len(cl.Elts))
				for idx, elt := range cl.Elts {
					entry, err := decodeOpEntry(elt)
					if err != nil {
						return nil, fmt.Errorf("opcodeDefs[%d]: %w", idx, err)
					}
					out = append(out, entry)
				}
				return out, nil
			}
		}
	}
	return nil, fmt.Errorf("opcodeDefs var not found")
}

// decodeOpEntry pulls Name / Operands / StackIn / StackOut /
// Description from a single composite literal element. Tolerates
// omitted fields (treats them as zero values) so adding new opcodes
// with default StackIn/StackOut requires no boilerplate.
func decodeOpEntry(elt ast.Expr) (opEntry, error) {
	cl, ok := elt.(*ast.CompositeLit)
	if !ok {
		return opEntry{}, fmt.Errorf("entry is not a struct literal")
	}
	var e opEntry
	for _, fe := range cl.Elts {
		kv, ok := fe.(*ast.KeyValueExpr)
		if !ok {
			return opEntry{}, fmt.Errorf("entry uses positional fields — please use named fields (Name: …, Operands: …)")
		}
		fname, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		switch fname.Name {
		case "Name":
			s, err := stringLit(kv.Value)
			if err != nil {
				return opEntry{}, fmt.Errorf("Name: %w", err)
			}
			e.Name = s
		case "Description":
			s, err := stringLit(kv.Value)
			if err != nil {
				return opEntry{}, fmt.Errorf("Description: %w", err)
			}
			e.Description = s
		case "StackIn":
			n, err := intLit(kv.Value)
			if err != nil {
				return opEntry{}, fmt.Errorf("StackIn: %w", err)
			}
			e.StackIn = n
		case "StackOut":
			n, err := intLit(kv.Value)
			if err != nil {
				return opEntry{}, fmt.Errorf("StackOut: %w", err)
			}
			e.StackOut = n
		case "Operands":
			ops, err := decodeOperandList(kv.Value)
			if err != nil {
				return opEntry{}, fmt.Errorf("Operands: %w", err)
			}
			e.Operands = ops
		}
	}
	if e.Name == "" {
		return opEntry{}, fmt.Errorf("entry has empty Name")
	}
	return e, nil
}

func decodeOperandList(expr ast.Expr) ([]operandEntry, error) {
	cl, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, fmt.Errorf("expected slice literal")
	}
	out := make([]operandEntry, 0, len(cl.Elts))
	for i, e := range cl.Elts {
		ocl, ok := e.(*ast.CompositeLit)
		if !ok {
			return nil, fmt.Errorf("operand[%d]: not a struct literal", i)
		}
		var op operandEntry
		for _, fe := range ocl.Elts {
			kv, ok := fe.(*ast.KeyValueExpr)
			if !ok {
				return nil, fmt.Errorf("operand[%d]: positional fields not supported", i)
			}
			fname := kv.Key.(*ast.Ident).Name
			switch fname {
			case "Name":
				s, err := stringLit(kv.Value)
				if err != nil {
					return nil, fmt.Errorf("operand[%d].Name: %w", i, err)
				}
				op.Name = s
			case "Kind":
				id, ok := kv.Value.(*ast.Ident)
				if !ok {
					return nil, fmt.Errorf("operand[%d].Kind: must be a Kind ident", i)
				}
				op.Kind = id.Name
			}
		}
		out = append(out, op)
	}
	return out, nil
}

// stringLit unquotes a basic string literal AST node.
func stringLit(expr ast.Expr) (string, error) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", fmt.Errorf("expected string literal, got %T", expr)
	}
	return strconv.Unquote(lit.Value)
}

// intLit accepts a positive integer literal OR a negated one (for
// the StackIn: -1 "variable arity" sentinel).
func intLit(expr ast.Expr) (int, error) {
	switch v := expr.(type) {
	case *ast.BasicLit:
		if v.Kind != token.INT {
			return 0, fmt.Errorf("expected INT literal, got %s", v.Kind)
		}
		return strconv.Atoi(v.Value)
	case *ast.UnaryExpr:
		if v.Op != token.SUB {
			return 0, fmt.Errorf("unsupported unary op %s", v.Op)
		}
		n, err := intLit(v.X)
		if err != nil {
			return 0, err
		}
		return -n, nil
	}
	return 0, fmt.Errorf("expected int literal, got %T", expr)
}

// ── Rendering ─────────────────────────────────────────────────────────────────

// renderOpcodesGen builds the generated Go source as bytes, then runs
// it through go/format so the output is gofmt-clean regardless of the
// templating choices below.
func renderOpcodesGen(entries []opEntry) ([]byte, error) {
	var b bytes.Buffer

	b.WriteString("// Code generated by vm/cmd/vmgen. DO NOT EDIT.\n")
	b.WriteString("//\n")
	b.WriteString("// Source of truth: vm/opcodes_def.go.\n")
	b.WriteString("// Regenerate with: go generate ./vm/...\n\n")
	b.WriteString("package vm\n\n")

	// ── Const block ─────────────────────────────────────────────────
	b.WriteString("// opcode is a single byte identifying a VM instruction.\n")
	b.WriteString("type opcode uint8\n\n")
	b.WriteString("const (\n")
	for i, e := range entries {
		if i == 0 {
			fmt.Fprintf(&b, "\tOp%s opcode = iota\n", e.Name)
		} else {
			fmt.Fprintf(&b, "\tOp%s\n", e.Name)
		}
	}
	b.WriteString(")\n\n")

	fmt.Fprintf(&b, "// numOpcodes is the total count — used to size the dispatch\n")
	fmt.Fprintf(&b, "// and verification tables.\n")
	fmt.Fprintf(&b, "const numOpcodes = %d\n\n", len(entries))

	// ── Name table ──────────────────────────────────────────────────
	b.WriteString("// opcodeNames maps opcode → canonical mnemonic.\n")
	b.WriteString("// Used by the disassembler and error messages.\n")
	b.WriteString("var opcodeNames = [numOpcodes]string{\n")
	for _, e := range entries {
		fmt.Fprintf(&b, "\tOp%s: %q,\n", e.Name, e.Name)
	}
	b.WriteString("}\n\n")

	// ── Stack effect table ─────────────────────────────────────────
	b.WriteString("// stackEffect describes how many values an opcode pops and pushes.\n")
	b.WriteString("// Variable is true when the effect depends on an operand (CallBuiltin's\n")
	b.WriteString("// argc), in which case the disassembler / verifier must read the\n")
	b.WriteString("// operands to compute the real number.\n")
	b.WriteString("type stackEffect struct {\n")
	b.WriteString("\tIn, Out  int\n")
	b.WriteString("\tVariable bool\n")
	b.WriteString("}\n\n")

	b.WriteString("// opcodeStackEffects holds the static stack effect for every opcode.\n")
	b.WriteString("var opcodeStackEffects = [numOpcodes]stackEffect{\n")
	for _, e := range entries {
		variable := e.StackIn < 0 || e.StackOut < 0
		fmt.Fprintf(&b, "\tOp%s: {In: %d, Out: %d, Variable: %t},\n",
			e.Name, e.StackIn, e.StackOut, variable)
	}
	b.WriteString("}\n\n")

	// ── Operand layout table ───────────────────────────────────────
	b.WriteString("// opcodeOperandLayout describes the immediate operands an opcode\n")
	b.WriteString("// carries in the instruction stream. The disassembler walks each\n")
	b.WriteString("// instruction using this table; the stack-balance verifier uses it\n")
	b.WriteString("// to step over the operand bytes before reading the next opcode.\n")
	b.WriteString("var opcodeOperandLayout = [numOpcodes][]operandDef{\n")
	for _, e := range entries {
		if len(e.Operands) == 0 {
			fmt.Fprintf(&b, "\tOp%s: nil,\n", e.Name)
			continue
		}
		fmt.Fprintf(&b, "\tOp%s: {\n", e.Name)
		for _, op := range e.Operands {
			fmt.Fprintf(&b, "\t\t{Name: %q, Kind: %s},\n", op.Name, op.Kind)
		}
		b.WriteString("\t},\n")
	}
	b.WriteString("}\n\n")

	// ── Description table (for disassembly + tooling) ──────────────
	b.WriteString("// opcodeDescriptions holds the human-readable description of each opcode,\n")
	b.WriteString("// lifted verbatim from opcodes_def.go. Used by `klexdis --verbose` and\n")
	b.WriteString("// by error messages that surface unknown-opcode bugs.\n")
	b.WriteString("var opcodeDescriptions = [numOpcodes]string{\n")
	for _, e := range entries {
		fmt.Fprintf(&b, "\tOp%s: %q,\n", e.Name, e.Description)
	}
	b.WriteString("}\n\n")

	// ── Convenience accessors ──────────────────────────────────────
	b.WriteString("// String returns the canonical name of the opcode, or \"opcode(NN)\" for\n")
	b.WriteString("// out-of-range values. Safe to call on any uint8 — never panics.\n")
	b.WriteString("func (op opcode) String() string {\n")
	b.WriteString("\tif int(op) < numOpcodes {\n")
	b.WriteString("\t\treturn opcodeNames[op]\n")
	b.WriteString("\t}\n")
	b.WriteString("\treturn \"opcode(\" + itoa(int(op)) + \")\"\n")
	b.WriteString("}\n\n")

	// Tiny local itoa (avoids pulling strconv into the generated file).
	b.WriteString("// itoa is a minimal integer formatter, kept local so the generated file\n")
	b.WriteString("// has no external imports beyond what the source-of-truth itself needs.\n")
	b.WriteString("func itoa(n int) string {\n")
	b.WriteString("\tif n == 0 {\n")
	b.WriteString("\t\treturn \"0\"\n")
	b.WriteString("\t}\n")
	b.WriteString("\tneg := n < 0\n")
	b.WriteString("\tif neg {\n")
	b.WriteString("\t\tn = -n\n")
	b.WriteString("\t}\n")
	b.WriteString("\tvar buf [20]byte\n")
	b.WriteString("\ti := len(buf)\n")
	b.WriteString("\tfor n > 0 {\n")
	b.WriteString("\t\ti--\n")
	b.WriteString("\t\tbuf[i] = byte('0' + n%10)\n")
	b.WriteString("\t\tn /= 10\n")
	b.WriteString("\t}\n")
	b.WriteString("\tif neg {\n")
	b.WriteString("\t\ti--\n")
	b.WriteString("\t\tbuf[i] = '-'\n")
	b.WriteString("\t}\n")
	b.WriteString("\treturn string(buf[i:])\n")
	b.WriteString("}\n")

	// gofmt-clean the result.
	formatted, err := format.Source(b.Bytes())
	if err != nil {
		// Fall back to unformatted output so the user can diagnose;
		// also dump the un-formatted source to stderr.
		fmt.Fprintf(os.Stderr, "warning: go/format failed on generated source: %v\n---\n%s\n---\n", err, b.String())
		return b.Bytes(), nil
	}
	return formatted, nil
}

// ── IO ────────────────────────────────────────────────────────────────────────

// writeAtomic writes data to path via a same-directory temp file and
// os.Rename — so a crash mid-write can't leave a half-written generated
// file behind. Caller's previous output (if any) is replaced atomically.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".vmgen-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op if rename succeeded
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// findRoot walks up from cwd looking for a go.mod whose `module` line
// names `klex`. Mirrors the existing convention used by tools/kpkg and
// tools/syncdocs so all kLex tools resolve the same project root.
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
	fmt.Fprintf(os.Stderr, "vmgen: "+format+"\n", args...)
	os.Exit(1)
}
