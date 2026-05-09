// Test hex escape sequences \xHH

// Basic hex escapes
esc = "\x1b"
if len(esc) != 1 {
    println("FAIL: \\x1b should be 1 byte")
} else {
    println("PASS: \\x1b is 1 byte")
}

// Hex escapes in ANSI codes
red = "\x1b[31m"
if indexOf(red, "31") >= 0 {
    println("PASS: ANSI red code created")
} else {
    println("FAIL: ANSI red code not found")
}

// 256-color palette code
orange = "\x1b[38;5;208m"
if indexOf(orange, "208") >= 0 {
    println("PASS: 256-color code created")
} else {
    println("FAIL: 256-color code not found")
}

// True color RGB code
purple = "\x1b[38;2;128;0;255m"
if indexOf(purple, "128") >= 0 && indexOf(purple, "255") >= 0 {
    println("PASS: True color RGB code created")
} else {
    println("FAIL: True color code not found")
}

// Invalid hex (should preserve as literal)
invalid = "\xZZ"
if len(invalid) == 4 {  // \xZZ = 4 characters
    println("PASS: Invalid hex preserved as literal")
} else {
    println("FAIL: Invalid hex not preserved correctly")
}

// Test with colorize builtin
result = colorize("Test", "\x1b[32m")
if indexOf(result, "32m") >= 0 {
    println("PASS: colorize() works with hex escapes")
} else {
    println("FAIL: colorize() failed with hex escapes")
}

println("")
println("All hex escape tests completed")
