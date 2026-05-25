// refresh_thin.go — in-place refresh of thin/skeletal LSP builtin entries.
//
// The companion `--gen-lsp-stubs` flag only emits stubs for builtins that are
// ENTIRELY missing from snowball/froglsp/builtins.go. That leaves a separate
// failure mode untouched: entries that were bulk-added long ago to silence
// "unknown identifier" hovers, with a placeholder signature like
// `_processExec() -> any`, an empty Params slice, and a 2-4 word
// Documentation string. Hover renders, but it tells the user nothing.
//
// This file walks the LSP map via go/ast (robust against single-line vs
// multi-line entry shapes), classifies each entry as "thin" or not, and for
// every thin entry rewrites the BuiltinInfo composite literal in place using
// the Go-source `//` comment block harvested by extractHint. Byte-offset
// splicing preserves every untouched entry verbatim; go/format normalises
// the result.
//
// Heuristics for "thin" (any one triggers a refresh):
//
//  1. Signature contains "-> any" — the bulk-added placeholder return type.
//  2. Params is empty but the Go-source arity guard requires ≥1 argument.
//  3. Documentation is shorter than 60 chars AND the Go-source comment
//     block has substantively more prose to offer (>80 extra chars).
//
// A refresh is suppressed if the Go-source comment block is itself empty —
// rewriting one stub into another stub would just churn the file.

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

// lspEntry captures one entry in the builtinSignatures map literal, with the
// raw byte offsets of the composite-literal value so we can splice a
// replacement back into the file without disturbing surrounding entries.
type lspEntry struct {
	name       string
	signature  string
	doc        string
	params     []string
	valueStart int // byte offset of '{' opening the BuiltinInfo literal
	valueEnd   int // byte offset one past '}' closing it
}

// parseLSPEntries reads snowball/froglsp/builtins.go and returns every
// builtinSignatures map entry as an lspEntry plus the raw source bytes.
// Using go/ast (rather than line-by-line regex) means multi-line entries
// and odd formatting variants all parse cleanly.
func parseLSPEntries(root string) ([]lspEntry, []byte, error) {
	path := filepath.Join(root, "snowball", "froglsp", "builtins.go")
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, source, parser.AllErrors)
	if err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", path, err)
	}

	var entries []lspEntry
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
				if name.Name != "builtinSignatures" {
					continue
				}
				if i >= len(vs.Values) {
					continue
				}
				cl, ok := vs.Values[i].(*ast.CompositeLit)
				if !ok {
					continue
				}
				for _, elt := range cl.Elts {
					if e, ok := decodeEntry(elt, fset); ok {
						entries = append(entries, e)
					}
				}
			}
		}
	}
	return entries, source, nil
}

// decodeEntry pulls one map entry out of the AST. It tolerates omitted
// fields (treats them as empty strings / nil slice) but requires the value
// to be a composite literal — anything else (variable reference, function
// call) is skipped silently because we can't safely splice it.
func decodeEntry(elt ast.Expr, fset *token.FileSet) (lspEntry, bool) {
	kv, ok := elt.(*ast.KeyValueExpr)
	if !ok {
		return lspEntry{}, false
	}
	keyLit, ok := kv.Key.(*ast.BasicLit)
	if !ok || keyLit.Kind != token.STRING {
		return lspEntry{}, false
	}
	name, err := strconv.Unquote(keyLit.Value)
	if err != nil {
		return lspEntry{}, false
	}
	valLit, ok := kv.Value.(*ast.CompositeLit)
	if !ok {
		return lspEntry{}, false
	}
	e := lspEntry{
		name:       name,
		valueStart: fset.Position(valLit.Lbrace).Offset,
		valueEnd:   fset.Position(valLit.Rbrace).Offset + 1,
	}
	for _, fe := range valLit.Elts {
		fkv, ok := fe.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		fname, ok := fkv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		switch fname.Name {
		case "Signature":
			if lit, ok := fkv.Value.(*ast.BasicLit); ok {
				if v, err := strconv.Unquote(lit.Value); err == nil {
					e.signature = v
				}
			}
		case "Documentation":
			if lit, ok := fkv.Value.(*ast.BasicLit); ok {
				if v, err := strconv.Unquote(lit.Value); err == nil {
					e.doc = v
				}
			}
		case "Params":
			if pcl, ok := fkv.Value.(*ast.CompositeLit); ok {
				for _, pe := range pcl.Elts {
					if lit, ok := pe.(*ast.BasicLit); ok {
						if v, err := strconv.Unquote(lit.Value); err == nil {
							e.params = append(e.params, v)
						}
					}
				}
			}
		}
	}
	return e, true
}

// isThinLSPEntry classifies an entry as "needs refresh". See file header for
// the criteria. Returns false when the Go-source hint has nothing to offer
// — we won't rewrite a stub into another stub.
func isThinLSPEntry(e lspEntry, h hint, ga arity) bool {
	// 1. Placeholder return type — strongest single signal.
	if strings.Contains(e.signature, "-> any") {
		return true
	}
	// 2. Params-count outside the legal band the Go side accepts. Catches
	//    the empty-params bulk-add case AND the subtler case where
	//    someone hand-wrote (a, b, c) but Go actually enforces 4 args.
	//    A bounded-range arity (e.g. 4-5 for "4 required, 1 optional")
	//    is fine as long as the LSP listing falls inside [min, max] —
	//    publishing 5 params for a 4-5 builtin is correct, not "thin".
	//    Variadic Go (max == -1) is also OK to list short.
	if ga.found {
		n := len(e.params)
		switch {
		case ga.max == -1:
			if n < ga.min {
				return true
			}
		default:
			if n < ga.min || n > ga.max {
				return true
			}
		}
	}
	// 3. Very short LSP doc AND the Go-source has substantively more.
	if len(e.doc) < 60 && len(h.doc) > len(e.doc)+80 {
		return true
	}
	return false
}

// renderLSPEntryValue builds the replacement composite-literal text. Format
// matches the multi-line gofmt-style used throughout builtins.go:
//
//	{
//	    Signature:     "...",
//	    Documentation: "...",
//	    Params:        []string{"a", "b"},
//	}
//
// (no trailing comma — that's part of the surrounding map-element syntax
// and lives outside the bytes we replace.)
func renderLSPEntryValue(name string, h hint, ga arity) string {
	sig := h.sig
	if !strings.HasPrefix(sig, name+"(") {
		// extractHint occasionally returns a paren-only `(?)` fallback when
		// no signature line was extractable from the comment block. Prefix
		// the name so the result is at least syntactically a signature.
		if strings.HasPrefix(sig, "(?)") {
			sig = name + sig
		} else if sig == "" {
			sig = name + "(?) -> ?"
		}
	}
	sig = strings.ReplaceAll(sig, "→", "->")

	doc := stubDocFor(name, h)

	paramsStr := renderParamsLiteral(h.params)

	var b strings.Builder
	b.WriteString("{\n")
	fmt.Fprintf(&b, "\t\tSignature:     %s,\n", strconv.Quote(sig))
	fmt.Fprintf(&b, "\t\tDocumentation: %s,\n", strconv.Quote(doc))
	fmt.Fprintf(&b, "\t\tParams:        %s,\n", paramsStr)
	b.WriteString("\t}")
	return b.String()
}

// renderLSPEntryValueDocOnly builds a replacement composite literal that
// preserves the existing Signature and Params verbatim and replaces only
// the Documentation field. Used when the Go-source comment has rich prose
// but no machine-parseable `// name(args) → type` line — without this
// fallback we'd either skip the entry entirely (losing the doc upgrade)
// or clobber a hand-tuned signature with `(?)` placeholders.
func renderLSPEntryValueDocOnly(existingSig, newDoc string, existingParams []string) string {
	quotedParams := make([]string, 0, len(existingParams))
	for _, p := range existingParams {
		quotedParams = append(quotedParams, strconv.Quote(p))
	}
	paramsStr := "[]string{}"
	if len(quotedParams) > 0 {
		paramsStr = "[]string{" + strings.Join(quotedParams, ", ") + "}"
	}

	var b strings.Builder
	b.WriteString("{\n")
	fmt.Fprintf(&b, "\t\tSignature:     %s,\n", strconv.Quote(existingSig))
	fmt.Fprintf(&b, "\t\tDocumentation: %s,\n", strconv.Quote(newDoc))
	fmt.Fprintf(&b, "\t\tParams:        %s,\n", paramsStr)
	b.WriteString("\t}")
	return b.String()
}

// renderParamsLiteral converts a slice of raw param descriptors (possibly
// `name: type`, `?name`, or `name?` shapes) into a clean `[]string{"a", "b"}`
// Go literal — name-only, no type annotations, no optional markers. Empty
// input collapses to `[]string{}` (not nil; the existing entries use the
// explicit empty literal).
func renderParamsLiteral(raw []string) string {
	cleaned := make([]string, 0, len(raw))
	for _, p := range raw {
		p = strings.TrimSpace(p)
		if idx := strings.Index(p, ":"); idx > 0 {
			p = strings.TrimSpace(p[:idx])
		}
		p = strings.TrimPrefix(p, "?")
		p = strings.TrimSuffix(p, "?")
		p = strings.TrimPrefix(p, "...")
		p = strings.TrimSpace(p)
		if p != "" {
			cleaned = append(cleaned, strconv.Quote(p))
		}
	}
	if len(cleaned) == 0 {
		return "[]string{}"
	}
	return "[]string{" + strings.Join(cleaned, ", ") + "}"
}

// doRefreshThinLSP is the entry point for the --refresh-thin-lsp flag. It
// parses the LSP map, classifies every entry, splices replacement bytes for
// every thin one, runs go/format on the result, and writes back the file.
// With dryRun = true it prints the list and exits without touching disk.
//
// `verbose` (when paired with dry-run) also prints the rendered replacement
// text for every flagged entry — useful for eyeballing what the rewrite
// will look like before committing to disk.
//
// `nameFilter` (when non-empty) restricts the operation to entries whose
// name starts with the given prefix. Pass "_" to scope to underscore-
// prefixed primitives only — matches Karl's original ask.
func doRefreshThinLSP(root string, hints map[string]hint, goArity map[string]arity, dryRun, verbose bool, nameFilter string) {
	entries, source, err := parseLSPEntries(root)
	if err != nil {
		fatalf("%v", err)
	}

	type change struct {
		name       string
		valueStart int
		valueEnd   int
		newText    string
		reason     string
		docOnly    bool // true = preserved existing Signature + Params, replaced only Documentation
	}
	var changes []change
	for _, e := range entries {
		if nameFilter != "" && !strings.HasPrefix(e.name, nameFilter) {
			continue
		}
		h, hasHint := hints[e.name]
		if !hasHint || strings.TrimSpace(h.doc) == "" {
			continue
		}
		ga := goArity[e.name]
		if !isThinLSPEntry(e, h, ga) {
			continue
		}
		// Don't rewrite a stub into another stub: the new signature must be
		// concrete (not the `(?) -> ?` fallback) OR the doc must be
		// substantively richer than what's already there.
		newDoc := stubDocFor(e.name, h)
		if strings.Contains(h.sig, "(?)") && len(newDoc) <= len(e.doc)+20 {
			continue
		}
		// Regression guard: extractHint can mis-parse an EXAMPLE line
		// (`// acos(1) → 0.0`, `// range(stop) → [0, 1, ..., stop-1]`)
		// as a signature, harvesting numeric / quoted literals as
		// "param names" and bracketed example output as a "return type".
		// Validate both before trusting a full refresh; otherwise fall
		// back to doc-only refresh below.
		paramsValid := allValidParamNames(h.params)
		// h.sig is empty when no `// name(args) → type` line was
		// extractable; it contains `(?)` when only the return type was
		// recoverable; it ends with `-> ?` when only the args were
		// recoverable. Any of those means we can't trust the signature.
		sigUnknown := h.sig == "" ||
			strings.Contains(h.sig, "(?)") ||
			strings.HasSuffix(h.sig, " -> ?") ||
			!isSafeReturnType(h.sig)
		// Decide between full refresh and doc-only refresh:
		//   - Full refresh: new signature is concrete AND params parse.
		//   - Doc-only refresh: preserve existing Signature + Params,
		//     replace only Documentation. We do this whenever the new
		//     signature would lose information (`(?)` placeholder or
		//     return-type `?`, OR the harvested params look like junk).
		//     The existing entry is left untouched on signature/params
		//     so we never regress a hand-tuned hover.
		fullRefresh := !sigUnknown && paramsValid
		newText := ""
		if fullRefresh {
			newText = renderLSPEntryValue(e.name, h, ga)
		} else {
			// Doc-only refresh requires that the new doc is actually
			// richer — otherwise leave the entry as-is.
			newDocOnly := stubDocFor(e.name, h)
			if len(newDocOnly) <= len(e.doc)+20 {
				continue
			}
			newText = renderLSPEntryValueDocOnly(e.signature, newDocOnly, e.params)
		}
		reason := refreshReason(e, h, ga)
		if !fullRefresh {
			reason = "doc-only (" + reason + ")"
		}
		changes = append(changes, change{
			name:       e.name,
			valueStart: e.valueStart,
			valueEnd:   e.valueEnd,
			newText:    newText,
			reason:     reason,
			docOnly:    !fullRefresh,
		})
	}

	if len(changes) == 0 {
		fmt.Println("✓ No thin LSP entries detected — nothing to refresh.")
		return
	}

	// Splice from lowest offset upward so the cursor walks the source once.
	sort.Slice(changes, func(i, j int) bool {
		return changes[i].valueStart < changes[j].valueStart
	})
	var out bytes.Buffer
	cursor := 0
	for _, c := range changes {
		out.Write(source[cursor:c.valueStart])
		out.WriteString(c.newText)
		cursor = c.valueEnd
	}
	out.Write(source[cursor:])

	formatted, formatErr := format.Source(out.Bytes())
	if formatErr != nil {
		fmt.Fprintf(os.Stderr, "warning: gofmt failed on rebuilt file (%v); writing un-formatted output\n", formatErr)
		formatted = out.Bytes()
	}

	if dryRun {
		fmt.Printf("[dry-run] Would refresh %d thin entries:\n", len(changes))
		sort.Slice(changes, func(i, j int) bool { return changes[i].name < changes[j].name })
		for _, c := range changes {
			fmt.Printf("  - %-32s  (%s)\n", c.name, c.reason)
			if verbose {
				fmt.Println()
				for _, ln := range strings.Split(c.newText, "\n") {
					fmt.Println("        " + ln)
				}
				fmt.Println()
			}
		}
		return
	}

	path := filepath.Join(root, "snowball", "froglsp", "builtins.go")
	if err := os.WriteFile(path, formatted, 0644); err != nil {
		fatalf("cannot write %s: %v", path, err)
	}
	fmt.Printf("✓ Refreshed %d thin LSP entries in %s\n", len(changes), path)
	sort.Slice(changes, func(i, j int) bool { return changes[i].name < changes[j].name })
	for _, c := range changes {
		fmt.Printf("  - %-32s  (%s)\n", c.name, c.reason)
	}
}

// allValidParamNames returns true when every entry in params is a legal
// identifier (leading letter or underscore, then letters/digits/underscore,
// optionally suffixed with `?` or prefixed with `...` for the optional /
// variadic markers extractHint may produce). Returns true on empty input
// — a zero-arity builtin is fine.
//
// The check exists because extractHint sometimes mistakes an EXAMPLE line
// in a Go-source comment block (`// acos(1) → 0.0    acos(0) → ...`) for a
// signature, harvesting the numeric / quoted literals as "param names" and
// producing nonsense entries downstream.
func allValidParamNames(params []string) bool {
	for _, p := range params {
		p = strings.TrimSpace(p)
		p = strings.TrimPrefix(p, "...")
		p = strings.TrimSuffix(p, "?")
		// Strip type annotation if present (`name: type`).
		if idx := strings.Index(p, ":"); idx > 0 {
			p = strings.TrimSpace(p[:idx])
		}
		if p == "" || !isIdentifier(p) {
			return false
		}
	}
	return true
}

// isSafeReturnType checks the portion of a signature string AFTER the
// arrow. A legitimate kLex return type is one of:
//
//	identifier                     // int, string, channel
//	identifier?                    // string?
//	identifier | identifier        // string | null
//	(identifier, identifier, ...)  // (string, error)
//	identifier of identifier       // array of bytes
//
// Anything else — square brackets, digits-as-first-char, ellipsis, equals,
// commas-outside-parens — is almost certainly an example output that
// extractHint mis-parsed as a type (`range(stop) → [0, 1, ..., stop-1]`
// is the canonical fail case). Returning false routes the entry to
// doc-only refresh, preserving the existing signature.
//
// Conservative by design: false negatives just push the entry into the
// safer doc-only path; a false positive (publishing a garbage type)
// would silently corrupt hover output, which is what we're trying to
// prevent.
func isSafeReturnType(sig string) bool {
	idx := strings.Index(sig, "->")
	if idx < 0 {
		return false
	}
	rt := strings.TrimSpace(sig[idx+2:])
	if rt == "" {
		return false
	}
	// Forbidden characters: anything example-shaped.
	for _, r := range rt {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r == '_' || r == ' ' || r == '|' || r == ',' ||
			r == '(' || r == ')' || r == '?':
		default:
			return false
		}
	}
	return true
}

// isIdentifier is the kLex / Go-name-shape predicate.
func isIdentifier(s string) bool {
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r == '_':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return s != ""
}

// refreshReason returns a short tag explaining WHY this entry was flagged.
// Useful in the dry-run output so the user can sanity-check the heuristics.
func refreshReason(e lspEntry, h hint, ga arity) string {
	if strings.Contains(e.signature, "-> any") {
		return "placeholder signature"
	}
	if len(e.params) == 0 && ga.found && ga.min > 0 {
		return fmt.Sprintf("missing params (Go arity=%s)", ga)
	}
	if len(e.doc) < 60 && len(h.doc) > len(e.doc)+80 {
		return fmt.Sprintf("doc %dch → Go-source has %dch", len(e.doc), len(h.doc))
	}
	return "thin"
}
