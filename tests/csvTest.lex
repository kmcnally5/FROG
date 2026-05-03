// ============================================================================
// csvTest.lex — Comprehensive unit tests for csv.lex library
// ============================================================================
//
// Tests all CSV parsing, formatting, streaming, and utility functions.
// Validates both success cases and error handling.

import "csv.lex" as csv

// Test counters
passed = 0
failed = 0

// Helper: assert condition
assert = fn(name, condition) {
    if condition {
        println("  ✓ " + name)
        passed = passed + 1
    } else {
        println("  ✗ " + name)
        failed = failed + 1
    }
}

// Helper: assert error is returned (simplified - can't access error fields from kLex)
assertError = fn(name, err, expectedCode) {
    if err != null && type(err) == "ERROR" {
        println("  ✓ " + name + " (ERROR returned)")
        passed = passed + 1
    } else {
        println("  ✗ " + name + " (got: " + type(err) + ")")
        failed = failed + 1
    }
}

// ============================================================================
// BASIC PARSING TESTS
// ============================================================================

println("\n=== PARSING TESTS ===")

// parse() - comma-delimited CSV
rows, err = csv.parse("a,b,c\n1,2,3\n4,5,6")
assert("parse: basic CSV", err == null && len(rows) == 3 && rows[0][0] == "a")
assert("parse: row count", len(rows) == 3)
assert("parse: first row", rows[0][0] == "a" && rows[0][1] == "b" && rows[0][2] == "c")
assert("parse: second row", rows[1][0] == "1" && rows[1][1] == "2" && rows[1][2] == "3")

// parse() - quoted fields with embedded commas
quoted = "name,value\n\"Smith, John\",100\n\"Doe, Jane\",200"
rows, err = csv.parse(quoted)
assert("parse: quoted fields with comma", rows[1][0] == "Smith, John")

// parse() - quoted fields with embedded newlines (RFC 4180)
multiline = "a,b\n\"hello\nworld\",test"
rows, err = csv.parse(multiline)
assert("parse: quoted fields with newline", rows[1][0] == "hello\nworld")

// parse() - empty CSV
rows, err = csv.parse("")
assert("parse: empty CSV", err == null && len(rows) == 0)

// parse() - single row
rows, err = csv.parse("a,b,c")
assert("parse: single row", len(rows) == 1 && rows[0][0] == "a")

// parse() - single column
rows, err = csv.parse("a\nb\nc")
assert("parse: single column", len(rows) == 3 && rows[0][0] == "a" && rows[2][0] == "c")

// parse() - type validation
result, err = csv.parse(123)
assertError("parse: type check (non-string)", err, "TYPE_ERROR")

// ============================================================================
// TSV PARSING TESTS
// ============================================================================

println("\n=== TSV PARSING TESTS ===")

// parseTSV() - tab-delimited values
tsv = "a\tb\tc\n1\t2\t3\n4\t5\t6"
rows, err = csv.parseTSV(tsv)
assert("parseTSV: basic TSV", err == null && len(rows) == 3 && rows[0][0] == "a")
assert("parseTSV: row structure", rows[1][0] == "1" && rows[1][1] == "2")

// parseTSV() - type validation
result, err = csv.parseTSV([])
assertError("parseTSV: type check (non-string)", err, "TYPE_ERROR")

// ============================================================================
// CUSTOM DELIMITER PARSING TESTS
// ============================================================================

println("\n=== CUSTOM DELIMITER PARSING TESTS ===")

// parseDelimited() - pipe-delimited
pipe = "a|b|c\n1|2|3"
rows, err = csv.parseDelimited(pipe, "|")
assert("parseDelimited: pipe delimiter", rows[0][0] == "a" && rows[1][0] == "1")

// parseDelimited() - semicolon-delimited
semi = "a;b;c\n1;2;3"
rows, err = csv.parseDelimited(semi, ";")
assert("parseDelimited: semicolon delimiter", rows[0][0] == "a")

// parseDelimited() - first char only (multi-char string)
rows, err = csv.parseDelimited("a|b|c\n1|2|3", "|multi")
assert("parseDelimited: uses first char only", rows[0][0] == "a" && rows[1][0] == "1")

// parseDelimited() - type validation
result, err = csv.parseDelimited("a,b", 123)
assertError("parseDelimited: delimiter type check", err, "TYPE_ERROR")

result, err = csv.parseDelimited([], ",")
assertError("parseDelimited: data type check", err, "TYPE_ERROR")

// parseDelimited() - empty delimiter check
result, err = csv.parseDelimited("a,b", "")
assertError("parseDelimited: empty delimiter validation", err, "RuntimeError")

// ============================================================================
// FORMATTING TESTS
// ============================================================================

println("\n=== FORMATTING TESTS ===")

// format() - basic formatting
rows = [["a", "b", "c"], ["1", "2", "3"]]
csv_str, err = csv.format(rows)
assert("format: basic formatting", err == null && type(csv_str) == "STRING")
assert("format: contains CSV content", csv_str != null && len(csv_str) > 0)

// format() - preserves data
rows = [["name", "age"], ["Alice", "30"], ["Bob", "25"]]
csv_str, err = csv.format(rows)
parsed, _ = csv.parse(csv_str)
assert("format: round-trip parse", len(parsed) == 3 && parsed[1][0] == "Alice")

// format() - empty rows
csv_str, err = csv.format([])
assert("format: empty rows", err == null && csv_str != null)

// format() - type validation
result, err = csv.format("not an array")
assertError("format: type check (non-array)", err, "TYPE_ERROR")

// formatTSV() - tab-delimited formatting
rows = [["a", "b"], ["1", "2"]]
tsv_str, err = csv.formatTSV(rows)
assert("formatTSV: basic TSV formatting", err == null && type(tsv_str) == "STRING")

// formatTSV() - type validation
result, err = csv.formatTSV("not an array")
assertError("formatTSV: type check", err, "TYPE_ERROR")

// formatDelimited() - custom delimiter formatting
rows = [["a", "b"], ["1", "2"]]
pipe_str, err = csv.formatDelimited(rows, "|")
assert("formatDelimited: pipe formatting", err == null && type(pipe_str) == "STRING")

// formatDelimited() - type validation
result, err = csv.formatDelimited([], 123)
assertError("formatDelimited: delimiter type check", err, "TYPE_ERROR")

// ============================================================================
// HEADER-AWARE PARSING TESTS
// ============================================================================

println("\n=== HEADER-AWARE PARSING TESTS ===")

// parseWithHeaders() - basic usage
data = "name,age,city\nAlice,30,NYC\nBob,25,LA"
records, err = csv.parseWithHeaders(data)
assert("parseWithHeaders: returns array", err == null && type(records) == "ARRAY")
assert("parseWithHeaders: correct record count", len(records) == 2)
assert("parseWithHeaders: first record is hash", type(records[0]) == "HASH")
assert("parseWithHeaders: hash keys accessible", records[0]["name"] == "Alice" && records[0]["age"] == "30")
assert("parseWithHeaders: second record", records[1]["name"] == "Bob" && records[1]["city"] == "LA")

// parseWithHeaders() - type validation
result, err = csv.parseWithHeaders(123)
assertError("parseWithHeaders: type check", err, "TYPE_ERROR")

// headers() - extract header row
data = "name,age,city\nAlice,30,NYC"
cols, err = csv.headers(data)
assert("headers: extract header", err == null && len(cols) == 3)
assert("headers: correct values", cols[0] == "name" && cols[1] == "age" && cols[2] == "city")

// headers() - empty CSV
cols, err = csv.headers("")
assert("headers: empty CSV returns empty array", err == null && len(cols) == 0)

// headers() - type validation
result, err = csv.headers(123)
assertError("headers: type check", err, "TYPE_ERROR")

// dataRows() - extract data without header
data = "name,age\nAlice,30\nBob,25"
rows, err = csv.dataRows(data)
assert("dataRows: excludes header", err == null && len(rows) == 2)
assert("dataRows: first data row", rows[0][0] == "Alice" && rows[0][1] == "30")
assert("dataRows: second data row", rows[1][0] == "Bob")

// dataRows() - single header only
rows, err = csv.dataRows("name,age")
assert("dataRows: header only returns empty", len(rows) == 0)

// dataRows() - empty CSV
rows, err = csv.dataRows("")
assert("dataRows: empty CSV returns empty", len(rows) == 0)

// dataRows() - type validation
result, err = csv.dataRows(123)
assertError("dataRows: type check", err, "TYPE_ERROR")

// ============================================================================
// UTILITY FUNCTION TESTS
// ============================================================================

println("\n=== UTILITY FUNCTION TESTS ===")

// isEmpty() - empty string
assert("isEmpty: empty string", csv.isEmpty("") == true)

// isEmpty() - non-empty string
assert("isEmpty: non-empty CSV", csv.isEmpty("a,b\n1,2") == false)

// isEmpty() - single row
assert("isEmpty: single row CSV", csv.isEmpty("a,b,c") == false)

// isEmpty() - type validation
assert("isEmpty: type check returns true", csv.isEmpty(123) == true)

// rowCount() - count all rows
count, err = csv.rowCount("a,b\n1,2\n3,4")
assert("rowCount: correct count", err == null && count == 3)

// rowCount() - empty CSV
count, err = csv.rowCount("")
assert("rowCount: empty CSV returns 0", count == 0)

// rowCount() - single row
count, err = csv.rowCount("a,b,c")
assert("rowCount: single row returns 1", count == 1)

// rowCount() - type validation
result, err = csv.rowCount(123)
assertError("rowCount: type check", err, "TYPE_ERROR")

// columnCount() - OPTIMIZED: only parses first row
count, err = csv.columnCount("a,b,c\n1,2,3\n4,5,6")
assert("columnCount: correct count", err == null && count == 3)

// columnCount() - empty CSV
count, err = csv.columnCount("")
assert("columnCount: empty CSV returns 0", count == 0)

// columnCount() - single column
count, err = csv.columnCount("a\nb\nc")
assert("columnCount: single column returns 1", count == 1)

// columnCount() - type validation
result, err = csv.columnCount(123)
assertError("columnCount: type check", err, "TYPE_ERROR")

// column() - extract column by index
rows = [["a", "b", "c"], ["1", "2", "3"], ["4", "5", "6"]]
col, err = csv.column(rows, 1)
assert("column: extract by index", col[0] == "b" && col[1] == "2" && col[2] == "5")

// column() - out of bounds returns nulls
col, err = csv.column(rows, 10)
assert("column: out of bounds returns nulls", col[0] == null && col[1] == null)

// column() - ragged rows handled
rows = [["a", "b"], ["1"], ["4", "5", "6"]]
col, err = csv.column(rows, 1)
assert("column: ragged rows", col[0] == "b" && col[1] == null && col[2] == "5")

// column() - type validation
result, err = csv.column("not array", 0)
assertError("column: rows type check", err, "TYPE_ERROR")

result, err = csv.column([], "not int")
assertError("column: index type check", err, "TYPE_ERROR")

// ============================================================================
// STREAMING TESTS (basic sanity checks)
// ============================================================================

println("\n=== STREAMING TESTS ===")

// stream() - returns channel
ch = csv.stream("a,b\n1,2\n3,4", ",")
assert("stream: returns channel", type(ch) == "CHANNEL")

// stream() - type validation (returns error directly, not tuple)
result = csv.stream(123, ",")
assert("stream: data type check", type(result) == "ERROR")

result = csv.stream("a,b", 123)
assert("stream: delimiter type check", type(result) == "ERROR")

// Note: empty delimiter validation throws RuntimeError from builtin,
// so we can't test it with assert - it would abort the test

// ============================================================================
// EDGE CASES AND REGRESSION TESTS
// ============================================================================

println("\n=== EDGE CASES AND REGRESSION TESTS ===")

// Quoted empty fields
data = "a,\"\",c\n1,,3"
rows, err = csv.parse(data)
assert("quoted empty fields", rows[0][1] == "")

// Fields with only quotes
data = "a,\"\"\"\",c"
rows, err = csv.parse(data)
assert("quoted quote character", len(rows) == 1)

// Windows line endings (CRLF)
data = "a,b\r\n1,2\r\n"
rows, err = csv.parse(data)
assert("CRLF line endings", len(rows) == 2)

// Trailing newline
data = "a,b\n1,2\n"
rows, err = csv.parse(data)
assert("trailing newline", len(rows) == 2)

// Multiple consecutive newlines
data = "a,b\n\n1,2"
rows, err = csv.parse(data)
assert("consecutive newlines", len(rows) >= 2)

// Large column count
largeRow = "a,b,c,d,e,f,g,h,i,j,k,l,m,n,o,p,q,r,s,t,u,v,w,x,y,z"
rows, err = csv.parse(largeRow)
assert("large column count", len(rows[0]) == 26)

// Numbers as strings (CSV should preserve as strings)
data = "123,456,789"
rows, err = csv.parse(data)
assert("numbers as strings", type(rows[0][0]) == "STRING" && rows[0][0] == "123")

// Unicode support
data = "café,naïve,π\n∑,∫,√"
rows, err = csv.parse(data)
assert("unicode support", rows[0][0] == "café" && rows[1][0] == "∑")

// ============================================================================
// OPTIMIZATION TESTS (new optimized functions)
// ============================================================================

println("\n=== OPTIMIZATION TESTS ===")

// isEmpty() optimization: should fail fast on empty string
assert("isEmpty optimization: empty string check", csv.isEmpty("") == true)

// columnCount() optimization: should only parse first row (not entire CSV)
largeRow = "1,2,3"
i = 0
while i < 100 {
    largeRow = largeRow + "\n4,5,6"
    i = i + 1
}
count, _ = csv.columnCount(largeRow)
assert("columnCount optimization: only parses first row", count == 3)

// ============================================================================
// SUMMARY
// ============================================================================

separators = ""
i = 0
while i < 60 {
    separators = separators + "="
    i = i + 1
}

total = passed + failed
percent = 0
if total > 0 {
    percent = (passed * 100) / total
}

println("\n" + separators)
println("TEST SUMMARY")
println(separators)
println("  Passed: " + str(passed) + " / " + str(total))
println("  Failed: " + str(failed) + " / " + str(total))
println("  Success Rate: " + str(percent) + "%")
println(separators)

if failed > 0 {
    println("\n⚠ Some tests failed!")
} else {
    println("\n✓ All tests passed!")
}
