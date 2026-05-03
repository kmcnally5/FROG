package eval

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"fmt"
	"io"
)

func init() {
	// _gzipCompress(data) → (compressed, err)
	// Compresses a string using gzip (default level 6)
	Builtins["_gzipCompress"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return &Tuple{Elements: []Object{NULL, &String{Value: "_gzipCompress expects 1 argument"}}}
		}
		s, ok := args[0].(*String)
		if !ok {
			return &Tuple{Elements: []Object{NULL, &String{Value: fmt.Sprintf("_gzipCompress: argument must be string, got %s", args[0].Type())}}}
		}

		var buf bytes.Buffer
		w := gzip.NewWriter(&buf)
		if _, err := w.Write([]byte(s.Value)); err != nil {
			w.Close()
			return &Tuple{Elements: []Object{NULL, &String{Value: fmt.Sprintf("gzip write error: %s", err.Error())}}}
		}
		if err := w.Close(); err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: fmt.Sprintf("gzip close error: %s", err.Error())}}}
		}

		return &Tuple{Elements: []Object{&String{Value: buf.String()}, NULL}}
	}}

	// _gzipDecompress(data) → (decompressed, err)
	// Decompresses gzip-compressed data
	Builtins["_gzipDecompress"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return &Tuple{Elements: []Object{NULL, &String{Value: "_gzipDecompress expects 1 argument"}}}
		}
		s, ok := args[0].(*String)
		if !ok {
			return &Tuple{Elements: []Object{NULL, &String{Value: fmt.Sprintf("_gzipDecompress: argument must be string, got %s", args[0].Type())}}}
		}

		r, err := gzip.NewReader(bytes.NewReader([]byte(s.Value)))
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: fmt.Sprintf("gzip reader error: %s", err.Error())}}}
		}
		defer r.Close()

		decompressed, err := io.ReadAll(r)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: fmt.Sprintf("gzip read error: %s", err.Error())}}}
		}

		return &Tuple{Elements: []Object{&String{Value: string(decompressed)}, NULL}}
	}}

	// _deflateCompress(data) → (compressed, err)
	// Compresses a string using deflate (no gzip header)
	Builtins["_deflateCompress"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return &Tuple{Elements: []Object{NULL, &String{Value: "_deflateCompress expects 1 argument"}}}
		}
		s, ok := args[0].(*String)
		if !ok {
			return &Tuple{Elements: []Object{NULL, &String{Value: fmt.Sprintf("_deflateCompress: argument must be string, got %s", args[0].Type())}}}
		}

		var buf bytes.Buffer
		w, err := flate.NewWriter(&buf, flate.DefaultCompression)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: fmt.Sprintf("deflate writer error: %s", err.Error())}}}
		}
		if _, err := w.Write([]byte(s.Value)); err != nil {
			w.Close()
			return &Tuple{Elements: []Object{NULL, &String{Value: fmt.Sprintf("deflate write error: %s", err.Error())}}}
		}
		if err := w.Close(); err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: fmt.Sprintf("deflate close error: %s", err.Error())}}}
		}

		return &Tuple{Elements: []Object{&String{Value: buf.String()}, NULL}}
	}}

	// _deflateDecompress(data) → (decompressed, err)
	// Decompresses deflate-compressed data
	Builtins["_deflateDecompress"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return &Tuple{Elements: []Object{NULL, &String{Value: "_deflateDecompress expects 1 argument"}}}
		}
		s, ok := args[0].(*String)
		if !ok {
			return &Tuple{Elements: []Object{NULL, &String{Value: fmt.Sprintf("_deflateDecompress: argument must be string, got %s", args[0].Type())}}}
		}

		r := flate.NewReader(bytes.NewReader([]byte(s.Value)))
		defer r.Close()

		decompressed, err := io.ReadAll(r)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: fmt.Sprintf("deflate read error: %s", err.Error())}}}
		}

		return &Tuple{Elements: []Object{&String{Value: string(decompressed)}, NULL}}
	}}
}
