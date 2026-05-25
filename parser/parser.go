package parser

// parser.go turns a stream of tokens (from the lexer) into an AST.
//
// kLex uses a PRATT PARSER (also called a "top-down operator precedence" parser).
// The key idea: instead of writing separate grammar rules for every operator
// precedence level (like classical recursive-descent), each token type carries
// a precedence number. The parser uses those numbers to decide how tightly
// operators bind to their operands.
//
// This makes adding new operators trivial: just add an entry to the
// `precedences` map and a case in the primary/infix parse functions.
//
// TOKEN CONTRACT — very important:
// Every parse function is entered with curToken pointing at the FIRST token
// of the thing it is parsing, and exits with curToken pointing at the LAST
// token. The caller is responsible for advancing past that last token.
// parseBody() is the one place that advances between statements.
// Adding p.nextToken() calls at statement boundaries elsewhere breaks this
// contract and causes tokens to be skipped or double-counted.

import (
	"fmt"
	"klex/ast"
	"klex/lexer"
	"strconv"
	"strings"
)

type Parser struct {
	l *lexer.Lexer

	// The parser maintains a two-token window: the current token and one
	// lookahead (peekToken). This is enough to make all parsing decisions
	// without backtracking.
	curToken  lexer.Token
	peekToken lexer.Token

	program *ast.Program

	// noStructLit suppresses struct literal parsing inside conditions and
	// collection expressions (if/while/for/switch bodies). Without this flag,
	// `if x != handler {` would parse `handler {` as a struct literal.
	noStructLit bool
}

func (p *Parser) curPos() ast.Pos {
	return ast.Pos{Line: p.curToken.Line, Col: p.curToken.Col}
}

// addError records a parse error with position info. Errors are stored on
// the program rather than panicking so the parser can continue and report
// multiple errors in a single pass.
func (p *Parser) addError(msg string) {
	pos := p.curPos()
	p.program.Errors = append(p.program.Errors,
		fmt.Sprintf("%d:%d: %s", pos.Line, pos.Col, msg))
}

// New creates a parser and primes the two-token window by calling nextToken twice.
// After New(), curToken = tokens[0] and peekToken = tokens[1].
func New(l *lexer.Lexer) *Parser {
	p := &Parser{l: l}
	p.nextToken()
	p.nextToken()
	return p
}

// -------------------- TOKEN HELPERS --------------------

// nextToken slides the window forward: curToken becomes the old peekToken,
// and peekToken is fetched fresh from the lexer.
func (p *Parser) nextToken() {
	p.curToken = p.peekToken
	p.peekToken = p.l.NextToken()
}

// expectPeek advances only if the next token matches the expected type.
// Used to assert required syntax elements (e.g. the '{' after a while condition).
// Returns false (and lets the caller add an error) if the token doesn't match.
func (p *Parser) expectPeek(t lexer.TokenType) bool {
	if p.peekToken.Type == t {
		p.nextToken()
		return true
	}
	return false
}

// -------------------- PROGRAM --------------------

func (p *Parser) ParseProgram() *ast.Program {
	p.program = &ast.Program{}
	p.program.Statements = p.parseBody(lexer.TokenEOF)
	return p.program
}

// parseBody is the SINGLE canonical place that advances between statements.
// It loops until it sees the terminator token ('}' for blocks, EOF for the
// top level), parsing one statement at a time and calling nextToken() to move
// past each statement's last token before starting the next.
//
// Do NOT add nextToken() calls at statement boundaries anywhere else —
// all advancement between statements lives here.
func (p *Parser) parseBody(terminator lexer.TokenType) []ast.Node {
	var nodes []ast.Node
	for p.curToken.Type != terminator && p.curToken.Type != lexer.TokenEOF {
		stmt := p.parseStatement()
		if stmt != nil {
			nodes = append(nodes, stmt)
		}
		p.nextToken() // advance past the last token of the statement just parsed
	}
	return nodes
}

// -------------------- STATEMENTS --------------------

func (p *Parser) parseIf() ast.Node {
	pos := p.curPos()
	p.nextToken() // move to condition

	p.noStructLit = true
	condition := p.parseExpression(LOWEST)
	p.noStructLit = false

	if !p.expectPeek(lexer.TokenLBrace) {
		return nil
	}
	p.nextToken()
	body := p.parseBody(lexer.TokenRBrace)

	var elseBody []ast.Node
	if p.peekToken.Type == lexer.TokenElse {
		p.nextToken() // consume 'else'
		if p.peekToken.Type == lexer.TokenIf {
			p.nextToken() // move to 'if'
			elseBody = []ast.Node{p.parseIf()}
		} else {
			if !p.expectPeek(lexer.TokenLBrace) {
				return nil
			}
			p.nextToken()
			elseBody = p.parseBody(lexer.TokenRBrace)
		}
	}

	return &ast.IfStmt{Pos: pos, Condition: condition, Body: body, ElseBody: elseBody}
}

func (p *Parser) parseReturn() ast.Node {
	pos := p.curPos()
	if p.peekToken.Type == lexer.TokenRBrace || p.peekToken.Type == lexer.TokenEOF {
		return &ast.ReturnStmt{Pos: pos, Value: &ast.NullLiteral{Pos: pos}}
	}
	p.nextToken()
	first := p.parseExpression(LOWEST)

	// Single return value — the common case.
	if p.peekToken.Type != lexer.TokenComma {
		return &ast.ReturnStmt{Pos: pos, Value: first}
	}

	// Multiple return values: collect all comma-separated expressions.
	elements := []ast.Node{first}
	for p.peekToken.Type == lexer.TokenComma {
		p.nextToken() // consume ','
		p.nextToken() // move to next expression
		elements = append(elements, p.parseExpression(LOWEST))
	}
	return &ast.ReturnStmt{Pos: pos, Value: &ast.TupleLiteral{Pos: pos, Elements: elements}}
}

// parseMultiAssign handles: name, name, ... = expr
// Called when we see IDENT followed by ',' at statement level.
func (p *Parser) parseMultiAssign() ast.Node {
	pos := p.curPos()
	names := []string{p.curToken.Literal}

	for p.peekToken.Type == lexer.TokenComma {
		p.nextToken() // consume ','
		if p.peekToken.Type != lexer.TokenIdent {
			p.addError("expected variable name in multi-assignment — e.g. `a, b = f()`")
			return nil
		}
		p.nextToken() // move to ident
		names = append(names, p.curToken.Literal)
	}

	if !p.expectPeek(lexer.TokenAssign) {
		p.addError("expected '=' in multi-assignment — e.g. `a, b = f()` not `a, b f()`")
		return nil
	}
	p.nextToken() // move to value expression
	value := p.parseExpression(LOWEST)

	return &ast.MultiAssignStmt{Pos: pos, Names: names, Value: value}
}

// parseFor parses: for <ident> in <expr> { <body> }
//              or: for <ident>, <ident> in <expr> { <body> }
func (p *Parser) parseFor() ast.Node {
	pos := p.curPos()

	if !p.expectPeek(lexer.TokenIdent) {
		p.addError("expected variable name after 'for' — e.g. `for x in arr {` or `for i, x in arr {`")
		return nil
	}
	varName := p.curToken.Literal

	// Optional second variable: for k, v in ...
	var valueVar string
	if p.peekToken.Type == lexer.TokenComma {
		p.nextToken() // consume comma
		if !p.expectPeek(lexer.TokenIdent) {
			p.addError("expected second variable name after ',' in for-in — e.g. `for i, x in arr {`")
			return nil
		}
		valueVar = p.curToken.Literal
	}

	if !p.expectPeek(lexer.TokenIn) {
		p.addError("expected 'in' after for-loop variable — e.g. `for x in arr {`")
		return nil
	}

	p.nextToken()
	p.noStructLit = true
	collection := p.parseExpression(LOWEST)
	p.noStructLit = false

	if !p.expectPeek(lexer.TokenLBrace) {
		p.addError("expected '{' to start for-in body — e.g. `for x in arr { ... }`")
		return nil
	}
	p.nextToken()
	body := p.parseBody(lexer.TokenRBrace)

	return &ast.ForInStmt{Pos: pos, Variable: varName, ValueVar: valueVar, Collection: collection, Body: body}
}

// asEnumPattern rewrites a CallExpr of the form Enum.Variant(a, b) — where the
// function is a DotExpr and every argument is a bare Ident — into an EnumPattern.
// Any other expression is returned unchanged.
func asEnumPattern(expr ast.Node) ast.Node {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return expr
	}
	// Full form:  case Shape.Circle(r)  — function is a DotExpr
	// Short form: case Circle(r)        — function is a bare Ident
	switch call.Function.(type) {
	case *ast.DotExpr, *ast.Ident:
		// valid pattern target — fall through
	default:
		return expr
	}
	bindings := make([]string, len(call.Args))
	for i, arg := range call.Args {
		id, ok := arg.(*ast.Ident)
		if !ok {
			return expr
		}
		bindings[i] = id.Value
	}
	return &ast.EnumPattern{Pos: call.Pos, Pattern: call.Function, Bindings: bindings}
}

// parseSwitch handles both forms:
//
//	switch expr  { case val, val { } default { } }   — value switch
//	switch       { case bool_expr { } default { } }  — expression switch
func (p *Parser) parseSwitch() ast.Node {
	pos := p.curPos()
	node := &ast.SwitchStmt{Pos: pos}

	// If the next token is '{', this is an expression switch (no subject).
	// Otherwise parse the subject expression.
	if p.peekToken.Type != lexer.TokenLBrace {
		p.nextToken()
		p.noStructLit = true
		node.Subject = p.parseExpression(LOWEST)
		p.noStructLit = false
	}

	if !p.expectPeek(lexer.TokenLBrace) {
		return nil
	}
	p.nextToken() // move past '{'

	for p.curToken.Type != lexer.TokenRBrace && p.curToken.Type != lexer.TokenEOF {
		if p.curToken.Type == lexer.TokenDefault {
			if !p.expectPeek(lexer.TokenLBrace) {
				return nil
			}
			p.nextToken()
			node.Default = p.parseBody(lexer.TokenRBrace)
			node.HasDefault = true
			p.nextToken() // move past '}'
			continue
		}

		if p.curToken.Type == lexer.TokenCase {
			p.nextToken() // move to first value
			var sc ast.SwitchCase
			// Collect one or more comma-separated match values / expressions.
			// noStructLit: prevent `case SomeIdent {` from being read as a struct literal.
			p.noStructLit = true
			sc.Values = append(sc.Values, asEnumPattern(p.parseExpression(LOWEST)))
			for p.peekToken.Type == lexer.TokenComma {
				p.nextToken() // consume ','
				p.nextToken() // move to next value
				sc.Values = append(sc.Values, asEnumPattern(p.parseExpression(LOWEST)))
			}
			p.noStructLit = false
			if !p.expectPeek(lexer.TokenLBrace) {
				return nil
			}
			p.nextToken()
			sc.Body = p.parseBody(lexer.TokenRBrace)
			node.Cases = append(node.Cases, sc)
			p.nextToken() // move past '}'
			continue
		}

		p.nextToken()
	}

	return node
}

// parseSelect handles:
//
//	select {
//	    case val, ok = recv(ch) { }   — recv with two bindings
//	    case val    = recv(ch) { }    — recv with one binding
//	    case          recv(ch) { }    — recv, discard value
//	    case send(ch, expr)    { }    — send
//	    default                { }    — non-blocking fallback
//	}
//
// curToken is TokenSelect on entry; exits with curToken at the outer '}'.
func (p *Parser) parseSelect() ast.Node {
	pos := p.curPos()
	node := &ast.SelectStmt{Pos: pos}

	if !p.expectPeek(lexer.TokenLBrace) {
		p.addError("expected '{' after `select` — e.g. `select { case x = recv(ch) { ... } default { ... } }`")
		return nil
	}
	p.nextToken() // move past '{'

	for p.curToken.Type != lexer.TokenRBrace && p.curToken.Type != lexer.TokenEOF {
		if p.curToken.Type == lexer.TokenDefault {
			if !p.expectPeek(lexer.TokenLBrace) {
				return nil
			}
			p.nextToken()
			body := p.parseBody(lexer.TokenRBrace)
			node.Cases = append(node.Cases, ast.SelectCase{
				Kind: ast.SelectDefault,
				Body: body,
			})
			p.nextToken() // move past '}'
			continue
		}

		if p.curToken.Type == lexer.TokenCase {
			sc := p.parseSelectCase()
			if sc == nil {
				return nil
			}
			node.Cases = append(node.Cases, *sc)
			p.nextToken() // move past case body '}'
			continue
		}

		p.addError("expected 'case' or 'default' in select — e.g. `case x = recv(ch) { ... }` or `default { ... }`, got '" + p.curToken.Literal + "'")
		return nil
	}

	return node
}

// parseSelectCase parses one case arm of a select statement.
// curToken is TokenCase on entry; exits with curToken at the case body '}'.
func (p *Parser) parseSelectCase() *ast.SelectCase {
	casePos := p.curPos()
	p.nextToken() // move past 'case'

	// ---- send case: send(ch, val) { } ----
	if p.curToken.Type == lexer.TokenIdent && p.curToken.Literal == "send" &&
		p.peekToken.Type == lexer.TokenLParen {
		p.nextToken() // move past 'send' to '('
		p.nextToken() // move to channel expression
		chanExpr := p.parseExpression(LOWEST)
		if !p.expectPeek(lexer.TokenComma) {
			p.addError("expected ',' between channel and value in select send case — e.g. `case send(ch, value) { ... }`")
			return nil
		}
		p.nextToken() // move to value expression
		valExpr := p.parseExpression(LOWEST)
		if !p.expectPeek(lexer.TokenRParen) {
			p.addError("expected ')' after select send case arguments — e.g. `case send(ch, value) { ... }`")
			return nil
		}
		if !p.expectPeek(lexer.TokenLBrace) {
			return nil
		}
		p.nextToken()
		body := p.parseBody(lexer.TokenRBrace)
		return &ast.SelectCase{Pos: casePos, Kind: ast.SelectSend, Chan: chanExpr, SendVal: valExpr, Body: body}
	}

	// ---- recv, no binding: recv(ch) { } ----
	if p.curToken.Type == lexer.TokenIdent && p.curToken.Literal == "recv" &&
		p.peekToken.Type == lexer.TokenLParen {
		chanExpr, ok := p.parseRecvArgs()
		if !ok {
			return nil
		}
		if !p.expectPeek(lexer.TokenLBrace) {
			return nil
		}
		p.nextToken()
		body := p.parseBody(lexer.TokenRBrace)
		return &ast.SelectCase{Pos: casePos, Kind: ast.SelectRecv, Chan: chanExpr, Body: body}
	}

	// ---- recv with one binding: val = recv(ch) { } ----
	if p.curToken.Type == lexer.TokenIdent && p.peekToken.Type == lexer.TokenAssign {
		var1 := p.curToken.Literal
		p.nextToken() // move past var name to '='
		p.nextToken() // move past '=' — must be 'recv'
		if p.curToken.Literal != "recv" || p.peekToken.Type != lexer.TokenLParen {
			p.addError("expected recv(...) on right-hand side of select case binding — e.g. `case x = recv(ch) { ... }` or `case val, ok = recv(ch) { ... }`")
			return nil
		}
		chanExpr, ok := p.parseRecvArgs()
		if !ok {
			return nil
		}
		if !p.expectPeek(lexer.TokenLBrace) {
			return nil
		}
		p.nextToken()
		body := p.parseBody(lexer.TokenRBrace)
		return &ast.SelectCase{Pos: casePos, Kind: ast.SelectRecv, Chan: chanExpr, Vars: []string{var1}, Body: body}
	}

	// ---- recv with two bindings: val, ok = recv(ch) { } ----
	if p.curToken.Type == lexer.TokenIdent && p.peekToken.Type == lexer.TokenComma {
		var1 := p.curToken.Literal
		p.nextToken() // move past var1 to ','
		p.nextToken() // move past ',' to var2
		if p.curToken.Type != lexer.TokenIdent {
			p.addError("expected second variable name in select recv case — e.g. `case val, ok = recv(ch) { ... }` where ok is true if the channel is still open")
			return nil
		}
		var2 := p.curToken.Literal
		p.nextToken() // move past var2 to '='
		if p.curToken.Type != lexer.TokenAssign {
			p.addError("expected '=' after variable names in select recv case — e.g. `case val, ok = recv(ch) { ... }`")
			return nil
		}
		p.nextToken() // move past '=' — must be 'recv'
		if p.curToken.Literal != "recv" || p.peekToken.Type != lexer.TokenLParen {
			p.addError("expected recv(...) on right-hand side of select case binding — e.g. `case x = recv(ch) { ... }` or `case val, ok = recv(ch) { ... }`")
			return nil
		}
		chanExpr, ok := p.parseRecvArgs()
		if !ok {
			return nil
		}
		if !p.expectPeek(lexer.TokenLBrace) {
			return nil
		}
		p.nextToken()
		body := p.parseBody(lexer.TokenRBrace)
		return &ast.SelectCase{Pos: casePos, Kind: ast.SelectRecv, Chan: chanExpr, Vars: []string{var1, var2}, Body: body}
	}

	p.addError("unrecognised select case — expected recv(ch) { … }, send(ch, val) { … }, x = recv(ch) { … }, or val, ok = recv(ch) { … }")
	return nil
}

// parseRecvArgs parses the (ch) part of recv(ch).
// curToken is 'recv' on entry; exits with curToken at ')'.
func (p *Parser) parseRecvArgs() (ast.Node, bool) {
	p.nextToken() // move past 'recv' to '('
	p.nextToken() // move to channel expression
	chanExpr := p.parseExpression(LOWEST)
	if !p.expectPeek(lexer.TokenRParen) {
		p.addError("expected ')' after channel in select recv case — e.g. `case x = recv(ch) { ... }`, single channel argument only")
		return nil, false
	}
	return chanExpr, true
}

func (p *Parser) parseWhile() ast.Node {
	pos := p.curPos()
	p.nextToken()

	p.noStructLit = true
	condition := p.parseExpression(LOWEST)
	p.noStructLit = false

	if !p.expectPeek(lexer.TokenLBrace) {
		return nil
	}
	p.nextToken()
	body := p.parseBody(lexer.TokenRBrace)

	return &ast.WhileStmt{Pos: pos, Condition: condition, Body: body}
}

// parseStatement dispatches to the appropriate parse function based on the
// current token. If no keyword matches, it falls through to expression parsing.
func (p *Parser) parseStatement() ast.Node {
	if p.curToken.Type == lexer.TokenIf {
		return p.parseIf()
	}

	// Guard against assigning to reserved literals like `null = 3`.
	// We still consume the full assignment so the parser can continue
	// and report other errors in the same file.
	if p.peekToken.Type == lexer.TokenAssign {
		switch p.curToken.Type {
		case lexer.TokenNull, lexer.TokenTrue, lexer.TokenFalse:
			p.addError("cannot assign to reserved literal: " + p.curToken.Literal)
			p.nextToken() // move to '='
			p.nextToken() // move to start of RHS
			p.parseExpression(LOWEST) // consume full RHS, leaves curToken at its last token
			return nil
		}
	}

	// Multi-assign: name, name = expr
	if p.curToken.Type == lexer.TokenIdent && p.peekToken.Type == lexer.TokenComma {
		return p.parseMultiAssign()
	}

	if p.curToken.Type == lexer.TokenIdent && p.peekToken.Type == lexer.TokenAssign {
		return p.parseAssign()
	}

	// Named function: `fn foo(x) { }` is syntactic sugar for `foo = fn(x) { }`
	if p.curToken.Type == lexer.TokenFn && p.peekToken.Type == lexer.TokenIdent {
		return p.parseFunctionLiteral()
	}

	if p.curToken.Type == lexer.TokenReturn {
		return p.parseReturn()
	}

	if p.curToken.Type == lexer.TokenImport {
		return p.parseImport()
	}

	if p.curToken.Type == lexer.TokenFor {
		return p.parseFor()
	}

	if p.curToken.Type == lexer.TokenWhile {
		return p.parseWhile()
	}

	if p.curToken.Type == lexer.TokenSwitch {
		return p.parseSwitch()
	}

	if p.curToken.Type == lexer.TokenSelect {
		return p.parseSelect()
	}

	if p.curToken.Type == lexer.TokenBreak {
		return &ast.BreakStmt{Pos: p.curPos()}
	}

	if p.curToken.Type == lexer.TokenContinue {
		return &ast.ContinueStmt{Pos: p.curPos()}
	}

	if p.curToken.Type == lexer.TokenStruct {
		return p.parseStruct()
	}

	if p.curToken.Type == lexer.TokenEnum {
		return p.parseEnum()
	}

	if p.curToken.Type == lexer.TokenLet {
		return p.parseLet()
	}

	if p.curToken.Type == lexer.TokenConst {
		return p.parseConst()
	}

	// Default: parse as an expression statement.
	// After parsing, check if the expression is an index expression followed
	// by '=' — if so, it's an indexed assignment (m["key"] = val).
	expr := p.parseExpression(LOWEST)

	// Detect compound assignment operators (+=, -=, *=, /=, %=).
	// These are not valid in kLex — emit a clear, actionable error and
	// consume the RHS to prevent cascading parse errors.
	switch p.peekToken.Type {
	case lexer.TokenPlusAssign, lexer.TokenMinusAssign,
		lexer.TokenMulAssign, lexer.TokenDivAssign, lexer.TokenModAssign:
		op := p.peekToken.Literal
		base := string(op[0])
		varName := "x"
		if id, ok := expr.(*ast.Ident); ok {
			varName = id.Value
		}
		p.nextToken() // consume the compound operator
		p.nextToken() // move to the RHS
		p.parseExpression(LOWEST) // consume RHS — prevents cascading errors
		p.addError(fmt.Sprintf(
			"operator %s is not valid in kLex — use %s = %s %s <value>",
			op, varName, varName, base,
		))
		return nil
	}

	if p.peekToken.Type == lexer.TokenAssign {
		if indexExpr, ok := expr.(*ast.IndexExpr); ok {
			p.nextToken() // consume '='
			p.nextToken() // move to value
			val := p.parseExpression(LOWEST)
			return &ast.IndexAssignStmt{Pos: indexExpr.Pos, Left: indexExpr, Value: val}
		}
		if dotExpr, ok := expr.(*ast.DotExpr); ok {
			p.nextToken() // consume '='
			p.nextToken() // move to value
			val := p.parseExpression(LOWEST)
			return &ast.DotAssignStmt{Pos: dotExpr.Pos, Left: dotExpr, Value: val}
		}
	}
	return expr
}

// -------------------- ASSIGNMENT --------------------

func (p *Parser) parseAssign() ast.Node {
	pos := p.curPos()
	name := p.curToken.Literal
	p.nextToken() // move to '='
	p.nextToken() // move to first token of the value expression
	value := p.parseExpression(LOWEST)
	return &ast.AssignStmt{Pos: pos, Name: name, Value: value}
}

// parseLet handles `let name = expr` — an explicit local-scope declaration —
// and `let a, b, ... = expr` — multiple declarations from a tuple RHS.
// curToken is TokenLet on entry; exits with curToken at last token of expr.
func (p *Parser) parseLet() ast.Node {
	pos := p.curPos()
	p.nextToken() // move to identifier
	if p.curToken.Type != lexer.TokenIdent {
		p.addError("expected identifier after 'let' — e.g. `let x = 0` — got " + p.curToken.Literal)
		return nil
	}
	firstName := p.curToken.Literal

	// Multi-let: `let a, b, ... = expr`
	if p.peekToken.Type == lexer.TokenComma {
		names := []string{firstName}
		for p.peekToken.Type == lexer.TokenComma {
			p.nextToken() // consume ','
			if p.peekToken.Type != lexer.TokenIdent {
				p.addError("expected variable name after ',' in `let " + strings.Join(names, ", ") + ", …` — e.g. `let a, b = f()`")
				return nil
			}
			p.nextToken() // move to ident
			names = append(names, p.curToken.Literal)
		}
		if !p.expectPeek(lexer.TokenAssign) {
			p.addError("expected '=' in `let " + strings.Join(names, ", ") + " = …`")
			return nil
		}
		p.nextToken() // move to first token of value expression
		value := p.parseExpression(LOWEST)
		return &ast.MultiLetStmt{Pos: pos, Names: names, Value: value}
	}

	if p.peekToken.Type != lexer.TokenAssign {
		p.addError("expected '=' after 'let " + firstName + "' — e.g. `let " + firstName + " = 0`")
		return nil
	}
	p.nextToken() // move to '='
	p.nextToken() // move to first token of the value expression
	value := p.parseExpression(LOWEST)
	return &ast.LetStmt{Pos: pos, Name: firstName, Value: value}
}

// parseConst handles `const name = expr` — an immutable binding in the current scope.
// curToken is TokenConst on entry; exits with curToken at last token of expr.
func (p *Parser) parseConst() ast.Node {
	pos := p.curPos()
	p.nextToken() // move to identifier
	if p.curToken.Type != lexer.TokenIdent {
		p.addError("expected identifier after 'const' — e.g. `const PI = 3.14` — got " + p.curToken.Literal)
		return nil
	}
	name := p.curToken.Literal
	if p.peekToken.Type != lexer.TokenAssign {
		p.addError("expected '=' after 'const " + name + "' — e.g. `const " + name + " = 3.14`")
		return nil
	}
	p.nextToken() // move to '='
	p.nextToken() // move to first token of the value expression
	value := p.parseExpression(LOWEST)
	return &ast.ConstStmt{Pos: pos, Name: name, Value: value}
}

// -------------------- EXPRESSIONS (PRATT PARSER) --------------------

// Precedence levels — higher number = binds tighter.
// Example: 1 + 2 * 3 parses as 1 + (2 * 3) because PRODUCT > SUM.
const (
	_ int = iota
	LOWEST
	PIPE     // |>  (lowest infix — whole expression on left is piped)
	LOGICAL  // && ||
	COMPARE  // == != < > <= >=
	SUM      // + -
	PRODUCT  // * / %
	PREFIX   // -x  !x
	CALL     // foo(args)
	INDEX    // arr[i]  map[k]  obj.prop
)

// precedences maps token types to their precedence level.
// Only infix/postfix operators appear here — prefix operators are handled
// separately in parsePrimary via parsePrefixExpression.
var precedences = map[lexer.TokenType]int{
	lexer.TokenPipe:     PIPE,
	lexer.TokenOr:       LOGICAL,
	lexer.TokenAnd:      LOGICAL,
	lexer.TokenEQ:       COMPARE,
	lexer.TokenNotEq:    COMPARE,
	lexer.TokenGT:       COMPARE,
	lexer.TokenLT:       COMPARE,
	lexer.TokenGTE:      COMPARE,
	lexer.TokenLTE:      COMPARE,
	lexer.TokenPlus:     SUM,
	lexer.TokenMinus:    SUM,
	lexer.TokenAsterisk: PRODUCT,
	lexer.TokenSlash:    PRODUCT,
	lexer.TokenPercent:  PRODUCT,
	lexer.TokenLParen:    CALL,  // foo(args) — '(' as infix means "call the left side"
	lexer.TokenLBracket:  INDEX, // arr[i] — '[' is treated as an infix operator
	lexer.TokenDot:       INDEX, // obj.prop — '.' binds as tightly as '['
	lexer.TokenQuestion:  INDEX, // expr?   — postfix unwrap binds as tightly as indexing
}

// parseExpression is the heart of the Pratt parser.
//
// It starts by parsing a "primary" (a literal, identifier, or prefix expression),
// then repeatedly checks if the next token has higher precedence than the
// current context. If it does, the current result becomes the LEFT side of an
// infix expression, and we keep building up the tree.
//
// The `precedence` argument is the binding power of the CALLING context.
// The loop continues as long as the next operator binds tighter than the caller.
// This is what makes operator precedence work without explicit grammar rules.
func (p *Parser) parseExpression(precedence int) ast.Node {
	left := p.parsePrimary()

	for p.peekToken.Type != lexer.TokenEOF && precedence < p.peekPrecedence() {
		p.nextToken()
		switch p.curToken.Type {
		case lexer.TokenLBracket:
			left = p.parseIndexExpr(left)
		case lexer.TokenLParen:
			left = p.parseCallInfix(left)
		case lexer.TokenDot:
			left = p.parseDotExpr(left)
		case lexer.TokenPipe:
			left = p.parsePipeExpr(left)
		case lexer.TokenQuestion:
			left = p.parseUnwrapExpr(left)
		default:
			left = p.parseInfixExpression(left)
		}
	}

	return left
}

// parseHashLiteral parses a hash/dictionary literal: {"key": value, ...}
// It is called from parsePrimary when a '{' is seen in expression position.
func (p *Parser) parseHashLiteral() ast.Node {
	hash := &ast.HashLiteral{Pos: p.curPos()}
	p.nextToken() // move past '{'

	for p.curToken.Type != lexer.TokenRBrace && p.curToken.Type != lexer.TokenEOF {
		key := p.parseExpression(LOWEST)
		if key == nil {
			return nil
		}
		if !p.expectPeek(lexer.TokenColon) {
			p.addError("expected ':' after hash key — e.g. `{\"name\": value}`")
			return nil
		}
		p.nextToken() // move to value expression
		val := p.parseExpression(LOWEST)
		if val == nil {
			return nil
		}
		hash.Pairs = append(hash.Pairs, ast.HashPair{Key: key, Value: val})
		if p.peekToken.Type == lexer.TokenComma {
			p.nextToken() // move to ','
			p.nextToken() // move to next key
		} else {
			p.nextToken() // move to '}'
		}
	}

	return hash
}

func (p *Parser) parseArrayLiteral() ast.Node {
	arr := &ast.ArrayLiteral{Pos: p.curPos()}

	p.nextToken() // move past '['

	for p.curToken.Type != lexer.TokenRBracket && p.curToken.Type != lexer.TokenEOF {
		el := p.parseExpression(LOWEST)
		if el != nil {
			arr.Elements = append(arr.Elements, el)
		}
		if p.peekToken.Type == lexer.TokenComma {
			p.nextToken()
			p.nextToken()
		} else {
			p.nextToken()
		}
	}

	return arr
}

// parseIndexExpr parses the [index] part of expr[index].
// `left` is the already-parsed expression being indexed.
func (p *Parser) parseIndexExpr(left ast.Node) ast.Node {
	pos := p.curPos()
	p.nextToken() // move past '['
	index := p.parseExpression(LOWEST)
	if !p.expectPeek(lexer.TokenRBracket) {
		return nil
	}
	return &ast.IndexExpr{Pos: pos, Left: left, Index: index}
}

func (p *Parser) parsePrefixExpression() ast.Node {
	pos := p.curPos()
	op := p.curToken.Literal
	p.nextToken()
	// Parse the operand at PREFIX precedence so that !-x works correctly
	// (PREFIX is very high — almost nothing binds tighter).
	return &ast.PrefixExpr{Pos: pos, Operator: op, Right: p.parseExpression(PREFIX)}
}

// parseFunctionLiteral handles both named and anonymous functions.
// Named:     fn foo(x) { } → desugared to AssignStmt{Name:"foo", Value: FunctionLiteral}
// Anonymous: fn(x) { }    → just a FunctionLiteral
func (p *Parser) parseFunctionLiteral() ast.Node {
	if p.peekToken.Type == lexer.TokenIdent {
		pos := p.curPos()
		p.nextToken() // move to the function name
		name := p.curToken.Literal
		lit := p.parseFunctionBody()
		if lit == nil {
			return nil
		}
		return &ast.AssignStmt{Pos: pos, Name: name, Value: lit}
	}
	return p.parseFunctionBody()
}

func (p *Parser) parseFunctionBody() *ast.FunctionLiteral {
	fn := &ast.FunctionLiteral{Pos: p.curPos()}

	if !p.expectPeek(lexer.TokenLParen) {
		return nil
	}
	p.nextToken() // move past '('

	// Parse parameter names with optional type annotations.
	// Type-first annotation syntax: type name or type name = default
	// Variadic: type ...name
	// All-or-nothing rule: either all params annotated or none.
	seenDefault := false
	var annotatedCount int
	for p.curToken.Type != lexer.TokenRParen && p.curToken.Type != lexer.TokenEOF {
		if p.curToken.Type == lexer.TokenIdent {
			// Check for type-first annotation: if curToken is IDENT and peekToken is also IDENT,
			// then curToken is the type and peekToken is the param name.
			// Note: TokenEllipsis after a param name means variadic, not a type annotation.
			var paramType string
			paramName := p.curToken.Literal

			if p.peekToken.Type == lexer.TokenIdent {
				// Type annotation detected (next token is another identifier, so current is type name)
				paramType = p.curToken.Literal
				annotatedCount++
				p.nextToken() // advance to param name
				paramName = p.curToken.Literal
			}

			fn.Params = append(fn.Params, paramName)
			fn.ParamTypes = append(fn.ParamTypes, paramType)

			if p.peekToken.Type == lexer.TokenEllipsis {
				fn.Defaults = append(fn.Defaults, nil)
				p.nextToken() // consume '...'
				fn.Variadic = true
				p.nextToken() // should now be ')'
				break
			}

			if p.peekToken.Type == lexer.TokenAssign {
				// Default value: name = expr
				p.nextToken() // consume '='
				p.nextToken() // move to first token of default expr
				defExpr := p.parseExpression(LOWEST)
				fn.Defaults = append(fn.Defaults, defExpr)
				seenDefault = true
			} else {
				if seenDefault {
					p.addError("required parameter " + paramName + " cannot follow a parameter with a default value")
				}
				fn.Defaults = append(fn.Defaults, nil)
			}
		} else if p.curToken.Type != lexer.TokenComma {
			// Non-identifier, non-comma token in a parameter slot — almost
			// always a reserved keyword (case, if, while, …) being used as
			// a parameter name. Silently swallowing it would leave the
			// function with fewer parameters than the source declared,
			// producing confusing "expects 0 argument(s), got N" errors
			// at every call site instead of a parse error here at the
			// source of the mistake. See memory:
			//   feedback_klex_reserved_param_names.md
			p.addError("invalid parameter name '" + p.curToken.Literal +
				"': reserved keywords cannot be used as parameter names — rename it (e.g. 'tc', 'item', 'arg')")
			return nil
		}
		if p.peekToken.Type == lexer.TokenComma {
			p.nextToken()
			p.nextToken()
		} else {
			p.nextToken()
		}
	}

	// Enforce all-or-nothing: either all params annotated or none
	if annotatedCount > 0 && annotatedCount < len(fn.Params) {
		p.addError("all parameters must be annotated or none — cannot mix annotated and unannotated parameters")
		return nil
	}

	// Check for return type annotation: ) : type {
	if p.peekToken.Type == lexer.TokenColon {
		p.nextToken() // consume ':'
		if p.peekToken.Type != lexer.TokenIdent {
			p.addError("expected return-type name after ':' — e.g. `fn add(int a, int b) : int { ... }` (kLex uses type-first param syntax, not `name: type`)")
			return nil
		}
		p.nextToken() // move to type name
		fn.ReturnType = p.curToken.Literal
	}

	if !p.expectPeek(lexer.TokenLBrace) {
		return nil
	}
	p.nextToken() // move past '{'
	fn.Body = p.parseBody(lexer.TokenRBrace)

	return fn
}

// parsePrimary handles the "atom" of an expression — the leftmost, innermost
// part that cannot be broken down further by precedence rules.
// This includes literals, identifiers, prefix operators, and grouped expressions.
func (p *Parser) parsePrimary() ast.Node {
	switch p.curToken.Type {
	case lexer.TokenIllegal:
		p.addError(p.curToken.Literal)
		return nil

	case lexer.TokenLParen:
		// Grouped expression: (expr). Parse the inner expression, then
		// assert the closing ')'.
		p.nextToken()
		expr := p.parseExpression(LOWEST)
		if p.peekToken.Type != lexer.TokenRParen {
			return nil
		}
		p.nextToken()
		return expr

	case lexer.TokenFn:
		return p.parseFunctionLiteral()

	case lexer.TokenNull:
		return &ast.NullLiteral{Pos: p.curPos()}

	case lexer.TokenTrue:
		return &ast.BoolLiteral{Pos: p.curPos(), Value: true}

	case lexer.TokenFalse:
		return &ast.BoolLiteral{Pos: p.curPos(), Value: false}

	case lexer.TokenInt:
		// ParseInt with base 0 handles 0x (hex), 0b (binary), 0o (octal), and decimal.
		n, err := strconv.ParseInt(p.curToken.Literal, 0, 64)
		if err != nil {
			p.addError("integer literal " + p.curToken.Literal + " overflows int")
			return nil
		}
		return &ast.IntLiteral{Pos: p.curPos(), Value: int(n)}

	case lexer.TokenFloat:
		v, err := strconv.ParseFloat(p.curToken.Literal, 64)
		if err != nil {
			p.addError("invalid float literal: " + p.curToken.Literal)
			return nil
		}
		return &ast.FloatLiteral{Pos: p.curPos(), Value: v}

	case lexer.TokenStr, lexer.TokenRawStr:
		return &ast.StringLiteral{Pos: p.curPos(), Value: p.curToken.Literal}

	case lexer.TokenBytes:
		// Lexer packs the decoded bytes into Literal as a Go string. Convert
		// to []byte here so downstream code never has to remember that the
		// string carries raw bytes rather than utf-8 text.
		return &ast.BytesLiteral{Pos: p.curPos(), Value: []byte(p.curToken.Literal)}

	case lexer.TokenInterpStr:
		return p.parseInterpolatedString()

	case lexer.TokenLBracket:
		return p.parseArrayLiteral()

	case lexer.TokenLBrace:
		// '{' in expression position is always a hash literal.
		// In statement position (if/while/fn bodies), '{' is consumed by
		// expectPeek before parsePrimary is ever called, so there's no ambiguity.
		return p.parseHashLiteral()

	case lexer.TokenNot, lexer.TokenMinus:
		return p.parsePrefixExpression()

	case lexer.TokenIdent:
		ident := &ast.Ident{Pos: p.curPos(), Value: p.curToken.Literal}
		// Identifier immediately followed by '{' is a struct literal: Point { x: 1 }
		// Suppressed inside conditions/collections (if/while/for/switch) via noStructLit
		// so that `if x != handler {` does not parse `handler {` as a struct literal.
		if !p.noStructLit && p.peekToken.Type == lexer.TokenLBrace {
			name := p.curToken.Literal
			pos := p.curPos()
			p.nextToken() // move to '{'
			return p.parseStructLiteral(name, pos)
		}
		return ident
	}

	return nil
}

// parsePipeExpr handles the pipeline operator: left |> right
// curToken is |> on entry; exits with curToken at the last token of the right side.
// The right side is parsed at PIPE precedence so that chained pipes are
// left-associative: a |> f() |> g() parses as (a |> f()) |> g().
func (p *Parser) parsePipeExpr(left ast.Node) ast.Node {
	pos := p.curPos()
	p.nextToken() // move to the start of the right-hand expression
	right := p.parseExpression(PIPE)
	return &ast.PipeExpr{Pos: pos, Left: left, Right: right}
}

// parseUnwrapExpr handles the postfix ? operator: expr?
// curToken is '?' on entry. No right operand is consumed — ? is purely postfix.
// The operand must be a 2-element tuple (value, err) at runtime.
func (p *Parser) parseUnwrapExpr(left ast.Node) ast.Node {
	return &ast.UnwrapExpr{Pos: p.curPos(), Value: left}
}

// parseInfixExpression handles binary operators: left OP right.
// Called after the left side is already parsed and curToken is the operator.
func (p *Parser) parseInfixExpression(left ast.Node) ast.Node {
	pos := p.curPos()
	op := p.curToken.Literal
	precedence := p.curPrecedence()
	p.nextToken() // move to the right-hand side
	// We pass the CURRENT operator's precedence to parseExpression so that
	// higher-precedence operators further right bind first (left-associativity).
	return &ast.InfixExpr{Pos: pos, Left: left, Operator: op, Right: p.parseExpression(precedence)}
}

// -------------------- FUNCTION CALLS --------------------

// parseCallInfix handles a call when curToken is already '('.
// Used by the infix loop so that any expression can be called:
// foo(args), math.add(args), fn(x){x+1}(5) all route through here.
func (p *Parser) parseCallInfix(function ast.Node) ast.Node {
	pos := p.curPos()
	call := &ast.CallExpr{Pos: pos, Function: function}

	p.nextToken() // move past '(' to first arg or ')'

	for p.curToken.Type != lexer.TokenRParen && p.curToken.Type != lexer.TokenEOF {
		arg := p.parseExpression(LOWEST)
		if arg != nil {
			call.Args = append(call.Args, arg)
		}
		if p.peekToken.Type == lexer.TokenComma {
			p.nextToken()
			p.nextToken()
		} else {
			p.nextToken()
		}
	}

	return call
}

// parseDotExpr handles property access when curToken is '.'.
// The property name must be an identifier: math.add, obj.field
func (p *Parser) parseDotExpr(left ast.Node) ast.Node {
	pos := p.curPos()
	if p.peekToken.Type != lexer.TokenIdent {
		p.addError("expected property name after '.' — e.g. `point.x` or `module.functionName`")
		return nil
	}
	p.nextToken() // move to property name
	return &ast.DotExpr{Pos: pos, Left: left, Property: p.curToken.Literal}
}

// parseEnum handles: enum Name { Variant(field, field)  ZeroFieldVariant }
func (p *Parser) parseEnum() ast.Node {
	pos := p.curPos()

	if p.peekToken.Type != lexer.TokenIdent {
		p.addError("expected enum name after 'enum' — e.g. `enum Color { Red Green Blue }`")
		return nil
	}
	p.nextToken()
	name := p.curToken.Literal

	if !p.expectPeek(lexer.TokenLBrace) {
		p.addError("expected '{' after enum name — e.g. `enum Color { Red Green Blue }`")
		return nil
	}
	p.nextToken() // move past '{'

	decl := &ast.EnumDecl{Pos: pos, Name: name}

	for p.curToken.Type != lexer.TokenRBrace && p.curToken.Type != lexer.TokenEOF {
		if p.curToken.Type != lexer.TokenIdent {
			p.nextToken()
			continue
		}

		variant := ast.VariantDecl{Name: p.curToken.Literal}

		// Optional field list: Variant(field, field, ...)
		if p.peekToken.Type == lexer.TokenLParen {
			p.nextToken() // move to '('
			p.nextToken() // move past '(' to first field or ')'
			for p.curToken.Type != lexer.TokenRParen && p.curToken.Type != lexer.TokenEOF {
				if p.curToken.Type == lexer.TokenIdent {
					variant.Fields = append(variant.Fields, p.curToken.Literal)
				}
				if p.peekToken.Type == lexer.TokenComma {
					p.nextToken()
					p.nextToken()
				} else {
					p.nextToken()
				}
			}
			// curToken is now ')'
		}

		decl.Variants = append(decl.Variants, variant)
		p.nextToken() // advance past variant name or closing ')'
	}

	return decl
}

// parseStruct handles: struct Name { field, field  fn method(params) { body } }
// Fields are bare identifiers (comma or whitespace separated).
// Methods are named fn declarations inside the body.
func (p *Parser) parseStruct() ast.Node {
	pos := p.curPos()

	if p.peekToken.Type != lexer.TokenIdent {
		p.addError("expected struct name after 'struct' — e.g. `struct Point { x, y }`")
		return nil
	}
	p.nextToken()
	name := p.curToken.Literal

	if !p.expectPeek(lexer.TokenLBrace) {
		p.addError("expected '{' after struct name — e.g. `struct Point { x, y }`")
		return nil
	}
	p.nextToken() // move past '{'

	decl := &ast.StructDecl{Pos: pos, Name: name}

	for p.curToken.Type != lexer.TokenRBrace && p.curToken.Type != lexer.TokenEOF {
		// Method declaration inside a struct body.
		if p.curToken.Type == lexer.TokenFn {
			if p.peekToken.Type != lexer.TokenIdent {
				p.addError("expected method name after 'fn' in struct body — e.g. `struct Foo { x, fn bar() { ... } }`")
				return nil
			}
			p.nextToken() // move to method name
			mname := p.curToken.Literal
			mpos := p.curPos()
			lit := p.parseFunctionBody()
			if lit == nil {
				return nil
			}
			decl.Methods = append(decl.Methods, &ast.MethodDecl{
				Pos:        mpos,
				Name:       mname,
				Params:     lit.Params,
				ParamTypes: lit.ParamTypes,
				Defaults:   lit.Defaults,
				Variadic:   lit.Variadic,
				ReturnType: lit.ReturnType,
				Body:       lit.Body,
			})
			p.nextToken() // advance past the method's last token ('}')
			continue
		}

		// Field declaration: one or more comma-separated identifiers.
		if p.curToken.Type == lexer.TokenIdent {
			decl.Fields = append(decl.Fields, p.curToken.Literal)
			for p.peekToken.Type == lexer.TokenComma {
				p.nextToken() // consume ','
				if p.peekToken.Type != lexer.TokenIdent {
					p.addError("expected field name after ',' in struct definition — e.g. `struct Point { x, y, z }`")
					return nil
				}
				p.nextToken()
				decl.Fields = append(decl.Fields, p.curToken.Literal)
			}
			p.nextToken() // advance past the last field name
			continue
		}

		p.nextToken() // skip any unexpected token and keep going
	}

	return decl
}

// parseStructLiteral parses: Name { field: expr, field: expr }
// Called from parsePrimary when an identifier is immediately followed by '{'.
func (p *Parser) parseStructLiteral(name string, pos ast.Pos) ast.Node {
	lit := &ast.StructLiteral{Pos: pos, Name: name}
	p.nextToken() // move past '{'

	for p.curToken.Type != lexer.TokenRBrace && p.curToken.Type != lexer.TokenEOF {
		if p.curToken.Type != lexer.TokenIdent {
			p.addError("expected field name in struct literal — e.g. `Point { x: 10, y: 20 }`")
			return nil
		}
		fieldName := p.curToken.Literal
		if !p.expectPeek(lexer.TokenColon) {
			p.addError("expected ':' after field name in struct literal — e.g. `Point { x: 10, y: 20 }`")
			return nil
		}
		p.nextToken() // move to value expression
		val := p.parseExpression(LOWEST)
		if val == nil {
			return nil
		}
		lit.Fields = append(lit.Fields, ast.FieldInit{Name: fieldName, Value: val})

		if p.peekToken.Type == lexer.TokenComma {
			p.nextToken() // consume ','
			p.nextToken() // move to next field name
		} else {
			p.nextToken() // move to '}'
		}
	}

	return lit
}

// parseImport handles: import "file.lex" as name
func (p *Parser) parseImport() ast.Node {
	pos := p.curPos()

	if p.peekToken.Type != lexer.TokenStr {
		p.addError("expected quoted file path after 'import' — e.g. `import \"stdlib/fs.lex\" as fs`")
		return nil
	}
	p.nextToken()
	path := p.curToken.Literal

	if !p.expectPeek(lexer.TokenAs) {
		p.addError("expected `as` after import path — e.g. `import \"stdlib/fs.lex\" as fs`")
		return nil
	}
	if !p.expectPeek(lexer.TokenIdent) {
		p.addError("expected alias name after `as` — e.g. `import \"stdlib/fs.lex\" as fs`")
		return nil
	}
	alias := p.curToken.Literal

	return &ast.ImportStmt{Pos: pos, Path: path, Alias: alias}
}

// parseInterpolatedString consumes the structured segments the lexer built
// from the interpolated string token. Each text segment is already fully
// escape-decoded (including {{ / }} collapse and \xHH); each expression
// segment is already lexed into kLex tokens with accurate outer-source
// positions. The parser just feeds the tokens through ParseProgram via the
// replay-lexer constructor and unwraps the first statement.
//
// Before this rewrite the parser walked raw bytes from curToken.Literal,
// hand-rolling brace counting AND a parallel escape switch that had to be
// kept in lock-step with lexer.readString. Drift between the two surfaces
// caused several real bugs (\} only worked in plain strings, \" inside an
// expression triggered TokenIllegal in the inner lexer, etc.). All of that
// logic now lives in exactly one place: lexer.readString.
func (p *Parser) parseInterpolatedString() ast.Node {
	pos := p.curPos()
	tok := p.curToken

	segments := make([]ast.StringSegment, 0, len(tok.Segments))
	for _, seg := range tok.Segments {
		if !seg.IsExpr {
			segments = append(segments, ast.StringSegment{Text: seg.Text})
			continue
		}

		// Re-parse the pre-lexed expression tokens.
		innerProg := New(lexer.NewReplay(seg.Tokens)).ParseProgram()
		if len(innerProg.Statements) == 0 && len(innerProg.Errors) == 0 {
			p.program.Errors = append(p.program.Errors,
				fmt.Sprintf("%d:%d: empty expression in string interpolation — e.g. `\"x={value}\"`, not `\"x={}\"`",
					seg.Line, seg.Col))
			return nil
		}
		if len(innerProg.Errors) > 0 {
			// Inner errors already carry their outer-source line:col because the
			// lexer tagged each replay token with its real position. Surface the
			// inner message verbatim — it's already pointing at the right place.
			p.program.Errors = append(p.program.Errors,
				fmt.Sprintf("invalid expression in string interpolation: %s", innerProg.Errors[0]))
			return nil
		}
		segments = append(segments, ast.StringSegment{IsExpr: true, Expr: innerProg.Statements[0]})
	}

	return &ast.InterpolatedString{Pos: pos, Segments: segments}
}

// stripPosPrefix removes a leading "<line>:<col>: " prefix from an inner
// parser error message so the outer parseInterpolatedString can wrap it
// with the outer string's source position instead.
func stripPosPrefix(msg string) string {
	colon := -1
	for j := 0; j < len(msg); j++ {
		c := msg[j]
		if c >= '0' && c <= '9' {
			continue
		}
		if c == ':' && j > 0 {
			colon = j
			break
		}
		return msg
	}
	if colon < 0 || colon+1 >= len(msg) {
		return msg
	}
	// Past first colon — expect digits then second colon and a space.
	rest := msg[colon+1:]
	k := 0
	for k < len(rest) && rest[k] >= '0' && rest[k] <= '9' {
		k++
	}
	if k == 0 || k >= len(rest) || rest[k] != ':' {
		return msg
	}
	rest = rest[k+1:]
	for len(rest) > 0 && rest[0] == ' ' {
		rest = rest[1:]
	}
	return rest
}

// -------------------- PRECEDENCE HELPERS --------------------

func (p *Parser) peekPrecedence() int {
	if p, ok := precedences[p.peekToken.Type]; ok {
		return p
	}
	return LOWEST
}

func (p *Parser) curPrecedence() int {
	if p, ok := precedences[p.curToken.Type]; ok {
		return p
	}
	return LOWEST
}

// -------------------- UTIL --------------------

