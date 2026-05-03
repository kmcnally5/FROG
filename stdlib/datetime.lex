// datetime.lex
// Date and time handling for kLex.
//
// All times are in local time. The unix field is always seconds since
// 1970-01-01 00:00:00 UTC regardless of local timezone.
//
// Layout strings use Go's reference time: Mon Jan 2 15:04:05 MST 2006
// Use the named constants below instead of writing layouts by hand.
//
// Usage:
//   import "datetime.lex" as dt
//   now = dt.now()
//   println(now.year)
//   println(dt.format(now, dt.ISO8601))
//   parsed, err = dt.parse("2024-01-15", dt.DATE)

// Named layout constants — pass these to format() and parse().
ISO8601  = "2006-01-02T15:04:05Z07:00"
DATE     = "2006-01-02"
TIME     = "15:04:05"
DATETIME = "2006-01-02 15:04:05"
RFC1123  = "Mon, 02 Jan 2006 15:04:05 MST"

struct DateTime {
    year, month, day, hour, minute, second, unix, weekday
}

// nowNanos returns the current time in nanoseconds (high-resolution).
// Use for accurate benchmarking and timing measurements.
fn nowNanos() {
    return _timeNanos()
}

// now returns a DateTime representing the current local time.
fn now() {
    year, month, day, hour, minute, second, unix, weekday = _timeNow()
    return DateTime {
        year: year, month: month, day: day,
        hour: hour, minute: minute, second: second,
        unix: unix, weekday: weekday
    }
}

// fromUnix converts a unix timestamp (integer seconds) to a DateTime.
fn fromUnix(unix) {
    year, month, day, hour, minute, second, weekday = _timeFields(unix)
    return DateTime {
        year: year, month: month, day: day,
        hour: hour, minute: minute, second: second,
        unix: unix, weekday: weekday
    }
}

// format formats a DateTime using a layout string.
// Use the named constants (ISO8601, DATE, TIME, DATETIME, RFC1123) or
// write a custom Go reference-time layout.
fn format(dt, layout) {
    return _timeFormat(dt.unix, layout)
}

// parse parses a time string using a layout string.
// Returns (DateTime, err). On failure DateTime is null and err is a string.
fn parse(s, layout) {
    unix, err = _timeParse(s, layout)
    if err != null { return null, err }
    return fromUnix(unix), null
}

// diffSeconds returns the difference in seconds between two DateTimes (a - b).
fn diffSeconds(a, b) {
    return a.unix - b.unix
}

// addSeconds returns a new DateTime offset by n seconds from dt.
fn addSeconds(dt, n) {
    return fromUnix(dt.unix + n)
}

// isWeekend returns true if the DateTime falls on Saturday or Sunday.
fn isWeekend(dt) {
    return dt.weekday == "Saturday" || dt.weekday == "Sunday"
}
