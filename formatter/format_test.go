package formatter

import (
	"strings"
	"testing"
)

// roundTripIdempotent verifies that formatting twice produces the same
// result as formatting once — the formatter must converge in one pass.
func TestFormatterIsIdempotent(t *testing.T) {
	cases := []string{
		"",
		"x = 1\n",
		"fn foo() {\n    return 1\n}\n",
		"if x {\n    y = 2\n}\n",
	}
	for _, in := range cases {
		once := string(Format([]byte(in)))
		twice := string(Format([]byte(once)))
		if once != twice {
			t.Errorf("not idempotent for %q\n  once:  %q\n  twice: %q", in, once, twice)
		}
	}
}

func TestBasicIndent(t *testing.T) {
	in := `fn foo() {
x = 1
return x
}
`
	want := `fn foo() {
    x = 1
    return x
}
`
	got := string(Format([]byte(in)))
	if got != want {
		t.Errorf("basic indent\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func TestNestedIndent(t *testing.T) {
	in := `fn foo() {
if x {
y = 2
if z {
w = 3
}
}
}
`
	want := `fn foo() {
    if x {
        y = 2
        if z {
            w = 3
        }
    }
}
`
	got := string(Format([]byte(in)))
	if got != want {
		t.Errorf("nested indent\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func TestTrailingWhitespaceStripped(t *testing.T) {
	in := "x = 1   \ny = 2\t\n"
	want := "x = 1\ny = 2\n"
	got := string(Format([]byte(in)))
	if got != want {
		t.Errorf("trailing ws\nwant: %q\ngot:  %q", want, got)
	}
}

func TestBlankLineCollapsing(t *testing.T) {
	// Policy: up to 2 consecutive blank lines preserved (sibling separator
	// + section break). Runs of 3+ collapse to 2.
	in := "x = 1\n\n\n\ny = 2\n"
	want := "x = 1\n\n\ny = 2\n"
	got := string(Format([]byte(in)))
	if got != want {
		t.Errorf("blank line collapse\nwant: %q\ngot:  %q", want, got)
	}
}

func TestSingleAndDoubleBlanksPreserved(t *testing.T) {
	// Single blank → kept.
	got := string(Format([]byte("x = 1\n\ny = 2\n")))
	if got != "x = 1\n\ny = 2\n" {
		t.Errorf("single blank not preserved: %q", got)
	}
	// Double blank → kept (used for section breaks).
	got = string(Format([]byte("x = 1\n\n\ny = 2\n")))
	if got != "x = 1\n\n\ny = 2\n" {
		t.Errorf("double blank not preserved: %q", got)
	}
}

func TestLeadingBlankLinesRemoved(t *testing.T) {
	in := "\n\n\nx = 1\n"
	want := "x = 1\n"
	got := string(Format([]byte(in)))
	if got != want {
		t.Errorf("leading blanks\nwant: %q\ngot:  %q", want, got)
	}
}

func TestSingleTrailingNewline(t *testing.T) {
	for _, in := range []string{
		"x = 1",
		"x = 1\n",
		"x = 1\n\n",
		"x = 1\n\n\n",
	} {
		got := string(Format([]byte(in)))
		if !strings.HasSuffix(got, "\n") || strings.HasSuffix(got, "\n\n") {
			t.Errorf("trailing newline policy violated for %q → %q", in, got)
		}
	}
}

func TestEmptyInput(t *testing.T) {
	got := string(Format([]byte("")))
	if got != "" {
		t.Errorf("empty input\nwant: %q\ngot:  %q", "", got)
	}
}

func TestBraceInsideString(t *testing.T) {
	// The "{" inside the string must NOT count as a block opener.
	in := `x = "hello { world"
y = 2
`
	want := `x = "hello { world"
y = 2
`
	got := string(Format([]byte(in)))
	if got != want {
		t.Errorf("brace in string\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func TestBraceInsideLineComment(t *testing.T) {
	// The "{" in the comment must not affect indentation.
	in := `x = 1 // not a block: {
y = 2
`
	want := `x = 1 // not a block: {
y = 2
`
	got := string(Format([]byte(in)))
	if got != want {
		t.Errorf("brace in comment\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func TestInterpolation(t *testing.T) {
	// "{name}" is interpolation — the inside is an expression, not a block.
	// Should NOT count as a block opener.
	in := `name = "kLex"
greeting = "Hello {name}!"
y = 2
`
	want := `name = "kLex"
greeting = "Hello {name}!"
y = 2
`
	got := string(Format([]byte(in)))
	if got != want {
		t.Errorf("interpolation\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func TestRawStringSingleLine(t *testing.T) {
	in := "x = `literal { content` \ny = 2\n"
	want := "x = `literal { content`\ny = 2\n"
	got := string(Format([]byte(in)))
	if got != want {
		t.Errorf("raw string single line\nwant: %q\ngot:  %q", want, got)
	}
}

func TestRawStringMultiLine(t *testing.T) {
	// Multi-line backtick raw string contents must be emitted VERBATIM.
	// No re-indent, no trimming inside the raw region.
	in := "x = `line one\n  weird indent\n    line three\n`\ny = 2\n"
	want := "x = `line one\n  weird indent\n    line three\n`\ny = 2\n"
	got := string(Format([]byte(in)))
	if got != want {
		t.Errorf("raw string multi-line\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func TestClosingBraceUnindents(t *testing.T) {
	in := `fn foo() {
    if x {
        y = 1
        }
    }
`
	want := `fn foo() {
    if x {
        y = 1
    }
}
`
	got := string(Format([]byte(in)))
	if got != want {
		t.Errorf("closing brace unindent\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func TestBracesOnSameLine(t *testing.T) {
	// A line like `} else {` should un-indent (because it starts with })
	// but the NET brace count is 0, so subsequent lines stay at the
	// SAME depth as inside the original block.
	in := `fn foo() {
if x {
y = 1
} else {
z = 2
}
}
`
	want := `fn foo() {
    if x {
        y = 1
    } else {
        z = 2
    }
}
`
	got := string(Format([]byte(in)))
	if got != want {
		t.Errorf("} else {\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func TestNormalizeCRLF(t *testing.T) {
	in := "x = 1\r\ny = 2\r\n"
	want := "x = 1\ny = 2\n"
	got := string(Format([]byte(in)))
	if got != want {
		t.Errorf("CRLF normalize\nwant: %q\ngot:  %q", want, got)
	}
}

func TestAlreadyFormatted(t *testing.T) {
	// Idempotency under realistic input.
	in := `fn add(a, b) {
    return a + b
}

fn main() {
    x = add(1, 2)
    println(x)
}
`
	got := string(Format([]byte(in)))
	if got != in {
		t.Errorf("clean input changed\nwant:\n%s\ngot:\n%s", in, got)
	}
}
