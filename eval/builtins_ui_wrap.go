package eval

// builtins_ui_wrap.go — soft-wrap text layout helpers shared between the
// desktop (GL) and browser (Canvas2D) builds.
//
// These are in their own untagged file so builtins_ui_widgets.go (also
// untagged) can call them regardless of build target.  The desktop-only
// builtins_ui.go used to host them, but WASM builds exclude that file
// (//go:build !js) and the textArea soft-wrap implementation needs them.
//
// uiTextWidth is NOT replicated here — it uses gfx.fontCellW which is
// desktop-only.  Instead every width measurement goes through
// activeRenderer.textWidth(), which is implemented correctly for both GL
// and Canvas2D.

import "strings"

// wrappedLine is one visual row produced by soft-wrapping.  It carries the
// rune-offset bookkeeping that lets the textArea map mouse positions to
// underlying text offsets (and vice versa) for selection / caret placement.
//
//	startRune  — first rune index in the underlying text this line covers
//	runeCount  — number of runes in `text` (== len([]rune(text)))
//
// Visual lines do NOT include separator characters the wrap consumed:
// a hard newline between two lines lives at runes[prev.endRune()] and is
// not part of either visual line; a soft-wrap on a space similarly skips
// that single space.  Per-rune fallback breaks leave no gap — the next
// visual line begins exactly where the previous ended.
type wrappedLine struct {
	text      string
	startRune int
	runeCount int
}

func (l wrappedLine) endRune() int { return l.startRune + l.runeCount }

// softWrapTextWithOffsets is the offset-aware companion to softWrapText.
// It runs the same wrap algorithm then re-derives rune offsets by walking
// the original text — correct for all three cases the wrap produces (hard
// newline consumed, space-eaten soft wrap, per-rune fallback split).
func softWrapTextWithOffsets(text string, maxW, scale float32) []wrappedLine {
	lines := softWrapText(text, maxW, scale)
	runes := []rune(text)
	out := make([]wrappedLine, len(lines))
	cursor := 0
	for i, line := range lines {
		lineRunes := []rune(line)
		if !runesMatchAt(runes, cursor, lineRunes) && cursor < len(runes) {
			cursor++
		}
		out[i] = wrappedLine{
			text:      line,
			startRune: cursor,
			runeCount: len(lineRunes),
		}
		cursor += len(lineRunes)
	}
	if len(out) == 0 {
		out = append(out, wrappedLine{startRune: 0, runeCount: 0})
	}
	return out
}

func runesMatchAt(haystack []rune, pos int, needle []rune) bool {
	if pos+len(needle) > len(haystack) {
		return false
	}
	for i, r := range needle {
		if haystack[pos+i] != r {
			return false
		}
	}
	return true
}

// taLineForOffset returns the visual-line index that hosts the cursor at the
// given rune offset.  Picks the FIRST line whose [startRune, endRune] range
// contains offset so a cursor at a line boundary stays at the end of the
// earlier line (text just typed appears there, natural typing behaviour).
func taLineForOffset(lines []wrappedLine, offset int) int {
	for i := range lines {
		if offset >= lines[i].startRune && offset <= lines[i].endRune() {
			return i
		}
	}
	if len(lines) == 0 {
		return 0
	}
	return len(lines) - 1
}

// softWrapText returns visual lines for text such that each line's rendered
// width does not exceed maxW.  Hard newlines are preserved; each hard line
// is additionally broken on word boundaries when it overflows.  A single
// word longer than maxW falls back to a per-rune break so the cursor never
// escapes the field.  Empty hard lines survive wrapping as empty visual lines.
func softWrapText(text string, maxW float32, scale float32) []string {
	var out []string
	for _, hard := range strings.Split(text, "\n") {
		out = append(out, wrapOneLine(hard, maxW, scale)...)
	}
	if len(out) == 0 {
		return []string{""}
	}
	return out
}

func wrapOneLine(line string, maxW float32, scale float32) []string {
	if line == "" {
		return []string{""}
	}
	if activeRenderer.textWidth(line, scale) <= maxW {
		return []string{line}
	}
	// Word-boundary pass.
	var lines []string
	words := strings.Split(line, " ")
	current := ""
	for _, w := range words {
		candidate := w
		if current != "" {
			candidate = current + " " + w
		}
		if activeRenderer.textWidth(candidate, scale) > maxW && current != "" {
			lines = append(lines, current)
			current = w
		} else {
			current = candidate
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	// Fallback pass: any single-word line still over budget broken by runes.
	var final []string
	for _, l := range lines {
		if activeRenderer.textWidth(l, scale) <= maxW {
			final = append(final, l)
			continue
		}
		chunk := ""
		for _, r := range []rune(l) {
			try := chunk + string(r)
			if activeRenderer.textWidth(try, scale) > maxW && chunk != "" {
				final = append(final, chunk)
				chunk = string(r)
			} else {
				chunk = try
			}
		}
		if chunk != "" {
			final = append(final, chunk)
		}
	}
	if len(final) == 0 {
		return []string{""}
	}
	return final
}
