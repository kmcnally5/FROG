package eval

import (
	"strings"
	"testing"
)

// TestHighlightKLex checks that the per-rune tokeniser assigns the right
// category to keywords, numbers, comments, strings, builtins and operators.
// The source is ASCII, so byte index == rune index and strings.Index gives
// the rune offset directly.
func TestHighlightKLex(t *testing.T) {
	src := "let x = 42  // a comment\n" +
		"println(\"hello\")\n" +
		"let s = `raw`\n"
	runes := []rune(src)
	cats := highlightKLex(runes)
	if len(cats) != len(runes) {
		t.Fatalf("cats length %d != runes length %d", len(cats), len(runes))
	}

	catOf := func(sub string) synCat {
		i := strings.Index(src, sub)
		if i < 0 {
			t.Fatalf("substring %q not found in source", sub)
		}
		return cats[i]
	}

	cases := []struct {
		name string
		sub  string
		want synCat
	}{
		{"keyword let", "let", synKeyword},
		{"number 42", "42", synNumber},
		{"comment", "// a comment", synComment},
		{"builtin println", "println", synBuiltin},
		{"double-quoted string", "\"hello\"", synString},
		{"raw string", "`raw`", synString},
		{"operator =", "=", synOperator},
		{"plain identifier x", "x ", synDefault},
	}
	for _, c := range cases {
		if got := catOf(c.sub); got != c.want {
			t.Errorf("%s: got category %d, want %d", c.name, got, c.want)
		}
	}
}

// TestSyntaxColor confirms synDefault passes the caller's normal colour
// through, and that the palette adapts to background luminance.
func TestSyntaxColor(t *testing.T) {
	normal := [4]float32{0.9, 0.9, 0.9, 1}
	darkBg := [4]float32{0.10, 0.12, 0.16, 1}
	lightBg := [4]float32{0.96, 0.97, 0.98, 1}

	if got := syntaxColor(synDefault, darkBg, normal); got != normal {
		t.Errorf("synDefault must return the normal colour, got %v", got)
	}
	if syntaxColor(synKeyword, darkBg, normal) == syntaxColor(synKeyword, lightBg, normal) {
		t.Errorf("keyword colour should differ between dark and light backgrounds")
	}
	if syntaxColor(synString, darkBg, normal) == normal {
		t.Errorf("string colour should differ from the normal text colour")
	}
}
