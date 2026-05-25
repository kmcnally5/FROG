package lexer

import "testing"

// TestUnterminatedStringDoesNotPanic locks OFI #16. Pre-fix, readChar()
// advanced l.position past len(l.input) once EOF was reached, and the
// nested-string scanner inside an interpolation block ran one extra
// readChar() past EOF — making l.input[start:l.position] in readString
// panic with "slice bounds out of range".
//
// The fix is in readChar(): l.position now saturates at len(l.input) on
// EOF rather than incrementing forever. The lexer must yield a clean
// TokenIllegal with "unterminated string literal" for every malformed
// input below, with no Go-level panic.
func TestUnterminatedStringDoesNotPanic(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"bare unterminated", `"abc`},
		{"unterminated with escape", `"abc\`},
		{"unterminated with hex escape", `"abc\x`},
		{"unterminated inside interp brace", `"x{y`},
		{"unterminated nested string in interp", `"x{f("y`},
		{"trailing backslash inside interp", `"a{b}\`},
		{"deeply nested interp ending at EOF", `"a{b("c{d("e`},
		{"empty input after opening quote", `"`},
		{"unterminated raw string", "`abc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("lexer panicked on %q: %v", tc.input, r)
				}
			}()
			l := New(tc.input)
			// Drain tokens to EOF — any internal panic surfaces here.
			sawIllegal := false
			for {
				tok := l.NextToken()
				if tok.Type == TokenIllegal {
					sawIllegal = true
				}
				if tok.Type == TokenEOF {
					break
				}
			}
			if !sawIllegal {
				t.Fatalf("expected TokenIllegal for unterminated input %q, got none", tc.input)
			}
		})
	}
}

// TestReadCharSaturatesAtEOF locks the invariant that drives the fix:
// after the first EOF read, further readChar() calls leave l.position
// at len(l.input) rather than walking off the end.
func TestReadCharSaturatesAtEOF(t *testing.T) {
	l := New("ab")
	// New() primes by calling readChar once. l.ch == 'a', l.position == 0.
	l.readChar() // 'b', position 1
	l.readChar() // EOF, position 2
	if l.ch != 0 {
		t.Fatalf("expected ch=0 at EOF, got %q", l.ch)
	}
	if l.position != len(l.input) {
		t.Fatalf("expected position=%d at first EOF, got %d", len(l.input), l.position)
	}
	for i := 0; i < 100; i++ {
		l.readChar()
	}
	if l.position != len(l.input) {
		t.Fatalf("position drifted past EOF after extra readChar() calls: got %d, want %d", l.position, len(l.input))
	}
	if l.ch != 0 {
		t.Fatalf("ch should remain 0 after EOF saturation, got %q", l.ch)
	}
}
