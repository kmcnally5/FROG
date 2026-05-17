// bitwiseTest.lex — unit tests for bitwise builtins and base-prefixed literals

passed = 0
failed = 0

fn check(name, got, expected) {
    if got == expected {
        passed = passed + 1
    } else {
        failed = failed + 1
        println("FAIL: " + name + " — expected " + str(expected) + " got " + str(got))
    }
}

// ── Base-prefixed integer literals ──────────────────────────────────────────

check("hex 0xFF",         0xFF,        255)
check("hex 0x0F",         0x0F,        15)
check("hex uppercase",    0XFF,        255)
check("hex 0xDEAD",       0xDEAD,      57005)
check("binary 0b1010",    0b1010,      10)
check("binary 0b11110000",0b11110000,  240)
check("binary uppercase", 0B1010,      10)
check("octal 0o755",      0o755,       493)
check("octal 0o111",      0o111,       73)
check("octal uppercase",  0O777,       511)
check("zero literal",     0,           0)

// ── bitAnd ──────────────────────────────────────────────────────────────────

check("bitAnd 0b1100 & 0b1010",  bitAnd(0b1100, 0b1010),  8)
check("bitAnd 0xFF & 0x0F",      bitAnd(0xFF, 0x0F),       15)
check("bitAnd anything & 0",     bitAnd(0xDEAD, 0),        0)
check("bitAnd anything & -1",    bitAnd(42, -1),            42)

// ── bitOr ───────────────────────────────────────────────────────────────────

check("bitOr 0b1100 | 0b0011",   bitOr(0b1100, 0b0011),   15)
check("bitOr 0 | 0",             bitOr(0, 0),               0)
check("bitOr identity",          bitOr(42, 0),              42)

// ── bitXor ──────────────────────────────────────────────────────────────────

check("bitXor 0b1100 ^ 0b1010",  bitXor(0b1100, 0b1010),  6)
check("bitXor self == 0",        bitXor(0xFF, 0xFF),        0)
check("bitXor with 0",           bitXor(42, 0),             42)

// ── bitNot ──────────────────────────────────────────────────────────────────

check("bitNot 0",    bitNot(0),   -1)
check("bitNot -1",   bitNot(-1),  0)
check("bitNot 1",    bitNot(1),   -2)

// ── bitShiftLeft ─────────────────────────────────────────────────────────────

check("bitShiftLeft 1 << 4",    bitShiftLeft(1, 4),    16)
check("bitShiftLeft 3 << 8",    bitShiftLeft(3, 8),    768)
check("bitShiftLeft 1 << 0",    bitShiftLeft(1, 0),    1)

// ── bitShiftRight ────────────────────────────────────────────────────────────

check("bitShiftRight 16 >> 4",   bitShiftRight(16, 4),   1)
check("bitShiftRight 256 >> 3",  bitShiftRight(256, 3),  32)
check("bitShiftRight 42 >> 0",   bitShiftRight(42, 0),   42)

// ── Practical examples ───────────────────────────────────────────────────────

// File permission check
let mode = 0o755
check("exec bit set",   bitAnd(mode, 0o111) != 0,  true)
check("write bit set",  bitAnd(mode, 0o222) != 0,  true)

// Flag manipulation
const READ  = 0b001
const WRITE = 0b010
const EXEC  = 0b100
let perms = bitOr(READ, WRITE)
check("READ|WRITE",          perms,                           3)
check("has READ",            bitAnd(perms, READ) != 0,        true)
check("has EXEC — false",    bitAnd(perms, EXEC) != 0,        false)
let toggled = bitXor(perms, WRITE)
check("toggle WRITE off",    bitAnd(toggled, WRITE) != 0,     false)

// Nibble extraction
let byte_val = 0xAB
let hi_nibble = bitShiftRight(byte_val, 4)
let lo_nibble = bitAnd(byte_val, 0x0F)
check("high nibble of 0xAB",  hi_nibble,  10)
check("low nibble of 0xAB",   lo_nibble,  11)

// ── Summary ──────────────────────────────────────────────────────────────────

println("bitwise: " + str(passed) + " passed, " + str(failed) + " failed")
