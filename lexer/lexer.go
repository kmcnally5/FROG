package lexer

// byteStrings holds a pre-allocated single-character string for every possible
// byte value (0–255). Indexing this table at token time is allocation-free —
// the strings are created once at program startup and reused forever.
var byteStrings [256]string

func init() {
	for i := range byteStrings {
		byteStrings[i] = string([]byte{byte(i)})
	}
}

// The lexer (also called a "scanner" or "tokeniser") is the first stage of
// the interpreter pipeline. Its job is to read raw source text character by
// character and group those characters into meaningful units called TOKENS.
//
// Think of it like reading a sentence: before you can understand grammar
// (that's the parser's job), you first have to recognise the individual words.
// The lexer does that for kLex source code.
//
// Example:  x = 1 + 2
// Tokens:   IDENT("x")  ASSIGN  INT(1)  PLUS  INT(2)  EOF


// TokenType is just a string label that names what kind of token something is.
// Using a named string type (rather than an int enum) makes debug output
// human-readable without needing a lookup table.
type TokenType string

const (
	// Control tokens — not real language constructs, just signals.
	TokenIf      TokenType = "IF"
	TokenElse    TokenType = "ELSE"
	TokenEOF     TokenType = "EOF"     // end of file — tells the parser to stop
	TokenIllegal TokenType = "ILLEGAL" // unknown character — causes a parse error

	// Boolean and null literals are keywords, not identifiers.
	// The lexer handles them by checking every identifier against a keyword map.
	TokenTrue  TokenType = "TRUE"
	TokenFalse TokenType = "FALSE"
	TokenNull  TokenType = "NULL"

	// Value-carrying tokens — the lexer stores the raw text in Token.Literal.
	TokenIdent    TokenType = "IDENT"       // variable name, e.g. "foo"
	TokenInt      TokenType = "INT"         // integer literal, e.g. "42"
	TokenFloat    TokenType = "FLOAT"       // float literal, e.g. "3.14"
	TokenStr      TokenType = "STRING"      // plain string literal, e.g. "hello"
	TokenInterpStr TokenType = "INTERP_STR" // interpolated string, e.g. "Hello {name}"
	TokenRawStr   TokenType = "RAW_STR"    // backtick raw string, e.g. `hello\nworld`
	TokenBytes    TokenType = "BYTES"      // bytes literal, e.g. b"\x00\x01abc"

	// Statement keywords
	TokenFn     TokenType = "FN"
	TokenReturn TokenType = "RETURN"
	TokenWhile  TokenType = "WHILE"
	TokenBreak  TokenType = "BREAK"
	TokenFor    TokenType = "FOR"
	TokenIn     TokenType = "IN"
	TokenImport TokenType = "IMPORT"
	TokenAs     TokenType = "AS"
	TokenContinue TokenType = "CONTINUE"
	TokenSwitch   TokenType = "SWITCH"
	TokenCase     TokenType = "CASE"
	TokenDefault  TokenType = "DEFAULT"
	TokenStruct   TokenType = "STRUCT"
	TokenEnum     TokenType = "ENUM"
	TokenLet      TokenType = "LET"
	TokenConst    TokenType = "CONST"
	TokenSelect   TokenType = "SELECT"

	// Comparison operators — two-character tokens need special handling
	// because the lexer must peek at the next character to decide which token
	// to produce (e.g. '=' alone vs '==' together).
	TokenEQ    TokenType = "=="
	TokenNotEq TokenType = "!="
	TokenGTE   TokenType = ">="
	TokenLTE   TokenType = "<="

	// Arithmetic operators
	TokenPlus     TokenType = "+"
	TokenMinus    TokenType = "-"
	TokenAsterisk TokenType = "*"
	TokenSlash    TokenType = "/"
	TokenPercent  TokenType = "%"

	// Compound assignment — not valid in kLex; recognised so the parser
	// can emit a clear, actionable error rather than cascading garbage.
	TokenPlusAssign  TokenType = "+="
	TokenMinusAssign TokenType = "-="
	TokenMulAssign   TokenType = "*="
	TokenDivAssign   TokenType = "/="
	TokenModAssign   TokenType = "%="

	// Pipeline operator — pipes the left value as the first argument of the right call.
	TokenPipe TokenType = "|>"

	// Delimiters
	TokenLParen   TokenType = "("
	TokenRParen   TokenType = ")"
	TokenComma    TokenType = ","
	TokenLBrace   TokenType = "{"
	TokenRBrace   TokenType = "}"
	TokenLBracket TokenType = "["
	TokenRBracket TokenType = "]"
	TokenColon    TokenType = ":"
	TokenDot      TokenType = "."
	TokenEllipsis TokenType = "..."

	// Single-character operators
	TokenAssign   TokenType = "="
	TokenGT       TokenType = ">"
	TokenLT       TokenType = "<"
	TokenQuestion TokenType = "?" // postfix error-propagation: expr?

	// Logical operators
	TokenAnd TokenType = "&&"
	TokenOr  TokenType = "||"
	TokenNot TokenType = "!"
)

// keywords maps reserved words to their token types.
// Any identifier that matches an entry here becomes a keyword token instead.
// This is why you can't name a variable "if" or "while".
var keywords = map[string]TokenType{
	"if":       TokenIf,
	"else":     TokenElse,
	"true":     TokenTrue,
	"false":    TokenFalse,
	"null":     TokenNull,
	"fn":       TokenFn,
	"return":   TokenReturn,
	"while":    TokenWhile,
	"break":    TokenBreak,
	"for":      TokenFor,
	"in":       TokenIn,
	"import":   TokenImport,
	"as":       TokenAs,
	"continue": TokenContinue,
	"switch":   TokenSwitch,
	"case":     TokenCase,
	"default":  TokenDefault,
	"struct":   TokenStruct,
	"enum":     TokenEnum,
	"let":      TokenLet,
	"const":    TokenConst,
	"select":   TokenSelect,
}

// Token is the unit the lexer produces and the parser consumes.
// Every token carries its type, its raw text (Literal), and its source
// position (Line/Col) so that error messages can point to the right place.
//
// Segments is populated only for TokenInterpStr. The lexer fully decodes
// escape sequences in literal segments and pre-tokenizes embedded
// expressions, so the parser never has to walk raw bytes inside an
// interpolated string. Other token types leave Segments nil.
type Token struct {
	Type     TokenType
	Literal  string // the raw text from the source, e.g. "42" or "myVar"
	Line     int    // 1-based line number
	Col      int    // 1-based column number
	Segments []InterpSegment
}

// InterpSegment is one piece of an interpolated string, produced by the
// lexer. The parser walks these directly — no raw-byte re-scanning.
//
//   - IsExpr == false: Text holds a literal run with all \-escape sequences
//     decoded. Inside text segments, `{{` collapses to a single `{` and
//     `}}` to a single `}` so source can carry literal braces without
//     triggering interpolation.
//   - IsExpr == true: Tokens holds the kLex tokens for the embedded
//     expression, lexed in the original source so each token's Line/Col
//     points back to the user's file. The slice is terminated by a synthetic
//     TokenEOF so the parser can reuse its standard end-of-stream logic.
//
// Line and Col record where this segment begins in the outer source — used
// for precise error messages that reference the interpolation position
// rather than a meaningless inner offset.
type InterpSegment struct {
	IsExpr bool
	Text   string
	Tokens []Token
	Line   int
	Col    int
}

// Lexer holds all state needed to walk through the source string.
// It keeps track of three positions:
//   - position     — the character we are currently reading
//   - readPosition — the character we will read next (used for peeking)
//   - ch           — the actual byte at position
//
// Keeping a one-character lookahead (readPosition) lets us handle two-character
// tokens like ==, !=, <=, >= without backtracking.
type Lexer struct {
	input        string
	position     int  // current char index
	readPosition int  // next char index
	ch           byte // current char under inspection
	line         int  // current line (1-based)
	col          int  // current column (1-based)

	// Replay mode: when non-nil, NextToken yields pre-recorded tokens in
	// order instead of scanning input. This lets the parser re-use its
	// normal pipeline to parse expression segments that the outer lexer
	// already pulled out of an interpolated string literal — no second
	// trip through the source bytes, no duplicate escape-handling code.
	replay    []Token
	replayIdx int
}

// New creates a lexer and positions it at the first character of the input.
func New(input string) *Lexer {
	l := &Lexer{input: input, line: 1, col: 0}
	l.readChar() // prime the pump: sets ch to input[0]
	return l
}

// NewReplay returns a Lexer that yields tokens from the given slice instead
// of scanning input text. Used to re-parse the pre-lexed expression tokens
// captured inside an interpolated string token's Segments. The caller is
// expected to end the slice with a TokenEOF — the lexer keeps returning
// that EOF for any reads past the end (matching real lexer behaviour).
func NewReplay(tokens []Token) *Lexer {
	return &Lexer{replay: tokens, line: 1, col: 1}
}

// readChar advances the lexer by one character, updating position tracking.
// When we reach the end of input, ch is set to 0 (the null byte), which acts
// as our EOF sentinel. Once at EOF, further calls are idempotent: l.position
// saturates at len(l.input) so callers slicing l.input[start:l.position]
// after an unterminated literal cannot run off the end.
func (l *Lexer) readChar() {
	if l.readPosition > len(l.input) {
		// Already saturated past EOF — do nothing.
		return
	}
	if l.readPosition == len(l.input) {
		l.ch = 0
	} else {
		l.ch = l.input[l.readPosition]
	}
	l.position = l.readPosition
	l.readPosition++

	// Track line and column so tokens know where they appear in the source.
	if l.ch == '\n' {
		l.line++
		l.col = 0
	} else {
		l.col++
	}
}

// peekChar returns the next character without consuming it.
// This is the "one character lookahead" that lets us distinguish
// '=' (assign) from '==' (equals), '>' from '>=', etc.
func (l *Lexer) peekChar() byte {
	if l.readPosition >= len(l.input) {
		return 0
	}
	return l.input[l.readPosition]
}

func (l *Lexer) pos() (int, int) {
	return l.line, l.col
}

// NextToken is the core lexer method. The parser calls it repeatedly to get
// one token at a time. Each call:
//  1. Skips whitespace
//  2. Looks at the current character
//  3. Decides what kind of token it is
//  4. Advances past it and returns the token
//
// Identifiers and numbers are handled separately because they span multiple
// characters (e.g. "myVariable", "1024").
func (l *Lexer) NextToken() Token {
	// Replay mode shortcut: yield the next stored token. Past-the-end reads
	// repeat the trailing EOF so the parser's normal end-of-stream logic
	// works without special-casing.
	if l.replay != nil {
		if l.replayIdx >= len(l.replay) {
			return Token{Type: TokenEOF, Line: l.line, Col: l.col}
		}
		tok := l.replay[l.replayIdx]
		l.replayIdx++
		return tok
	}

	var tok Token

	l.skipWhitespace()

	line, col := l.pos()

	switch l.ch {

	case '+':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TokenPlusAssign, Literal: "+=", Line: line, Col: col}
		} else {
			tok = Token{Type: TokenPlus, Literal: byteStrings[l.ch], Line: line, Col: col}
		}
	case '-':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TokenMinusAssign, Literal: "-=", Line: line, Col: col}
		} else {
			tok = Token{Type: TokenMinus, Literal: byteStrings[l.ch], Line: line, Col: col}
		}
	case '*':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TokenMulAssign, Literal: "*=", Line: line, Col: col}
		} else {
			tok = Token{Type: TokenAsterisk, Literal: byteStrings[l.ch], Line: line, Col: col}
		}
	case '/':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TokenDivAssign, Literal: "/=", Line: line, Col: col}
		} else {
			tok = Token{Type: TokenSlash, Literal: byteStrings[l.ch], Line: line, Col: col}
		}
	case '%':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TokenModAssign, Literal: "%=", Line: line, Col: col}
		} else {
			tok = Token{Type: TokenPercent, Literal: byteStrings[l.ch], Line: line, Col: col}
		}
	case '(':
		tok = Token{Type: TokenLParen, Literal: byteStrings[l.ch], Line: line, Col: col}
	case ')':
		tok = Token{Type: TokenRParen, Literal: byteStrings[l.ch], Line: line, Col: col}
	case ',':
		tok = Token{Type: TokenComma, Literal: byteStrings[l.ch], Line: line, Col: col}
	case ':':
		tok = Token{Type: TokenColon, Literal: byteStrings[l.ch], Line: line, Col: col}
	case '.':
		if l.peekChar() == '.' && l.readPosition+1 < len(l.input) && l.input[l.readPosition+1] == '.' {
			l.readChar() // consume second '.'
			l.readChar() // consume third '.'
			tok = Token{Type: TokenEllipsis, Literal: "...", Line: line, Col: col}
		} else {
			tok = Token{Type: TokenDot, Literal: byteStrings[l.ch], Line: line, Col: col}
		}

	// '=' could be assignment (=) or equality (==) — peek to decide.
	case '=':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TokenEQ, Literal: "==", Line: line, Col: col}
		} else {
			tok = Token{Type: TokenAssign, Literal: "=", Line: line, Col: col}
		}
	case '{':
		tok = Token{Type: TokenLBrace, Literal: byteStrings[l.ch], Line: line, Col: col}
	case '}':
		tok = Token{Type: TokenRBrace, Literal: byteStrings[l.ch], Line: line, Col: col}
	case '[':
		tok = Token{Type: TokenLBracket, Literal: byteStrings[l.ch], Line: line, Col: col}
	case ']':
		tok = Token{Type: TokenRBracket, Literal: byteStrings[l.ch], Line: line, Col: col}

	// '>' could be greater-than or greater-than-or-equal.
	case '>':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TokenGTE, Literal: ">=", Line: line, Col: col}
		} else {
			tok = Token{Type: TokenGT, Literal: byteStrings[l.ch], Line: line, Col: col}
		}

	// '<' could be less-than or less-than-or-equal.
	case '<':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TokenLTE, Literal: "<=", Line: line, Col: col}
		} else {
			tok = Token{Type: TokenLT, Literal: byteStrings[l.ch], Line: line, Col: col}
		}

	case '"':
		content, segments, ok := l.readString()
		if !ok {
			// readString returns the error message in `content` when ok=false.
			// Empty content means a generic EOF — use the canonical message.
			msg := content
			if msg == "" {
				msg = "unterminated string literal"
			}
			tok = Token{Type: TokenIllegal, Literal: msg, Line: line, Col: col}
		} else if segments != nil {
			tok = Token{Type: TokenInterpStr, Line: line, Col: col, Segments: segments}
		} else {
			tok = Token{Type: TokenStr, Literal: content, Line: line, Col: col}
		}

	case '`':
		content := l.readRawString()
		if l.ch == 0 {
			tok = Token{Type: TokenIllegal, Literal: "unterminated raw string literal", Line: line, Col: col}
		} else {
			tok = Token{Type: TokenRawStr, Literal: content, Line: line, Col: col}
		}

	// 0 means we hit the end of the input string.
	case 0:
		tok = Token{Type: TokenEOF, Literal: "", Line: line, Col: col}

	// '&' is only valid as '&&' — a single '&' is illegal in kLex.
	case '&':
		if l.peekChar() == '&' {
			l.readChar()
			tok = Token{Type: TokenAnd, Literal: "&&", Line: line, Col: col}
		} else {
			tok = Token{Type: TokenIllegal, Literal: byteStrings[l.ch], Line: line, Col: col}
		}

	case '|':
		if l.peekChar() == '|' {
			l.readChar()
			tok = Token{Type: TokenOr, Literal: "||", Line: line, Col: col}
		} else if l.peekChar() == '>' {
			l.readChar()
			tok = Token{Type: TokenPipe, Literal: "|>", Line: line, Col: col}
		} else {
			tok = Token{Type: TokenIllegal, Literal: byteStrings[l.ch], Line: line, Col: col}
		}

	// '!' could be logical-not or not-equal (!=).
	case '!':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TokenNotEq, Literal: "!=", Line: line, Col: col}
		} else {
			tok = Token{Type: TokenNot, Literal: byteStrings[l.ch], Line: line, Col: col}
		}

	// '?' is the postfix error-propagation operator: expr?
	case '?':
		tok = Token{Type: TokenQuestion, Literal: "?", Line: line, Col: col}

	default:
		// b"…" — bytes literal. The `b` prefix is consumed only when it's
		// IMMEDIATELY followed by an opening quote; bare `b` continues to lex
		// as a normal identifier so existing code using `b` as a variable
		// name keeps working.
		if l.ch == 'b' && l.peekChar() == '"' {
			l.readChar() // consume the 'b'
			content, ok := l.readBytesString()
			if !ok {
				tok = Token{Type: TokenIllegal, Literal: "unterminated bytes literal", Line: line, Col: col}
			} else {
				tok = Token{Type: TokenBytes, Literal: content, Line: line, Col: col}
			}
			break
		}
		if isLetter(l.ch) {
			// Read the full identifier first, then check if it's a keyword.
			// Keywords are just reserved identifiers — same characters, different meaning.
			lit := l.readIdentifier()
			tokType := TokenIdent
			if kw, ok := keywords[lit]; ok {
				tokType = kw
			}
			// Return early: readIdentifier already advanced past the last character,
			// so we must NOT call readChar() again at the bottom of the function.
			return Token{Type: tokType, Literal: lit, Line: line, Col: col}
		} else if isDigit(l.ch) {
			lit, isFloat := l.readNumber()
			tokType := TokenInt
			if isFloat {
				tokType = TokenFloat
			}
			return Token{Type: tokType, Literal: lit, Line: line, Col: col}
		} else {
			tok = Token{Type: TokenIllegal, Literal: byteStrings[l.ch], Line: line, Col: col}
		}
	}

	// Advance past the current character so the next call to NextToken
	// starts fresh. Multi-character reads (identifiers, numbers, strings)
	// return early above to skip this step.
	l.readChar()
	return tok
}

// readIdentifier consumes letters, digits, and underscores until it hits
// something else. Returns the raw identifier string.
func (l *Lexer) readIdentifier() string {
	start := l.position
	for isLetter(l.ch) || isDigit(l.ch) || l.ch == '_' {
		l.readChar()
	}
	return l.input[start:l.position]
}

// readNumber consumes an integer or float literal and returns (literal, isFloat).
// Handles decimal, hex (0x), binary (0b), and octal (0o) integer prefixes.
// A float is detected when a '.' followed by a digit is found after a decimal integer.
func (l *Lexer) readNumber() (string, bool) {
	start := l.position
	// Detect base prefixes when the first digit is '0'
	if l.ch == '0' {
		switch l.peekChar() {
		case 'x', 'X':
			l.readChar() // consume '0'
			l.readChar() // consume 'x'/'X'
			for isHexDigit(l.ch) {
				l.readChar()
			}
			return l.input[start:l.position], false
		case 'b', 'B':
			l.readChar() // consume '0'
			l.readChar() // consume 'b'/'B'
			for l.ch == '0' || l.ch == '1' {
				l.readChar()
			}
			return l.input[start:l.position], false
		case 'o', 'O':
			l.readChar() // consume '0'
			l.readChar() // consume 'o'/'O'
			for l.ch >= '0' && l.ch <= '7' {
				l.readChar()
			}
			return l.input[start:l.position], false
		}
	}
	// Decimal integer or float
	for isDigit(l.ch) {
		l.readChar()
	}
	if l.ch == '.' && isDigit(l.peekChar()) {
		l.readChar() // consume '.'
		for isDigit(l.ch) {
			l.readChar()
		}
		return l.input[start:l.position], true
	}
	return l.input[start:l.position], false
}

func isHexDigit(ch byte) bool {
	return isDigit(ch) || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
}

// readString reads from after the opening quote to (but not past) the closing
// quote. It handles both plain and interpolated strings in one pass.
//
// Returns (content, segments, ok) where:
//   - ok == false: the string was unterminated (hit EOF before closing quote).
//   - segments != nil: the string contains interpolations. All escape
//     sequences in literal segments have been decoded, and each {…}
//     expression has been pre-lexed into kLex tokens with accurate Line/Col
//     from the outer source. content is empty in this case — the parser
//     consumes Segments directly.
//   - segments == nil: plain string. content holds the fully decoded value
//     with all escapes applied.
//
// Supported escapes in literal segments:
//
//	\"  → literal double quote
//	\n  → newline
//	\r  → carriage return
//	\t  → tab
//	\b  → backspace
//	\\  → literal backslash
//	\{  → literal {  (alternative to {{)
//	\}  → literal }  (alternative to }})
//	\xHH → byte with hex code HH
//
// Literal-brace forms in text segments:
//
//	{{  → literal {  (Python-style doubling; suppresses interpolation)
//	}}  → literal }
//
// Unknown escapes are preserved as the backslash followed by the next char.
//
// Inside an interpolation block ({…}) the lexer recurses through NextToken
// to gather kLex tokens, tracking brace depth at the token level so nested
// braces in function bodies / hash literals don't end the interpolation
// early. Inner string literals inside the expression — including ones that
// themselves contain interpolations — are handled by that recursive call.
func (l *Lexer) readString() (string, []InterpSegment, bool) {
	l.readChar() // skip opening quote

	var textBuf []byte
	var segments []InterpSegment
	textLine := l.line
	textCol := l.col

	flushText := func() {
		if len(textBuf) > 0 {
			segments = append(segments, InterpSegment{
				IsExpr: false,
				Text:   string(textBuf),
				Line:   textLine,
				Col:    textCol,
			})
			textBuf = nil
		}
	}

	for l.ch != '"' && l.ch != 0 {
		// Escape sequences in text segments.
		if l.ch == '\\' && l.readPosition < len(l.input) {
			l.readChar() // consume backslash, look at next char
			switch l.ch {
			case '"':
				textBuf = append(textBuf, '"')
			case 'n':
				textBuf = append(textBuf, '\n')
			case 't':
				textBuf = append(textBuf, '\t')
			case '\\':
				textBuf = append(textBuf, '\\')
			case 'r':
				textBuf = append(textBuf, '\r')
			case 'b':
				textBuf = append(textBuf, '\b')
			case '{':
				textBuf = append(textBuf, '{')
			case '}':
				textBuf = append(textBuf, '}')
			case 'x':
				if l.position+2 < len(l.input) {
					v1 := hexCharToInt(l.input[l.position+1])
					v2 := hexCharToInt(l.input[l.position+2])
					if v1 >= 0 && v2 >= 0 {
						l.readChar()
						l.readChar()
						textBuf = append(textBuf, byte((v1<<4)|v2))
					} else {
						textBuf = append(textBuf, '\\', 'x')
					}
				} else {
					textBuf = append(textBuf, '\\', 'x')
				}
			default:
				textBuf = append(textBuf, '\\', l.ch)
			}
			l.readChar()
			continue
		}

		// {{ → literal { (no interpolation).
		if l.ch == '{' && l.peekChar() == '{' {
			textBuf = append(textBuf, '{')
			l.readChar()
			l.readChar()
			continue
		}

		// }} → literal } (symmetric with {{).
		if l.ch == '}' && l.peekChar() == '}' {
			textBuf = append(textBuf, '}')
			l.readChar()
			l.readChar()
			continue
		}

		// { → start of an interpolation expression.
		if l.ch == '{' {
			flushText()
			exprLine := l.line
			exprCol := l.col
			l.readChar() // past the opening '{'

			var exprTokens []Token
			depth := 1
			for {
				tok := l.NextToken()
				if tok.Type == TokenEOF {
					return "", nil, false
				}
				if tok.Type == TokenIllegal {
					return "", nil, false
				}
				if tok.Type == TokenLBrace {
					depth++
				} else if tok.Type == TokenRBrace {
					depth--
					if depth == 0 {
						break // matched closing brace; not included in tokens
					}
				}
				exprTokens = append(exprTokens, tok)
			}
			// Terminate with synthetic EOF so the replay parser sees a clean end.
			exprTokens = append(exprTokens, Token{Type: TokenEOF, Line: l.line, Col: l.col})
			segments = append(segments, InterpSegment{
				IsExpr: true,
				Tokens: exprTokens,
				Line:   exprLine,
				Col:    exprCol,
			})
			// After NextToken consumed the closing '}', l.ch is positioned
			// at the next char. The next text segment starts here.
			textLine = l.line
			textCol = l.col
			continue
		}

		// Bare '}' at top level is reserved. kLex treats both braces as
		// special everywhere — explicit, no implicit "sometimes literal,
		// sometimes meaningful" rule. The user must either escape (`\}` or
		// `}}` for a literal close-brace) or use a raw string (`…` with
		// backticks) for JSON-shaped text.
		//
		// Scan through the rest of the string before returning so the lexer
		// is positioned past the closing quote — otherwise one bad `}`
		// would cascade into a flood of unrelated parse errors on the rest
		// of the file.
		if l.ch == '}' {
			for l.ch != '"' && l.ch != 0 {
				if l.ch == '\\' && l.readPosition < len(l.input) {
					l.readChar() // skip the escape character
				}
				l.readChar()
			}
			return "bare '}' is reserved inside a string — use '}}' or '\\}' for a literal close-brace, or use a `…` raw string for JSON/template text", nil, false
		}

		textBuf = append(textBuf, l.ch)
		l.readChar()
	}

	if l.ch == 0 {
		return "", nil, false
	}

	if segments != nil {
		flushText()
		return "", segments, true
	}
	return string(textBuf), nil, true
}

// readBytesString reads a bytes literal: b"…". The opening 'b' has already
// been consumed by NextToken; l.ch is positioned on the opening '"'. Returns
// the raw byte content packed into a Go string (Go strings can hold arbitrary
// non-utf8 byte sequences), plus an ok flag that is false on EOF before a
// closing quote.
//
// Bytes literals are deliberately a smaller language than string literals:
//   - No interpolation — `{` is just a literal `{`.
//   - Escape sequences supported: \" \\ \n \r \t \0 and \xHH (two hex digits).
//   - Any other \X is treated as literal backslash plus X, matching string
//     literal behaviour for unknown escapes.
//
// The intent is to keep the byte-level meaning of the literal predictable:
// `b"abc"` is three bytes 0x61 0x62 0x63 regardless of utf-8 considerations,
// and `b"\xff\xfe"` is exactly two bytes 0xff 0xfe even though that sequence
// is not valid utf-8.
func (l *Lexer) readBytesString() (string, bool) {
	l.readChar() // skip opening quote
	var buf []byte
	for l.ch != '"' && l.ch != 0 {
		if l.ch == '\\' {
			l.readChar() // consume backslash, examine the escape character
			switch l.ch {
			case '"':
				buf = append(buf, '"')
			case '\\':
				buf = append(buf, '\\')
			case 'n':
				buf = append(buf, '\n')
			case 'r':
				buf = append(buf, '\r')
			case 't':
				buf = append(buf, '\t')
			case '0':
				buf = append(buf, 0)
			case 'x':
				if l.position+2 < len(l.input) {
					val1 := hexCharToInt(l.input[l.position+1])
					val2 := hexCharToInt(l.input[l.position+2])
					if val1 >= 0 && val2 >= 0 {
						l.readChar() // advance to hex1
						l.readChar() // advance to hex2
						buf = append(buf, byte((val1<<4)|val2))
					} else {
						buf = append(buf, '\\', 'x')
					}
				} else {
					buf = append(buf, '\\', 'x')
				}
			default:
				buf = append(buf, '\\', l.ch)
			}
		} else {
			buf = append(buf, l.ch)
		}
		l.readChar()
	}
	if l.ch == 0 {
		return "", false
	}
	return string(buf), true
}

// readRawString reads a backtick-delimited raw string literal.
// No escape processing and no interpolation — every character is literal,
// including newlines. The opening backtick has already been consumed by
// NextToken; this function reads up to and including the closing backtick.
func (l *Lexer) readRawString() string {
	l.readChar() // skip opening backtick
	start := l.position
	for l.ch != '`' && l.ch != 0 {
		l.readChar()
	}
	// l.ch is now the closing backtick; NextToken's readChar() call will advance past it
	return l.input[start:l.position]
}

// skipWhitespace advances past spaces, tabs, newlines, and // line comments.
// A // comment runs from the double-slash to the end of the line and is
// treated identically to whitespace — the parser never sees it.
func (l *Lexer) skipWhitespace() {
	for {
		if l.ch == ' ' || l.ch == '\t' || l.ch == '\n' || l.ch == '\r' {
			l.readChar()
		} else if l.ch == '/' && l.peekChar() == '/' {
			for l.ch != '\n' && l.ch != 0 {
				l.readChar()
			}
		} else {
			break
		}
	}
}

func isLetter(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}

func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

func hexCharToInt(ch byte) int {
	if ch >= '0' && ch <= '9' {
		return int(ch - '0')
	}
	if ch >= 'a' && ch <= 'f' {
		return int(ch - 'a' + 10)
	}
	if ch >= 'A' && ch <= 'F' {
		return int(ch - 'A' + 10)
	}
	return -1
}
