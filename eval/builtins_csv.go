package eval

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"klex/ast"
	"strings"
)

// csvReadAll parses CSV data with a specified delimiter
// Returns ([][]string, Object) where Object is nil on success or an *Error on failure
func csvReadAll(data string, delim rune, fieldsPerRecord int) ([][]string, Object) {
	r := csv.NewReader(strings.NewReader(data))
	r.Comma = delim
	r.FieldsPerRecord = fieldsPerRecord
	records, err := r.ReadAll()
	if err != nil {
		return nil, &Error{
			Kind:         "RuntimeError",
			Code:         "CSV_ERROR",
			Message:      fmt.Sprintf("CSV parse error: %s", err.Error()),
			IsUserError:  true,
			Pos:          ast.Pos{},
		}
	}
	return records, nil
}

// objectToRows converts a kLex *Array of *Array of *String to Go [][]string
// Each element must be an *Array of *String values
func objectToRows(arr *Array) ([][]string, Object) {
	if arr == nil {
		return nil, typeError("expected array, got nil", ast.Pos{})
	}

	rows := make([][]string, len(arr.Elements))
	for i, elem := range arr.Elements {
		row, ok := elem.(*Array)
		if !ok {
			return nil, typeError(
				fmt.Sprintf("array element %d must be array, got %s", i, elem.Type()),
				ast.Pos{},
			)
		}

		cells := make([]string, len(row.Elements))
		for j, cell := range row.Elements {
			s, ok := cell.(*String)
			if !ok {
				return nil, typeError(
					fmt.Sprintf("row %d, column %d must be string, got %s", i, j, cell.Type()),
					ast.Pos{},
				)
			}
			cells[j] = s.Value
		}
		rows[i] = cells
	}
	return rows, nil
}

// stringsToArray converts Go [][]string to kLex *Array of *Array of *String
func stringsToArray(records [][]string) *Array {
	rows := make([]Object, len(records))
	for i, record := range records {
		cells := make([]Object, len(record))
		for j, cell := range record {
			cells[j] = &String{Value: cell}
		}
		rows[i] = &Array{Elements: cells}
	}
	return &Array{Elements: rows}
}

func init() {
	// _csvParse(data) → (array_of_arrays, error)
	// Parses CSV with comma delimiter (RFC 4180)
	Builtins["_csvParse"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_csvParse expects 1 argument", ast.Pos{})
		}
		s, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_csvParse: argument must be string, got %s", args[0].Type()), ast.Pos{})
		}

		records, errObj := csvReadAll(s.Value, ',', 0)
		if errObj != nil {
			return &Tuple{Elements: []Object{NULL, errObj}}
		}
		return &Tuple{Elements: []Object{stringsToArray(records), NULL}}
	}}

	// _tsvParse(data) → (array_of_arrays, error)
	// Parses TSV with tab delimiter
	Builtins["_tsvParse"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_tsvParse expects 1 argument", ast.Pos{})
		}
		s, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_tsvParse: argument must be string, got %s", args[0].Type()), ast.Pos{})
		}

		records, errObj := csvReadAll(s.Value, '\t', 0)
		if errObj != nil {
			return &Tuple{Elements: []Object{NULL, errObj}}
		}
		return &Tuple{Elements: []Object{stringsToArray(records), NULL}}
	}}

	// _csvParseDelim(data, delim) → (array_of_arrays, error)
	// Parses CSV with custom delimiter (first rune of delim string)
	Builtins["_csvParseDelim"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_csvParseDelim expects 2 arguments", ast.Pos{})
		}
		s, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_csvParseDelim: data must be string, got %s", args[0].Type()), ast.Pos{})
		}
		delimObj, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_csvParseDelim: delimiter must be string, got %s", args[1].Type()), ast.Pos{})
		}

		if len(delimObj.Value) == 0 {
			return runtimeError("_csvParseDelim: delimiter must not be empty", ast.Pos{})
		}

		delim := []rune(delimObj.Value)[0]
		records, errObj := csvReadAll(s.Value, delim, 0)
		if errObj != nil {
			return &Tuple{Elements: []Object{NULL, errObj}}
		}
		return &Tuple{Elements: []Object{stringsToArray(records), NULL}}
	}}

	// _csvFormat(rows) → (string, error)
	// Formats array of arrays to CSV (comma-delimited)
	Builtins["_csvFormat"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_csvFormat expects 1 argument", ast.Pos{})
		}
		arr, ok := args[0].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("_csvFormat: argument must be array, got %s", args[0].Type()), ast.Pos{})
		}

		records, errObj := objectToRows(arr)
		if errObj != nil {
			return &Tuple{Elements: []Object{NULL, errObj}}
		}

		var buf bytes.Buffer
		w := csv.NewWriter(&buf)
		w.Comma = ','
		err := w.WriteAll(records)
		w.Flush()
		if err == nil {
			err = w.Error()
		}
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &Error{
				Kind:        "RuntimeError",
				Code:        "CSV_FORMAT_ERROR",
				Message:     fmt.Sprintf("CSV format error: %s", err.Error()),
				IsUserError: true,
				Pos:         ast.Pos{},
			}}}
		}

		return &Tuple{Elements: []Object{&String{Value: buf.String()}, NULL}}
	}}

	// _tsvFormat(rows) → (string, error)
	// Formats array of arrays to TSV (tab-delimited)
	Builtins["_tsvFormat"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_tsvFormat expects 1 argument", ast.Pos{})
		}
		arr, ok := args[0].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("_tsvFormat: argument must be array, got %s", args[0].Type()), ast.Pos{})
		}

		records, errObj := objectToRows(arr)
		if errObj != nil {
			return &Tuple{Elements: []Object{NULL, errObj}}
		}

		var buf bytes.Buffer
		w := csv.NewWriter(&buf)
		w.Comma = '\t'
		err := w.WriteAll(records)
		w.Flush()
		if err == nil {
			err = w.Error()
		}
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &Error{
				Kind:        "RuntimeError",
				Code:        "TSV_FORMAT_ERROR",
				Message:     fmt.Sprintf("TSV format error: %s", err.Error()),
				IsUserError: true,
				Pos:         ast.Pos{},
			}}}
		}

		return &Tuple{Elements: []Object{&String{Value: buf.String()}, NULL}}
	}}

	// _csvFormatDelim(rows, delim) → (string, error)
	// Formats array of arrays with custom delimiter
	Builtins["_csvFormatDelim"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_csvFormatDelim expects 2 arguments", ast.Pos{})
		}
		arr, ok := args[0].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("_csvFormatDelim: rows must be array, got %s", args[0].Type()), ast.Pos{})
		}
		delimObj, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_csvFormatDelim: delimiter must be string, got %s", args[1].Type()), ast.Pos{})
		}

		if len(delimObj.Value) == 0 {
			return runtimeError("_csvFormatDelim: delimiter must not be empty", ast.Pos{})
		}

		records, errObj := objectToRows(arr)
		if errObj != nil {
			return &Tuple{Elements: []Object{NULL, errObj}}
		}

		delim := []rune(delimObj.Value)[0]
		var buf bytes.Buffer
		w := csv.NewWriter(&buf)
		w.Comma = delim
		err := w.WriteAll(records)
		w.Flush()
		if err == nil {
			err = w.Error()
		}
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &Error{
				Kind:        "RuntimeError",
				Code:        "CSV_FORMAT_ERROR",
				Message:     fmt.Sprintf("CSV format error: %s", err.Error()),
				IsUserError: true,
				Pos:         ast.Pos{},
			}}}
		}

		return &Tuple{Elements: []Object{&String{Value: buf.String()}, NULL}}
	}}

	// _csvParseHeaders(data) → (array_of_hashes, error)
	// Parses CSV treating first row as column headers
	// Returns array of hashes where each hash maps column name to value
	// Uses lenient parsing (FieldsPerRecord=-1) to handle ragged rows
	Builtins["_csvParseHeaders"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_csvParseHeaders expects 1 argument", ast.Pos{})
		}
		s, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_csvParseHeaders: argument must be string, got %s", args[0].Type()), ast.Pos{})
		}

		records, errObj := csvReadAll(s.Value, ',', -1)
		if errObj != nil {
			return &Tuple{Elements: []Object{NULL, errObj}}
		}

		if len(records) == 0 {
			return &Tuple{Elements: []Object{&Array{Elements: []Object{}}, NULL}}
		}

		headers := records[0]
		hashes := make([]Object, len(records)-1)

		for i := 1; i < len(records); i++ {
			row := records[i]
			pairs := make(map[HashKey]HashPair)

			for colIdx, headerName := range headers {
				cellVal := ""
				if colIdx < len(row) {
					cellVal = row[colIdx]
				}

				key := &String{Value: headerName}
				pairs[HashKey{Type: STRING_OBJ, Value: headerName}] = HashPair{
					Key:   key,
					Value: &String{Value: cellVal},
				}
			}

			hashes[i-1] = &Hash{Pairs: pairs}
		}

		return &Tuple{Elements: []Object{&Array{Elements: hashes}, NULL}}
	}}

	// _csvStream(data, delim) → Channel
	// Streams CSV rows as they are parsed. Each element sent to channel is an Array of Strings.
	// Channel closes automatically when parsing completes or on error.
	// Allows overlapping parsing and processing for better parallelization.
	// Optimized: Large buffer + async parser prevent lock contention with multiple workers.
	Builtins["_csvStream"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_csvStream expects 2 arguments", ast.Pos{})
		}
		dataObj, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_csvStream: data must be string, got %s", args[0].Type()), ast.Pos{})
		}
		delimObj, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_csvStream: delimiter must be string, got %s", args[1].Type()), ast.Pos{})
		}

		if len(delimObj.Value) == 0 {
			return runtimeError("_csvStream: delimiter must not be empty", ast.Pos{})
		}

		delim := []rune(delimObj.Value)[0]
		data := dataObj.Value

		// Create channel with large buffer to decouple parsing from processing.
		// Large buffer allows parser to stay ahead of slow workers, reducing lock contention.
		// With buffer=50000 and 10 workers, each recv() call has high probability of hitting buffer
		// instead of blocking on mutex. This reduces context switch overhead from O(500K) to O(10).
		ch := make(chan Object, 50000)
		done := make(chan struct{})

		// Spawn parser goroutine
		go func() {
			defer close(ch)
			r := csv.NewReader(strings.NewReader(data))
			r.Comma = delim
			r.FieldsPerRecord = 0

			for {
				record, err := r.Read()
				if err != nil {
					if errors.Is(err, io.EOF) {
						// Normal end of file
						break
					}
					// Send error as an object
					ch <- &Error{
						Kind:        "RuntimeError",
						Code:        "CSV_STREAM_ERROR",
						Message:     fmt.Sprintf("CSV parse error: %s", err.Error()),
						IsUserError: true,
						Pos:         ast.Pos{},
					}
					return
				}

				// Convert record to kLex Array of Strings
				elements := make([]Object, len(record))
				for i, field := range record {
					elements[i] = &String{Value: field}
				}
				row := &Array{Elements: elements}

				// Send row to channel (will block if buffer full, allowing backpressure)
				select {
				case ch <- row:
				case <-done:
					// Channel was cancelled by receiver
					return
				}
			}
		}()

		return &Channel{ch: ch, done: done}
	}}

	// _csvFirstRowCols(data, delim) → (column_count, error)
	// Optimized: Parses only first row and returns column count (early exit)
	// Used by columnCount() to avoid parsing entire CSV
	Builtins["_csvFirstRowCols"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_csvFirstRowCols expects 2 arguments", ast.Pos{})
		}
		dataObj, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_csvFirstRowCols: data must be string, got %s", args[0].Type()), ast.Pos{})
		}
		delimObj, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_csvFirstRowCols: delimiter must be string, got %s", args[1].Type()), ast.Pos{})
		}

		if len(delimObj.Value) == 0 {
			return runtimeError("_csvFirstRowCols: delimiter must not be empty", ast.Pos{})
		}

		delim := []rune(delimObj.Value)[0]
		r := csv.NewReader(strings.NewReader(dataObj.Value))
		r.Comma = delim
		r.FieldsPerRecord = 0

		record, err := r.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				// Empty CSV
				return &Tuple{Elements: []Object{&Integer{Value: 0}, NULL}}
			}
			return &Tuple{Elements: []Object{NULL, &Error{
				Kind:        "RuntimeError",
				Code:        "CSV_ERROR",
				Message:     fmt.Sprintf("CSV parse error: %s", err.Error()),
				IsUserError: true,
				Pos:         ast.Pos{},
			}}}
		}

		return &Tuple{Elements: []Object{&Integer{Value: len(record)}, NULL}}
	}}

	// _csvHasRows(data, delim) → (bool, error)
	// Optimized: Checks if CSV has at least one row (early exit after first read)
	// Used by isEmpty() to avoid parsing entire CSV
	Builtins["_csvHasRows"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_csvHasRows expects 2 arguments", ast.Pos{})
		}
		dataObj, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_csvHasRows: data must be string, got %s", args[0].Type()), ast.Pos{})
		}
		delimObj, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_csvHasRows: delimiter must be string, got %s", args[1].Type()), ast.Pos{})
		}

		if len(delimObj.Value) == 0 {
			return runtimeError("_csvHasRows: delimiter must not be empty", ast.Pos{})
		}

		delim := []rune(delimObj.Value)[0]
		r := csv.NewReader(strings.NewReader(dataObj.Value))
		r.Comma = delim
		r.FieldsPerRecord = 0

		_, err := r.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				// Empty CSV
				return &Tuple{Elements: []Object{FALSE, NULL}}
			}
			return &Tuple{Elements: []Object{NULL, &Error{
				Kind:        "RuntimeError",
				Code:        "CSV_ERROR",
				Message:     fmt.Sprintf("CSV parse error: %s", err.Error()),
				IsUserError: true,
				Pos:         ast.Pos{},
			}}}
		}

		return &Tuple{Elements: []Object{TRUE, NULL}}
	}}
}
