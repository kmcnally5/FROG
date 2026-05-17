// stdlib/log.lex — structured logging
//
// Four log levels (debug < info < warn < error) with optional JSON output
// and structured key-value fields. Default level is INFO — debug messages
// are suppressed unless setLevel("debug") is called.
//
// Usage:
//   import "stdlib/log.lex" as log
//
//   log.info("Server started", {"port": 8080, "workers": 40})
//   log.warn("Retry attempt", {"url": url, "attempt": n})
//   log.error("Connection failed", {"host": "db01", "err": err.message})
//
// Human output (default):
//   [2026-05-17 14:23:01] INFO   Server started  port=8080  workers=40
//
// JSON output (log.setJSON(true)):
//   {"ts":"2026-05-17T14:23:01Z","level":"INFO","msg":"Server started","port":8080,"workers":40}

// ── Internal level constants ────────────────────────────────────────────────
_LOG_DEBUG = 0
_LOG_INFO  = 1
_LOG_WARN  = 2
_LOG_ERROR = 3

// ── Mutable state ───────────────────────────────────────────────────────────
_log_minLevel = _LOG_INFO
_log_json     = false
_log_prefix   = ""

// ── Helpers ──────────────────────────────────────────────────────────────────

fn _pad2(n) {
    if n < 10 { return "0" + str(n) }
    return str(n)
}

fn _ts_human() {
    year, month, day, hour, minute, second, unix, weekday = _timeNow()
    d = str(year) + "-" + _pad2(month) + "-" + _pad2(day)
    t = _pad2(hour) + ":" + _pad2(minute) + ":" + _pad2(second)
    return d + " " + t
}

fn _ts_iso() {
    year, month, day, hour, minute, second, unix, weekday = _timeNow()
    d = str(year) + "-" + _pad2(month) + "-" + _pad2(day)
    t = _pad2(hour) + ":" + _pad2(minute) + ":" + _pad2(second)
    return d + "T" + t + "Z"
}

fn _json_escape(s) {
    s = replace(s, "\\", "\\\\")
    s = replace(s, "\"", "\\\"")
    return s
}

fn _json_val(v) {
    t = type(v)
    if t == "STRING"  { return "\"" + _json_escape(v) + "\"" }
    if t == "INTEGER" { return str(v) }
    if t == "FLOAT"   { return str(v) }
    if t == "BOOLEAN" { return str(v) }
    if t == "NULL"    { return "null" }
    return "\"" + _json_escape(str(v)) + "\""
}

fn _fields_json(fields) {
    if fields == null { return "" }
    out = ""
    ks = keys(fields)
    i = 0
    while i < len(ks) {
        k = ks[i]
        out = out + ",\"" + k + "\":" + _json_val(fields[k])
        i = i + 1
    }
    return out
}

fn _fields_human(fields) {
    if fields == null { return "" }
    out = ""
    ks = keys(fields)
    i = 0
    while i < len(ks) {
        k = ks[i]
        out = out + "  " + k + "=" + str(fields[k])
        i = i + 1
    }
    return out
}

fn _write(levelNum, label, msg, fields) {
    if levelNum < _log_minLevel { return }
    if _log_json {
        ts   = _ts_iso()
        // Build JSON object — use "\{" for literal { so the lexer
        // does not treat it as string interpolation.
        line = "\{"
        line = line + "\"ts\":\"" + ts + "\""
        line = line + ",\"level\":\"" + trim(label) + "\""
        if _log_prefix != "" {
            line = line + ",\"logger\":\"" + _log_prefix + "\""
        }
        line = line + ",\"msg\":\"" + _json_escape(msg) + "\""
        line = line + _fields_json(fields)
        line = line + "}"
        println(line)
    } else {
        ts  = _ts_human()
        pfx = ""
        if _log_prefix != "" { pfx = "[" + _log_prefix + "] " }
        println("[" + ts + "] " + label + "  " + pfx + msg + _fields_human(fields))
    }
}

// ── Public API ────────────────────────────────────────────────────────────────

// setLevel sets the minimum log level. Messages below this are suppressed.
// Valid: "debug", "info", "warn", "error"
fn setLevel(name) {
    if name == "debug" { _log_minLevel = _LOG_DEBUG  return }
    if name == "info"  { _log_minLevel = _LOG_INFO   return }
    if name == "warn"  { _log_minLevel = _LOG_WARN   return }
    if name == "error" { _log_minLevel = _LOG_ERROR  return }
}

// setJSON switches to JSON output (true) or human-readable (false, default).
fn setJSON(on) {
    _log_json = on
}

// setPrefix attaches a logger name to every message.
// Human output: [name] prefix.   JSON output: "logger":"name" field.
fn setPrefix(p) {
    _log_prefix = p
}

// debug logs at DEBUG level. Suppressed at the default INFO level.
fn debug(msg, fields = null) {
    _write(_LOG_DEBUG, "DEBUG", msg, fields)
}

// info logs at INFO level.
fn info(msg, fields = null) {
    _write(_LOG_INFO, "INFO ", msg, fields)
}

// warn logs at WARN level.
fn warn(msg, fields = null) {
    _write(_LOG_WARN, "WARN ", msg, fields)
}

// error logs at ERROR level.
fn error(msg, fields = null) {
    _write(_LOG_ERROR, "ERROR", msg, fields)
}
