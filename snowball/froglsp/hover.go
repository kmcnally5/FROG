package main

import (
	"fmt"
	"klex/ast"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

Variants with no fields need no parentheses. Use switch for pattern matching.`,

	"switch": `**switch** — Conditional dispatch with pattern matching.

Value form: compares one expression against multiple values with ==.
Expression form: each case is a boolean expression.
Enum pattern matching: matches enum variants with field bindings.

Cases run in order; first match wins. No fallthrough. Optional default runs if no case matches.`,

	"case": `**case** — A branch in a switch statement.

Matches a value, expression, or enum pattern. First matching case runs; no fallthrough to subsequent cases.`,

	"default": `**default** — Fallback case in switch or select.

Runs if no other case matches in a switch, or if no channel operation is ready in a select.`,

	"select": `**select** — Wait on multiple channel operations.

Simultaneously waits for channel operations (send/recv) across multiple cases. Picks one ready case at random if several are ready. Optional default makes it non-blocking.

Syntax: select { case val, ok = recv(ch) { } ... default { } }`,
}


// fileCache stores content of imported files to avoid re-reading from disk
var (
	fileCache     = make(map[string]string)
	fileCacheLock sync.RWMutex
)

// HoverAtPosition returns hover information for the identifier at the given position
func HoverAtPosition(doc *DocumentState, pos Position) *Hover {
	if doc.AST == nil || doc.Symbols == nil {
		return nil
	}

	// Find the node at this position
	node := FindNodeAtPosition(doc.AST, pos.Line, pos.Character)
	if node == nil {
		return nil
	}

	// Handle regular identifier first (variables, function names, etc.)
	// This takes priority over statement keywords
	if ident, ok := node.(*ast.Ident); ok {
		return hoverForIdentifier(doc, ident.Value)
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
		// Extract comments from the current document for functions and variables (not parameters)
		var comments string
		if sym.Kind == KindFunction || sym.Kind == KindVariable || sym.Kind == KindConst {
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
		return nil
	}

	// Resolve the import to a file path
	docURI := URIToPath(doc.URI)
	docDir := filepath.Dir(docURI)

	// Add .lex extension if not already present
	libFileName := importPath
	if !strings.HasSuffix(libFileName, ".lex") {
		libFileName = libFileName + ".lex"
	}

	// Try multiple locations: same dir, then stdlib
	var libFile string
	candidates := []string{
		filepath.Join(docDir, libFileName),                    // same directory as importing file
		filepath.Join(filepath.Dir(filepath.Dir(docDir)), "stdlib", libFileName), // stdlib dir (go up 2 levels from tests)
		filepath.Join(filepath.Dir(docDir), "stdlib", libFileName), // stdlib dir (go up 1 level)
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			libFile = candidate
			break
		}
	}

	if libFile == "" {
		return nil
	}

	// Get or parse the imported file
	libContent := getFileContent(libFile)
	if libContent == "" {
		return nil
	}

	// Parse the library and build symbols
	libAST, libSymbols := ParseDocumentAndBuildSymbols(libFile, libContent)
	if libAST == nil {
		return nil
	}

	// Look up the symbol in the library
	sym, ok := libSymbols.Symbols[propertyName]
	if !ok {
		return nil
	}

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
	if defLine < 2 || defLine > len(lines) {
		return ""
	}

	// Start from the line before the definition
	commentLines := []string{}
	for i := defLine - 2; i >= 0; i-- {
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

// getFileContent reads a file from disk, using cache to avoid re-reading
func getFileContent(filePath string) string {
	fileCacheLock.RLock()
	if content, ok := fileCache[filePath]; ok {
		fileCacheLock.RUnlock()
		return content
	}
	fileCacheLock.RUnlock()

	// Read from disk
	data, err := os.ReadFile(filePath)
	if err != nil {
		return ""
	}

	content := string(data)

	// Cache it
	fileCacheLock.Lock()
	fileCache[filePath] = content
	fileCacheLock.Unlock()

	return content
}

func renderBuiltinHover(name string, info BuiltinInfo) string {
	return fmt.Sprintf("```klex\n%s\n```\n\n%s", info.Signature, info.Documentation)
}

func renderSymbolHover(sym *Symbol) string {
	var content strings.Builder

	switch sym.Kind {
	case KindFunction:
		// Reconstruct function signature
		sig := renderFunctionSignature(sym)
		content.WriteString(fmt.Sprintf("```klex\nfn %s\n```\n", sig))
		content.WriteString(fmt.Sprintf("\n**Function** \n\n"))
		content.WriteString(fmt.Sprintf("| | |\n|---|---|\n"))
		content.WriteString(fmt.Sprintf("| **Defined** | Line %d |\n", sym.DefPos.Line))
		if sym.ReturnType != "" {
			content.WriteString(fmt.Sprintf("| **Returns** | `%s` |\n", sym.ReturnType))
		}

	case KindVariable:
		// Show type if available
		typeStr := ""
		if sym.Type != "" && sym.Type != "unknown" {
			typeStr = fmt.Sprintf("`%s`", sym.Type)
		}
		content.WriteString(fmt.Sprintf("```klex\n%s\n```\n", sym.Name))

		// Determine if from tuple unpacking
		varKind := "Variable"
		if sym.FromTuple {
			varKind = "Tuple element"
		}
		content.WriteString(fmt.Sprintf("\n**%s**\n\n", varKind))
		content.WriteString(fmt.Sprintf("| | |\n|---|---|\n"))
		if typeStr != "" {
			content.WriteString(fmt.Sprintf("| **Type** | %s |\n", typeStr))
		}
		content.WriteString(fmt.Sprintf("| **Defined** | Line %d |\n", sym.DefPos.Line))

	case KindConst:
		// Show type if available
		typeStr := ""
		if sym.Type != "" && sym.Type != "unknown" {
			typeStr = fmt.Sprintf("`%s`", sym.Type)
		}
		content.WriteString(fmt.Sprintf("```klex\nconst %s\n```\n", sym.Name))
		content.WriteString(fmt.Sprintf("\n**Constant**\n\n"))
		content.WriteString(fmt.Sprintf("| | |\n|---|---|\n"))
		if typeStr != "" {
			content.WriteString(fmt.Sprintf("| **Type** | %s |\n", typeStr))
		}
		content.WriteString(fmt.Sprintf("| **Defined** | Line %d |\n", sym.DefPos.Line))

	case KindModule:
		content.WriteString(fmt.Sprintf("```klex\nimport \"%s\"\n```\n", sym.Name))
		content.WriteString(fmt.Sprintf("\n**Module**\n\n"))
		content.WriteString(fmt.Sprintf("| | |\n|---|---|\n"))
		content.WriteString(fmt.Sprintf("| **Imported** | Line %d |\n", sym.DefPos.Line))

	case KindBuiltin:
		content.WriteString(fmt.Sprintf("```klex\n%s()\n```\n", sym.Name))
		content.WriteString(fmt.Sprintf("\n**Built-in Function**\n"))

	case KindParameter:
		content.WriteString(fmt.Sprintf("```klex\n%s\n```\n", sym.Name))
		typeStr := ""
		if sym.Type != "" && sym.Type != "unknown" {
			typeStr = fmt.Sprintf("`%s`", sym.Type)
		}
		content.WriteString(fmt.Sprintf("\n**Parameter**\n\n"))
		content.WriteString(fmt.Sprintf("| | |\n|---|---|\n"))
		if typeStr != "" {
			content.WriteString(fmt.Sprintf("| **Type** | %s |\n", typeStr))
		}
		content.WriteString(fmt.Sprintf("| **Declared** | Line %d |\n", sym.DefPos.Line))
	}

	return content.String()
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

func computeActiveParam(call *ast.CallExpr, pos Position) int {
	// Count how many arguments are before the cursor position
	count := 0
	for _, arg := range call.Args {
		if argPos, ok := arg.(*ast.Ident); ok {
			if argPos.Pos.Line < pos.Line+1 || (argPos.Pos.Line == pos.Line+1 && argPos.Pos.Col <= pos.Character) {
				count++
			}
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
