package main

import (
	"klex/ast"
	"klex/lexer"
	"klex/parser"
)

type SymbolKind int

const (
	KindVariable SymbolKind = iota
	KindFunction
	KindModule
	KindBuiltin
	KindConst
	KindParameter
)

type Symbol struct {
	Name         string
	Kind         SymbolKind
	Type         string     // inferred type: "string", "integer", "boolean", "array", "hash", "function", "channel", "struct", etc.
	StructType   string     // if Type is "struct", this is the struct name (e.g., "Person")
	Params       []string   // for functions
	ParamTypes   []string   // parallel to Params; "" means unannotated
	Defaults     []bool     // parallel to Params; true = has default
	Variadic     bool
	ReturnType   string     // "" means no annotation
	Body         []ast.Node // non-nil only for KindFunction; used for return type inference
	FromTuple    bool       // true if this variable came from tuple unpacking (x, y = func())
	DefURI       string
	DefPos       ast.Pos
}

type StructDef struct {
	Name   string
	Fields []string // field names in order
	DefPos ast.Pos
}

type SymbolTable struct {
	Symbols       map[string]*Symbol
	Refs          map[string][]ast.Pos // name -> list of reference positions
	Structs       map[string]*StructDef // struct name -> definition
	ReturnTypes   map[string]string     // function name -> inferred return type (cache)
}

func NewSymbolTable() *SymbolTable {
	return &SymbolTable{
		Symbols:     make(map[string]*Symbol),
		Refs:        make(map[string][]ast.Pos),
		Structs:     make(map[string]*StructDef),
		ReturnTypes: make(map[string]string),
	}
}

// ParseDocumentAndBuildSymbols parses the source and builds a symbol table
func ParseDocumentAndBuildSymbols(uri, source string) (*ast.Program, *SymbolTable) {
	lex := lexer.New(source)
	p := parser.New(lex)
	program := p.ParseProgram()

	symtab := NewSymbolTable()
	walkAST(program, uri, symtab)

	return program, symtab
}

// walkAST traverses the AST and collects symbols
func walkAST(program *ast.Program, uri string, symtab *SymbolTable) {
	if program == nil {
		return
	}

	for _, stmt := range program.Statements {
		walkStatement(stmt, uri, symtab)
	}
}

func walkStatement(node ast.Node, uri string, symtab *SymbolTable) {
	if node == nil {
		return
	}

	switch n := node.(type) {
	case *ast.AssignStmt:
		// Named function: fn foo(x) { ... }
		if fn, ok := n.Value.(*ast.FunctionLiteral); ok {
			symtab.Symbols[n.Name] = &Symbol{
				Name:       n.Name,
				Kind:       KindFunction,
				Type:       "function",
				Params:     fn.Params,
				ParamTypes: fn.ParamTypes,
				Defaults:   paramsHaveDefaults(fn.Defaults, fn.Params),
				Variadic:   fn.Variadic,
				ReturnType: fn.ReturnType,
				Body:       fn.Body,
				DefURI:     uri,
				DefPos:     n.Pos,
			}
			// Add parameters to symbol table so they can be hovered
			for i, param := range fn.Params {
				paramType := "unknown"
				// Use annotation if available
				if i < len(fn.ParamTypes) && fn.ParamTypes[i] != "" {
					paramType = fn.ParamTypes[i]
				}
				symtab.Symbols[param] = &Symbol{
					Name:   param,
					Kind:   KindParameter,
					Type:   paramType,
					DefURI: uri,
					DefPos: fn.Pos, // use function position as parameter position
				}
				// Mark which parameters have defaults
				if i < len(fn.Defaults) && fn.Defaults[i] != nil {
					symtab.Symbols[param].Defaults = []bool{true}
				}
			}
			walkBody(fn.Body, uri, symtab)
		} else {
			// Bare assignment: x = val
			// Create a symbol if it doesn't exist (first assignment), otherwise just reference it
			if _, exists := symtab.Symbols[n.Name]; !exists {
				typ, structName := inferTypeFromExpressionWithStructs(n.Value, symtab)
				symtab.Symbols[n.Name] = &Symbol{
					Name:       n.Name,
					Kind:       KindVariable,
					Type:       typ,
					StructType: structName,
					DefURI:     uri,
					DefPos:     n.Pos,
				}
			} else {
				// Existing variable, just record the reference
				symtab.Refs[n.Name] = append(symtab.Refs[n.Name], n.Pos)
			}
			walkExpression(n.Value, uri, symtab)
		}

	case *ast.LetStmt:
		typ, structName := inferTypeFromExpressionWithStructs(n.Value, symtab)
		symtab.Symbols[n.Name] = &Symbol{
			Name:       n.Name,
			Kind:       KindVariable,
			Type:       typ,
			StructType: structName,
			DefURI:     uri,
			DefPos:     n.Pos,
		}
		walkExpression(n.Value, uri, symtab)

	case *ast.ConstStmt:
		typ, structName := inferTypeFromExpressionWithStructs(n.Value, symtab)
		symtab.Symbols[n.Name] = &Symbol{
			Name:       n.Name,
			Kind:       KindConst,
			Type:       typ,
			StructType: structName,
			DefURI:     uri,
			DefPos:     n.Pos,
		}
		walkExpression(n.Value, uri, symtab)

	case *ast.MultiAssignStmt:
		// Handle tuple unpacking: x, y = func()
		// For now, mark all targets as unknown type since we can't infer tuple element types
		for i, name := range n.Names {
			symtab.Symbols[name] = &Symbol{
				Name:      name,
				Kind:      KindVariable,
				Type:      "unknown", // We can't infer individual tuple element types without more analysis
				FromTuple: true,      // Mark that this came from tuple unpacking
				DefURI:    uri,
				DefPos:    n.Pos, // All have same position for the assignment
			}
			// Store line number for each variable by offset if needed
			if i > 0 {
				// Approximate line numbers for multiple assignments on same line
				symtab.Symbols[name].DefPos = ast.Pos{Line: n.Pos.Line, Col: n.Pos.Col + i}
			}
		}
		walkExpression(n.Value, uri, symtab)

	case *ast.ImportStmt:
		symtab.Symbols[n.Alias] = &Symbol{
			Name:   n.Alias,
			Kind:   KindModule,
			DefURI: uri,
			DefPos: n.Pos,
		}

	case *ast.ReturnStmt:
		walkExpression(n.Value, uri, symtab)

	case *ast.IfStmt:
		walkExpression(n.Condition, uri, symtab)
		walkBody(n.Body, uri, symtab)
		walkBody(n.ElseBody, uri, symtab)

	case *ast.WhileStmt:
		walkExpression(n.Condition, uri, symtab)
		walkBody(n.Body, uri, symtab)

	case *ast.ForInStmt:
		walkExpression(n.Collection, uri, symtab)
		walkBody(n.Body, uri, symtab)

	case *ast.SwitchStmt:
		walkExpression(n.Subject, uri, symtab)
		for _, c := range n.Cases {
			for _, val := range c.Values {
				walkExpression(val, uri, symtab)
			}
			walkBody(c.Body, uri, symtab)
		}
		walkBody(n.Default, uri, symtab)

	case *ast.SelectStmt:
		for _, c := range n.Cases {
			walkExpression(c.Chan, uri, symtab)
			walkExpression(c.SendVal, uri, symtab)
			walkBody(c.Body, uri, symtab)
		}

	case *ast.StructDecl:
		// Register the struct definition
		symtab.Structs[n.Name] = &StructDef{
			Name:   n.Name,
			Fields: n.Fields,
			DefPos: n.Pos,
		}
		// Also add as a symbol for reference tracking
		symtab.Symbols[n.Name] = &Symbol{
			Name:   n.Name,
			Kind:   KindVariable,
			Type:   "struct",
			DefURI: uri,
			DefPos: n.Pos,
		}

	case *ast.EnumDecl:
		symtab.Symbols[n.Name] = &Symbol{
			Name:   n.Name,
			Kind:   KindVariable,
			DefURI: uri,
			DefPos: n.Pos,
		}
	}
}

func walkBody(nodes []ast.Node, uri string, symtab *SymbolTable) {
	for _, node := range nodes {
		walkStatement(node, uri, symtab)
	}
}

func walkExpression(node ast.Node, uri string, symtab *SymbolTable) {
	if node == nil {
		return
	}

	switch n := node.(type) {
	case *ast.Ident:
		symtab.Refs[n.Value] = append(symtab.Refs[n.Value], n.Pos)

	case *ast.CallExpr:
		walkExpression(n.Function, uri, symtab)
		for _, arg := range n.Args {
			walkExpression(arg, uri, symtab)
		}

	case *ast.InfixExpr:
		walkExpression(n.Left, uri, symtab)
		walkExpression(n.Right, uri, symtab)

	case *ast.PrefixExpr:
		walkExpression(n.Right, uri, symtab)

	case *ast.IndexExpr:
		walkExpression(n.Left, uri, symtab)
		walkExpression(n.Index, uri, symtab)

	case *ast.DotExpr:
		walkExpression(n.Left, uri, symtab)

	case *ast.PipeExpr:
		walkExpression(n.Left, uri, symtab)
		walkExpression(n.Right, uri, symtab)

	case *ast.ArrayLiteral:
		for _, elem := range n.Elements {
			walkExpression(elem, uri, symtab)
		}

	case *ast.HashLiteral:
		for _, pair := range n.Pairs {
			walkExpression(pair.Key, uri, symtab)
			walkExpression(pair.Value, uri, symtab)
		}

	case *ast.FunctionLiteral:
		// Add parameters to symbol table so they can be hovered
		for i, param := range n.Params {
			symtab.Symbols[param] = &Symbol{
				Name:   param,
				Kind:   KindParameter,
				Type:   "unknown", // parameter types are not tracked
				DefURI: uri,
				DefPos: n.Pos, // use function position as parameter position
			}
			// Mark which parameters have defaults
			if i < len(n.Defaults) && n.Defaults[i] != nil {
				symtab.Symbols[param].Defaults = []bool{true}
			}
		}
		walkBody(n.Body, uri, symtab)

	case *ast.StructLiteral:
		for _, field := range n.Fields {
			walkExpression(field.Value, uri, symtab)
		}

	case *ast.InterpolatedString:
		for _, seg := range n.Segments {
			walkExpression(seg.Expr, uri, symtab)
		}

	case *ast.TupleLiteral:
		for _, elem := range n.Elements {
			walkExpression(elem, uri, symtab)
		}
	}
}

func paramsHaveDefaults(defaults []ast.Node, params []string) []bool {
	result := make([]bool, len(params))
	for i := 0; i < len(params) && i < len(defaults); i++ {
		result[i] = defaults[i] != nil
	}
	return result
}

// FindNodeAtPosition finds the deepest AST node at the given 0-based line/col
func FindNodeAtPosition(program *ast.Program, line, col int) ast.Node {
	// Convert LSP 0-based to kLex 1-based
	line++

	var closest ast.Node
	var closestDist int = -1

	var walk func(ast.Node)
	walk = func(node ast.Node) {
		if node == nil {
			return
		}

		// Get position of this node
		var nodePos ast.Pos
		switch n := node.(type) {
		case *ast.Ident:
			nodePos = n.Pos
		case *ast.IntLiteral:
			nodePos = n.Pos
		case *ast.FloatLiteral:
			nodePos = n.Pos
		case *ast.StringLiteral:
			nodePos = n.Pos
		case *ast.BoolLiteral:
			nodePos = n.Pos
		case *ast.NullLiteral:
			nodePos = n.Pos
		case *ast.ArrayLiteral:
			nodePos = n.Pos
		case *ast.HashLiteral:
			nodePos = n.Pos
		case *ast.FunctionLiteral:
			nodePos = n.Pos
		case *ast.CallExpr:
			nodePos = n.Pos
		case *ast.InfixExpr:
			nodePos = n.Pos
		case *ast.PrefixExpr:
			nodePos = n.Pos
		case *ast.IndexExpr:
			nodePos = n.Pos
		case *ast.DotExpr:
			nodePos = n.Pos
		case *ast.PipeExpr:
			nodePos = n.Pos
		case *ast.AssignStmt:
			nodePos = n.Pos
		case *ast.LetStmt:
			nodePos = n.Pos
		case *ast.ConstStmt:
			nodePos = n.Pos
		case *ast.ReturnStmt:
			nodePos = n.Pos
		case *ast.IfStmt:
			nodePos = n.Pos
		case *ast.WhileStmt:
			nodePos = n.Pos
		case *ast.ForInStmt:
			nodePos = n.Pos
		case *ast.SwitchStmt:
			nodePos = n.Pos
		case *ast.BreakStmt:
			nodePos = n.Pos
		case *ast.ContinueStmt:
			nodePos = n.Pos
		case *ast.MultiAssignStmt:
			nodePos = n.Pos
		case *ast.ImportStmt:
			nodePos = n.Pos
		case *ast.IndexAssignStmt:
			nodePos = n.Pos
		case *ast.DotAssignStmt:
			nodePos = n.Pos
		case *ast.SelectStmt:
			nodePos = n.Pos
		case *ast.StructDecl:
			nodePos = n.Pos
		case *ast.EnumDecl:
			nodePos = n.Pos
		default:
			return
		}

		// Check if cursor is on or after this node's start position
		if nodePos.Line < line || (nodePos.Line == line && nodePos.Col <= col) {
			// Calculate "distance" (prefer closer/more specific nodes)
			distance := (line - nodePos.Line) * 1000 + (col - nodePos.Col)
			if closestDist < 0 || distance < closestDist {
				closest = node
				closestDist = distance
			}
		}
	}

	// Walk all statement nodes and their children
	for _, stmt := range program.Statements {
		walkAST2(stmt, walk)
	}

	return closest
}

// walkAST2 recursively walks AST nodes
func walkAST2(node ast.Node, visit func(ast.Node)) {
	if node == nil {
		return
	}

	visit(node)

	switch n := node.(type) {
	case *ast.AssignStmt:
		walkAST2(n.Value, visit)
	case *ast.LetStmt:
		walkAST2(n.Value, visit)
	case *ast.ConstStmt:
		walkAST2(n.Value, visit)
	case *ast.ReturnStmt:
		walkAST2(n.Value, visit)
	case *ast.IfStmt:
		walkAST2(n.Condition, visit)
		for _, stmt := range n.Body {
			walkAST2(stmt, visit)
		}
		for _, stmt := range n.ElseBody {
			walkAST2(stmt, visit)
		}
	case *ast.WhileStmt:
		walkAST2(n.Condition, visit)
		for _, stmt := range n.Body {
			walkAST2(stmt, visit)
		}
	case *ast.ForInStmt:
		walkAST2(n.Collection, visit)
		for _, stmt := range n.Body {
			walkAST2(stmt, visit)
		}
	case *ast.SwitchStmt:
		walkAST2(n.Subject, visit)
		for _, c := range n.Cases {
			for _, val := range c.Values {
				walkAST2(val, visit)
			}
			for _, stmt := range c.Body {
				walkAST2(stmt, visit)
			}
		}
		for _, stmt := range n.Default {
			walkAST2(stmt, visit)
		}
	case *ast.SelectStmt:
		for _, c := range n.Cases {
			walkAST2(c.Chan, visit)
			walkAST2(c.SendVal, visit)
			for _, stmt := range c.Body {
				walkAST2(stmt, visit)
			}
		}
	case *ast.CallExpr:
		walkAST2(n.Function, visit)
		for _, arg := range n.Args {
			walkAST2(arg, visit)
		}
	case *ast.InfixExpr:
		walkAST2(n.Left, visit)
		walkAST2(n.Right, visit)
	case *ast.PrefixExpr:
		walkAST2(n.Right, visit)
	case *ast.IndexExpr:
		walkAST2(n.Left, visit)
		walkAST2(n.Index, visit)
	case *ast.DotExpr:
		walkAST2(n.Left, visit)
	case *ast.PipeExpr:
		walkAST2(n.Left, visit)
		walkAST2(n.Right, visit)
	case *ast.ArrayLiteral:
		for _, elem := range n.Elements {
			walkAST2(elem, visit)
		}
	case *ast.HashLiteral:
		for _, pair := range n.Pairs {
			walkAST2(pair.Key, visit)
			walkAST2(pair.Value, visit)
		}
	case *ast.FunctionLiteral:
		for _, stmt := range n.Body {
			walkAST2(stmt, visit)
		}
	case *ast.StructLiteral:
		for _, field := range n.Fields {
			walkAST2(field.Value, visit)
		}
	case *ast.InterpolatedString:
		for _, seg := range n.Segments {
			walkAST2(seg.Expr, visit)
		}
	case *ast.TupleLiteral:
		for _, elem := range n.Elements {
			walkAST2(elem, visit)
		}
	case *ast.MultiAssignStmt:
		walkAST2(n.Value, visit)
	case *ast.ImportStmt:
		// No child nodes to walk
	case *ast.IndexAssignStmt:
		walkAST2(n.Left.Left, visit)
		walkAST2(n.Left.Index, visit)
		walkAST2(n.Value, visit)
	case *ast.DotAssignStmt:
		walkAST2(n.Left.Left, visit)
		walkAST2(n.Value, visit)
	case *ast.StructDecl:
		// Struct methods have bodies that should be walked, but they're not in StructDecl
		// This would be handled in Phase 2 when we support struct methods
	case *ast.EnumDecl:
		// No child expressions to walk
	}
}

// GetIdentAtPosition finds an Ident node at the given position
func GetIdentAtPosition(program *ast.Program, line, col int) *ast.Ident {
	node := FindNodeAtPosition(program, line, col)
	if ident, ok := node.(*ast.Ident); ok {
		return ident
	}
	return nil
}

// GetCallExprAtPosition finds a CallExpr at the given position
func GetCallExprAtPosition(program *ast.Program, line, col int) *ast.CallExpr {
	node := FindNodeAtPosition(program, line, col)
	if call, ok := node.(*ast.CallExpr); ok {
		return call
	}
	return nil
}

// inferTypeFromExpressionWithStructs infers the type and struct name for expressions
func inferTypeFromExpressionWithStructs(expr ast.Node, symtab *SymbolTable) (string, string) {
	if expr == nil {
		return "unknown", ""
	}

	switch e := expr.(type) {
	case *ast.IntLiteral:
		return "integer", ""
	case *ast.FloatLiteral:
		return "float", ""
	case *ast.StringLiteral:
		return "string", ""
	case *ast.BoolLiteral:
		return "boolean", ""
	case *ast.NullLiteral:
		return "null", ""
	case *ast.ArrayLiteral:
		return "array", ""
	case *ast.HashLiteral:
		return "hash", ""
	case *ast.FunctionLiteral:
		return "function", ""
	case *ast.StructLiteral:
		return "struct", e.Name
	case *ast.TupleLiteral:
		return "tuple", ""
	default:
		typ := inferTypeFromExpression(expr, symtab)
		return typ, ""
	}
}

// collectReturnTypes recursively walks a function body and appends the inferred type
// of each return statement to *out. Does not recurse into nested function bodies.
func collectReturnTypes(nodes []ast.Node, out *[]string, symtab *SymbolTable) {
	for _, node := range nodes {
		switch n := node.(type) {
		case *ast.ReturnStmt:
			if n.Value == nil {
				*out = append(*out, "null")
			} else {
				typ := inferTypeFromExpression(n.Value, symtab)
				*out = append(*out, typ)
			}

		case *ast.IfStmt:
			collectReturnTypes(n.Body, out, symtab)
			if n.ElseBody != nil {
				collectReturnTypes(n.ElseBody, out, symtab)
			}

		case *ast.WhileStmt:
			collectReturnTypes(n.Body, out, symtab)

		case *ast.ForInStmt:
			collectReturnTypes(n.Body, out, symtab)

		case *ast.SwitchStmt:
			for _, caseStmt := range n.Cases {
				collectReturnTypes(caseStmt.Body, out, symtab)
			}
			if n.HasDefault {
				collectReturnTypes(n.Default, out, symtab)
			}

		case *ast.FunctionLiteral:
			// Do not recurse into nested function bodies
		}
	}
}

// inferReturnTypeFromBody analyzes a function body and infers its return type.
// Uses a sentinel value ("__analyzing__") to detect recursive calls.
func inferReturnTypeFromBody(body []ast.Node, funcName string, symtab *SymbolTable) string {
	if symtab == nil {
		return "unknown"
	}

	// Check for explicit return type annotation first
	if sym, ok := symtab.Symbols[funcName]; ok && sym.ReturnType != "" {
		return sym.ReturnType
	}

	// Check for recursive call
	if cached, ok := symtab.ReturnTypes[funcName]; ok && cached == "__analyzing__" {
		return "unknown"
	}

	// Mark as analyzing to prevent infinite recursion
	symtab.ReturnTypes[funcName] = "__analyzing__"

	// Collect all return types from the body
	var returnTypes []string
	collectReturnTypes(body, &returnTypes, symtab)

	// If no explicit returns, function implicitly returns null
	if len(returnTypes) == 0 {
		symtab.ReturnTypes[funcName] = "null"
		return "null"
	}

	// Deduplicate and filter null entries
	typeSet := make(map[string]bool)
	var nonNullTypes []string
	for _, t := range returnTypes {
		if t != "null" {
			nonNullTypes = append(nonNullTypes, t)
			typeSet[t] = true
		} else {
			typeSet[t] = true
		}
	}

	// Disagreement strategy:
	// - If all returns were null → "null"
	// - If exactly one non-null type → return that type (ignoring nulls)
	// - If multiple distinct non-null types → "unknown"
	var result string
	if len(nonNullTypes) == 0 {
		result = "null"
	} else if len(typeSet) == 1 && len(nonNullTypes) > 0 {
		// All non-null returns are the same type
		result = nonNullTypes[0]
	} else if len(typeSet) == 2 && typeSet["null"] {
		// Only null and one other type
		for t := range typeSet {
			if t != "null" {
				result = t
				break
			}
		}
	} else if len(typeSet) > 1 {
		// Multiple distinct types or multiple non-null types
		result = "unknown"
	} else {
		result = nonNullTypes[0]
	}

	// Cache the result
	symtab.ReturnTypes[funcName] = result
	return result
}

// inferTypeFromExpression infers the type of an expression
func inferTypeFromExpression(expr ast.Node, symtab *SymbolTable) string {
	if expr == nil {
		return "unknown"
	}

	switch e := expr.(type) {
	case *ast.IntLiteral:
		return "integer"
	case *ast.FloatLiteral:
		return "float"
	case *ast.StringLiteral:
		return "string"
	case *ast.BoolLiteral:
		return "boolean"
	case *ast.NullLiteral:
		return "null"
	case *ast.ArrayLiteral:
		return "array"
	case *ast.HashLiteral:
		return "hash"
	case *ast.FunctionLiteral:
		return "function"
	case *ast.StructLiteral:
		return "struct"
	case *ast.TupleLiteral:
		return "tuple"
	case *ast.InfixExpr:
		// Arithmetic operators typically produce numbers
		switch e.Operator {
		case "+", "-", "*", "/", "%":
			// Division always produces float, others could be int or float
			if e.Operator == "/" {
				return "float"
			}
			// For +, -, *, % check left operand type
			leftType := inferTypeFromExpression(e.Left, symtab)
			if leftType == "float" {
				return "float"
			}
			rightType := inferTypeFromExpression(e.Right, symtab)
			if rightType == "float" {
				return "float"
			}
			// Both are integers or unknown
			if leftType == "integer" || rightType == "integer" {
				return "integer"
			}
			return "number"
		case "==", "!=", "<", ">", "<=", ">=", "&&", "||":
			return "boolean"
		default:
			return "unknown"
		}
	case *ast.PrefixExpr:
		// ! produces boolean, - produces number
		switch e.Operator {
		case "!":
			return "boolean"
		case "-":
			rightType := inferTypeFromExpression(e.Right, symtab)
			if rightType == "integer" || rightType == "float" {
				return rightType
			}
			return "number"
		default:
			return "unknown"
		}
	case *ast.IndexExpr:
		// arr[i] returns the element type (unknown for us)
		// hash[key] returns the value type (unknown)
		return "unknown"
	case *ast.Ident:
		// Look up the identifier in the symbol table to propagate its type
		if symtab != nil {
			if sym, ok := symtab.Symbols[e.Value]; ok && sym.Type != "" && sym.Type != "unknown" {
				return sym.Type
			}
		}
		return "unknown"
	case *ast.InterpolatedString:
		// Interpolated strings always produce string type
		return "string"
	case *ast.CallExpr:
		// Try to infer from function name
		if ident, ok := e.Function.(*ast.Ident); ok {
			funcName := ident.Value

			// Check builtin functions
			switch funcName {
			// Array functions
			case "makeArray", "slice", "push", "pop", "concat", "filter", "map", "range":
				return "array"
			// Hash functions
			case "keys", "values":
				return "array"
			// String functions
			case "split", "substr", "upper", "lower", "trim", "replace":
				return "string"
			case "str", "format":
				return "string"
			// Number functions
			case "int", "float", "len", "ceil", "floor", "round", "sqrt", "min", "max":
				return "integer"
			case "rand", "randInt":
				return "integer"
			// Type checking
			case "type":
				return "string"
			// Channel/async
			case "channel":
				return "channel"
			case "async":
				return "task"
			case "error":
				return "error"
			// Concurrency functions that return channels/values
			case "recv":
				return "tuple" // (value, ok)
			case "recvNonBlock":
				return "unknown" // could be any type
			// Network functions
			case "tcpDial", "dnsLookup", "netRead", "netWrite":
				return "tuple" // these all return (value, error) tuples
			case "tcpListen":
				return "channel" // returns a channel of connections
			case "netClose":
				return "null" // returns null
			default:
				// Check user-defined functions
				if symtab != nil {
					if sym, ok := symtab.Symbols[funcName]; ok && sym.Kind == KindFunction {
						// Check for explicit return type annotation first
						if sym.ReturnType != "" {
							return sym.ReturnType
						}
						// Check cache
						if cached, ok := symtab.ReturnTypes[funcName]; ok && cached != "__analyzing__" {
							return cached
						}
						// Analyze function body to infer return type
						if sym.Body != nil {
							return inferReturnTypeFromBody(sym.Body, funcName, symtab)
						}
					}
				}
				return "unknown"
			}
		}
		return "unknown"
	default:
		return "unknown"
	}
}

// TryResolveImport resolves an import path to find the module file
func TryResolveImport(currentDocURI string, importPath string) string {
	// For now, return empty (Phase 2: cross-file definitions)
	return ""
}

// LookupSymbol finds a symbol by name in the symbol table
func LookupSymbol(symtab *SymbolTable, name string) *Symbol {
	return symtab.Symbols[name]
}

// IsBuiltin checks if a name is a builtin
func IsBuiltin(name string) bool {
	_, ok := builtinSignatures[name]
	return ok
}

// GetBuiltinDoc returns the documentation for a builtin
func GetBuiltinDoc(name string) (signature string, doc string) {
	if sig, ok := builtinSignatures[name]; ok {
		return sig.Signature, sig.Documentation
	}
	return "", ""
}
