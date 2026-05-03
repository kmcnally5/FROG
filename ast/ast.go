package ast

// The AST (Abstract Syntax Tree) is the data structure that represents a
// parsed program. After the lexer turns source text into tokens, the parser
// reads those tokens and builds this tree.
//
// "Abstract" means we throw away syntactic noise (parentheses, commas, braces)
// and keep only the structure that matters for evaluation.
//
// Example:  x = 1 + 2 * 3
//
//   AssignStmt
//   ├── Name: "x"
//   └── Value: InfixExpr(+)
//       ├── Left:  IntLiteral(1)
//       └── Right: InfixExpr(*)
//           ├── Left:  IntLiteral(2)
//           └── Right: IntLiteral(3)
//
// The evaluator walks this tree recursively to compute the result.

// Pos holds the source position of a node (1-based line and column).
// Carrying position on every node means error messages can say "line 5, col 3"
// rather than just "something went wrong".
type Pos struct {
	Line int
	Col  int
}

// Node is the interface every AST node must implement.
// TokenLiteral is used for debugging — it returns a short label for the node.
// All real behaviour comes from the evaluator switching on the concrete type.
type Node interface {
	TokenLiteral() string
}

// Program is the root of every AST. The parser produces exactly one Program
// per source file. Errors are accumulated here (rather than panicking) so the
// parser can report multiple problems in a single pass.
type Program struct {
	Statements []Node
	Errors     []string
}

func (p *Program) TokenLiteral() string { return "program" }

// CallExpr represents a function call: foo(a, b)
// Function can be an Ident ("foo") or any expression that evaluates to a function.
type CallExpr struct {
	Pos
	Function Node   // the thing being called
	Args     []Node // the arguments passed in
}

func (c *CallExpr) TokenLiteral() string { return "call" }

// IntLiteral is a literal integer value in the source, e.g. 42.
type IntLiteral struct {
	Pos
	Value int
}

func (i *IntLiteral) TokenLiteral() string { return "int" }

// FloatLiteral is a literal floating-point value in the source, e.g. 3.14.
type FloatLiteral struct {
	Pos
	Value float64
}

func (f *FloatLiteral) TokenLiteral() string { return "float" }

// StringLiteral is a quoted string in the source, e.g. "hello".
type StringLiteral struct {
	Pos
	Value string
}

func (s *StringLiteral) TokenLiteral() string { return "string" }

// StringSegment is one piece of an InterpolatedString.
// When IsExpr is false, Text holds a literal run of characters.
// When IsExpr is true, Expr holds the embedded expression to evaluate.
type StringSegment struct {
	IsExpr bool
	Text   string
	Expr   Node
}

// InterpolatedString is a string containing embedded expressions: "Hello {name}"
// The parser splits the raw source into alternating literal and expression segments.
type InterpolatedString struct {
	Pos
	Segments []StringSegment
}

func (i *InterpolatedString) TokenLiteral() string { return "interp_string" }

// AssignStmt covers both variable creation and update: x = expr
// kLex has no separate "declare" keyword — assignment does both.
// Named function definitions (fn foo(...) { }) are also desugared into
// AssignStmt at parse time: AssignStmt{Name: "foo", Value: FunctionLiteral}.
type AssignStmt struct {
	Pos
	Name  string
	Value Node
}

func (a *AssignStmt) TokenLiteral() string { return a.Name }

// LetStmt is an explicit local-scope declaration: let x = expr
// Unlike AssignStmt (which walks the scope chain via Assign), LetStmt always
// creates the binding in the current scope via Set — it never touches an outer scope.
type LetStmt struct {
	Pos
	Name  string
	Value Node
}

func (l *LetStmt) TokenLiteral() string { return l.Name }

// ConstStmt declares an immutable binding in the current scope: const x = expr
// Like LetStmt, it always creates in the current scope (never walks the chain).
// Unlike LetStmt, any attempt to reassign the name at any point is a RuntimeError.
type ConstStmt struct {
	Pos
	Name  string
	Value Node
}

func (c *ConstStmt) TokenLiteral() string { return c.Name }

// Ident is a reference to a variable, e.g. x or myArray.
// At eval time, we look the name up in the current Environment.
type Ident struct {
	Pos
	Value string // the variable name
}

func (i *Ident) TokenLiteral() string { return i.Value }

// InfixExpr is any binary operation: left OP right
// Examples: 1 + 2, a == b, x && y
type InfixExpr struct {
	Pos
	Left     Node
	Operator string // "+", "-", "==", "&&", etc.
	Right    Node
}

func (i *InfixExpr) TokenLiteral() string { return i.Operator }

// IfStmt covers both if { } and if { } else { }.
// ElseBody is nil when there is no else clause.
type IfStmt struct {
	Pos
	Condition Node
	Body      []Node
	ElseBody  []Node // nil if no else branch
}

func (i *IfStmt) TokenLiteral() string { return "if" }

// PrefixExpr is a unary operator applied to one operand: !x, -n
type PrefixExpr struct {
	Pos
	Operator string // "!" or "-"
	Right    Node
}

func (p *PrefixExpr) TokenLiteral() string { return p.Operator }

// NullLiteral is the keyword `null` written in source code.
type NullLiteral struct {
	Pos
}

func (n *NullLiteral) TokenLiteral() string { return "null" }

// BoolLiteral is `true` or `false` written in source code.
type BoolLiteral struct {
	Pos
	Value bool
}

func (b *BoolLiteral) TokenLiteral() string {
	if b.Value {
		return "true"
	}
	return "false"
}

// FunctionLiteral is an anonymous function expression: fn(x, y) { ... }
// Named functions (fn foo(x) { }) are parsed as AssignStmt{Value: FunctionLiteral}
// so that "foo" ends up in scope like any other variable.
type FunctionLiteral struct {
	Pos
	Params      []string // parameter names in declaration order
	ParamTypes  []string // parallel to Params; "" means unannotated
	Defaults    []Node   // parallel to Params; nil entry means the param is required
	Variadic    bool     // true if the last param collects remaining args as an array
	ReturnType  string   // "" means no return type annotation
	Body        []Node   // the statements inside the function body
}

func (f *FunctionLiteral) TokenLiteral() string { return "fn" }

// ReturnStmt exits the current function with a value: return expr
type ReturnStmt struct {
	Pos
	Value Node
}

func (r *ReturnStmt) TokenLiteral() string { return "return" }

// WhileStmt is the only loop construct in kLex: while condition { }
// The evaluator re-evaluates Condition before each iteration.
type WhileStmt struct {
	Pos
	Condition Node
	Body      []Node
}

func (w *WhileStmt) TokenLiteral() string { return "while" }

// SwitchCase is one arm of a switch statement.
// Values holds the match expressions (compared with == for value switch,
// evaluated as booleans for expression switch). Body is the block to run.
type SwitchCase struct {
	Values []Node
	Body   []Node
}

// SwitchStmt dispatches on a value or a set of boolean expressions.
// Subject is nil for an expression switch (switch { case x > 10 { } }).
// Cases are tried in order; the first match wins, no fallthrough.
// Default runs when no case matches; it is optional.
// HasDefault is true even when the default body is empty — this lets the
// evaluator distinguish "no default clause" from "explicit empty default {}",
// which matters for exhaustive enum switch checking.
type SwitchStmt struct {
	Pos
	Subject    Node         // nil for expression switch
	Cases      []SwitchCase
	Default    []Node       // nil if no default clause, or empty if default {}
	HasDefault bool         // true when a default keyword was present
}

func (s *SwitchStmt) TokenLiteral() string { return "switch" }

// ForInStmt is a range-based loop: for x in collection { }
// With two variables:  for k, v in hash  — k=key, v=value
//                      for i, v in array — i=index, v=element
// Like while, break and continue work inside for-in loops.
type ForInStmt struct {
	Pos
	Variable   string // loop variable: element (single) or key/index (two-var)
	ValueVar   string // optional second variable for value (empty = single-var form)
	Collection Node   // expression that produces the array or hash to iterate over
	Body       []Node
}

func (f *ForInStmt) TokenLiteral() string { return "for" }

// BreakStmt exits the nearest enclosing while loop immediately.
// Implemented as a signal object (BreakSignal) that bubbles up through eval.
type BreakStmt struct {
	Pos
}

func (b *BreakStmt) TokenLiteral() string { return "break" }

// ContinueStmt skips the rest of the current loop body and goes back to the
// condition check. Like break, it bubbles up as a signal object.
type ContinueStmt struct {
	Pos
}

func (c *ContinueStmt) TokenLiteral() string { return "continue" }

// ArrayLiteral is a literal array: [1, 2, 3]
// Each element is an arbitrary expression evaluated at runtime.
type ArrayLiteral struct {
	Pos
	Elements []Node
}

func (a *ArrayLiteral) TokenLiteral() string { return "[" }

// IndexExpr is an index access: expr[index]
// Used for both array indexing (arr[0]) and hash lookup (map["key"]).
// The evaluator decides which based on the runtime type of Left.
type IndexExpr struct {
	Pos
	Left  Node // the thing being indexed (array or hash)
	Index Node // the key or position
}

func (i *IndexExpr) TokenLiteral() string { return "index" }

// HashPair holds one key-value pair inside a HashLiteral.
// Both Key and Value are arbitrary expressions.
type HashPair struct {
	Key   Node
	Value Node
}

// HashLiteral is a literal hash/dictionary: {"key": value, ...}
// Valid key types at runtime are string, integer, and boolean.
type HashLiteral struct {
	Pos
	Pairs []HashPair
}

func (h *HashLiteral) TokenLiteral() string { return "{" }

// IndexAssignStmt is an indexed assignment: expr[key] = value
// This is a statement rather than an expression because kLex assignment
// is statement-level throughout (there are no assignment expressions).
// Covers both hash assignment (m["k"] = v) and array mutation (arr[0] = v).
// TupleLiteral represents multiple comma-separated return values: return a, b
// The parser produces this when it sees more than one expression after `return`.
// At eval time it becomes a Tuple object.
type TupleLiteral struct {
	Pos
	Elements []Node
}

func (t *TupleLiteral) TokenLiteral() string { return "tuple" }

// MultiAssignStmt is a multiple-variable assignment: a, b = expr
// The RHS must evaluate to a Tuple with exactly as many elements as there
// are names on the left. Any count mismatch is a RuntimeError.
type MultiAssignStmt struct {
	Pos
	Names []string // left-hand side variable names
	Value Node     // must evaluate to a Tuple
}

func (m *MultiAssignStmt) TokenLiteral() string { return "multi=" }

// DotExpr is a property access on a module: math.add
// Left is the module expression, Property is the name after the dot.
type DotExpr struct {
	Pos
	Left     Node
	Property string
}

func (d *DotExpr) TokenLiteral() string { return "." }

// ImportStmt loads a kLex file and binds it as a module: import "math.lex" as math
// Path is the file path exactly as written. Alias is the name bound in scope.
type ImportStmt struct {
	Pos
	Path  string // e.g. "math.lex"
	Alias string // e.g. "math"
}

func (i *ImportStmt) TokenLiteral() string { return "import" }

type IndexAssignStmt struct {
	Pos
	Left  *IndexExpr // the index expression being written to
	Value Node       // the new value
}

func (i *IndexAssignStmt) TokenLiteral() string { return "[]=" }

// DotAssignStmt is a dot assignment: obj.prop = value
type DotAssignStmt struct {
	Pos
	Left  *DotExpr // the dot expression being written to
	Value Node     // the new value
}

func (d *DotAssignStmt) TokenLiteral() string { return ".=" }

// MethodDecl is a method inside a struct body: fn name(params) { body }
type MethodDecl struct {
	Pos
	Name        string
	Params      []string
	ParamTypes  []string // parallel to Params; "" means unannotated
	Defaults    []Node   // parallel to Params; nil entry means the param is required
	Variadic    bool
	ReturnType  string   // "" means no return type annotation
	Body        []Node
}

func (m *MethodDecl) TokenLiteral() string { return "method" }

// StructDecl declares a named struct type: struct Point { x, y  fn area() { } }
type StructDecl struct {
	Pos
	Name    string
	Fields  []string      // field names in declaration order
	Methods []*MethodDecl // method definitions
}

func (s *StructDecl) TokenLiteral() string { return "struct" }

// FieldInit is one named field in a struct literal: x: expr
type FieldInit struct {
	Name  string
	Value Node
}

// StructLiteral creates a struct instance: Point { x: 1, y: 2 }
type StructLiteral struct {
	Pos
	Name   string      // the struct type name
	Fields []FieldInit // field initialisers in source order
}

func (s *StructLiteral) TokenLiteral() string { return "struct_lit" }

// VariantDecl is one variant inside an enum body: Circle(r) or Point (no fields).
type VariantDecl struct {
	Name   string
	Fields []string // field names in declaration order; nil for zero-field variants
}

// EnumDecl declares a named enum type with one or more variants.
// Each variant may carry zero or more named fields.
type EnumDecl struct {
	Pos
	Name     string
	Variants []VariantDecl
}

func (e *EnumDecl) TokenLiteral() string { return "enum" }

// EnumPattern is a destructuring case arm: case Shape.Circle(r) { }
// Pattern is the variant selector; Bindings are new local names bound
// positionally to the variant's fields when the match succeeds.
type EnumPattern struct {
	Pos
	Pattern  Node
	Bindings []string
}

func (e *EnumPattern) TokenLiteral() string { return "pattern" }

// SelectCaseKind distinguishes the three kinds of select case.
type SelectCaseKind uint8

const (
	SelectRecv    SelectCaseKind = iota // case val, ok = recv(ch) { }
	SelectSend                          // case send(ch, val) { }
	SelectDefault                       // default { }
)

// SelectCase is one arm of a select statement.
// Kind determines which fields are relevant:
//   SelectRecv:    Chan is the source channel; Vars are the binding names ([val] or [val, ok])
//   SelectSend:    Chan is the target channel; SendVal is the value to send
//   SelectDefault: Body only; no blocking
type SelectCase struct {
	Pos
	Kind    SelectCaseKind
	Chan    Node     // channel expression (recv and send)
	SendVal Node     // value expression (send only)
	Vars    []string // bound variable names (recv only; 0, 1, or 2 elements)
	Body    []Node
}

// PipeExpr is the pipeline operator: left |> right
// The left value is passed as the first argument to the right-hand callable.
// If the right side is a CallExpr, the left value is prepended to its argument list.
// If the right side is a bare function reference, it is called with the left value only.
//   "hello world" |> split(" ")         →  split("hello world", " ")
//   [1,2,3]       |> map(fn(x){x*2})    →  map([1,2,3], fn(x){x*2})
//   "  hi  "      |> trim               →  trim("  hi  ")
type PipeExpr struct {
	Pos
	Left  Node
	Right Node // CallExpr (extra args) or any expression (no extra args)
}

func (p *PipeExpr) TokenLiteral() string { return "|>" }

// SelectStmt blocks until one of its channel cases can proceed, then runs
// that case's body. If multiple cases are ready simultaneously, one is chosen
// at random (Go semantics). An optional default case makes the select
// non-blocking — if no channel is immediately ready, default runs instead.
//
// Syntax:
//   select {
//       case val, ok = recv(ch1) { }
//       case send(ch2, expr)     { }
//       default                  { }
//   }
type SelectStmt struct {
	Pos
	Cases []SelectCase
}

func (s *SelectStmt) TokenLiteral() string { return "select" }
