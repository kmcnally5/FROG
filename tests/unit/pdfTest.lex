// pdfTest.lex — unit tests for stdlib/pdf.lex.
//
// Fixtures live in tests/fixtures/pdf/ — small PDFs produced by macOS
// cupsfilter from plain-text inputs. If you're on a non-mac platform and
// the fixtures are missing, regenerate them with any PDF generator that
// embeds text (cupsfilter, pandoc, Word "Save As", browser "Print to PDF").

import "stdlib/pdf.lex" as pdf


// Paths are relative to the kLex repo root (where the tests are run from).
let HELLO_PDF   = "tests/fixtures/pdf/hello.pdf"
let TWOPAGE_PDF = "tests/fixtures/pdf/twopage.pdf"


// ── pageCount ─────────────────────────────────────────────────────────────

println("=== pageCount ===")

let n, err = pdf.pageCount(HELLO_PDF)
assert(err == null,                "pageCount: hello.pdf err must be null")
assert(n == 1,                     "pageCount: hello.pdf expected 1 page, got " + str(n))

n, err = pdf.pageCount(TWOPAGE_PDF)
assert(err == null,                "pageCount: twopage.pdf err must be null")
assert(n == 2,                     "pageCount: twopage.pdf expected 2 pages, got " + str(n))

// Missing file → typed PDF_OPEN error.
n, err = pdf.pageCount("does/not/exist.pdf")
assert(err != null,                "pageCount: missing file must error")
assert(err.code == "PDF_OPEN",     "pageCount: missing file → PDF_OPEN, got " + err.code)

// Non-string path → PDF_BAD_INPUT.
n, err = pdf.pageCount(42)
assert(err != null,                "pageCount: int path must error")
assert(err.code == "PDF_BAD_INPUT", "pageCount: non-string → PDF_BAD_INPUT")

println("  pageCount: PASS")


// ── extractText ──────────────────────────────────────────────────────────

println("=== extractText ===")

let text, err = pdf.extractText(HELLO_PDF)
assert(err == null,                "extractText: err must be null")
assert(type(text) == "STRING",     "extractText: returns a string")
assert(len(text) > 0,              "extractText: produced text from a text PDF")
// Sanity check — fixture contains "Hello" and "kLex" verbatim.
assert(indexOf(text, "Hello") >= 0, "extractText: 'Hello' present in output")
assert(indexOf(text, "kLex") >= 0,  "extractText: 'kLex' present in output")

// Two-page extract joins both pages' text.
text, err = pdf.extractText(TWOPAGE_PDF)
assert(err == null,                "extractText: twopage err must be null")
assert(indexOf(text, "Page one") >= 0 || indexOf(text, "page one") >= 0,
       "extractText: page-1 text appears")
assert(indexOf(text, "Second page") >= 0 || indexOf(text, "second page") >= 0,
       "extractText: page-2 text appears")

// Missing file → typed error.
text, err = pdf.extractText("does/not/exist.pdf")
assert(err != null,                "extractText: missing file errors")
assert(err.code == "PDF_OPEN",     "extractText: PDF_OPEN code")

println("  extractText: PASS")


// ── extractByPage ────────────────────────────────────────────────────────

println("=== extractByPage ===")

let pages, err = pdf.extractByPage(HELLO_PDF)
assert(err == null,                "extractByPage: 1-page err must be null")
assert(type(pages) == "ARRAY",     "extractByPage: returns array")
assert(len(pages) == 1,            "extractByPage: 1 page returned")
assert(indexOf(pages[0], "Hello") >= 0, "extractByPage: page-0 contains 'Hello'")

pages, err = pdf.extractByPage(TWOPAGE_PDF)
assert(err == null,                "extractByPage: 2-page err must be null")
assert(len(pages) == 2,            "extractByPage: 2 pages returned")
// Each page should be a string (possibly empty, but a string).
assert(type(pages[0]) == "STRING", "extractByPage: page 0 is string")
assert(type(pages[1]) == "STRING", "extractByPage: page 1 is string")

println("  extractByPage: PASS")


println("")
println("ALL PDF TESTS PASSED")
