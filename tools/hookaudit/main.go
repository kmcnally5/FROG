// hookaudit — kLex agentic-hook completeness audit tool.
//
// Walks eval/builtins_*.go and inventories every Builtins["…"] registration,
// classifying it by whether the body reads user input and whether it fires
// the appropriate agentic hook (FireUiEventHook, FireBridgeCallHook,
// FireAsyncSpawnHook, FireAsyncDoneHook, FireErrorBubbleHook).
//
// The motivating bug: Phase 3 UI hooks were wired into stdlib/ui.lex widgets,
// but every real application uses the Go-side widget builtins in builtins_ui.go,
// which were missed entirely. This tool catches that class of gap automatically.
//
// Usage (from kLex repo root):
//
//	go run ./tools/hookaudit                  — human report to stdout
//	go run ./tools/hookaudit --json           — JSON inventory
//	go run ./tools/hookaudit --missing        — only widgets missing hooks
//
// Heuristics are conservative — false positives are fine, false negatives
// are not. Treat each "MISSING" as "look at this," not "this is broken."
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// BuiltinSite is one Builtins["xxx"] registration we audited.
type BuiltinSite struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Name   string `json:"name"`
	Status string `json:"status"` // one of: ok, missing, non_interactive, infra
	Reads  string `json:"reads,omitempty"`  // comma-joined input signals
	Fires  string `json:"fires,omitempty"`  // comma-joined fire-hook calls
	Notes  string `json:"notes,omitempty"`
}

// Input signals — references to these on `gfx.X` mean the builtin reads
// some form of user input and is a candidate for UI-event instrumentation.
var inputSignals = map[string]bool{
	"mouseJustClicked":   true,
	"mouseDown":          true,
	"mouseRightClicked":  true,
	"mouseRightDown":     true,
	"mouseScrollY":       true,
	"mouseScrollX":       true,
	"uiScrollDelta":      true,
	"uiScrollX":          true,
	"charBuf":            true,
	"uiBackspaceCount":   true,
	"uiDeleteCount":      true,
	"uiLeftCount":        true,
	"uiRightCount":       true,
	"uiUpCount":          true,
	"uiDownCount":        true,
	"keys":               true,
}

// Hook-fire functions — calls to these mean the widget participates in the
// agentic hook protocol.
var fireFuncs = map[string]bool{
	"FireUiEventHook":     true,
	"FireBridgeCallHook":  true,
	"FireAsyncSpawnHook":  true,
	"FireAsyncDoneHook":   true,
	"FireErrorBubbleHook": true,
}

// uiRegisterElement is the "this is a widget, not just an input reader"
// tell. Pure input primitives (mouseX, mouseY, mouseClicked, etc.) read
// gfx state but don't register elements; they're correctly non-widgets.
const widgetRegisterCall = "uiRegisterElement"

// Files / dirs we scan.
var scanGlobs = []string{"eval/builtins_*.go"}

func main() {
	var (
		asJSON     = flag.Bool("json", false, "emit JSON instead of the human report")
		onlyMiss   = flag.Bool("missing", false, "limit output to MISSING sites")
		outPath    = flag.String("out", "", "write JSON to this file (implies --json)")
	)
	flag.Parse()

	sites, err := walkAll()
	if err != nil {
		fmt.Fprintf(os.Stderr, "hookaudit: %v\n", err)
		os.Exit(1)
	}

	sort.Slice(sites, func(i, j int) bool {
		if sites[i].File != sites[j].File {
			return sites[i].File < sites[j].File
		}
		return sites[i].Line < sites[j].Line
	})

	if *onlyMiss {
		kept := sites[:0]
		for _, s := range sites {
			if s.Status == "missing" {
				kept = append(kept, s)
			}
		}
		sites = kept
	}

	if *outPath != "" {
		f, err := os.Create(*outPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "hookaudit: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		enc := json.NewEncoder(f)
		enc.SetIndent("", "  ")
		_ = enc.Encode(sites)
		fmt.Fprintf(os.Stderr, "wrote %d sites → %s\n", len(sites), *outPath)
		return
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(sites)
		return
	}
	printReport(sites)

	// Non-zero exit if any MISSING were found — useful as a CI gate.
	for _, s := range sites {
		if s.Status == "missing" {
			os.Exit(2)
		}
	}
}

// ── AST walking ──────────────────────────────────────────────────────────

func walkAll() ([]BuiltinSite, error) {
	var sites []BuiltinSite
	fset := token.NewFileSet()

	for _, glob := range scanGlobs {
		matches, err := filepath.Glob(glob)
		if err != nil {
			return nil, err
		}
		for _, fpath := range matches {
			if strings.HasSuffix(fpath, "_test.go") {
				continue
			}
			node, err := parser.ParseFile(fset, fpath, nil, 0)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warn: parse %s: %v\n", fpath, err)
				continue
			}
			walkFile(node, fset, fpath, &sites)
		}
	}
	return sites, nil
}

func walkFile(file *ast.File, fset *token.FileSet, fpath string, out *[]BuiltinSite) {
	ast.Inspect(file, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		name, body, ok := extractBuiltinReg(assign)
		if !ok {
			return true
		}

		reads, fires := analyzeBody(body)
		pos := fset.Position(assign.Pos())

		site := BuiltinSite{
			File: fpath,
			Line: pos.Line,
			Name: name,
		}

		// Classify.
		isWidget := containsCall(body, widgetRegisterCall)
		hasInputRead := len(reads) > 0
		hasFire := len(fires) > 0

		switch {
		case hasInputRead && isWidget && !hasFire:
			site.Status = "missing"
			site.Reads = strings.Join(reads, ",")
			site.Notes = "widget reads user input but does not call any Fire*Hook"
		case hasInputRead && isWidget && hasFire:
			site.Status = "ok"
			site.Reads = strings.Join(reads, ",")
			site.Fires = strings.Join(fires, ",")
		case isWidget && !hasInputRead:
			site.Status = "non_interactive"
			site.Notes = "widget but does not read user input (display-only)"
		case !isWidget && hasInputRead:
			site.Status = "infra"
			site.Reads = strings.Join(reads, ",")
			site.Notes = "input primitive (returns input state, not a widget)"
		default:
			site.Status = "infra"
		}

		*out = append(*out, site)
		return true
	})
}

// extractBuiltinReg matches `Builtins["NAME"] = &Builtin{Fn: func(...) Object {...}}`
// and returns the name and the body block of the Fn func literal.
func extractBuiltinReg(stmt *ast.AssignStmt) (string, *ast.BlockStmt, bool) {
	if len(stmt.Lhs) != 1 || len(stmt.Rhs) != 1 {
		return "", nil, false
	}
	idx, ok := stmt.Lhs[0].(*ast.IndexExpr)
	if !ok {
		return "", nil, false
	}
	ident, ok := idx.X.(*ast.Ident)
	if !ok || ident.Name != "Builtins" {
		return "", nil, false
	}
	lit, ok := idx.Index.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", nil, false
	}
	name, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", nil, false
	}

	// RHS: &Builtin{Fn: <funclit>, ...}
	unary, ok := stmt.Rhs[0].(*ast.UnaryExpr)
	if !ok || unary.Op != token.AND {
		return "", nil, false
	}
	comp, ok := unary.X.(*ast.CompositeLit)
	if !ok {
		return "", nil, false
	}
	for _, elt := range comp.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok || key.Name != "Fn" {
			continue
		}
		fnLit, ok := kv.Value.(*ast.FuncLit)
		if !ok {
			continue
		}
		return name, fnLit.Body, true
	}
	return "", nil, false
}

// analyzeBody walks a function body and returns the input-state field
// names it reads (e.g. "mouseJustClicked") and the Fire*Hook calls it
// makes (e.g. "FireUiEventHook"). Both lists are deduplicated.
func analyzeBody(body *ast.BlockStmt) (reads []string, fires []string) {
	readSet := map[string]bool{}
	fireSet := map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		switch e := n.(type) {
		case *ast.SelectorExpr:
			// gfx.X — record X if it's a known input signal.
			if id, ok := e.X.(*ast.Ident); ok && id.Name == "gfx" {
				if inputSignals[e.Sel.Name] {
					readSet[e.Sel.Name] = true
				}
			}
		case *ast.CallExpr:
			// Direct call to FireUiEventHook(...) / etc.
			if id, ok := e.Fun.(*ast.Ident); ok {
				if fireFuncs[id.Name] {
					fireSet[id.Name] = true
				}
			}
			// Or qualified eval.FireUiEventHook(...) — match by selector name.
			if sel, ok := e.Fun.(*ast.SelectorExpr); ok {
				if fireFuncs[sel.Sel.Name] {
					fireSet[sel.Sel.Name] = true
				}
			}
		}
		return true
	})
	for k := range readSet {
		reads = append(reads, k)
	}
	for k := range fireSet {
		fires = append(fires, k)
	}
	sort.Strings(reads)
	sort.Strings(fires)
	return
}

// containsCall returns true if body contains a call to a bare identifier
// `name` (used to detect `uiRegisterElement(...)` usage).
func containsCall(body *ast.BlockStmt, name string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if id, ok := call.Fun.(*ast.Ident); ok && id.Name == name {
			found = true
		}
		return true
	})
	return found
}

// ── Reporting ────────────────────────────────────────────────────────────

func printReport(sites []BuiltinSite) {
	fmt.Println("╔══════════════════════════════════════════════════════╗")
	fmt.Println("║  kLex Agentic-Hook Completeness Audit                ║")
	fmt.Println("╚══════════════════════════════════════════════════════╝")
	fmt.Println()

	if len(sites) == 0 {
		fmt.Println("  No builtin registrations found.")
		return
	}

	byStatus := map[string]int{}
	byFile := map[string]int{}
	for _, s := range sites {
		byStatus[s.Status]++
		byFile[s.File]++
	}

	fmt.Printf("Total builtins scanned: %d\n", len(sites))
	fmt.Println()
	fmt.Println("By status:")
	for _, k := range []string{"ok", "missing", "non_interactive", "infra"} {
		if byStatus[k] == 0 {
			continue
		}
		marker := "  "
		if k == "missing" {
			marker = "✗ "
		} else if k == "ok" {
			marker = "✓ "
		}
		fmt.Printf("  %s%-18s  %4d\n", marker, k, byStatus[k])
	}
	fmt.Println()

	// Always show missing first if any.
	missing := 0
	for _, s := range sites {
		if s.Status == "missing" {
			missing++
		}
	}
	if missing > 0 {
		fmt.Println("─── MISSING (interactive widgets without hooks) ───")
		fmt.Println()
		for _, s := range sites {
			if s.Status != "missing" {
				continue
			}
			fmt.Printf("  ✗ %s:%d  %s\n", s.File, s.Line, s.Name)
			fmt.Printf("    reads:  %s\n", s.Reads)
			fmt.Printf("    notes:  %s\n", s.Notes)
			fmt.Println()
		}
	} else {
		fmt.Println("✓ All interactive widgets are instrumented.")
		fmt.Println()
	}

	// Full inventory table (compact).
	fmt.Println("─── Full inventory ───")
	fmt.Println()
	fmt.Printf("  %-22s  %-16s  %s\n", "NAME", "STATUS", "FILE:LINE")
	fmt.Printf("  %-22s  %-16s  %s\n", strings.Repeat("─", 22), strings.Repeat("─", 16), strings.Repeat("─", 30))
	for _, s := range sites {
		statusTag := s.Status
		if s.Status == "ok" {
			statusTag = "✓ ok"
		} else if s.Status == "missing" {
			statusTag = "✗ MISSING"
		}
		fmt.Printf("  %-22s  %-16s  %s:%d\n", s.Name, statusTag, s.File, s.Line)
	}
	fmt.Println()
	fmt.Println("Tip: run with --missing to focus on widgets that need attention,")
	fmt.Println("     or --json for a machine-readable inventory.")
}
