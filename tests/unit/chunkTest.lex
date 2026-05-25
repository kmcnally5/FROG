// chunkTest.lex — unit tests for stdlib/ai/chunk.lex.

import "stdlib/ai/chunk.lex" as chunk


// ── byChars ───────────────────────────────────────────────────────────────

println("=== byChars ===")

// Empty text → empty array.
assert(len(chunk.byChars("", 10, 2)) == 0, "byChars: empty input → empty array")

// Text shorter than size → one chunk.
let out = chunk.byChars("abc", 10, 0)
assert(len(out) == 1,                "byChars: short text → 1 chunk")
assert(out[0]["text"] == "abc",      "byChars: short text content")
assert(out[0]["start"] == 0,         "byChars: short text start")
assert(out[0]["end"] == 3,           "byChars: short text end")
assert(out[0]["index"] == 0,         "byChars: short text index")

// Exact-size text → one chunk.
out = chunk.byChars("0123456789", 10, 0)
assert(len(out) == 1,                "byChars: exact-size → 1 chunk")
assert(out[0]["text"] == "0123456789", "byChars: exact-size content")

// No overlap — text split into adjacent chunks.
out = chunk.byChars("0123456789ABCDEFGH", 10, 0)
assert(len(out) == 2,                "byChars: 18 chars, size 10, overlap 0 → 2 chunks")
assert(out[0]["text"] == "0123456789", "byChars: chunk 0 text")
assert(out[0]["start"] == 0,         "byChars: chunk 0 start")
assert(out[0]["end"] == 10,          "byChars: chunk 0 end")
assert(out[1]["text"] == "ABCDEFGH", "byChars: chunk 1 text (trailing partial)")
assert(out[1]["start"] == 10,        "byChars: chunk 1 start")
assert(out[1]["end"] == 18,          "byChars: chunk 1 end")
assert(out[1]["index"] == 1,         "byChars: chunk 1 index")

// With overlap — chunks share trailing characters.
out = chunk.byChars("0123456789ABCDEFGH", 10, 3)
// stride = 7. Chunks start at 0, 7, 14 — 3 chunks, last one a partial.
assert(len(out) == 3,                "byChars: 18 chars, size 10, overlap 3 → 3 chunks")
assert(out[0]["text"] == "0123456789", "byChars: overlap chunk 0")
assert(out[1]["start"] == 7,         "byChars: overlap chunk 1 starts at stride")
assert(out[1]["text"] == "789ABCDEFG", "byChars: overlap chunk 1 covers 7..17")
assert(out[2]["start"] == 14,        "byChars: overlap chunk 2 starts at 14")
assert(out[2]["text"] == "EFGH",     "byChars: overlap chunk 2 is the trailing partial")

// Overlap >= size → clamped to size-1.
out = chunk.byChars("0123456789", 5, 99)
// Should not infinite-loop; clamped to overlap=4 → stride=1.
assert(len(out) > 0,                 "byChars: overlap clamp does not infinite-loop")

println("  byChars: PASS")


// ── byWords ───────────────────────────────────────────────────────────────

println("=== byWords ===")

// Empty / whitespace-only → empty.
assert(len(chunk.byWords("", 5, 0))     == 0, "byWords: empty input → empty")
assert(len(chunk.byWords("   ", 5, 0))  == 0, "byWords: whitespace → empty")

// Single word → single chunk.
out = chunk.byWords("hello", 5, 0)
assert(len(out) == 1,                "byWords: single word → 1 chunk")
assert(out[0]["text"] == "hello",    "byWords: single word content")
assert(out[0]["start"] == 0,         "byWords: single word start")
assert(out[0]["end"] == 5,           "byWords: single word end")

// 6 words, size 3, overlap 0 → 2 chunks of 3 words each.
out = chunk.byWords("one two three four five six", 3, 0)
assert(len(out) == 2,                "byWords: 6 words, size 3 → 2 chunks")
assert(out[0]["text"] == "one two three", "byWords: chunk 0 text")
assert(out[1]["text"] == "four five six",  "byWords: chunk 1 text")

// Offsets land on word boundaries.
let text = "alpha beta gamma delta"
out = chunk.byWords(text, 2, 0)
assert(out[0]["start"] == 0,         "byWords: offset 0 = 'alpha' start")
assert(out[0]["end"]   == 10,        "byWords: offset 10 = end of 'beta'")
assert(out[1]["start"] == 11,        "byWords: offset 11 = 'gamma' start")

// Overlap.
out = chunk.byWords("a b c d e f g", 3, 1)
// stride = 2 — chunks start at word 0, 2, 4 (then word 6 last single).
// 7 words: chunks at words [0..2], [2..4], [4..6], [6..6].
assert(len(out) >= 3,                "byWords: overlap produces multiple chunks")

println("  byWords: PASS")


// ── byParagraphs ─────────────────────────────────────────────────────────

println("=== byParagraphs ===")

// Empty input.
assert(len(chunk.byParagraphs("", 0)) == 0, "byParagraphs: empty → empty array")

// Single paragraph, no separator.
out = chunk.byParagraphs("just one paragraph here", 0)
assert(len(out) == 1,                "byParagraphs: no \\n\\n → 1 chunk")
assert(out[0]["text"] == "just one paragraph here", "byParagraphs: single chunk text")
assert(out[0]["start"] == 0,         "byParagraphs: single chunk start")
assert(out[0]["end"] == 23,          "byParagraphs: single chunk end")

// Three paragraphs, no merge (maxChars=0).
text = "First.\n\nSecond.\n\nThird paragraph."
out = chunk.byParagraphs(text, 0)
assert(len(out) == 3,                "byParagraphs: 3 paras / maxChars=0 → 3 chunks")
assert(out[0]["text"] == "First.",   "byParagraphs: para 0")
assert(out[1]["text"] == "Second.",  "byParagraphs: para 1")
assert(out[2]["text"] == "Third paragraph.", "byParagraphs: para 2")
// Offsets must line up so substr(text, start, end) reproduces the chunk.
assert(substr(text, out[0]["start"], out[0]["end"]) == "First.",          "byParagraphs: offset 0 round-trip")
assert(substr(text, out[1]["start"], out[1]["end"]) == "Second.",         "byParagraphs: offset 1 round-trip")
assert(substr(text, out[2]["start"], out[2]["end"]) == "Third paragraph.", "byParagraphs: offset 2 round-trip")

// Merge mode: three short paragraphs, maxChars big enough to merge all.
text = "Short A.\n\nShort B.\n\nShort C."
out = chunk.byParagraphs(text, 1000)
assert(len(out) == 1,                "byParagraphs: maxChars large → all merged")
assert(out[0]["text"] == "Short A.\n\nShort B.\n\nShort C.", "byParagraphs: merged content")

// Merge mode: two-at-a-time grouping.
// Each paragraph is 8 chars + 2 sep = 10. maxChars=20 → two paras fit, third spills.
text = "AAAAAAAA\n\nBBBBBBBB\n\nCCCCCCCC"
out = chunk.byParagraphs(text, 20)
assert(len(out) == 2,                "byParagraphs: maxChars=20 → 2 merged chunks (2+1)")
assert(out[0]["text"] == "AAAAAAAA\n\nBBBBBBBB", "byParagraphs: first merge")
assert(out[1]["text"] == "CCCCCCCC",            "byParagraphs: trailing solo")

// Oversized paragraph survives as a single chunk (no internal split).
let big = "X"
let i = 0
while i < 100 {
    big = big + "X"
    i = i + 1
}
text = big + "\n\nshort"
out = chunk.byParagraphs(text, 10)
assert(len(out) == 2,                "byParagraphs: oversized para emits as-is")
assert(len(out[0]["text"]) > 10,     "byParagraphs: oversized chunk > maxChars (caller must re-chunk)")

println("  byParagraphs: PASS")


println("")
println("ALL CHUNK TESTS PASSED")
