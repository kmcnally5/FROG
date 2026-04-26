// stdlib/encoding.lex — ASCII encoding utilities (printable range 32–126)
//
// Usage:
//   import "encoding.lex" as enc
//   println(enc.ord("A"))    // 65
//   println(enc.chr(65))     // A

START = 32

ASCII_STR = " !\"#$%&'()*+,-./0123456789:;<=>?@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_`abcdefghijklmnopqrstuvwxyz\{|}~"

// ord returns the ASCII code of c. Returns 63 ('?') for out-of-range characters.
fn ord(c) {
    i = indexOf(ASCII_STR, c)
    if i == -1 {
        return 63
    }
    return START + i
}

// chr returns the character for ASCII code n, or null if out of range.
fn chr(n) {
    i = n - START
    if i < 0 { return null }
    if i >= len(ASCII_STR) { return null }
    return ASCII_STR[i]
}

// bytes converts a string to an array of ASCII codes.
fn bytes(s) {
    out = []
    i = 0
    while i < len(s) {
        out = push(out, ord(s[i]))
        i = i + 1
    }
    return out
}

// stringFromBytes converts an array of ASCII codes back to a string.
// Out-of-range codes become '?'.
fn stringFromBytes(arr) {
    out = ""
    i = 0
    while i < len(arr) {
        c = chr(arr[i])
        if c == null {
            out = out + "?"
        } else {
            out = out + c
        }
        i = i + 1
    }
    return out
}
