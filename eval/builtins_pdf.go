package eval

import (
	"bytes"
	"fmt"
	"io"

	"klex/ast"

	"github.com/ledongthuc/pdf"
)

// builtins_pdf.go — PDF text extraction primitives.
//
// Backed by github.com/ledongthuc/pdf, a pure-Go PDF reader. Handles the
// 90%+ of real-world PDFs that have embedded text (anything produced by
// Word, LaTeX, modern browsers, most academic papers). Scanned / image-only
// PDFs return an empty string — those need OCR, which isn't in scope here.
//
// All three builtins return a (value, err) 2-tuple matching the kLex
// stdlib convention. err is a string on failure, null on success.

func init() {
	// _pdfExtractText(path) → (text, err)
	//
	// Returns the concatenated plain text of every page in the PDF at
	// `path`. Page boundaries are NOT marked in the output — pages are
	// just streamed back-to-back. Use _pdfExtractByPage when you need
	// per-page text (e.g. for source attribution).
	Builtins["_pdfExtractText"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_pdfExtractText expects 1 argument (path)", ast.Pos{})
		}
		path, ok := args[0].(*String)
		if !ok {
			return typeError("_pdfExtractText: path must be a string", ast.Pos{})
		}

		f, r, err := pdf.Open(path.Value)
		if err != nil {
			return pdfErrTuple(err)
		}
		defer f.Close()

		stream, err := r.GetPlainText()
		if err != nil {
			return pdfErrTuple(err)
		}
		var buf bytes.Buffer
		if _, err := io.Copy(&buf, stream); err != nil {
			return pdfErrTuple(err)
		}
		return &Tuple{Elements: []Object{&String{Value: buf.String()}, NULL}}
	}}

	// _pdfPageCount(path) → (n, err)
	//
	// Returns the number of pages in the PDF. Useful for progress reporting
	// before extracting page-by-page.
	Builtins["_pdfPageCount"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_pdfPageCount expects 1 argument (path)", ast.Pos{})
		}
		path, ok := args[0].(*String)
		if !ok {
			return typeError("_pdfPageCount: path must be a string", ast.Pos{})
		}

		f, r, err := pdf.Open(path.Value)
		if err != nil {
			return pdfErrTuple(err)
		}
		defer f.Close()

		return &Tuple{Elements: []Object{&Integer{Value: r.NumPage()}, NULL}}
	}}

	// _pdfExtractByPage(path) → (pages, err)
	//
	// Returns an array of strings, one per page, in document order. Each
	// element is that page's plain text (may be empty for image-only pages).
	// Use this when you want to keep page-level source attribution in a RAG
	// pipeline (cite "page N of foo.pdf").
	//
	// Pages where the underlying parser fails fall back to empty strings —
	// one bad page doesn't kill the whole extraction. The (err) return is
	// reserved for fatal open/decode failures of the file itself.
	Builtins["_pdfExtractByPage"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_pdfExtractByPage expects 1 argument (path)", ast.Pos{})
		}
		path, ok := args[0].(*String)
		if !ok {
			return typeError("_pdfExtractByPage: path must be a string", ast.Pos{})
		}

		f, r, err := pdf.Open(path.Value)
		if err != nil {
			return pdfErrTuple(err)
		}
		defer f.Close()

		n := r.NumPage()
		out := make([]Object, n)
		for i := 1; i <= n; i++ {
			p := r.Page(i)
			if p.V.IsNull() {
				out[i-1] = &String{Value: ""}
				continue
			}
			text, perr := p.GetPlainText(nil)
			if perr != nil {
				// Per-page failure → empty string, not fatal.
				out[i-1] = &String{Value: ""}
				continue
			}
			out[i-1] = &String{Value: text}
		}
		return &Tuple{Elements: []Object{&Array{Elements: out}, NULL}}
	}}
}

// pdfErrTuple wraps a Go error into the (null, errstring) tuple shape kLex
// builtins use for fatal failures.
func pdfErrTuple(err error) Object {
	return &Tuple{Elements: []Object{NULL, &String{Value: fmt.Sprintf("pdf: %v", err)}}}
}
