// json.lex
// Minimal JSON parser + stringifier for kLex

// --------------------------------------------------
// PUBLIC API
// --------------------------------------------------

fn parse(input) {
    p = _Parser { s: input, i: 0 }
    val, err = p.parseValue()
    if err != null { return null, err }

    p.skipWhitespace()
    if p.i != len(p.s) {
        return null, "unexpected trailing characters"
    }

    return val, null
}

fn stringify(v) {
    return _stringify(v)
}

// --------------------------------------------------
// PARSER
// --------------------------------------------------

struct _Parser {
    s, i

    fn peek() {
        if self.i >= len(self.s) { return null }
        return self.s[self.i]
    }

    fn next() {
        ch = self.peek()
        self.i = self.i + 1
        return ch
    }

    fn skipWhitespace() {
        while true {
            ch = self.peek()
            if ch == " " || ch == "\n" || ch == "\r" || ch == "\t" {
                self.i = self.i + 1
            } else {
                break
            }
        }
    }

    fn parseValue() {
        self.skipWhitespace()
        ch = self.peek()

        if ch == "\"" { return self.parseString() }
        if ch == "\{" { return self.parseObject() }
        if ch == "["  { return self.parseArray() }
        if ch == "t"  { return self.parseTrue() }
        if ch == "f"  { return self.parseFalse() }
        if ch == "n"  { return self.parseNull() }
        return self.parseNumber()
    }

    // ---------- literals ----------

    fn parseTrue() {
        if substr(self.s, self.i, self.i + 4) == "true" {
            self.i = self.i + 4
            return true, null
        }
        return null, "invalid token"
    }

    fn parseFalse() {
        if substr(self.s, self.i, self.i + 5) == "false" {
            self.i = self.i + 5
            return false, null
        }
        return null, "invalid token"
    }

    fn parseNull() {
        if substr(self.s, self.i, self.i + 4) == "null" {
            self.i = self.i + 4
            return null, null
        }
        return null, "invalid token"
    }

    // ---------- string ----------

    fn parseString() {
        if self.next() != "\"" {
            return null, "expected quote"
        }

        // Pre-allocate a char buffer to the remaining input length (upper
        // bound on the parsed string's length, since escapes collapse 2→1).
        // Each char written by index; final assembly is one join() —
        // O(n) instead of O(n²) from repeated string concat.
        cap = len(self.s) - self.i
        buf = makeArray(cap, "")
        n   = 0

        while true {
            ch = self.next()
            if ch == null { return null, "unterminated string" }

            if ch == "\"" { break }

            if ch == "\\" {
                esc = self.next()
                if esc == "n"       { buf[n] = "\n" }
                else if esc == "r"  { buf[n] = "\r" }
                else if esc == "t"  { buf[n] = "\t" }
                else if esc == "\"" { buf[n] = "\"" }
                else if esc == "\\" { buf[n] = "\\" }
                else { return null, "invalid escape" }
            } else {
                buf[n] = ch
            }
            n = n + 1
        }

        return join(slice(buf, 0, n), ""), null
    }

    // ---------- number ----------

    fn parseNumber() {
        start = self.i

        while true {
            ch = self.peek()
            if ch == null { break }

            if indexOf("0123456789.-", ch) != -1 {
                self.i = self.i + 1
            } else {
                break
            }
        }

        raw = substr(self.s, start, self.i)

        if indexOf(raw, ".") != -1 {
            return float(raw), null
        }

        return int(raw), null
    }

    // ---------- array ----------

    fn parseArray() {
        self.next() // consume [

        self.skipWhitespace()
        if self.peek() == "]" {
            self.next()
            return makeArray(0, null), null
        }

        // Doubling buffer + slice at the end — same pattern as
        // stream.lex collect(). Avoids O(n²) push() growth.
        cap = 16
        buf = makeArray(cap, null)
        n   = 0

        while true {
            val, err = self.parseValue()
            if err != null { return null, err }

            if n >= cap {
                newCap = cap * 2
                newBuf = makeArray(newCap, null)
                i = 0
                while i < n {
                    newBuf[i] = buf[i]
                    i = i + 1
                }
                buf = newBuf
                cap = newCap
            }
            buf[n] = val
            n = n + 1

            self.skipWhitespace()
            ch = self.next()

            if ch == "]" { break }
            if ch != "," { return null, "expected , or ]" }
        }

        return slice(buf, 0, n), null
    }

    // ---------- object ----------

    fn parseObject() {
        obj = {}

        self.next() // consume {

        self.skipWhitespace()
        if self.peek() == "}" {
            self.next()
            return obj, null
        }

        while true {
            self.skipWhitespace()
            key, err = self.parseString()
            if err != null { return null, err }

            self.skipWhitespace()
            if self.next() != ":" {
                return null, "expected :"
            }

            val, err = self.parseValue()
            if err != null { return null, err }

            obj[key] = val

            self.skipWhitespace()
            ch = self.next()

            if ch == "}" { break }
            if ch != "," { return null, "expected , or }" }
        }

        return obj, null
    }
}

// --------------------------------------------------
// STRINGIFY
// --------------------------------------------------

fn _stringify(v) {
    t = type(v)

    if t == "NULL"    { return "null" }
    if t == "BOOLEAN" {
        if v { return "true" }
        return "false"
    }

    if t == "INTEGER" || t == "FLOAT" {
        return str(v)
    }

    if t == "STRING" {
        return "\"" + _escape(v) + "\""
    }

    if t == "ARRAY" {
        n     = len(v)
        parts = makeArray(n, "")
        i     = 0
        while i < n {
            parts[i] = _stringify(v[i])
            i = i + 1
        }
        return "[" + join(parts, ",") + "]"
    }

    if t == "HASH" {
        ks    = keys(v)
        n     = len(ks)
        parts = makeArray(n, "")
        i     = 0
        while i < n {
            k = ks[i]
            parts[i] = "\"" + _escape(k) + "\":" + _stringify(v[k])
            i = i + 1
        }
        return "\{" + join(parts, ",") + "}"
    }

    return "null"
}

// ---------- helpers ----------

fn _escape(s) {
    s = replace(s, "\\", "\\\\")
    s = replace(s, "\"", "\\\"")
    s = replace(s, "\n", "\\n")
    s = replace(s, "\r", "\\r")
    s = replace(s, "\t", "\\t")
    return s
}

