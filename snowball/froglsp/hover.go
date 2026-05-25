package main

import (
	"fmt"
	"klex/ast"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
)

// KeywordDocumentation provides hover information for language keywords
var keywordDocumentation = map[string]string{
	"let": `**let** — Declare a variable strictly local to the current scope.

Creates a binding that is scoped to the current block, regardless of whether the same name exists in an outer scope. This prevents accidental capture or modification of outer variables.

Unlike bare assignment (x = val) which walks the scope chain, let always creates in the current scope.`,

	"const": `**const** — Declare an immutable binding.

Creates a binding that can never be reassigned from any scope. Any attempt to reassign a constant is a RuntimeError.

Use const for configuration values, mathematical constants, and anything that must not change after initialisation.`,

	"if": `**if** — Conditional execution.

Evaluates a boolean condition. If true, executes the body. Can be followed by else if and else for alternative branches.

The condition must be boolean — integers are not truthy, and type coercion is not performed.`,

	"else": `**else** — Fallback branch for if statement.

Executes when the preceding if (or else if) condition is false. May be followed by another if for chain conditions.`,

	"while": `**while** — Loop while condition is true.

Repeatedly executes the body while the condition remains true. Exits when the condition becomes false or break is encountered.

The condition must be boolean.`,

	"for": `**for** — Iterate over arrays, hashes, or channels.

Single variable iterates elements: for x in arr { ... }
Two variables iterate index/key and value: for i, v in arr { ... }

Works with arrays, hashes, and channels. Use range() for numeric sequences.`,

	"break": `**break** — Exit the nearest enclosing loop.

Immediately terminates a while, for, or for-in loop. Does not cross a function boundary.`,

	"continue": `**continue** — Skip to next loop iteration.

Jumps to the start of the next iteration in the nearest enclosing while, for, or for-in loop. Does not cross a function boundary.`,

	"return": `**return** — Exit a function.

Returns from the enclosing function, optionally with a value. Without a value, return produces null. A function that reaches the end without a return also produces null.`,

	"fn": `**fn** — Declare a function.

Functions are first-class values that can be stored in variables, passed as arguments, and returned from other functions.

Named: fn add(a, b) { a + b }
Anonymous: fn(x) { x * 2 }

Functions support closures and recursion.`,

	"import": `**import** — Load a module from a .lex file.

Syntax: import "path/to/module.lex" as name

The interpreter searches for modules in the directory specified by KLEX_PATH. Everything defined at the top level of the imported file is accessible through the module name.`,

	"struct": `**struct** — Declare a struct type.

Structs are nominal typed values with a fixed set of named fields and optional methods.

Syntax: struct Point { x, y }

Each declared field must be provided at construction time. Instances are passed by reference.`,

	"enum": `**enum** — Declare an enum type (sum type).

Enums are values that are exactly one of a fixed set of named variants, each of which can carry its own named fields.

Syntax: enum Shape { Circle(radius) Rect(w, h) Point }

Variants with no fields need no parentheses. Use switch with destructuring patterns to match and bind fields.`,

	"switch": `**switch** — Conditional dispatch with pattern matching.

Value form: compares one expression against multiple values with ==.
Expression form: each case is a boolean expression.
Enum pattern matching: matches enum variants and binds their fields.

Short form (recommended):
  switch s {
    case Circle(r)    { println(r) }
    case Rect(w, h)   { println(w * h) }
    case Point()      { println("point") }
  }

Full form (qualified type name):
  switch s {
    case Shape.Circle(r)  { println(r) }
    case Shape.Point      { println("point") }
  }

Bindings are scoped to the case body only. Cases run in order; first match wins. No fallthrough.`,

	"case": `**case** — A branch in a switch statement.

Matches a value, expression, or enum pattern. First matching case runs; no fallthrough to subsequent cases.`,

	"default": `**default** — Fallback case in switch or select.

Runs if no other case matches in a switch, or if no channel operation is ready in a select.`,

	"select": `**select** — Wait on multiple channel operations.

Simultaneously waits for channel operations (send/recv) across multiple cases. Picks one ready case at random if several are ready. Optional default makes it non-blocking.

Syntax: select { case val, ok = recv(ch) { } ... default { } }`,

	"?": `**?** — Postfix error-propagation operator.

Unwraps a (value, err) tuple. If err is not null, returns the error immediately from the enclosing function. If err is null, evaluates to value.

Operand must be a 2-element tuple — applying ? to any other type is a TypeError.

Example:
  content = readFile(path)?       // propagates error or unwraps content
  data    = parseJSON(content)?   // propagates error or unwraps data
  return transform(data)?

Equivalent to:
  content, err = readFile(path)
  if err != null { return err }`,
}


// (Imported-file content is now served by parsed_cache.go — mtime-keyed LRU
// over (content, AST, symbols). See getParsedFile.)

// HoverAtPosition returns hover information for the identifier at the given position
func HoverAtPosition(doc *DocumentState, pos Position) (result *Hover) {
	defer func() {
		if r := recover(); r != nil {
			LogMessage("HOVER PANIC: %v, at line %d char %d", r, pos.Line, pos.Character)
			LogMessage("HOVER PANIC STACK:\n%s", debug.Stack())
			result = nil
		}
	}()

	if doc.AST == nil || doc.Symbols == nil {
		return nil
	}

	// Find the node at this position
	// LSP uses 0-based positions; convert to 1-based for kLex
	nodeLine := pos.Line + 1
	node := FindNodeAtPosition(doc.AST, pos.Line, pos.Character)
	if node == nil {
		LogMessage("HOVER: no node found at line=%d(display=%d) char=%d", nodeLine, pos.Line, pos.Character)
		return nil
	}

	// DEBUG: Log what node was found
	LogMessage("HOVER: Found node type=%T", node)
	if ident, ok := node.(*ast.Ident); ok {
		LogMessage("HOVER: Found Ident='%s' at (%d,%d)", ident.Value, ident.Pos.Line, ident.Pos.Col)
	} else if assign, ok := node.(*ast.AssignStmt); ok {
		LogMessage("HOVER: Found AssignStmt name='%s' at (%d,%d)", assign.Name, assign.Pos.Line, assign.Pos.Col)
	} else if letStmt, ok := node.(*ast.LetStmt); ok {
		LogMessage("HOVER: Found LetStmt name='%s' at (%d,%d)", letStmt.Name, letStmt.Pos.Line, letStmt.Pos.Col)
	}

	// Handle regular identifier first (variables, function names, etc.)
	// This takes priority over statement keywords
	if ident, ok := node.(*ast.Ident); ok {
		LogMessage("HOVER: Returning identifier hover for '%s'", ident.Value)
		return hoverForIdentifier(doc, ident.Value)
	}

	// Check MultiAssignStmt before other statement types
	if multiAssign, ok := node.(*ast.MultiAssignStmt); ok {
		LogMessage("HOVER: Found MultiAssignStmt at line %d, names: %v", multiAssign.Pos.Line, multiAssign.Names)
		// Determine which variable the cursor is hovering over
		// The names are on the left side of the assignment, comma-separated
		// We need to estimate which name based on cursor position

		// For now, we'll check the document text around the position to find the actual identifier
		lines := strings.Split(doc.Text, "\n")
		if pos.Line < len(lines) {
			line := lines[pos.Line]

			// Find the identifier at the cursor position
			// Walk backwards to find the start of the identifier
			start := pos.Character
			for start > 0 && isIdentifierChar(rune(line[start-1])) {
				start--
			}

			// Walk forwards to find the end of the identifier
			end := pos.Character
			for end < len(line) && isIdentifierChar(rune(line[end])) {
				end++
			}

			if start < len(line) && end <= len(line) && start < end {
				ident := line[start:end]
				LogMessage("HOVER: Found identifier '%s' at position %d-%d", ident, start, end)
				// Check if this identifier is in the Names list
				for _, name := range multiAssign.Names {
					if name == ident {
						LogMessage("HOVER: Found '%s' in MultiAssignStmt names, showing hover", ident)
						return hoverForIdentifier(doc, name)
					}
				}
				LogMessage("HOVER: Identifier '%s' NOT in MultiAssignStmt names %v", ident, multiAssign.Names)
			}
		}
	}

	// MultiLetStmt: same cursor-on-name lookup as MultiAssignStmt
	if multiLet, ok := node.(*ast.MultiLetStmt); ok {
		LogMessage("HOVER: Found MultiLetStmt at line %d, names: %v", multiLet.Pos.Line, multiLet.Names)
		lines := strings.Split(doc.Text, "\n")
		if pos.Line < len(lines) {
			line := lines[pos.Line]
			start := pos.Character
			for start > 0 && isIdentifierChar(rune(line[start-1])) {
				start--
			}
			end := pos.Character
			for end < len(line) && isIdentifierChar(rune(line[end])) {
				end++
			}
			if start < len(line) && end <= len(line) && start < end {
				ident := line[start:end]
				for _, name := range multiLet.Names {
					if name == ident {
						return hoverForIdentifier(doc, name)
					}
				}
			}
		}
	}

	// For LetStmt/ConstStmt/AssignStmt, check if cursor is on the variable name
	// (not on the keyword). If so, show the variable's type.
	if letStmt, ok := node.(*ast.LetStmt); ok {
		// letStmt.Name is the variable name (e.g., "shadow_hit")
		// If cursor is reasonably close to the statement start and after the keyword "let",
		// assume cursor is on the variable name
		if pos.Character > letStmt.Pos.Col+3 { // "let" is 3 chars
			return hoverForIdentifier(doc, letStmt.Name)
		}
	}

	if constStmt, ok := node.(*ast.ConstStmt); ok {
		if pos.Character > constStmt.Pos.Col+5 { // "const" is 5 chars
			return hoverForIdentifier(doc, constStmt.Name)
		}
	}

	if assignStmt, ok := node.(*ast.AssignStmt); ok {
		// Check if this is a named function definition (AssignStmt with FunctionLiteral)
		if _, isFn := assignStmt.Value.(*ast.FunctionLiteral); isFn {
			// For named functions, if cursor is after "fn " (or just checking any reasonable position),
			// show the function's symbol info instead of generic "fn" keyword
			return hoverForIdentifier(doc, assignStmt.Name)
		}
		if pos.Character > assignStmt.Pos.Col+2 { // conservative estimate for potential type annotation
			return hoverForIdentifier(doc, assignStmt.Name)
		}
	}

	// Check if we're hovering on a FunctionLiteral (named function definition)
	// Find which function this FunctionLiteral belongs to by matching position
	if fnLit, ok := node.(*ast.FunctionLiteral); ok {
		for name, sym := range doc.Symbols.Symbols {
			if sym.Kind == KindFunction && sym.DefPos.Line == fnLit.Pos.Line {
				return hoverForIdentifier(doc, name)
			}
		}
	}

	// Check if we're in a CallExpr - show function signature
	if call, ok := node.(*ast.CallExpr); ok {
		if ident, ok := call.Function.(*ast.Ident); ok {
			return hoverForIdentifier(doc, ident.Value)
		}
		if dotExpr, ok := call.Function.(*ast.DotExpr); ok {
			return hoverForDotExpr(doc, dotExpr, pos)
		}
	}

	// Check if we're in a DotExpr (e.g., lib.invokeFunc)
	if dotExpr, ok := node.(*ast.DotExpr); ok {
		return hoverForDotExpr(doc, dotExpr, pos)
	}

	// Finally, check for keyword documentation (only if no identifier was found)
	if keyword := getKeywordFromNode(node); keyword != "" {
		if doc, ok := keywordDocumentation[keyword]; ok {
			return &Hover{
				Contents: MarkupContent{
					Kind:  "markdown",
					Value: doc,
				},
			}
		}
	}

	return nil
}

// getKeywordFromNode extracts the keyword associated with a statement node
func getKeywordFromNode(node ast.Node) string {
	switch n := node.(type) {
	case *ast.LetStmt:
		return "let"
	case *ast.ConstStmt:
		return "const"
	case *ast.IfStmt:
		return "if"
	case *ast.WhileStmt:
		return "while"
	case *ast.ForInStmt:
		return "for"
	case *ast.BreakStmt:
		return "break"
	case *ast.ContinueStmt:
		return "continue"
	case *ast.ReturnStmt:
		return "return"
	case *ast.FunctionLiteral:
		return "fn"
	case *ast.AssignStmt:
		// Named functions desugar to AssignStmt with FunctionLiteral value
		if _, ok := n.Value.(*ast.FunctionLiteral); ok {
			return "fn"
		}
	case *ast.ImportStmt:
		return "import"
	case *ast.StructDecl:
		return "struct"
	case *ast.EnumDecl:
		return "enum"
	case *ast.SwitchStmt:
		return "switch"
	case *ast.SelectStmt:
		return "select"
	}
	return ""
}

func isIdentifierChar(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_'
}

func hoverForIdentifier(doc *DocumentState, name string) *Hover {
	// Check if it's a builtin
	if info, ok := builtinSignatures[name]; ok {
		return &Hover{
			Contents: MarkupContent{
				Kind:  "markdown",
				Value: renderBuiltinHover(name, info),
			},
		}
	}

	// Check if it's a user-defined symbol
	if sym, ok := doc.Symbols.Symbols[name]; ok {
		// Skip parameters for now (causing panic in renderSymbolHover)
		if sym.Kind == KindParameter {
			return nil
		}

		// Extract comments from the current document for functions and variables (not parameters)
		var comments string
		if (sym.Kind == KindFunction || sym.Kind == KindVariable || sym.Kind == KindConst) && sym.DefPos.Line > 0 {
			comments = extractCommentsAboveSymbol(doc.Text, sym.DefPos.Line)
		}

		var content string
		if comments != "" {
			content = comments + "\n\n" + renderSymbolHover(sym)
		} else {
			content = renderSymbolHover(sym)
		}
		return &Hover{
			Contents: MarkupContent{
				Kind:  "markdown",
				Value: content,
			},
		}
	}

	return nil
}

// hoverForDotExpr handles hover for expressions like lib.invokeFunc or instance.field
func hoverForDotExpr(doc *DocumentState, dotExpr *ast.DotExpr, pos Position) *Hover {
	// Extract name from left side
	var leftName string
	if ident, ok := dotExpr.Left.(*ast.Ident); ok {
		leftName = ident.Value
	} else {
		return nil
	}

	// Get the property/method name
	propertyName := dotExpr.Property

	// First, check if this is a struct field access (instance.field)
	if sym, ok := doc.Symbols.Symbols[leftName]; ok {
		if sym.Type == "struct" && sym.StructType != "" {
			// This is a struct instance, look up the struct definition
			if structDef, ok := doc.Symbols.Structs[sym.StructType]; ok {
				// Check if the property is a valid field
				for _, field := range structDef.Fields {
					if field == propertyName {
						// Show struct field information with better formatting
						content := fmt.Sprintf("```klex\n%s.%s\n```\n\n**Struct field** of `%s`\n\nDefined at line %d",
							leftName, propertyName, sym.StructType, structDef.DefPos.Line)
						return &Hover{
							Contents: MarkupContent{
								Kind:  "markdown",
								Value: content,
							},
						}
					}
				}
			}
		}
	}

	// Otherwise, treat as module access (lib.function)
	// Find the import statement for this module
	var importPath string
	for _, stmt := range doc.AST.Statements {
		if importStmt, ok := stmt.(*ast.ImportStmt); ok {
			if importStmt.Alias == leftName {
				importPath = importStmt.Path
				break
			}
		}
	}

	if importPath == "" {
		LogMessage("HOVER DotExpr: no import found for alias '%s'", leftName)
		return nil
	}

	LogMessage("HOVER DotExpr: alias='%s' property='%s' importPath='%s'", leftName, propertyName, importPath)

	// Resolve the import to a file path
	docURI := URIToPath(doc.URI)
	docDir := filepath.Dir(docURI)

	// Add .lex extension if not already present
	libFileName := importPath
	if !strings.HasSuffix(libFileName, ".lex") {
		libFileName = libFileName + ".lex"
	}

	// Try multiple locations: same dir, stdlib, and project-root-relative paths.
	// Imports like "projects/frogBroker/broker_auth.lex" are relative to the
	// project root, not to the importing file's directory. Walk up ancestor
	// directories until the file is found or we run out of parents.
	var libFile string
	candidates := []string{
		filepath.Join(docDir, libFileName),                                            // same directory
		filepath.Join(filepath.Dir(filepath.Dir(docDir)), "stdlib", libFileName),      // stdlib (2 up)
		filepath.Join(filepath.Dir(docDir), "stdlib", libFileName),                    // stdlib (1 up)
	}

	// Walk up from docDir trying the import path relative to each ancestor
	ancestor := docDir
	for i := 0; i < 8; i++ {
		candidates = append(candidates, filepath.Join(ancestor, libFileName))
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			break
		}
		ancestor = parent
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			libFile = candidate
			break
		}
	}

	if libFile == "" {
		LogMessage("HOVER DotExpr: could not resolve '%s' — tried %d candidates from docDir='%s'", libFileName, len(candidates), docDir)
		return nil
	}
	LogMessage("HOVER DotExpr: resolved to '%s'", libFile)

	// Cache-keyed read + parse. parsed_cache.go returns the most recent
	// (content, AST, symbols) for libFile, re-parsing only when the file's
	// mtime changes. Bound by an LRU cap so this can't grow unbounded.
	libContent, libAST, libSymbols := getParsedFile(libFile)
	if libContent == "" || libAST == nil {
		return nil
	}

	// Look up the symbol in the library
	sym, ok := libSymbols.Symbols[propertyName]
	if !ok {
		LogMessage("HOVER DotExpr: symbol '%s' not found in '%s' (available: %v)", propertyName, libFile, func() []string {
			keys := make([]string, 0, len(libSymbols.Symbols))
			for k := range libSymbols.Symbols {
				keys = append(keys, k)
			}
			return keys
		}())
		return nil
	}
	LogMessage("HOVER DotExpr: found symbol '%s' at line %d", propertyName, sym.DefPos.Line)

	// Extract comments from the library file
	comments := extractCommentsAboveSymbol(libContent, sym.DefPos.Line)
	if comments != "" {
		return &Hover{
			Contents: MarkupContent{
				Kind:  "markdown",
				Value: comments + "\n\n" + renderSymbolHover(sym),
			},
		}
	}

	return &Hover{
		Contents: MarkupContent{
			Kind:  "markdown",
			Value: renderSymbolHover(sym),
		},
	}
}

// extractCommentsAboveSymbol extracts consecutive // comment lines above a definition
// Returns markdown formatted with first line as bold
func extractCommentsAboveSymbol(source string, defLine int) string {
	lines := strings.Split(source, "\n")
	// defLine is 1-based; lines array is 0-based
	// Need at least 2 lines (0-based index 0 and 1) to safely go back
	if defLine < 2 || defLine > len(lines) {
		LogMessage("extractComments: defLine %d out of range, total lines: %d", defLine, len(lines))
		return ""
	}

	// Start from the line before the definition (convert 1-based to 0-based)
	commentLines := []string{}
	for i := defLine - 2; i >= 0; i-- {
		if i < 0 || i >= len(lines) {
			LogMessage("extractComments: index %d out of bounds for %d lines", i, len(lines))
			break
		}
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "//") {
			// Extract comment text (remove // and trim)
			comment := strings.TrimSpace(strings.TrimPrefix(line, "//"))
			commentLines = append([]string{comment}, commentLines...) // prepend to keep order
		} else if line != "" {
			// Stop at first non-comment, non-empty line
			break
		}
	}

	if len(commentLines) == 0 {
		return ""
	}

	// Format with first line bold, rest as normal
	result := "**" + commentLines[0] + "**"
	for _, line := range commentLines[1:] {
		result += "\n\n" + line
	}

	return result
}

// renderBuiltinHover produces the visual structure for builtin hovers.
// Layout:
//   ─── Builtin · <Category> ───────────────────
//
//   ```klex
//   <signature>
//   ```
//
//   <one-liner summary>          (first sentence of doc)
//
//   **Parameters**               (table — only if params exist)
//   | Name | Description |
//
//   <remaining documentation paragraphs, including any inline
//    "Example:" blocks rendered as kLex code fences>
//
//   ───────────────────────────────────────────
//   **See also** — `sibling1` · `sibling2` · `sibling3`
//   **Source** — `eval/builtins_xxx.go`
func renderBuiltinHover(name string, info BuiltinInfo) string {
	cat := categoryFor(name)
	var b strings.Builder

	// Header: small caps-style classification line. Uses a leading
	// horizontal rule so VS Code's hover renders a visible divider.
	b.WriteString(fmt.Sprintf("**kLex builtin · %s**\n\n", cat.Display))

	// Signature in a fenced code block with the kLex language tag so
	// the editor's syntax highlighter colours it.
	b.WriteString("```klex\n")
	b.WriteString(prettySignature(info.Signature))
	b.WriteString("\n```\n\n")

	// Body: first sentence as the summary, rest of the doc text wrapped
	// underneath. The doc text in builtinSignatures often contains
	// inline "Example:" sections — we preserve them verbatim and let
	// markdown render them.
	summary, rest := splitFirstSentence(info.Documentation)
	if summary != "" {
		b.WriteString(summary)
		b.WriteString("\n\n")
	}

	// Parameters table (skip if zero params or if there's only a single
	// rest-param like `...vals` — table for one row is visual clutter).
	if shouldRenderParamTable(info.Params) {
		b.WriteString("**Parameters**\n\n")
		b.WriteString("| Name | |\n|------|---|\n")
		for _, p := range info.Params {
			b.WriteString(fmt.Sprintf("| `%s` | |\n", p))
		}
		b.WriteString("\n")
	}

	// Remaining doc body — paragraphs, examples, error tables that the
	// builtin author already wrote. We pass them through unchanged so
	// they format the same as today.
	if rest != "" {
		b.WriteString(rest)
		b.WriteString("\n\n")
	}

	// Cross-references — "See also" with up to 3 siblings from the
	// same category. Skips when there are no siblings (e.g. for the
	// only HTTP builtin in a thin category).
	siblings := siblingsInCategory(name, 3)
	if len(siblings) > 0 {
		b.WriteString("---\n")
		b.WriteString("**See also** — ")
		for i, s := range siblings {
			if i > 0 {
				b.WriteString(" · ")
			}
			b.WriteString("`")
			b.WriteString(s)
			b.WriteString("`")
		}
		b.WriteString("\n\n")
	}

	// Source pointer for builtins — tells the curious reader which
	// Go file to read if they want to understand the implementation.
	if cat.Source != "" {
		if len(siblings) == 0 {
			b.WriteString("---\n")
		}
		b.WriteString("**Source** — `")
		b.WriteString(cat.Source)
		b.WriteString("`\n")
	}

	return b.String()
}

// prettySignature replaces ASCII `->` with `→` so the hover signature
// reads more like a function-type annotation than a CLI flag. Left as
// a one-line transformation rather than a regex so the cost is nil and
// the input format stays identical to what's already in builtinSignatures.
func prettySignature(sig string) string {
	return strings.ReplaceAll(sig, " -> ", " → ")
}

// splitFirstSentence separates the leading summary sentence from the
// remainder of a doc string. Used to put the one-line summary directly
// under the signature and push the detail paragraphs below the
// parameter table.
//
// Heuristic: find the first ". " (period + space) that isn't followed
// by a lowercase letter (which would indicate an abbreviation like
// "e.g."). If none, treat the whole text as summary.
func splitFirstSentence(doc string) (summary, rest string) {
	doc = strings.TrimSpace(doc)
	if doc == "" {
		return "", ""
	}
	// Look for the first sentence terminator that's followed by whitespace
	// AND a capital letter or newline (i.e. start of a new sentence/block).
	for i := 0; i < len(doc)-1; i++ {
		if (doc[i] == '.' || doc[i] == '!' || doc[i] == '?') && i+1 < len(doc) {
			next := doc[i+1]
			if next == '\n' {
				return strings.TrimSpace(doc[:i+1]), strings.TrimSpace(doc[i+2:])
			}
			if next == ' ' && i+2 < len(doc) {
				ch := doc[i+2]
				if ch >= 'A' && ch <= 'Z' {
					return strings.TrimSpace(doc[:i+1]), strings.TrimSpace(doc[i+2:])
				}
			}
		}
	}
	return doc, ""
}

// shouldRenderParamTable returns true when a parameter table adds value.
// Skips for empty params and single rest-style params like `...vals`
// where the table would be one row of clutter.
func shouldRenderParamTable(params []string) bool {
	if len(params) == 0 {
		return false
	}
	if len(params) == 1 && strings.HasPrefix(params[0], "...") {
		return false
	}
	return true
}

// renderSymbolHover formats a user-defined symbol (function, variable,
// const, module, struct, enum, parameter) with the same multi-section
// visual style as builtins. The header row classifies the symbol's
// kLex kind; the body shows the canonical signature in a fenced code
// block, followed by a fact table.
func renderSymbolHover(sym *Symbol) string {
	var b strings.Builder

	switch sym.Kind {
	case KindFunction:
		b.WriteString("**kLex function**\n\n")
		b.WriteString("```klex\nfn ")
		b.WriteString(renderFunctionSignature(sym))
		b.WriteString("\n```\n\n")
		b.WriteString("| | |\n|---|---|\n")
		b.WriteString(fmt.Sprintf("| **Defined** | Line %d |\n", sym.DefPos.Line))
		if sym.ReturnType != "" {
			b.WriteString(fmt.Sprintf("| **Returns** | `%s` |\n", sym.ReturnType))
		}
		if len(sym.Params) > 0 {
			b.WriteString(fmt.Sprintf("| **Arity** | %d |\n", len(sym.Params)))
		}

	case KindVariable:
		varKind := "kLex variable"
		if sym.FromTuple {
			varKind = "kLex tuple element"
		}
		b.WriteString(fmt.Sprintf("**%s**\n\n", varKind))
		b.WriteString(fmt.Sprintf("```klex\n%s\n```\n\n", sym.Name))
		b.WriteString("| | |\n|---|---|\n")
		if sym.Type != "" && sym.Type != "unknown" {
			b.WriteString(fmt.Sprintf("| **Type** | `%s` |\n", sym.Type))
		}
		b.WriteString(fmt.Sprintf("| **Defined** | Line %d |\n", sym.DefPos.Line))

	case KindConst:
		b.WriteString("**kLex constant**\n\n")
		b.WriteString(fmt.Sprintf("```klex\nconst %s\n```\n\n", sym.Name))
		b.WriteString("| | |\n|---|---|\n")
		if sym.Type != "" && sym.Type != "unknown" {
			b.WriteString(fmt.Sprintf("| **Type** | `%s` |\n", sym.Type))
		}
		b.WriteString(fmt.Sprintf("| **Defined** | Line %d |\n", sym.DefPos.Line))

	case KindModule:
		b.WriteString("**kLex module**\n\n")
		b.WriteString(fmt.Sprintf("```klex\nimport \"%s\"\n```\n\n", sym.Name))
		b.WriteString("| | |\n|---|---|\n")
		b.WriteString(fmt.Sprintf("| **Imported** | Line %d |\n", sym.DefPos.Line))

	case KindBuiltin:
		// Should rarely hit this branch — builtins flow through
		// renderBuiltinHover via hoverForIdentifier's builtin lookup.
		b.WriteString("**kLex builtin**\n\n")
		b.WriteString(fmt.Sprintf("```klex\n%s()\n```\n", sym.Name))

	case KindParameter:
		b.WriteString("**kLex parameter**\n\n")
		b.WriteString(fmt.Sprintf("```klex\n%s\n```\n\n", sym.Name))
		b.WriteString("| | |\n|---|---|\n")
		if sym.Type != "" && sym.Type != "unknown" {
			b.WriteString(fmt.Sprintf("| **Type** | `%s` |\n", sym.Type))
		}
		b.WriteString(fmt.Sprintf("| **Declared** | Line %d |\n", sym.DefPos.Line))
	}

	return b.String()
}

func renderFunctionSignature(sym *Symbol) string {
	var params []string
	for i, p := range sym.Params {
		// Add type annotation if present
		param := p
		if i < len(sym.ParamTypes) && sym.ParamTypes[i] != "" {
			param = sym.ParamTypes[i] + " " + p
		}
		if i < len(sym.Defaults) && sym.Defaults[i] {
			param = param + "?"
		}
		if sym.Variadic && i == len(sym.Params)-1 {
			param = "..." + param
		}
		params = append(params, param)
	}
	sig := fmt.Sprintf("%s(%s)", sym.Name, strings.Join(params, ", "))
	// Add return type if present
	if sym.ReturnType != "" {
		sig = sig + ": " + sym.ReturnType
	}
	return sig
}

// SignatureHelpAtPosition returns signature help for the function at the given position
func SignatureHelpAtPosition(doc *DocumentState, pos Position) *SignatureHelp {
	if doc.AST == nil {
		return nil
	}

	// Find the call expression at this position (or before it)
	call := findEnclosingCall(doc.AST, pos.Line, pos.Character)
	if call == nil {
		return nil
	}

	// Get the function name
	var funcName string
	switch fn := call.Function.(type) {
	case *ast.Ident:
		funcName = fn.Value
	case *ast.DotExpr:
		if ident, ok := fn.Left.(*ast.Ident); ok {
			funcName = ident.Value + "." + fn.Property
		}
	default:
		return nil
	}

	// Check builtins first
	if info, ok := builtinSignatures[funcName]; ok {
		sigs := []SignatureInformation{
			{
				Label:         funcName,
				Documentation: info.Documentation,
				Parameters:    buildParameterInfo(info.Params),
			},
		}
		return &SignatureHelp{
			Signatures:      sigs,
			ActiveSignature: 0,
			ActiveParameter: computeActiveParam(call, pos),
		}
	}

	// Check user-defined functions
	if sym, ok := doc.Symbols.Symbols[funcName]; ok && sym.Kind == KindFunction {
		paramInfo := make([]ParameterInformation, len(sym.Params))
		for i, p := range sym.Params {
			paramInfo[i] = ParameterInformation{Label: p}
		}

		sigs := []SignatureInformation{
			{
				Label:      renderFunctionSignature(sym),
				Parameters: paramInfo,
			},
		}
		return &SignatureHelp{
			Signatures:      sigs,
			ActiveSignature: 0,
			ActiveParameter: computeActiveParam(call, pos),
		}
	}

	return nil
}

func buildParameterInfo(params []string) []ParameterInformation {
	var result []ParameterInformation
	for _, p := range params {
		result = append(result, ParameterInformation{Label: p})
	}
	return result
}

// computeActiveParam returns the 0-based index of the argument the cursor
// is currently on inside `call`. It counts every arg whose start position
// is at or before the cursor — regardless of arg shape (literal, call,
// infix, etc.). The prior version only matched *ast.Ident, so signature
// help misreported the active param whenever earlier args weren't bare
// identifiers (e.g. `foo(1, "two", bar(), |here|)` reported 0 instead of 3).
func computeActiveParam(call *ast.CallExpr, pos Position) int {
	// LSP positions are 0-based; AST positions are 1-based.
	cursorLine := pos.Line + 1
	cursorCol := pos.Character
	count := 0
	for _, arg := range call.Args {
		argPos, ok := getNodePos(arg)
		if !ok {
			continue
		}
		if argPos.Line < cursorLine || (argPos.Line == cursorLine && argPos.Col <= cursorCol) {
			count++
		}
	}
	if count > 0 {
		count-- // switch to 0-based
	}
	return count
}

// findEnclosingCall finds the CallExpr that encloses the given position
func findEnclosingCall(program *ast.Program, line, col int) *ast.CallExpr {
	// This is a simplified heuristic: find the most recent CallExpr on or before this line
	var closest *ast.CallExpr
	var closestLine int

	var walk func(ast.Node)
	walk = func(node ast.Node) {
		if node == nil {
			return
		}

		call, isCall := node.(*ast.CallExpr)
		if isCall && call.Pos.Line <= line+1 {
			if call.Pos.Line > closestLine {
				closest = call
				closestLine = call.Pos.Line
			}
		}

		// Recursively walk children
		walkAST2(node, walk)
	}

	walkAST2(program, walk)
	return closest
}
