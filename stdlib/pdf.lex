// stdlib/pdf.lex — PDF text extraction for kLex.
// @module    pdf
// @version   1.0.0
// @since     klex 0.3.35
// @author    karl
// @summary   PDF text extraction for kLex.
//
// Pure-Go reader (github.com/ledongthuc/pdf) baked into the binary —
// no Python or external dependencies. Handles the 90%+ of real-world
// PDFs that have embedded text: Word/Office exports, LaTeX, modern
// browser "print to PDF", most academic papers and reports.
//
// What does NOT work:
//   - Scanned / image-only PDFs (no OCR — bytes have no text layer)
//   - Encrypted PDFs (v1 doesn't pass a password)
//   - Complex tables / multi-column layouts (text comes out, but flow
//     across columns can scramble — fine for RAG, awkward for display)
//
// API:
//   pdf.extractText(path)       → (text, err)    — whole document as one string
//   pdf.extractByPage(path)     → (pages, err)   — array of per-page strings
//   pdf.pageCount(path)         → (n, err)       — page count
//
// Errors use typed codes (PDF_OPEN, PDF_DECODE, PDF_BAD_INPUT) so callers
// can branch without parsing strings.
//
// Usage with the AI library set:
//   import "stdlib/pdf.lex"          as pdf
//   import "stdlib/ai/chunk.lex"     as chunk
//   import "stdlib/ai/vector_store.lex" as vs
//   import "stdlib/ai/ollama.lex"    as ollama
//
//   text   = pdf.extractText("/path/to/manual.pdf")?
//   pieces = chunk.byParagraphs(text, 800)
//   for p in pieces {
//       v, _ = ollama.embed(c, p["text"])
//       store.add("manual:" + str(p["index"]), v["embeddings"][0], p)
//   }


// extractText returns the entire PDF's text concatenated page-by-page.
// Page boundaries are not preserved — pages stream back-to-back. Use
// extractByPage() when you need per-page attribution.
fn extractText(path) {
    if type(path) != "STRING" {
        return null, error("PDF_BAD_INPUT", "extractText: path must be a string")
    }
    let text, err = _pdfExtractText(path)
    if err != null {
        return null, _classifyError(err)
    }
    return text, null
}


// extractByPage returns an array of strings, one per page, in document
// order. Pages with no extractable text (e.g. image-only) become "".
// The (err) return covers fatal open/decode failures of the file itself,
// not per-page parse hiccups.
fn extractByPage(path) {
    if type(path) != "STRING" {
        return null, error("PDF_BAD_INPUT", "extractByPage: path must be a string")
    }
    let pages, err = _pdfExtractByPage(path)
    if err != null {
        return null, _classifyError(err)
    }
    return pages, null
}


// pageCount returns the number of pages in the PDF. Cheap — does not
// extract any text. Use to size a progress bar before extractByPage().
fn pageCount(path) {
    if type(path) != "STRING" {
        return 0, error("PDF_BAD_INPUT", "pageCount: path must be a string")
    }
    let n, err = _pdfPageCount(path)
    if err != null {
        return 0, _classifyError(err)
    }
    return n, null
}


// _classifyError maps the raw error string from the Go-side primitive into
// a typed kLex error with a stable code. Falls back to PDF_DECODE for
// anything we don't recognise — callers can branch on err.code without
// parsing messages.
fn _classifyError(rawMsg) {
    if indexOf(rawMsg, "no such file") >= 0 || indexOf(rawMsg, "does not exist") >= 0 {
        return error("PDF_OPEN", rawMsg)
    }
    if indexOf(rawMsg, "encrypted") >= 0 || indexOf(rawMsg, "password") >= 0 {
        return error("PDF_ENCRYPTED", rawMsg)
    }
    if indexOf(rawMsg, "permission denied") >= 0 {
        return error("PDF_OPEN", rawMsg)
    }
    return error("PDF_DECODE", rawMsg)
}
