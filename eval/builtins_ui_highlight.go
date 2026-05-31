package eval

// builtins_ui_highlight.go — kLex source syntax highlighting for the
// textArea widget's opt-in `syntax: "klex"` mode.
//
// highlightKLex walks the source once and returns a per-rune category
// slice. The textArea draw loop groups consecutive same-category runes
// into runs and draws each run in its category colour. Colours are
// derived from the editor background luminance (syntaxColor) so they
// adapt to any theme — dark or light — without storing extra palette
// slots or touching the theme presets.
//
// The tokeniser is intentionally LENIENT — it pattern-matches rather
// than parses (the same approach as the docs highlighter). Mis-tokenised
// weird code is purely cosmetic; real correctness is the interpreter's
// job. This file is untagged so both the GL (desktop) and Canvas2D
// (WASM) renderers share it.

type synCat uint8

const (
	synDefault synCat = iota
	synKeyword
	synString
	synComment
	synNumber
	synOperator
	synBuiltin
)

// synKeywords mirrors the reserved-word set in lexer/lexer.go.
var synKeywords = map[string]bool{
	"if": true, "else": true, "true": true, "false": true, "null": true,
	"fn": true, "return": true, "while": true, "break": true, "continue": true,
	"for": true, "in": true, "import": true, "as": true,
	"switch": true, "case": true, "default": true,
	"struct": true, "enum": true, "let": true, "const": true, "select": true,
}

// highlightKLex returns a category for every rune in src. The result
// has len(src) entries; cats[i] is the category of src[i].
func highlightKLex(src []rune) []synCat {
	n := len(src)
	cats := make([]synCat, n)
	i := 0
	for i < n {
		c := src[i]
		switch {
		// // line comment — to end of line.
		case c == '/' && i+1 < n && src[i+1] == '/':
			j := i
			for j < n && src[j] != '\n' {
				cats[j] = synComment
				j++
			}
			i = j

		// "..." string — escape-aware so \" doesn't end it.
		case c == '"':
			cats[i] = synString
			j := i + 1
			for j < n {
				if src[j] == '\\' && j+1 < n {
					cats[j] = synString
					cats[j+1] = synString
					j += 2
					continue
				}
				cats[j] = synString
				if src[j] == '"' {
					j++
					break
				}
				j++
			}
			i = j

		// `...` raw string — no escapes, runs to the next backtick.
		case c == '`':
			cats[i] = synString
			j := i + 1
			for j < n && src[j] != '`' {
				cats[j] = synString
				j++
			}
			if j < n {
				cats[j] = synString
				j++
			}
			i = j

		// Numbers: decimal, hex/binary/octal prefix, float.
		case c >= '0' && c <= '9':
			j := i + 1
			if c == '0' && j < n && isSynNumPrefix(src[j]) {
				j++
				for j < n && isSynHexDigit(src[j]) {
					j++
				}
			} else {
				for j < n && src[j] >= '0' && src[j] <= '9' {
					j++
				}
				if j+1 < n && src[j] == '.' && src[j+1] >= '0' && src[j+1] <= '9' {
					j++
					for j < n && src[j] >= '0' && src[j] <= '9' {
						j++
					}
				}
			}
			for k := i; k < j; k++ {
				cats[k] = synNumber
			}
			i = j

		// Identifier → keyword / builtin / plain.
		case isSynIdentStart(c):
			j := i + 1
			for j < n && isSynIdentChar(src[j]) {
				j++
			}
			cat := synDefault
			word := string(src[i:j])
			if synKeywords[word] {
				cat = synKeyword
			} else if _, ok := Builtins[word]; ok {
				cat = synBuiltin
			}
			if cat != synDefault {
				for k := i; k < j; k++ {
					cats[k] = cat
				}
			}
			i = j

		// Operators — greedy up to 3 chars (==, !=, ->, |>, <=, …).
		case isSynOpChar(c):
			j := i + 1
			for j < n && isSynOpChar(src[j]) && j-i < 3 {
				j++
			}
			for k := i; k < j; k++ {
				cats[k] = synOperator
			}
			i = j

		default:
			i++ // whitespace / punctuation stays synDefault
		}
	}
	return cats
}

// syntaxColor maps a category to an RGBA colour. It chooses a dark-bg or
// light-bg palette from the luminance of the editor background so the
// colours stay readable under any theme. synDefault returns the caller's
// normal text colour so untokenised text matches the rest of the widget.
func syntaxColor(cat synCat, bg, normal [4]float32) [4]float32 {
	if cat == synDefault {
		return normal
	}
	light := bg[0]*0.299+bg[1]*0.587+bg[2]*0.114 > 0.5
	if light {
		switch cat {
		case synKeyword:
			return [4]float32{0.52, 0.18, 0.68, 1}
		case synString:
			return [4]float32{0.10, 0.50, 0.20, 1}
		case synComment:
			return [4]float32{0.55, 0.58, 0.63, 1}
		case synNumber:
			return [4]float32{0.70, 0.38, 0.08, 1}
		case synOperator:
			return [4]float32{0.32, 0.45, 0.55, 1}
		case synBuiltin:
			return [4]float32{0.10, 0.48, 0.55, 1}
		}
		return normal
	}
	switch cat {
	case synKeyword:
		return [4]float32{0.80, 0.58, 0.98, 1}
	case synString:
		return [4]float32{0.56, 0.82, 0.50, 1}
	case synComment:
		return [4]float32{0.45, 0.50, 0.58, 1}
	case synNumber:
		return [4]float32{0.96, 0.72, 0.42, 1}
	case synOperator:
		return [4]float32{0.62, 0.78, 0.86, 1}
	case synBuiltin:
		return [4]float32{0.45, 0.82, 0.88, 1}
	}
	return normal
}

// catAt returns cats[idx], or synDefault if idx is out of range. Keeps the
// textArea draw loop panic-proof against any rune-offset edge case at a
// soft-wrap boundary (a stray miscolour is acceptable; a frame panic is not).
func catAt(cats []synCat, idx int) synCat {
	if idx < 0 || idx >= len(cats) {
		return synDefault
	}
	return cats[idx]
}

func isSynIdentStart(c rune) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isSynIdentChar(c rune) bool {
	return isSynIdentStart(c) || (c >= '0' && c <= '9')
}

func isSynNumPrefix(c rune) bool {
	return c == 'x' || c == 'X' || c == 'b' || c == 'B' || c == 'o' || c == 'O'
}

func isSynHexDigit(c rune) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') || c == '_'
}

func isSynOpChar(c rune) bool {
	switch c {
	case '+', '-', '*', '/', '%', '=', '!', '<', '>', '|', '&', '?', '~', '^':
		return true
	}
	return false
}
