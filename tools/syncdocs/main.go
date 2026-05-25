// syncdocs — kLex documentation sync tool
//
// Audits every gap between Go source, LSP docs, VS Code syntax, and the
// language/grammar reference files, then optionally fixes the mechanical ones.
//
// Usage (run from project root):
//
//	go run ./tools/syncdocs                    — full audit report
//	go run ./tools/syncdocs --fix-vscode       — rewrite VS Code syntax regex in-place
//	go run ./tools/syncdocs --gen-lsp-stubs    — print LSP entries for undocumented builtins
//	                                              (uses real docs scraped from Go comments)
//	go run ./tools/syncdocs --gen-lang-blocks  — print paste-ready markdown for missing
//	                                              KLEX_LANGUAGE.MD entries, grouped by
//	                                              source file (domain category)
//	go run ./tools/syncdocs --refresh-thin-lsp — rewrite present-but-thin LSP entries
//	                                              in place from Go-source comments
//	                                              (add --dry-run to preview without writing)
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ── Regexes ───────────────────────────────────────────────────────────────────

var (
	// Matches: Builtins["name"] = (string literal — ignores variable-based stubs like Builtins[n]=)
	reBuiltinReg = regexp.MustCompile(`Builtins\["([a-zA-Z_][^"]*?)"\]\s*=`)

	// Matches: "name": {Fn: ... in eval.go's var Builtins map literal.
	// The core builtins (len, str, int, channel, recv, send, etc.) live there.
	reBuiltinMapLit = regexp.MustCompile(`^\s+"([a-zA-Z_][^"]*?)":\s*\{Fn:`)

	// Matches: "name": { (top-level LSP map key — BuiltinInfo struct)
	reLSPKey = regexp.MustCompile(`^\s+"([a-zA-Z_][^"]*?)":\s*\{`)

	// Matches: typeError("name expects N arguments: a, b, c", ...) or runtimeError(...)
	reErrMsg = regexp.MustCompile(`(?:typeError|runtimeError)\("([^"]+)"`)

	// Matches: "name expects N-M arguments: arg1, arg2, ..." inside an error message
	reExpectsArgs = regexp.MustCompile(`^\w+\s+expects\s+\d+(?:-\d+)?\s+arguments?:\s*(.+)`)

	// Matches: // funcName(...) → type  or  // funcName(...) -> type
	//
	// The return-type capture is intentionally permissive — it allows whitespace
	// and parens (so `(string, err)`, `(int, error)`, `string|null` all parse)
	// and terminates on either an em-dash separator (` — prose...`) or end of
	// line. Earlier versions used `[^\s—\n]+` which truncated tuple return
	// types at the first comma's trailing space.
	reCommentSig = regexp.MustCompile(`//\s*([a-zA-Z_]\w*\s*\([^)]*\))\s*(?:→|->)\s*([^\n]+?)\s*(?:—|$)`)

	// Matches: // funcName(args) — description   (no arrow, em-dash divider)
	// Used as a fallback when reCommentSig doesn't fire. Most graphics/UI
	// builtins use this shape instead of the arrow form, so we want both.
	reCommentParenSig = regexp.MustCompile(`^//\s*([a-zA-Z_]\w*\s*\([^)]*\))`)

	// Arity guards in Go source. We scan a window of lines AFTER the registration
	// line and pull the FIRST guard we encounter — earlier guards win because
	// builtins always validate arity before validating types.
	//
	// Patterns we recognise (literal arity only — no constant references):
	//   len(args) != 2                  → exact arity 2
	//   len(args) < 1                   → min 1, no max  (variadic)
	//   len(args) >= 1                  → min 1, no max  (variadic)
	//   len(args) > N || len(args) < M  → range (we collect both halves)
	//   len(args) < N || len(args) > M  → range (same)
	reArityEq    = regexp.MustCompile(`len\(args\)\s*!=\s*(\d+)`)
	reArityLT    = regexp.MustCompile(`len\(args\)\s*<\s*(\d+)`)
	reArityGT    = regexp.MustCompile(`len\(args\)\s*>\s*(\d+)`)
	reArityLTEq  = regexp.MustCompile(`len\(args\)\s*<=\s*(\d+)`)
	reArityGTEq  = regexp.MustCompile(`len\(args\)\s*>=\s*(\d+)`)

	// LSP Signature literal: Signature:     "name(a: TYPE, b: TYPE)", -> TYPE  …
	// We only care about the argument list between the FIRST '(' and the matching ')'.
	reLSPSig = regexp.MustCompile(`Signature:\s*"([^"]+)"`)
)

// ── Types ─────────────────────────────────────────────────────────────────────

type hint struct {
	sig    string   // extracted signature, e.g. "foo(x, y) -> int"
	params []string // param names (no types)
	doc    string   // full doc body scraped from `//` comment block above
	//              // registration — first line is the signature line itself;
	//              // subsequent lines are the description. Newline-joined.
	file string // source file basename (e.g. "builtins_image_fx.go"). Used
	//          // by --gen-lang-blocks to group output by domain.
}

// arity describes the set of arg counts a builtin accepts.
//
// Most builtins are contiguous: 0, 1-3, 2+. Some are DISCRETE — e.g.
// drawImage uses `if len(args) != 3 && len(args) != 5` to accept {3, 5}
// (no 4-arg form). For discrete cases the `discrete` slice carries the
// exact accepted counts; min/max are still populated as the bounding box
// (useful for display).
//
// max == -1 means "unbounded" (variadic). Variadic + discrete is illegal
// (would mean "infinite set with holes") and we don't try to represent it.
//
// found == false means the extractor couldn't recover an arity (no guard,
// or the guard uses a constant we don't parse).
type arity struct {
	min      int
	max      int    // -1 = unbounded
	discrete []int  // nil = contiguous min..max; non-nil = exact accepted counts
	found    bool
}

func (a arity) String() string {
	if !a.found {
		return "?"
	}
	if a.discrete != nil {
		parts := make([]string, len(a.discrete))
		for i, v := range a.discrete {
			parts[i] = strconv.Itoa(v)
		}
		return strings.Join(parts, "|")
	}
	if a.max == -1 {
		return fmt.Sprintf("%d+", a.min)
	}
	if a.min == a.max {
		return fmt.Sprintf("%d", a.min)
	}
	return fmt.Sprintf("%d-%d", a.min, a.max)
}

// acceptedSet returns the canonical form for comparison:
//   (vals, false) for any bounded arity (vals is sorted, deduped, holds
//                 every accepted arg count)
//   (nil,  true)  for an unbounded arity — caller must compare via min
func (a arity) acceptedSet() (vals []int, unbounded bool) {
	if a.max == -1 {
		return nil, true
	}
	if a.discrete != nil {
		return a.discrete, false
	}
	vals = make([]int, 0, a.max-a.min+1)
	for v := a.min; v <= a.max; v++ {
		vals = append(vals, v)
	}
	return vals, false
}

// equals: two arities match iff they accept exactly the same set of
// argument counts. If either side is unknown we return true (silent —
// only report mismatches we are sure about).
func (a arity) equals(b arity) bool {
	if !a.found || !b.found {
		return true
	}
	aVals, aUnbounded := a.acceptedSet()
	bVals, bUnbounded := b.acceptedSet()
	if aUnbounded || bUnbounded {
		// Two unbounded arities are equal iff they share a min.
		// An unbounded vs bounded arity is never equal.
		return aUnbounded && bUnbounded && a.min == b.min
	}
	if len(aVals) != len(bVals) {
		return false
	}
	for i := range aVals {
		if aVals[i] != bVals[i] {
			return false
		}
	}
	return true
}

// normaliseDiscrete sorts and dedupes a slice of accepted arg counts,
// then collapses to a contiguous range if the result is contiguous (so
// {3, 4, 5} becomes range 3-5; {3, 5} stays discrete).
func normaliseDiscrete(vals []int) arity {
	if len(vals) == 0 {
		return arity{}
	}
	sort.Ints(vals)
	// dedupe in place
	out := vals[:1]
	for i := 1; i < len(vals); i++ {
		if vals[i] != vals[i-1] {
			out = append(out, vals[i])
		}
	}
	vals = out
	if len(vals) == 1 {
		return arity{min: vals[0], max: vals[0], found: true}
	}
	contiguous := true
	for i := 1; i < len(vals); i++ {
		if vals[i] != vals[i-1]+1 {
			contiguous = false
			break
		}
	}
	if contiguous {
		return arity{min: vals[0], max: vals[len(vals)-1], found: true}
	}
	return arity{min: vals[0], max: vals[len(vals)-1], discrete: vals, found: true}
}

type arityMismatch struct {
	name   string
	source arity // Go (truth)
	lsp    arity // LSP claim
}

// ── Main ──────────────────────────────────────────────────────────────────────

func main() {
	root := findRoot()

	fixVSCode := false
	genStubs := false
	genLangBlocks := false
	refreshThinLSP := false
	dryRun := false
	verbose := false
	nameFilter := ""
	for _, arg := range os.Args[1:] {
		switch {
		case arg == "--fix-vscode":
			fixVSCode = true
		case arg == "--gen-lsp-stubs":
			genStubs = true
		case arg == "--gen-lang-blocks":
			genLangBlocks = true
		case arg == "--refresh-thin-lsp":
			refreshThinLSP = true
		case arg == "--dry-run":
			dryRun = true
		case arg == "--verbose" || arg == "-v":
			verbose = true
		case strings.HasPrefix(arg, "--name-prefix="):
			nameFilter = strings.TrimPrefix(arg, "--name-prefix=")
		}
	}

	goNames, hints, goArity := extractGoBuiltins(root)
	lspNames := extractLSPBuiltins(root)
	lspArity := extractLSPArity(root)
	syntaxNames := extractSyntaxBuiltins(root)
	arityMismatches := compareArities(goArity, lspArity)

	goSet := toSet(goNames)
	lspSet := toSet(lspNames)

	missingLSP := diff(goNames, lspSet)
	staleLSP := diff(lspNames, goSet)
	missingSyntax := diff(goNames, syntaxNames)
	missingLang, missingGrammar := checkDocs(root, goNames)

	if fixVSCode {
		doFixVSCode(root, goNames)
		return
	}
	if genStubs {
		doGenStubs(missingLSP, hints)
		return
	}
	if genLangBlocks {
		doGenLangBlocks(missingLang, hints)
		return
	}
	if refreshThinLSP {
		doRefreshThinLSP(root, hints, goArity, dryRun, verbose, nameFilter)
		return
	}

	// ── Audit report ─────────────────────────────────────────────────────────
	fmt.Println("╔══════════════════════════════════════════════════════╗")
	fmt.Println("║  kLex Doc Sync Audit                                 ║")
	fmt.Println("╚══════════════════════════════════════════════════════╝")
	fmt.Println()

	printSection("MISSING FROM LSP  (snowball/froglsp/builtins.go)", missingLSP)
	printSection("STALE IN LSP  — documented but no longer registered in Go", staleLSP)
	printSection("MISSING FROM VS CODE SYNTAX  (klex.tmLanguage.json)", missingSyntax)
	printSection("NOT FOUND IN KLEX_LANGUAGE.MD", missingLang)
	printSection("NOT FOUND IN KLEX_GRAMMAR.MD", missingGrammar)
	printArityMismatches(arityMismatches)

	fmt.Println()
	fmt.Println("──────────────────────────────────────────────────────")
	fmt.Printf("  Go builtins registered : %d\n", len(goNames))
	fmt.Printf("  LSP documented         : %d  (%d missing, %d stale)\n", len(lspNames), len(missingLSP), len(staleLSP))
	fmt.Printf("  VS Code syntax gaps    : %d\n", len(missingSyntax))
	fmt.Printf("  Language doc gaps      : %d  (name not in any fenced block or inline-backtick span)\n", len(missingLang))
	fmt.Printf("  Grammar doc gaps       : %d  (same rule as language doc)\n", len(missingGrammar))
	fmt.Println()

	if len(missingSyntax) > 0 || len(missingLSP) > 0 || len(missingLang) > 0 {
		fmt.Println("To fix automatically:")
		if len(missingSyntax) > 0 {
			fmt.Println("  go run ./tools/syncdocs --fix-vscode        (rewrites syntax regex in-place)")
		}
		if len(missingLSP) > 0 {
			fmt.Println("  go run ./tools/syncdocs --gen-lsp-stubs     (LSP entries — uses real comment docs)")
		}
		if len(missingLang) > 0 {
			fmt.Println("  go run ./tools/syncdocs --gen-lang-blocks   (paste-ready markdown grouped by section)")
		}
		fmt.Println()
	}
}

// ── Extraction ────────────────────────────────────────────────────────────────

func extractGoBuiltins(root string) ([]string, map[string]hint, map[string]arity) {
	pattern := filepath.Join(root, "eval", "builtins_*.go")
	files, _ := filepath.Glob(pattern)
	files = append(files, filepath.Join(root, "eval", "eval.go"))

	seen := make(map[string]bool)
	hints := make(map[string]hint)
	arities := make(map[string]arity)

	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")

		for i, line := range lines {
			var name string

			if m := reBuiltinReg.FindStringSubmatch(line); m != nil {
				// Pattern: Builtins["name"] = — used in builtins_*.go and eval.go init()
				name = m[1]
			} else if m := reBuiltinMapLit.FindStringSubmatch(line); m != nil {
				// Pattern: "name": {Fn: — used in eval.go's var Builtins map literal
				name = m[1]
			} else {
				continue
			}

			seen[name] = true
			if _, exists := hints[name]; !exists {
				h := extractHint(lines, i)
				h.file = filepath.Base(f)
				hints[name] = h
			}
			if _, exists := arities[name]; !exists {
				arities[name] = extractArity(lines, i)
			}
		}
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, hints, arities
}

// extractArity finds the first len(args) guard in the block after the
// builtin's registration line and converts it into an arity range.
//
// Boundary discipline: scanning stops as soon as we see the registration
// of the NEXT builtin (either `Builtins["..."] =` or `"name": {Fn:`).
// Without this boundary, long graphics/UI builtins (>35 lines) would
// leak arity guards from the following builtin and produce false
// positives like "mouseClicked source=4" (it takes 0 args).
//
// We scan a generous window (60 lines max) for builtins that DO have a
// guard but place it after a chunk of state setup. In practice the guard
// is in the first 10 lines, but we keep slack for graphics/UI code.
func extractArity(lines []string, regLine int) arity {
	end := regLine + 60
	if end > len(lines) {
		end = len(lines)
	}

	for j := regLine + 1; j < end; j++ {
		line := lines[j]

		// Boundary: stop the moment we see the next builtin registered.
		// Either form — `Builtins["..."] =` or `"name": {Fn:` — counts.
		if reBuiltinReg.FindStringSubmatch(line) != nil {
			return arity{}
		}
		if reBuiltinMapLit.FindStringSubmatch(line) != nil {
			return arity{}
		}

		// `len(args) != A [&& len(args) != B [&& ...]]` — discrete arity set.
		// Catches single (`!= 3`) and multi (`!= 3 && != 5`) on the same line.
		if matches := reArityEq.FindAllStringSubmatch(line, -1); len(matches) > 0 {
			vals := make([]int, 0, len(matches))
			for _, m := range matches {
				vals = append(vals, atoi(m[1]))
			}
			// If there are multiple captures, they must be AND-joined to
			// represent a discrete accepted set. `||` between `!= N` clauses
			// would be a tautology (any int is != to either), so it doesn't
			// occur in real builtins — but we still gate on `&&` for safety.
			if len(vals) > 1 && !strings.Contains(line, "&&") {
				// Not an AND-chain; treat as a single arity check on the first.
				vals = vals[:1]
			}
			return normaliseDiscrete(vals)
		}

		// Compound range guard: look for both `<` and `>` on the same line.
		// Patterns we recognise (literal numbers only):
		//   if len(args) < 7 || len(args) > 8
		//   if len(args) > 8 || len(args) < 7
		hasLT := reArityLT.FindStringSubmatch(line)
		hasGT := reArityGT.FindStringSubmatch(line)
		if hasLT != nil && hasGT != nil {
			lo := atoi(hasLT[1])
			hi := atoi(hasGT[1])
			return arity{min: lo, max: hi, found: true}
		}

		// `len(args) < N` standalone — variadic minimum (no upper bound stated)
		if hasLT != nil {
			return arity{min: atoi(hasLT[1]), max: -1, found: true}
		}

		// `len(args) >= N` standalone — variadic minimum
		if m := reArityGTEq.FindStringSubmatch(line); m != nil {
			return arity{min: atoi(m[1]), max: -1, found: true}
		}
	}
	return arity{}
}

// extractLSPArity parses the LSP `builtins.go` file and returns the arity
// implied by each entry's `Signature` literal. We look at the substring
// between the FIRST '(' and the matching ')', count commas, and check for
// optional or variadic markers.
//
//	Signature: "foo(a, b)"            → 2 required, 2 max     → {2, 2}
//	Signature: "foo(a, b, ...rest)"   → 2 required, unbounded → {2, -1}
//	Signature: "foo(fn, ...args)"     → 1 required, unbounded → {1, -1}
//	Signature: "foo(a, b?)"           → 1 required, 2 max     → {1, 2}
//	Signature: "foo(a, [size])"       → 1 required, 2 max     → {1, 2}
//	Signature: "foo(a, b = 1)"        → 1 required, 2 max     → {1, 2}
func extractLSPArity(root string) map[string]arity {
	path := filepath.Join(root, "snowball", "froglsp", "builtins.go")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	result := make(map[string]arity)
	lines := strings.Split(string(data), "\n")

	currentName := ""
	for _, line := range lines {
		// New entry — capture the name to attach the next Signature to.
		if m := reLSPKey.FindStringSubmatch(line); m != nil {
			name := m[1]
			if name == "Signature" || name == "Documentation" || name == "Params" {
				continue
			}
			currentName = name
			continue
		}

		if currentName == "" {
			continue
		}

		// Same line might contain `Signature: "..."` either on its own line
		// or inline with the brace — match it independently.
		if m := reLSPSig.FindStringSubmatch(line); m != nil {
			result[currentName] = parseSigArityMulti(m[1])
			currentName = "" // claim consumed
		}
	}
	return result
}

// parseSigArityMulti handles LSP signatures that document multiple
// call shapes joined by ` | `. Each shape is parsed independently and
// the accepted arg counts are unioned into a single arity.
//
// Examples:
//   "drawImage(img, x, y) | drawImage(img, x, y, w, h) -> null"
//                                                     → {3, 5}
//   "name(a) -> int"                                  → 1
//
// We split on TOP-LEVEL pipes only — `|` characters inside parens are
// type unions (e.g. `dbQuery(conn: DB_CONN | DB_TX, …)`) and must NOT
// trigger a shape split.
func parseSigArityMulti(sig string) arity {
	shapes := splitTopLevelPipes(sig)
	if len(shapes) <= 1 {
		return parseSigArity(sig)
	}

	var vals []int
	anyUnbounded := false
	unboundedMin := -1
	for _, shape := range shapes {
		a := parseSigArity(shape)
		if !a.found {
			continue
		}
		if a.max == -1 {
			anyUnbounded = true
			if unboundedMin == -1 || a.min < unboundedMin {
				unboundedMin = a.min
			}
			continue
		}
		if a.discrete != nil {
			vals = append(vals, a.discrete...)
		} else {
			for v := a.min; v <= a.max; v++ {
				vals = append(vals, v)
			}
		}
	}

	if anyUnbounded {
		// If any shape is variadic, the whole signature is — collapse to
		// the smallest min across all shapes.
		return arity{min: unboundedMin, max: -1, found: true}
	}
	return normaliseDiscrete(vals)
}

// parseSigArity reads the arg list of a signature string like
// `name(a, b, ...rest)` and returns the inferred arity.
//
// We must find the close paren that BALANCES the first '(' — not just the
// last ')' in the string, because return-type annotations like
// `parseInt(str: string) -> (int, error)` contain their own parens and the
// naive last-paren approach treats the entire return type as part of the
// arg list (off-by-one+ errors).
func parseSigArity(sig string) arity {
	open := strings.Index(sig, "(")
	if open < 0 {
		return arity{}
	}
	close := matchingCloseParen(sig, open)
	if close < 0 {
		return arity{}
	}
	inner := strings.TrimSpace(sig[open+1 : close])
	if inner == "" {
		return arity{min: 0, max: 0, found: true}
	}

	// Split on top-level commas only (none of our signatures contain nested
	// commas, so a simple split is correct).
	rawParams := strings.Split(inner, ",")

	required := 0
	optional := 0
	variadic := false
	for _, p := range rawParams {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// Bracketed-optional form `[name]` or `[name: TYPE]` — strip the
		// brackets FIRST so the subsequent type-colon strip doesn't eat
		// the closing `]` and hide the optional marker.
		bracketed := false
		if strings.HasPrefix(p, "[") && strings.HasSuffix(p, "]") {
			p = strings.TrimSpace(p[1 : len(p)-1])
			bracketed = true
		}
		// Strip type annotation: "name: TYPE" → "name"
		if idx := strings.Index(p, ":"); idx >= 0 {
			p = strings.TrimSpace(p[:idx])
		}
		// Strip default: "name = 1" → "name"
		if idx := strings.Index(p, "="); idx >= 0 {
			p = strings.TrimSpace(p[:idx])
			// A "name = …" form means optional. But we've already stripped it,
			// so flag and continue.
			optional++
			continue
		}
		if bracketed {
			optional++
			continue
		}
		// Variadic marker prefix on the param name (`...args`) — most common form.
		if strings.HasPrefix(p, "...") {
			variadic = true
			continue
		}
		// Optional marker suffix (`name?`).
		if strings.HasSuffix(p, "?") {
			optional++
			continue
		}
		required++
	}

	if variadic {
		return arity{min: required, max: -1, found: true}
	}
	return arity{min: required, max: required + optional, found: true}
}

// compareArities runs through every name common to BOTH maps and returns
// the mismatches, sorted by name for stable output.
func compareArities(goArity, lspArity map[string]arity) []arityMismatch {
	var out []arityMismatch
	for name, src := range goArity {
		claim, ok := lspArity[name]
		if !ok {
			continue
		}
		if !src.equals(claim) {
			out = append(out, arityMismatch{name: name, source: src, lsp: claim})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// extractHint tries to infer a signature AND a doc body for a builtin from
// its surrounding source.
//
//  1. Scans upward from regLine collecting the contiguous `//` comment block.
//     Stops at the first non-comment line. No fixed line cap — comment blocks
//     can be 20+ lines for builtins with rich docs (Metal, UI, etc.).
//  2. Looks for a signature on any line of that block:
//     - First tries the arrow form  `// name(args) → type`  /  `... -> type`
//     - Falls back to paren-only    `// name(args) — description`  on the
//     first comment line (em-dash is the kLex house style for builtins
//     without explicit return-type annotation).
//  3. Captures the full doc body — every comment line, leading `// ` stripped,
//     joined with `\n` — into h.doc. Used by --gen-lsp-stubs and
//     --gen-lang-blocks so generated entries inherit real human-written prose
//     instead of "TODO" placeholders.
//  4. If no comment-derived signature was found, searches BELOW for a
//     typeError/runtimeError message listing arguments AND a `return &Type{}`
//     to infer the return type.
func extractHint(lines []string, regLine int) hint {
	var h hint

	// ── Pass 1: collect contiguous comment block above regLine ──────────
	var commentLines []string
	for j := regLine - 1; j >= 0; j-- {
		trimmed := strings.TrimSpace(lines[j])
		if !strings.HasPrefix(trimmed, "//") {
			break
		}
		commentLines = append([]string{trimmed}, commentLines...)
	}

	// ── Pass 2: extract signature from the comment block ────────────────
	if len(commentLines) > 0 {
		// Arrow shape wins if present — it has the return type explicit.
		for _, c := range commentLines {
			if m := reCommentSig.FindStringSubmatch(c); m != nil {
				sig := m[1]
				ret := strings.TrimSuffix(m[2], ".")
				ret = strings.ReplaceAll(ret, "→", "->")
				h.sig = sig + " -> " + ret
				if idx := strings.Index(sig, "("); idx >= 0 {
					inner := strings.TrimSuffix(sig[idx+1:], ")")
					h.params = splitParams(inner)
				}
				break
			}
		}
		// Paren-only fallback — return type stays as "?" and is filled in
		// later by the return-type scanner below.
		if h.sig == "" {
			first := commentLines[0]
			if m := reCommentParenSig.FindStringSubmatch(first); m != nil {
				sig := m[1]
				h.sig = sig + " -> ?"
				if idx := strings.Index(sig, "("); idx >= 0 {
					inner := strings.TrimSuffix(sig[idx+1:], ")")
					h.params = splitParams(inner)
				}
			}
		}

		// ── Capture doc body ────────────────────────────────────────────
		var docLines []string
		for _, c := range commentLines {
			body := strings.TrimPrefix(c, "//")
			if strings.HasPrefix(body, " ") {
				body = body[1:]
			}
			docLines = append(docLines, body)
		}
		h.doc = strings.TrimSpace(strings.Join(docLines, "\n"))
	}

	// ── Pass 3: scan below for return type + (if needed) error-message args
	searchEnd := regLine + 35
	if searchEnd > len(lines) {
		searchEnd = len(lines)
	}

	var retType string
	for j := regLine + 1; j < searchEnd; j++ {
		line := lines[j]

		// Infer return type from first concrete return statement
		if retType == "" {
			t := strings.TrimSpace(line)
			switch {
			case strings.Contains(t, "return &Boolean{"):
				retType = "bool"
			case strings.Contains(t, "return &Integer{"):
				retType = "int"
			case strings.Contains(t, "return &Float{"):
				retType = "float"
			case strings.Contains(t, "return &String{"):
				retType = "string"
			case strings.Contains(t, "return &Array{"):
				retType = "array"
			case strings.Contains(t, "return &Tuple{"):
				retType = "tuple"
			case strings.Contains(t, "return NULL"):
				retType = "null"
			}
		}

		// Extract params from error message
		if h.sig == "" {
			if em := reErrMsg.FindStringSubmatch(line); em != nil {
				msg := em[1]
				if am := reExpectsArgs.FindStringSubmatch(msg); am != nil {
					rawParams := am[1]
					// Strip trailing optional-param annotations like "[, size]"
					rawParams = regexp.MustCompile(`\s*\[.*`).ReplaceAllString(rawParams, "")
					h.params = splitParams(rawParams)
					if retType == "" {
						retType = "?"
					}
					// Extract name from the message (first word)
					parts := strings.Fields(msg)
					name := parts[0]
					h.sig = name + "(" + strings.Join(h.params, ", ") + ") -> " + retType
					break
				}
			}
		}
	}

	// If the paren-only fallback left us with "... -> ?" and we now have
	// a concrete return type from the source body, fill it in.
	if retType != "" && strings.HasSuffix(h.sig, " -> ?") {
		h.sig = strings.TrimSuffix(h.sig, " -> ?") + " -> " + retType
	}

	// If we found a return type but no signature at all, build a minimal hint.
	if h.sig == "" && retType != "" {
		h.sig = "(?) -> " + retType
	}

	return h
}

func extractLSPBuiltins(root string) []string {
	path := filepath.Join(root, "snowball", "froglsp", "builtins.go")
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var names []string
	seen := make(map[string]bool)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if m := reLSPKey.FindStringSubmatch(line); m != nil {
			name := m[1]
			// Skip struct field names (Signature, Documentation, Params, etc.)
			if name == "Signature" || name == "Documentation" || name == "Params" {
				continue
			}
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	sort.Strings(names)
	return names
}

func extractSyntaxBuiltins(root string) map[string]bool {
	path := filepath.Join(root, "editors", "vscode_froglsp", "klex-language", "syntaxes", "klex.tmLanguage.json")
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: cannot read syntax file %s: %v\n", path, err)
		return nil
	}

	// The file has multiple "match" lines (keywords, then builtins).
	// Each uses the pattern: \\b(name1|name2|...)\\b
	// We read raw bytes, so \\b is two chars: backslash-backslash-b.
	// Collect names from ALL match lines — keywords won't collide with builtins.
	result := make(map[string]bool)
	marker := `\\b(`
	endMarker := `)\\b`
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.Contains(line, `"match"`) || !strings.Contains(line, marker) {
			continue
		}
		start := strings.Index(line, marker)
		end := strings.LastIndex(line, endMarker)
		if start < 0 || end <= start {
			continue
		}
		content := line[start+len(marker) : end]
		for _, name := range strings.Split(content, "|") {
			name = strings.TrimSpace(name)
			if name != "" {
				result[name] = true
			}
		}
	}
	return result
}

// checkDocs reports which builtins are missing from the language + grammar
// reference files.
//
// "Missing" means the name does not appear inside a fenced code block (lines
// between ``` fences) AND does not appear inside inline backticks. Earlier
// versions of this check used a naive `strings.Contains` over the whole
// file, which produced false-negatives for any builtin whose name happened
// to be a substring of unrelated prose ("base64" in commentary, "len" in
// "alignment", etc.). The fenced-block + inline-backtick check matches the
// markdown convention used throughout the docs — builtin names are always
// rendered as code, never bare.
func checkDocs(root string, names []string) (langMissing, grammarMissing []string) {
	langCode := extractCodeText(readFile(filepath.Join(root, "docs", "KLEX_LANGUAGE.MD")))
	gramCode := extractCodeText(readFile(filepath.Join(root, "docs", "KLEX_GRAMMAR.MD")))

	for _, name := range names {
		if !codeContains(langCode, name) {
			langMissing = append(langMissing, name)
		}
		if !codeContains(gramCode, name) {
			grammarMissing = append(grammarMissing, name)
		}
	}
	return
}

// extractCodeText returns just the markdown content that lives inside code
// regions: fenced blocks (between ``` fences) AND inline-backtick spans.
// Everything else (prose, headings, tables outside code) is discarded so
// the contains-check operates only on identifier-bearing text.
func extractCodeText(md string) string {
	var sb strings.Builder
	inFence := false
	for _, line := range strings.Split(md, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			sb.WriteString(line)
			sb.WriteByte('\n')
			continue
		}
		// Inline-backtick spans: capture text between matched `...` pairs.
		// We do a simple state-machine scan; nested/escaped backticks are
		// rare in our docs and a false miss is acceptable.
		inTick := false
		var span strings.Builder
		for _, r := range line {
			if r == '`' {
				if inTick {
					sb.WriteString(span.String())
					sb.WriteByte('\n')
					span.Reset()
				}
				inTick = !inTick
				continue
			}
			if inTick {
				span.WriteRune(r)
			}
		}
	}
	return sb.String()
}

// codeContains checks whether `name` appears in `code` as a whole identifier
// (bounded by non-identifier chars or start/end-of-text). Prevents false
// positives like "len" matching inside "alignment".
func codeContains(code, name string) bool {
	for {
		i := strings.Index(code, name)
		if i < 0 {
			return false
		}
		before := byte(' ')
		if i > 0 {
			before = code[i-1]
		}
		after := byte(' ')
		if i+len(name) < len(code) {
			after = code[i+len(name)]
		}
		if !isIdentByte(before) && !isIdentByte(after) {
			return true
		}
		code = code[i+len(name):]
	}
}

func isIdentByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

// ── Fix actions ───────────────────────────────────────────────────────────────

func doFixVSCode(root string, allNames []string) {
	path := filepath.Join(root, "editors", "vscode_froglsp", "klex-language", "syntaxes", "klex.tmLanguage.json")
	data, err := os.ReadFile(path)
	if err != nil {
		fatalf("cannot read %s: %v", path, err)
	}

	sorted := make([]string, len(allNames))
	copy(sorted, allNames)
	sort.Strings(sorted)

	newRegex := `\\b(` + strings.Join(sorted, "|") + `)\\b(?=\\s*\\()`

	lines := strings.Split(string(data), "\n")
	replaced := false
	for i, line := range lines {
		// Target only the builtins match line (the longest one — has underscore-prefixed names)
		if strings.Contains(line, `"match"`) && strings.Contains(line, `\\b(`) && strings.Contains(line, `_aesDecrypt`) {
			indent := indentOf(line)
			lines[i] = indent + `"match": "` + newRegex + `"`
			replaced = true
			break
		}
	}
	if !replaced {
		fatalf("could not find match line in %s", path)
	}

	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644); err != nil {
		fatalf("cannot write %s: %v", path, err)
	}

	fmt.Printf("✓ VS Code syntax updated — %d builtins in regex\n", len(sorted))
	fmt.Printf("  %s\n", path)
}

func doGenStubs(missing []string, hints map[string]hint) {
	if len(missing) == 0 {
		fmt.Println("// All builtins are documented in the LSP — nothing to generate.")
		return
	}

	fmt.Printf("// ── %d missing LSP entries ──────────────────────────────────────────────────\n", len(missing))
	fmt.Printf("// Paste into snowball/froglsp/builtins.go inside the builtinSignatures map.\n")
	fmt.Printf("// Review each entry — signatures marked (?) need manual completion.\n\n")

	for _, name := range missing {
		h := hints[name]

		sig := name + "(?) -> ?"
		if h.sig != "" {
			// If the hint already starts with the name, use it directly
			if strings.HasPrefix(h.sig, name) {
				sig = h.sig
			} else if strings.HasPrefix(h.sig, "(?)") {
				sig = name + h.sig
			}
		}
		// Normalise arrow
		sig = strings.ReplaceAll(sig, "→", "->")

		paramsStr := "[]string{}"
		if len(h.params) > 0 {
			cleaned := make([]string, 0, len(h.params))
			for _, p := range h.params {
				p = strings.TrimSpace(p)
				// strip type annotations (e.g. "x: int" → "x")
				if idx := strings.Index(p, ":"); idx > 0 {
					p = strings.TrimSpace(p[:idx])
				}
				// strip optional marker
				p = strings.TrimPrefix(p, "?")
				if p != "" {
					cleaned = append(cleaned, `"`+p+`"`)
				}
			}
			if len(cleaned) > 0 {
				paramsStr = "[]string{" + strings.Join(cleaned, ", ") + "}"
			}
		}

		// Prefer the real comment-derived doc body. We use the section
		// AFTER the signature line if we can identify it (so we don't
		// emit "name(args) — text" twice), otherwise we use the whole
		// block. Falls back to a TODO placeholder only when no doc was
		// scraped.
		docStr := stubDocFor(name, h)

		fmt.Printf("\t%q: {\n", name)
		fmt.Printf("\t\tSignature:     %q,\n", sig)
		fmt.Printf("\t\tDocumentation: %q,\n", docStr)
		fmt.Printf("\t\tParams:        %s,\n", paramsStr)
		fmt.Printf("\t},\n\n")
	}
}

// stubDocFor returns the documentation string to use for an LSP stub entry.
//
// Strategy:
//   - If h.doc is empty (no Go-comment block above the registration), fall
//     back to "TODO: document NAME." so the gap is still visible.
//   - Otherwise, take the doc body and drop the leading signature line if
//     it just repeats `name(args)` — the LSP renders Signature separately,
//     so duplicating it in Documentation reads as noise.
//   - Em-dash glue (` — `) on the signature line is preserved as the first
//     sentence when the line starts with `name(args) — description`.
func stubDocFor(name string, h hint) string {
	if strings.TrimSpace(h.doc) == "" {
		return "TODO: document " + name + "."
	}
	lines := strings.Split(h.doc, "\n")
	if len(lines) == 0 {
		return h.doc
	}
	first := lines[0]
	// If the first line opens with `name(args)`, strip the signature prefix
	// so the doc reads as prose. Handles all three house-style shapes:
	//
	//   // name(a, b) → ret                       (signature only — drop whole line)
	//   // name(a, b) → ret  — prose              (signature + em-dash + prose — keep prose)
	//   // name(a, b)  — prose                    (no arrow, em-dash separator — keep prose)
	//
	// The return type may contain balanced parens / commas / unions; we
	// can't rely on whitespace as the boundary, so we look for either the
	// em-dash separator OR drop the whole first line if none is present.
	if strings.HasPrefix(first, name+"(") {
		if i := strings.Index(first, ")"); i >= 0 {
			after := strings.TrimSpace(first[i+1:])
			// If what's left starts with the return-type arrow, find the
			// em-dash that introduces prose. No em-dash → whole first
			// line is signature, drop it.
			if strings.HasPrefix(after, "→") || strings.HasPrefix(after, "->") {
				if idx := strings.Index(after, "—"); idx >= 0 {
					after = strings.TrimSpace(after[idx+len("—"):])
				} else {
					after = ""
				}
			} else {
				// No arrow — could be `name(args)  — prose` or just
				// `name(args)` with prose on subsequent lines. Strip an
				// em-dash/hyphen lead and use what remains.
				after = strings.TrimLeft(after, "— -")
				after = strings.TrimSpace(after)
			}
			if after != "" {
				lines[0] = after
			} else if len(lines) > 1 {
				lines = lines[1:]
			} else {
				return "TODO: document " + name + "."
			}
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// ── --gen-lang-blocks ─────────────────────────────────────────────────────────

// categoryOf maps a source filename (e.g. "builtins_image_fx.go") to the
// language-doc section it belongs in. Unknown files fall back to "Misc".
func categoryOf(file string) string {
	switch file {
	case "builtins_bridge.go", "builtins_bridge_pool.go":
		return "Bridge"
	case "builtins_bytes.go":
		return "Bytes"
	case "builtins_concurrent_hash.go":
		return "Concurrent Hash"
	case "builtins_console.go":
		return "Console"
	case "builtins_csv.go":
		return "CSV"
	case "builtins_fmt.go":
		return "Formatting"
	case "builtins_fs.go", "builtins_fs_windows.go":
		return "Filesystem"
	case "builtins_graphics.go":
		return "Graphics"
	case "builtins_http.go", "builtins_http_stream.go":
		return "HTTP"
	case "builtins_image_fx.go":
		return "Image FX"
	case "builtins_mcp.go":
		return "MCP"
	case "builtins_mtl_darwin.go", "builtins_mtl_stub.go":
		return "Metal / GPU"
	case "builtins_os.go":
		return "OS"
	case "builtins_parallel.go":
		return "Parallel / Async"
	case "builtins_pdf.go":
		return "PDF"
	case "builtins_strings.go":
		return "Strings"
	case "builtins_tensor.go":
		return "Tensor / FrogPy"
	case "builtins_ui.go":
		return "UI"
	case "builtins_vector.go":
		return "Vector"
	case "eval.go":
		return "Core"
	}
	return "Misc"
}

// doGenLangBlocks prints paste-ready markdown for missing language-doc
// entries, grouped by domain category. The output mirrors the existing
// column-aligned style used in KLEX_LANGUAGE.MD (signature in col 0, prose
// wrapping starting at col 25).
//
// Karl can paste each block directly under the matching section heading.
func doGenLangBlocks(missing []string, hints map[string]hint) {
	if len(missing) == 0 {
		fmt.Println("// All builtins are documented in KLEX_LANGUAGE.MD — nothing to generate.")
		return
	}

	// Group by category. Preserves alphabetical order of names within each
	// category because `missing` arrives sorted.
	byCategory := make(map[string][]string)
	for _, name := range missing {
		cat := categoryOf(hints[name].file)
		byCategory[cat] = append(byCategory[cat], name)
	}

	cats := make([]string, 0, len(byCategory))
	for c := range byCategory {
		cats = append(cats, c)
	}
	sort.Strings(cats)

	fmt.Printf("// ── %d missing language-doc entries across %d sections ──\n", len(missing), len(cats))
	fmt.Println("// Paste each block under the matching ### heading in docs/KLEX_LANGUAGE.MD.")
	fmt.Println()

	const sigCol = 25
	for _, cat := range cats {
		names := byCategory[cat]
		fmt.Printf("─── %s  (%d) ───\n\n", cat, len(names))
		fmt.Println("```")
		for _, name := range names {
			h := hints[name]
			sig := h.sig
			if sig == "" {
				sig = name + "(?)"
			}
			// Strip the ` -> type` tail for the column-aligned listing —
			// the table convention in KLEX_LANGUAGE.MD elides return types
			// when they're obvious from the description.
			if i := strings.Index(sig, " -> "); i >= 0 {
				sig = sig[:i]
			}
			desc := langDescFor(name, h)
			emitColumnEntry(sig, desc, sigCol)
		}
		fmt.Println("```")
		fmt.Println()
	}
}

// langDescFor returns the description text for a column-aligned language doc
// entry. Same pruning as stubDocFor — drop the leading `name(args)` if the
// first doc line repeats it.
func langDescFor(name string, h hint) string {
	if strings.TrimSpace(h.doc) == "" {
		return "TODO — describe " + name
	}
	lines := strings.Split(h.doc, "\n")
	first := lines[0]
	if strings.HasPrefix(first, name+"(") {
		if i := strings.Index(first, ")"); i >= 0 {
			after := strings.TrimSpace(first[i+1:])
			after = strings.TrimLeft(after, "— -")
			after = strings.TrimSpace(after)
			lines[0] = after
		}
	}
	// Join into a single paragraph for re-wrapping by emitColumnEntry.
	return strings.TrimSpace(strings.Join(lines, " "))
}

// emitColumnEntry prints `sig` left-aligned in a `sigCol`-wide column, then
// `desc` wrapped to ~72 chars per line, with continuation lines indented to
// the same column. Matches the visual style used throughout KLEX_LANGUAGE.MD.
func emitColumnEntry(sig, desc string, sigCol int) {
	const wrap = 72
	pad := strings.Repeat(" ", sigCol)
	// First-line indent: pad sig to sigCol unless it's wider than the column,
	// in which case the description starts on the next line.
	firstIndent := sig
	if len(sig) >= sigCol {
		fmt.Println(sig)
		firstIndent = pad
	} else {
		firstIndent = sig + strings.Repeat(" ", sigCol-len(sig))
	}
	width := wrap - sigCol
	if width < 30 {
		width = 30
	}
	words := strings.Fields(desc)
	var line string
	first := true
	for _, w := range words {
		if line == "" {
			line = w
			continue
		}
		if len(line)+1+len(w) > width {
			if first {
				fmt.Println(firstIndent + line)
				first = false
			} else {
				fmt.Println(pad + line)
			}
			line = w
			continue
		}
		line += " " + w
	}
	if line != "" {
		if first {
			fmt.Println(firstIndent + line)
		} else {
			fmt.Println(pad + line)
		}
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

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

func toSet(names []string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

// diff returns elements of 'from' that are not in 'exclude'.
func diff(from []string, exclude map[string]bool) []string {
	var out []string
	for _, n := range from {
		if !exclude[n] {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

func printSection(title string, items []string) {
	if len(items) == 0 {
		fmt.Printf("  ✓ %-55s 0\n", title)
		return
	}
	fmt.Printf("\n  ✗ %s (%d)\n", title, len(items))
	for _, item := range items {
		fmt.Printf("      - %s\n", item)
	}
}

// printArityMismatches renders the arity audit result. Format matches
// printSection so the report reads consistently.
func printArityMismatches(items []arityMismatch) {
	title := "ARITY MISMATCH  — LSP signature contradicts Go source"
	if len(items) == 0 {
		fmt.Printf("  ✓ %-55s 0\n", title)
		return
	}
	fmt.Printf("\n  ✗ %s (%d)\n", title, len(items))
	for _, m := range items {
		fmt.Printf("      - %-28s source=%s  lsp=%s\n", m.name, m.source.String(), m.lsp.String())
	}
}

// atoi is a tiny strconv.Atoi wrapper that returns 0 on parse failure.
// Used by extractArity where the regex captures already guarantee digits.
func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// matchingCloseParen returns the index of the ')' that balances the '(' at
// position openIdx. Returns -1 if no balanced close paren is found.
// Required because LSP signatures embed parens in return-type annotations
// (e.g. `parseInt(str) -> (int, error)`) and naive lastIndex(")") goes wrong.
func matchingCloseParen(s string, openIdx int) int {
	depth := 0
	for i := openIdx; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// splitTopLevelPipes splits a signature string on shape separators that
// appear at paren-depth 0 only. We recognise two separator conventions
// used in the LSP table:
//
//   1. ` | ` (literal pipe with surrounding spaces)
//   2. `\n`  (newline-escape — the LSP table favours this for multi-line
//             hovers; the Go source file stores it as the TWO-byte sequence
//             backslash+n, which is what we see when reading the file as
//             raw text. We deliberately do NOT match a real 0x0a newline
//             byte because Signature literals never span source lines —
//             our line-by-line regex couldn't capture them if they did.)
//
// Pipes inside parens are type unions (e.g. `(conn: DB_CONN | DB_TX, …)`)
// and must stay intact, hence the depth-0 gate.
//
// Why scan for the literal ` | ` (with surrounding spaces) rather than
// bare `|`: nothing in our signatures legitimately uses `|` as a non-OR
// punctuator, but the surrounding spaces match how the multi-shape
// convention is written (`f(a) | f(a, b)`) and skip false positives in
// edge cases where someone writes `int|null` without spaces.
func splitTopLevelPipes(s string) []string {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
		case '\\':
			// `\n` escape (two bytes: backslash + n) at depth 0 marks a
			// shape separator in the LSP table convention.
			if depth == 0 && i+1 < len(s) && s[i+1] == 'n' {
				parts = append(parts, s[start:i])
				start = i + 2
				i++ // skip the 'n' on the next iteration
			}
		case '|':
			// Match the literal ` | ` only at depth 0.
			if depth == 0 && i > 0 && s[i-1] == ' ' && i+1 < len(s) && s[i+1] == ' ' {
				parts = append(parts, s[start:i-1])
				start = i + 2
			}
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func splitParams(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func indentOf(line string) string {
	for i, ch := range line {
		if ch != ' ' && ch != '\t' {
			return line[:i]
		}
	}
	return ""
}

func readFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
