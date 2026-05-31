package eval

import "testing"

// mockRenderer is a minimal monospace uiRenderer for offset/layout tests.
// Only textWidth carries logic (cellW px per rune); the rest are no-ops.
type mockRenderer struct{ cellW float32 }

func (m mockRenderer) fillRoundedRect(x, y, w, h, r float32, color [4]float32)              {}
func (m mockRenderer) strokeRoundedRect(x, y, w, h, r, sw float32, color [4]float32)        {}
func (m mockRenderer) drawText(t string, x, y int, c bool, s float32, col [4]float32)       {}
func (m mockRenderer) textWidth(t string, scale float32) float32                            { return float32(len([]rune(t))) * m.cellW }
func (m mockRenderer) lineHeight(scale float32) float32                                     { return 16 }
func (m mockRenderer) textTopInset(scale float32) float32                                   { return 0 }
func (m mockRenderer) drawLine(x1, y1, x2, y2, lw float32, color [4]float32)                {}
func (m mockRenderer) fillPolygon(points []float32, color [4]float32)                       {}
func (m mockRenderer) fillArc(cx, cy, r, sa, ea float32, color [4]float32)                  {}
func (m mockRenderer) drawImage(img *Image, x, y, w, h float32)                             {}
func (m mockRenderer) pushClip(x, y, w, h float32)                                          {}
func (m mockRenderer) popClip()                                                            {}
func (m mockRenderer) dropShadow(x, y, w, h, r, oy, bl float32, color [4]float32)           {}

func withMockRenderer() func() {
	prev := activeRenderer
	activeRenderer = mockRenderer{cellW: 10}
	return func() { activeRenderer = prev }
}

// verifyOffsets asserts that each non-empty visual line's text equals the
// source runes at [startRune, startRune+runeCount). This is the invariant
// the caret/selection math depends on; a drift here misplaces the caret.
func verifyOffsets(t *testing.T, text string, lines []wrappedLine) {
	t.Helper()
	runes := []rune(text)
	for i, l := range lines {
		end := l.startRune + l.runeCount
		if l.startRune < 0 || end > len(runes) {
			t.Errorf("line %d %q: range [%d,%d) out of bounds (len %d)", i, l.text, l.startRune, end, len(runes))
			continue
		}
		if l.runeCount == 0 {
			continue
		}
		if got := string(runes[l.startRune:end]); got != l.text {
			t.Errorf("line %d: source at [%d,%d) = %q, but visual text = %q", i, l.startRune, end, got, l.text)
		}
	}
}

// TestSoftWrapOffsetsBlankLines reproduces the caret-drift bug: every line
// after a blank line must keep its true rune startRune. A wide maxW disables
// wrapping so this isolates the hard-newline / blank-line offset accounting.
func TestSoftWrapOffsetsBlankLines(t *testing.T) {
	defer withMockRenderer()()
	const wide = 100000

	for _, text := range []string{
		"A\n\nB",
		"x = 1\n\ny = 2\n\nif a (b",
		"\n\nleading blanks",
		"trailing\n\n",
		"a\n\n\nb", // multiple consecutive blanks
	} {
		lines := softWrapTextWithOffsets(text, wide, 0.5)
		verifyOffsets(t, text, lines)
	}
}

// TestSoftWrapOffsetsSoftWrap forces actual soft-wrapping (narrow width) and
// confirms the rune offsets stay correct across word-wrap breaks — the path
// the editor uses for long lines. cellW=10, so maxW=35 fits ~3 chars.
func TestSoftWrapOffsetsSoftWrap(t *testing.T) {
	defer withMockRenderer()()
	for _, text := range []string{
		"aa bb cc",            // simple word wrap on spaces
		"aa bb cc\n\ndd ee",   // word wrap mixed with a blank line
		"supercalifragilistic", // single token longer than the line (per-rune)
		"word averylongunbreakabletoken end",
	} {
		lines := softWrapTextWithOffsets(text, 35, 0.5)
		verifyOffsets(t, text, lines)
	}
}

// TestSoftWrapOffsetsRoundTrip checks the exact startRunes for a known case.
func TestSoftWrapOffsetsRoundTrip(t *testing.T) {
	defer withMockRenderer()()
	lines := softWrapTextWithOffsets("A\n\nB", 100000, 0.5)
	want := []struct {
		text  string
		start int
		count int
	}{
		{"A", 0, 1},
		{"", 2, 0},
		{"B", 3, 1},
	}
	if len(lines) != len(want) {
		t.Fatalf("line count: got %d, want %d", len(lines), len(want))
	}
	for i, w := range want {
		if lines[i].text != w.text || lines[i].startRune != w.start || lines[i].runeCount != w.count {
			t.Errorf("line %d: got {%q,%d,%d}, want {%q,%d,%d}",
				i, lines[i].text, lines[i].startRune, lines[i].runeCount, w.text, w.start, w.count)
		}
	}
}
