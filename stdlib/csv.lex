// ============================================================================
// csv.lex — CSV and TSV parsing and formatting
// ============================================================================
//
// Parses and formats RFC 4180-compliant CSV using Go's encoding/csv.
// Handles quoted fields with embedded commas, newlines, and quotes correctly.
//
// CRITICAL: All functions validate input types and return (data, error) tuples.
// Check error before using the result — error is null on success.
//
// Example:
//   rows, err = parse("a,b\n1,2\n3,4")
//   if err != null { return null, err }
//   println(rows[1][0])  // "3"

// ============================================================================
// PARSING
// ============================================================================

// parse(data) → (rows, error)
// Parses CSV-formatted string (comma-delimited).
// Returns array of arrays — each row is an array of strings.
// Use for standard CSV files.
//
// Example:
//   data = "name,age\nAlice,30\nBob,25"
//   rows, err = parse(data)
//   println(rows[0][0])  // "name"
fn parse(data) {
    if type(data) != "STRING" {
        return null, error("TYPE_ERROR", "parse expects string, got " + type(data))
    }
    return safe(_csvParse, data)
}

// parseTSV(data) → (rows, error)
// Parses tab-separated values (TSV format).
// Returns array of arrays.
// Use for TSV files or tab-delimited data.
//
// Example:
//   data = "a\tb\tc\n1\t2\t3"
//   rows, err = parseTSV(data)
fn parseTSV(data) {
    if type(data) != "STRING" {
        return null, error("TYPE_ERROR", "parseTSV expects string, got " + type(data))
    }
    return safe(_tsvParse, data)
}

// parseDelimited(data, delim) → (rows, error)
// Parses data with custom delimiter (first character of delim string).
// Returns array of arrays.
// Use for pipe-delimited, semicolon-delimited, or other formats.
//
// Example:
//   data = "a|b|c\n1|2|3"
//   rows, err = parseDelimited(data, "|")
fn parseDelimited(data, delim) {
    if type(data) != "STRING" {
        return null, error("TYPE_ERROR", "parseDelimited: data must be string, got " + type(data))
    }
    if type(delim) != "STRING" {
        return null, error("TYPE_ERROR", "parseDelimited: delimiter must be string, got " + type(delim))
    }
    return safe(_csvParseDelim, data, delim)
}

// ============================================================================
// STREAMING (async-friendly parsing)
// ============================================================================

// stream(data, delim) → Channel
// Streams CSV rows as a channel for memory-efficient processing and better parallelization.
// Each value sent to the channel is a row (array of strings).
// Channel closes automatically when parsing is complete.
// Allows overlapping parsing and worker processing — workers start as rows are parsed.
//
// CRITICAL: Check for errors by testing if a received value is an error type.
// If a row is an error, stop processing and close the channel.
//
// Example:
//   rowChannel = stream(csvData, ",")
//   rows = 0
//   for row <- rowChannel {
//       if type(row) == "ERROR" {
//           println("Parse error: " + row["message"])
//           break
//       }
//       rows = rows + 1
//   }
//   println("Processed " + str(rows) + " rows")
//
// For parallel processing with N workers:
//   rowChannel = stream(csvData, ",")
//   t1 = async(fn() { processRows(rowChannel, 0, 4) })
//   t2 = async(fn() { processRows(rowChannel, 4, 8) })
//   // Both workers read from same channel, automatically load-balanced
fn stream(data, delim) {
    if type(data) != "STRING" {
        return error("TYPE_ERROR", "stream: data must be string, got " + type(data))
    }
    if type(delim) != "STRING" {
        return error("TYPE_ERROR", "stream: delimiter must be string, got " + type(delim))
    }
    return _csvStream(data, delim)
}

// ============================================================================
// FORMATTING
// ============================================================================

// format(rows) → (csv_string, error)
// Formats array of arrays as CSV (comma-delimited).
// Returns CSV-formatted string.
// Each inner array becomes a row; each string in a row becomes a field.
//
// Example:
//   rows = [["a", "b"], ["1", "2"]]
//   csv, err = format(rows)
//   println(csv)  // "a,b\n1,2\n"
fn format(rows) {
    if type(rows) != "ARRAY" {
        return null, error("TYPE_ERROR", "format expects array, got " + type(rows))
    }
    return safe(_csvFormat, rows)
}

// formatTSV(rows) → (tsv_string, error)
// Formats array of arrays as TSV (tab-delimited).
// Returns TSV-formatted string.
//
// Example:
//   rows = [["a", "b"], ["1", "2"]]
//   tsv, err = formatTSV(rows)
fn formatTSV(rows) {
    if type(rows) != "ARRAY" {
        return null, error("TYPE_ERROR", "formatTSV expects array, got " + type(rows))
    }
    return safe(_tsvFormat, rows)
}

// formatDelimited(rows, delim) → (string, error)
// Formats array of arrays with custom delimiter.
// Returns formatted string using delim (first character of delim string).
//
// Example:
//   rows = [["a", "b"], ["1", "2"]]
//   piped, err = formatDelimited(rows, "|")
fn formatDelimited(rows, delim) {
    if type(rows) != "ARRAY" {
        return null, error("TYPE_ERROR", "formatDelimited: rows must be array, got " + type(rows))
    }
    if type(delim) != "STRING" {
        return null, error("TYPE_ERROR", "formatDelimited: delimiter must be string, got " + type(delim))
    }
    return safe(_csvFormatDelim, rows, delim)
}

// ============================================================================
// HEADER-AWARE PARSING
// ============================================================================

// parseWithHeaders(data) → (records, error)
// Parses CSV treating first row as column headers.
// Returns array of hashes, where each hash maps column name → cell value.
// Handles ragged rows gracefully (short rows padded with empty strings).
//
// Example:
//   data = "name,age\nAlice,30\nBob,25"
//   records, err = parseWithHeaders(data)
//   println(records[0]["name"])  // "Alice"
fn parseWithHeaders(data) {
    if type(data) != "STRING" {
        return null, error("TYPE_ERROR", "parseWithHeaders expects string, got " + type(data))
    }
    return safe(_csvParseHeaders, data)
}

// headers(data) → (header_row, error)
// Extracts the header row (first row) from CSV data.
// Returns the first row as an array of strings.
// Returns empty array if data has no rows.
//
// Example:
//   data = "name,age,city\nAlice,30,NYC"
//   cols, err = headers(data)
//   println(cols[0])  // "name"
fn headers(data) {
    if type(data) != "STRING" {
        return null, error("TYPE_ERROR", "headers expects string, got " + type(data))
    }
    rows, err = safe(_csvParse, data)
    if err != null {
        return null, err
    }
    if len(rows) == 0 {
        return [], null
    }
    return rows[0], null
}

// dataRows(data) → (rows, error)
// Extracts all rows except the first (header) row from CSV data.
// Returns array of arrays (rows without the header).
// Returns empty array if data has only a header or is empty.
//
// Example:
//   data = "name,age\nAlice,30\nBob,25"
//   rows, err = dataRows(data)
//   println(len(rows))  // 2 (Alice and Bob)
fn dataRows(data) {
    if type(data) != "STRING" {
        return null, error("TYPE_ERROR", "dataRows expects string, got " + type(data))
    }
    rows, err = safe(_csvParse, data)
    if err != null {
        return null, err
    }
    if len(rows) <= 1 {
        return [], null
    }

    // Extract rows[1:] manually using while loop
    dataLen = len(rows) - 1
    result = makeArray(dataLen, null)
    i = 1
    j = 0
    while i < len(rows) {
        result[j] = rows[i]
        i = i + 1
        j = j + 1
    }
    return result, null
}

// ============================================================================
// UTILITIES
// ============================================================================

// rowCount(data) → (count, error)
// Returns the number of rows in CSV data.
// Counts all rows including the header row (if present).
//
// Example:
//   data = "a,b\n1,2\n3,4"
//   count, err = rowCount(data)
//   println(count)  // 3
fn rowCount(data) {
    if type(data) != "STRING" {
        return null, error("TYPE_ERROR", "rowCount expects string, got " + type(data))
    }
    rows, err = safe(_csvParse, data)
    if err != null {
        return null, err
    }
    return len(rows), null
}

// columnCount(data) → (count, error)
// Returns the number of columns in CSV data.
// Counts columns in the first row (or 0 if data is empty).
// Optimized: only parses first row (early exit).
//
// Example:
//   data = "a,b,c\n1,2,3"
//   cols, err = columnCount(data)
//   println(cols)  // 3
fn columnCount(data) {
    if type(data) != "STRING" {
        return null, error("TYPE_ERROR", "columnCount expects string, got " + type(data))
    }
    return safe(_csvFirstRowCols, data, ",")
}

// column(rows, index) → (col_array, error)
// Extracts a single column (by index) from parsed rows.
// Returns array of values from that column.
// For rows shorter than the requested column, returns null values.
//
// Example:
//   rows = [["a", "b"], ["1", "2"], ["3", "4"]]
//   col, err = column(rows, 1)
//   println(col[0])  // "b"
fn column(rows, index) {
    if type(rows) != "ARRAY" {
        return null, error("TYPE_ERROR", "column: rows must be array, got " + type(rows))
    }
    if type(index) != "INTEGER" {
        return null, error("TYPE_ERROR", "column: index must be integer, got " + type(index))
    }

    result = makeArray(len(rows), null)
    i = 0
    while i < len(rows) {
        row = rows[i]
        if type(row) == "ARRAY" && index < len(row) {
            result[i] = row[index]
        } else {
            result[i] = null
        }
        i = i + 1
    }
    return result, null
}

// isEmpty(data) → boolean
// Returns true if CSV data is empty (no rows).
// Returns true on parse error.
// Useful for checking if data exists before processing.
// Optimized: checks string length first, then only parses until first row.
//
// Example:
//   if isEmpty(data) {
//       println("No data to process")
//   }
fn isEmpty(data) {
    if type(data) != "STRING" {
        return true
    }
    if len(data) == 0 {
        return true
    }
    hasRows, err = safe(_csvHasRows, data, ",")
    if err != null {
        return true
    }
    return !hasRows
}
