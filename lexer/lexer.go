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
	TokenAssign TokenType = "="
	TokenGT     TokenType = ">"
	TokenLT     TokenType = "<"

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
type Token struct {
	Type    TokenType
	Literal string // the raw text from the source, e.g. "42" or "myVar"
	Line    int    // 1-based line number
	Col     int    // 1-based column number
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
}

// New creates a lexer and positions it at the first character of the input.
func New(input string) *Lexer {
	l := &Lexer{input: input, line: 1, col: 0}
	l.readChar() // prime the pump: sets ch to input[0]
	return l
}

// readChar advances the lexer by one character, updating position tracking.
// When we reach the end of input, ch is set to 0 (the null byte), which acts
// as our EOF sentinel.
func (l *Lexer) readChar() {
	if l.readPosition >= len(l.input) {
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
	var tok Token

	l.skipWhitespace()

	line, col := l.pos()

	switch l.ch {

	case '+':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TokenPlusAssign, Literal: "+=", Line: line, Col: col}
		} else {
			tok = Token{TokenPlus, byteStrings[l.ch], line, col}
		}
	case '-':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TokenMinusAssign, Literal: "-=", Line: line, Col: col}
		} else {
			tok = Token{TokenMinus, byteStrings[l.ch], line, col}
		}
	case '*':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TokenMulAssign, Literal: "*=", Line: line, Col: col}
		} else {
			tok = Token{TokenAsterisk, byteStrings[l.ch], line, col}
		}
	case '/':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TokenDivAssign, Literal: "/=", Line: line, Col: col}
		} else {
			tok = Token{TokenSlash, byteStrings[l.ch], line, col}
		}
	case '%':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TokenModAssign, Literal: "%=", Line: line, Col: col}
		} else {
			tok = Token{TokenPercent, byteStrings[l.ch], line, col}
		}
	case '(':
		tok = Token{TokenLParen, byteStrings[l.ch], line, col}
	case ')':
		tok = Token{TokenRParen, byteStrings[l.ch], line, col}
	case ',':
		tok = Token{TokenComma, byteStrings[l.ch], line, col}
	case ':':
		tok = Token{TokenColon, byteStrings[l.ch], line, col}
	case '.':
		if l.peekChar() == '.' && l.readPosition+1 < len(l.input) && l.input[l.readPosition+1] == '.' {
			l.readChar() // consume second '.'
			l.readChar() // consume third '.'
			tok = Token{Type: TokenEllipsis, Literal: "...", Line: line, Col: col}
		} else {
			tok = Token{TokenDot, byteStrings[l.ch], line, col}
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
		tok = Token{TokenLBrace, byteStrings[l.ch], line, col}
	case '}':
		tok = Token{TokenRBrace, byteStrings[l.ch], line, col}
	case '[':
		tok = Token{TokenLBracket, byteStrings[l.ch], line, col}
	case ']':
		tok = Token{TokenRBracket, byteStrings[l.ch], line, col}

	// '>' could be greater-than or greater-than-or-equal.
	case '>':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TokenGTE, Literal: ">=", Line: line, Col: col}
		} else {
			tok = Token{TokenGT, byteStrings[l.ch], line, col}
		}

	// '<' could be less-than or less-than-or-equal.
	case '<':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TokenLTE, Literal: "<=", Line: line, Col: col}
		} else {
			tok = Token{TokenLT, byteStrings[l.ch], line, col}
		}

	case '"':
		content, hasInterp := l.readString()
		if l.ch == 0 {
			tok = Token{Type: TokenIllegal, Literal: "unterminated string literal", Line: line, Col: col}
		} else if hasInterp {
			tok = Token{Type: TokenInterpStr, Literal: content, Line: line, Col: col}
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
			tok = Token{TokenAnd, "&&", line, col}
		} else {
			tok = Token{TokenIllegal, byteStrings[l.ch], line, col}
		}

	case '|':
		if l.peekChar() == '|' {
			l.readChar()
			tok = Token{TokenOr, "||", line, col}
		} else if l.peekChar() == '>' {
			l.readChar()
			tok = Token{Type: TokenPipe, Literal: "|>", Line: line, Col: col}
		} else {
			tok = Token{TokenIllegal, byteStrings[l.ch], line, col}
		}

	// '!' could be logical-not or not-equal (!=).
	case '!':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TokenNotEq, Literal: "!=", Line: line, Col: col}
		} else {
			tok = Token{TokenNot, byteStrings[l.ch], line, col}
		}

	default:
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
			tok = Token{TokenIllegal, byteStrings[l.ch], line, col}
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
// A float is detected when a '.' followed by a digit is found after the integer part.
func (l *Lexer) readNumber() (string, bool) {
	start := l.position
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

// readString reads from after the opening quote to the closing quote.
// Returns (content, hasInterp) where hasInterp is true if the string contains
// at least one bare { (i.e. string interpolation).
//
// Three cases, each optimised:
//
//  1. No escapes, no interpolation (most common): returns l.input[start:end]
//     directly — a zero-allocation string slice of the source.
//
//  2. Escape sequences, no interpolation: builds procBuf incrementally,
//     copying raw segments in bulk before each escape and appending the
//     expanded byte(s) for each sequence. rawBuf is never allocated.
//
//  3. Interpolation present: returns l.input[start:end], the raw source slice
//     with escape sequences preserved as-is so the parser can locate {…}
//     boundaries before processing escapes segment by segment. rawBuf is
//     never allocated regardless of whether escapes are also present.
//
// Supported escapes (both modes):
//
//	\"  → literal double quote
//	\n  → newline
//	\r  → carriage return
//	\t  → tab
//	\b  → backspace
//	\\  → literal backslash
//	\{  → literal { (suppresses interpolation)
//
// Unknown escapes are preserved as-is.
func (l *Lexer) readString() (string, bool) {
	l.readChar() // skip opening quote
	start := l.position
	hasEscapes := false
	hasInterp := false
	var procBuf []byte

	for l.ch != '"' && l.ch != 0 {
		if l.ch == '\\' {
			if !hasEscapes {
				// First escape: bulk-copy everything before it into procBuf.
				procBuf = append(procBuf, l.input[start:l.position]...)
				hasEscapes = true
			}
			l.readChar() // consume backslash, examine the escape character
			switch l.ch {
			case '"':
				procBuf = append(procBuf, '"')
			case 'n':
				procBuf = append(procBuf, '\n')
			case 't':
				procBuf = append(procBuf, '\t')
			case '\\':
				procBuf = append(procBuf, '\\')
			case 'r':
				procBuf = append(procBuf, '\r')
			case 'b':
				procBuf = append(procBuf, '\b')
			case '{':
				procBuf = append(procBuf, '{') // \{ in a plain string is just {
			default:
				procBuf = append(procBuf, '\\', l.ch)
			}
		} else {
			if l.ch == '{' {
				hasInterp = true
			}
			if hasEscapes {
				procBuf = append(procBuf, l.ch)
			}
		}
		l.readChar()
	}

	// Interpolation: the parser needs the raw source form (escape sequences
	// preserved as literal backslash+char) so it can split on { boundaries.
	// l.input[start:l.position] is exactly that — zero allocation.
	if hasInterp {
		return l.input[start:l.position], true
	}
	// Plain string with no escapes: slice the source directly — zero allocation.
	if !hasEscapes {
		return l.input[start:l.position], false
	}
	// Plain string with escapes: procBuf holds the fully expanded content.
	return string(procBuf), false
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
