import "datetime.lex" as dt

// --- now ---
n = dt.now()
println(type(n.year) == "INTEGER")     // true
println(type(n.month) == "INTEGER")    // true
println(type(n.unix) == "INTEGER")     // true
println(type(n.weekday) == "STRING")   // true
println(n.year >= 2024)                // true
println(n.month >= 1 && n.month <= 12) // true
println(n.day >= 1 && n.day <= 31)     // true

// --- format ---
epoch = dt.fromUnix(0)
println(dt.format(epoch, dt.DATE))     // 1970-01-01
println(dt.format(epoch, dt.TIME))     // 00:00:00  (or offset for local tz)

// known timestamp: 2024-01-15 12:00:00 UTC = 1705320000
ts = dt.fromUnix(1705320000)
println(ts.year)                       // 2024
println(ts.month)                      // 1
println(ts.day)                        // 15

// --- parse ---
parsed, err = dt.parse("2024-06-01", dt.DATE)
println(err == null)                   // true
println(parsed.year)                   // 2024
println(parsed.month)                  // 6
println(parsed.day)                    // 1

// --- parse error ---
bad, err = dt.parse("not-a-date", dt.DATE)
println(err != null)                   // true
println(bad == null)                   // true

// --- diffSeconds ---
a = dt.fromUnix(1000)
b = dt.fromUnix(400)
println(dt.diffSeconds(a, b))          // 600

// --- addSeconds ---
c = dt.addSeconds(a, 60)
println(c.unix)                        // 1060

// --- isWeekend ---
// 2024-01-13 was a Saturday, unix = 1705104000
sat = dt.fromUnix(1705104000)
println(dt.isWeekend(sat))             // true
// 2024-01-15 was a Monday, unix = 1705276800
mon = dt.fromUnix(1705276800)
println(dt.isWeekend(mon))             // false
