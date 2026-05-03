// ============================================================================
// csvfrog.lex — CSV parser written entirely in FROG (recursive descent)
// ============================================================================
//
// Pure kLex implementation of RFC 4180 CSV parsing using character-by-character
// state machine (DFA). No Go builtins — demonstrates language capability.
//
// Design philosophy:
// - Single-pass character walker with explicit state tracking
// - Handles quoted fields with embedded delimiters/newlines
// - Handles escaped quotes ("" → single ")
// - Respects both \r\n and \n line endings
// - Does NOT trim spaces (they're part of field values per RFC 4180)
// - Returns (records, error) tuples consistently
// - RFC 4180 compliant: quotes only allowed at field start
//
// Example:
//   data = "name,\"quoted, value\"\nAlice,\"hello\nworld\""
//   records, err = parse(data)
//   // records[0][1] == "quoted, value"
//   // records[1][1] == "hello\nworld"

// ============================================================================
// CORE PARSER
// ============================================================================

// parse(text) → (records, error)
// Main entry point: parses CSV text (comma-delimited, handles quotes/newlines).
// Returns array of arrays (rows of fields) or error.
//
// Handles:
//   - Quoted fields: "field,with,commas" is ONE field
//   - Escaped quotes: "" for literal quote in quoted field
//   - Embedded newlines: "line\none" in quoted field is single field with newline
//   - Line endings: Both \r\n (Windows) and \n (Unix)
//   - Empty fields: "",,"" is three fields (first and third empty)
//   - RFC 4180 compliance: quotes only at field start
//
// Example:
//   csv = "a,b\n\"c,d\",e"
//   rows, err = parse(csv)
//   // rows[0] = ["a", "b"]
//   // rows[1] = ["c,d", "e"]
fn parse(text) {
    if type(text) != "STRING" {
        return null, error("TYPE_ERROR", "parse expects string, got " + type(text))
    }
    return parseCSVDelim(text, ",")
}

// parseTSV(text) → (records, error)
// TSV variant: parses tab-separated values.
//
// Example:
//   tsv = "a\tb\nc\td"
//   rows, err = parseTSV(tsv)
fn parseTSV(text) {
    if type(text) != "STRING" {
        return null, error("TYPE_ERROR", "parseTSV expects string, got " + type(text))
    }
    return parseCSVDelim(text, "\t")
}

// parseDelimited(text, delim) → (records, error)
// Custom delimiter variant. Uses first character of delim string.
// delim: single character (or multi-char string, first char used as delimiter)
//
// Example:
//   data = "a|b|c\n1|2|3"
//   rows, err = parseDelimited(data, "|")
fn parseDelimited(text, delim) {
    if type(text) != "STRING" {
        return null, error("TYPE_ERROR", "parseDelimited: text must be string, got " + type(text))
    }
    if type(delim) != "STRING" {
        return null, error("TYPE_ERROR", "parseDelimited: delimiter must be string, got " + type(delim))
    }
    if len(delim) == 0 {
        return null, error("INVALID_ARG", "delimiter must not be empty")
    }
    return parseCSVDelim(text, delim)
}

// ============================================================================
// INTERNAL PARSER STATE MACHINE
// ============================================================================

// parseCSVDelim(text, delim) → (records, error)
// Core state machine accepting custom delimiter (first character used).
// Implements RFC 4180 with generalized delimiter support.
//
// State machine with explicit tracking:
//   - inQuotes: boolean, are we inside a quoted field?
//   - currentField: string accumulator for the current field
//   - currentRecord: array accumulator for the current record
//   - records: final array of all records
//   - i: position in text
//   - delimChar: first character of delimiter
// Helper: finalize current field (join array of chars back to string)
fn finalizeField(fieldChars) {
    return join(fieldChars, "")
}

fn parseCSVDelim(text, delim) {
    records = makeArray(len(text) / 10 + 20, null)
    recordIdx = 0
    currentRecord = makeArray(50, null)
    recordFieldIdx = 0
    currentField = makeArray(len(text), null)
    fieldCharIdx = 0
    inQuotes = false
    i = 0
    delimChar = substr(delim, 0, 1)
    textChars = split(text, "")

    while i < len(textChars) {
        char = textChars[i]

        if inQuotes {
            // Inside a quoted field: only quotes and regular chars matter
            if char == "\"" {
                // Potential end of quoted field OR escaped quote
                if i + 1 < len(textChars) && textChars[i + 1] == "\"" {
                    // Escaped quote: "" → "
                    currentField[fieldCharIdx] = "\""
                    fieldCharIdx = fieldCharIdx + 1
                    i = i + 2
                } else {
                    // End of quoted field
                    inQuotes = false
                    i = i + 1
                }
            } else {
                // Regular character inside quotes (including comma, newline, etc.)
                currentField[fieldCharIdx] = char
                fieldCharIdx = fieldCharIdx + 1
                i = i + 1
            }
        } else {
            // Outside quoted field: delimiters and newlines terminate fields
            if char == "\"" {
                // RFC 4180: Quotes only allowed at field start
                if fieldCharIdx > 0 {
                    return null, error("PARSE_ERROR", "Unexpected quote character in unquoted field (quotes only allowed at field start)")
                }
                // Start of quoted field
                inQuotes = true
                i = i + 1
            } else if char == delimChar {
                // Field separator (comma, tab, etc.)
                fieldStr = join(slice(currentField, 0, fieldCharIdx), "")
                currentRecord[recordFieldIdx] = fieldStr
                recordFieldIdx = recordFieldIdx + 1
                fieldCharIdx = 0
                i = i + 1
            } else if char == "\n" {
                // Newline (Unix or second char of \r\n)
                // Only add record if it has content
                fieldStr = join(slice(currentField, 0, fieldCharIdx), "")
                if fieldStr != "" || recordFieldIdx > 0 {
                    currentRecord[recordFieldIdx] = fieldStr
                    recordFieldIdx = recordFieldIdx + 1
                    records[recordIdx] = slice(currentRecord, 0, recordFieldIdx)
                    recordIdx = recordIdx + 1
                }
                recordFieldIdx = 0
                fieldCharIdx = 0
                i = i + 1
            } else if char == "\r" {
                // Carriage return (Windows or Mac line ending)
                // Check for \r\n (Windows) vs bare \r (old Mac)
                if i + 1 < len(textChars) && textChars[i + 1] == "\n" {
                    // \r\n: skip both
                    i = i + 2
                } else {
                    // Bare \r: treat as line ending
                    i = i + 1
                }
                // Only add record if it has content
                fieldStr = join(slice(currentField, 0, fieldCharIdx), "")
                if fieldStr != "" || recordFieldIdx > 0 {
                    currentRecord[recordFieldIdx] = fieldStr
                    recordFieldIdx = recordFieldIdx + 1
                    records[recordIdx] = slice(currentRecord, 0, recordFieldIdx)
                    recordIdx = recordIdx + 1
                }
                recordFieldIdx = 0
                fieldCharIdx = 0
            } else {
                // Regular character: accumulate into current field
                currentField[fieldCharIdx] = char
                fieldCharIdx = fieldCharIdx + 1
                i = i + 1
            }
        }
    }

    // Handle any remaining field and record at end of text
    // (e.g., no trailing newline)
    fieldStr = join(slice(currentField, 0, fieldCharIdx), "")
    if fieldStr != "" || recordFieldIdx > 0 {
        currentRecord[recordFieldIdx] = fieldStr
        recordFieldIdx = recordFieldIdx + 1
        if recordFieldIdx > 0 {
            records[recordIdx] = slice(currentRecord, 0, recordFieldIdx)
            recordIdx = recordIdx + 1
        }
    }

    // If we ended in a quoted field, that's a parse error
    if inQuotes {
        return null, error("PARSE_ERROR", "CSV: unclosed quoted field at end of input")
    }

    return slice(records, 0, recordIdx), null
}

// ============================================================================
// UTILITIES (FROG-BASED)
// ============================================================================

// rowCount(text) → (count, error)
// Count rows in CSV text.
fn rowCount(text) {
    if type(text) != "STRING" {
        return null, error("TYPE_ERROR", "rowCount expects string, got " + type(text))
    }
    records, err = parse(text)
    if err != null {
        return null, err
    }
    return len(records), null
}

// columnCount(text) → (count, error)
// Count columns (fields in first row).
fn columnCount(text) {
    if type(text) != "STRING" {
        return null, error("TYPE_ERROR", "columnCount expects string, got " + type(text))
    }
    records, err = parse(text)
    if err != null {
        return null, err
    }
    if len(records) == 0 {
        return 0, null
    }
    return len(records[0]), null
}

// isEmpty(text) → boolean
// Quick check: is the CSV empty (no rows)?
fn isEmpty(text) {
    if type(text) != "STRING" {
        return true
    }
    records, err = parse(text)
    if err != null {
        return true
    }
    return len(records) == 0
}

// column(records, index) → (col_array, error)
// Extract a single column (by index) from parsed records.
// Returns array with values from that column, or null for missing cells.
// Validates index >= 0.
fn column(records, index) {
    if type(records) != "ARRAY" {
        return null, error("TYPE_ERROR", "column: records must be array, got " + type(records))
    }
    if type(index) != "INTEGER" {
        return null, error("TYPE_ERROR", "column: index must be integer, got " + type(index))
    }
    if index < 0 {
        return null, error("INVALID_ARG", "column index must be non-negative")
    }

    result = makeArray(len(records), null)
    i = 0
    while i < len(records) {
        row = records[i]
        if type(row) == "ARRAY" && index < len(row) {
            result[i] = row[index]
        } else {
            result[i] = null
        }
        i = i + 1
    }
    return result, null
}

// ============================================================================
// FORMATTING (Inverse operations)
// ============================================================================

// Helper: format field for output (quote and escape in single pass)
// Returns (formattedField, needsQuoting) — allows caller to decide wrapping
fn formatField(field, quoteChar, delimChar) {
    needsQuoting = false
    escaped = ""
    i = 0

    while i < len(field) {
        c = substr(field, i, i + 1)
        if c == delimChar || c == "\n" || c == "\r" || c == quoteChar {
            needsQuoting = true
        }
        if c == quoteChar {
            escaped = escaped + "\"" + "\""
        } else {
            escaped = escaped + c
        }
        i = i + 1
    }

    return escaped, needsQuoting
}

// format(records) → (csv_string, error)
// Inverse operation: convert array of arrays back to CSV string (comma-delimited).
// Quotes fields that contain delimiters, newlines, or quotes.
// Escapes quotes inside quoted fields by doubling.
// Converts null values to empty strings.
fn format(records) {
    if type(records) != "ARRAY" {
        return null, error("TYPE_ERROR", "format expects array, got " + type(records))
    }

    lines = makeArray(len(records), null)
    i = 0
    while i < len(records) {
        record = records[i]
        if type(record) != "ARRAY" {
            return null, error("TYPE_ERROR", "format: record must be array, got " + type(record))
        }

        fields = makeArray(len(record), null)
        j = 0
        while j < len(record) {
            field = record[j]

            // Convert nulls to empty strings
            if field == null {
                field = ""
            } else if type(field) != "STRING" {
                return null, error("TYPE_ERROR", "format: field must be string or null, got " + type(field))
            }

            // Format field (escape and check quoting need in one pass)
            escaped, needsQuoting = formatField(field, "\"", ",")
            if needsQuoting {
                fields[j] = "\"" + escaped + "\""
            } else {
                fields[j] = field
            }

            j = j + 1
        }

        // Join fields with comma
        line = join(fields, ",")
        lines[i] = line
        i = i + 1
    }

    // Join records with newline
    return join(lines, "\n"), null
}

// formatTSV(records) → (tsv_string, error)
// TSV variant: use tab as delimiter.
// Quotes fields containing tabs, newlines, CR, or quotes.
fn formatTSV(records) {
    if type(records) != "ARRAY" {
        return null, error("TYPE_ERROR", "formatTSV expects array, got " + type(records))
    }

    lines = makeArray(len(records), null)
    i = 0
    while i < len(records) {
        record = records[i]
        if type(record) != "ARRAY" {
            return null, error("TYPE_ERROR", "formatTSV: record must be array, got " + type(record))
        }

        fields = makeArray(len(record), null)
        j = 0
        while j < len(record) {
            field = record[j]

            // Convert nulls to empty strings
            if field == null {
                field = ""
            } else if type(field) != "STRING" {
                return null, error("TYPE_ERROR", "formatTSV: field must be string or null, got " + type(field))
            }

            // Format field and quote if necessary (tab, newline, CR, or quotes)
            escaped, needsQuoting = formatField(field, "\"", "\t")
            if needsQuoting {
                fields[j] = "\"" + escaped + "\""
            } else {
                fields[j] = field
            }

            j = j + 1
        }

        line = join(fields, "\t")
        lines[i] = line
        i = i + 1
    }

    return join(lines, "\n"), null
}

// formatDelimited(records, delim) → (string, error)
// Custom delimiter variant. Uses first character of delim string.
// Quotes fields containing the delimiter, newlines, CR, or quotes.
fn formatDelimited(records, delim) {
    if type(records) != "ARRAY" {
        return null, error("TYPE_ERROR", "formatDelimited: records must be array, got " + type(records))
    }
    if type(delim) != "STRING" {
        return null, error("TYPE_ERROR", "formatDelimited: delimiter must be string, got " + type(delim))
    }
    if len(delim) == 0 {
        return null, error("INVALID_ARG", "delimiter must not be empty")
    }

    delimChar = substr(delim, 0, 1)

    lines = makeArray(len(records), null)
    i = 0
    while i < len(records) {
        record = records[i]
        if type(record) != "ARRAY" {
            return null, error("TYPE_ERROR", "formatDelimited: record must be array, got " + type(record))
        }

        fields = makeArray(len(record), null)
        j = 0
        while j < len(record) {
            field = record[j]

            // Convert nulls to empty strings
            if field == null {
                field = ""
            } else if type(field) != "STRING" {
                return null, error("TYPE_ERROR", "formatDelimited: field must be string or null, got " + type(field))
            }

            // Format field (escape and check quoting need in one pass)
            escaped, needsQuoting = formatField(field, "\"", delimChar)
            if needsQuoting {
                fields[j] = "\"" + escaped + "\""
            } else {
                fields[j] = field
            }

            j = j + 1
        }

        line = join(fields, delimChar)
        lines[i] = line
        i = i + 1
    }

    return join(lines, "\n"), null
}
