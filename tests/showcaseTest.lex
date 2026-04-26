// ===================================================================
// tests/showcaseTest.lex
//
// THE FROG REPORT — kLex Language Showcase
//
// This program demonstrates the full FROG language in a single
// coherent run:
//
//   Enums + exhaustive pattern matching
//   Structs with methods
//   map / filter / reduce / pipeline |>
//   Closures, default parameters, const, let, _ discard
//   assert for validation
//   Async / await — parallel HTTP requests
//   JSON parsing
//   Formatted output with bar charts
//   User input
//   File I/O (saves a report)
//   Standard library: http, json, datetime, math, array, strings, fs, async
//
// Run:
//   KLEX_PATH=/path/to/stdlib ./klex tests/showcaseTest.lex
// ===================================================================

import "http.lex"       as http
import "json.lex"       as json
import "datetime.lex"   as dt
import "math.lex"       as math
import "array.lex"      as arr
import "strings.lex"    as s
import "fs.lex"         as fs
import "async.lex"      as alib
import "functional.lex" as f

// ===================================================================
// CONSTANTS
// ===================================================================

const VERSION    = __version__()
const BAR_WIDTH  = 18
const COL_WIDTH  = 60
const API_BASE   = "https://jsonplaceholder.typicode.com"
const REPORT_OUT = "/tmp/frog_report.txt"

// ===================================================================
// UTILITIES
// ===================================================================

fn rule(char = "─") {
    return s.repeat(char, COL_WIDTH)
}

fn section(title) {
    println("")
    println(rule("─"))
    println("  " + title)
    println(rule("─"))
}

fn bar(n, maxVal, width = BAR_WIDTH) {
    if maxVal == 0 { return s.repeat("░", width) }
    filled = (n * width) / maxVal
    empty  = width - filled
    return s.repeat("█", filled) + s.repeat("░", empty)
}

fn printKV(label, value, width = 22) {
    println(format("  %-*s %s", width, label + ":", str(value)))
}

// ===================================================================
// BANNER
// ===================================================================

fn printBanner() {
    println("")
    println(s.repeat("═", COL_WIDTH))
    println("")
    println("    ███████╗██████╗  ██████╗  ██████╗ ")
    println("    ██╔════╝██╔══██╗██╔═══██╗██╔════╝ ")
    println("    █████╗  ██████╔╝██║   ██║██║  ███╗")
    println("    ██╔══╝  ██╔══██╗██║   ██║██║   ██║")
    println("    ██║     ██║  ██║╚██████╔╝╚██████╔╝")
    println("    ╚═╝     ╚═╝  ╚═╝ ╚═════╝  ╚═════╝ ")
    println("")
    println("    Functional · Reactive · Opinionated · Governed")
    println("    kLex Language Showcase  v" + VERSION)
    println("")
    println(s.repeat("═", COL_WIDTH))
    println("")
}

// ===================================================================
// DATA MODEL
// ===================================================================

enum Priority { Critical High Medium Low }
enum Status   { Open InProgress Done Blocked }

struct Task {
    id, title, owner, priority, status, points

    fn priorityLabel() {
        switch self.priority {
            case Priority.Critical { return "CRIT" }
            case Priority.High     { return "HIGH" }
            case Priority.Medium   { return "MED " }
            case Priority.Low      { return "LOW " }
        }
    }

    fn statusLabel() {
        switch self.status {
            case Status.Open       { return "Open      " }
            case Status.InProgress { return "In Prog   " }
            case Status.Done       { return "Done   ✓  " }
            case Status.Blocked    { return "Blocked ! " }
        }
    }

    fn isDone() {
        return self.status == Status.Done
    }

    fn isBlocked() {
        return self.status == Status.Blocked
    }
}

fn makeTask(id, title, owner, priority, status, points) {
    return Task {
        id: id, title: title, owner: owner,
        priority: priority, status: status, points: points
    }
}

fn priorityScore(p) {
    switch p {
        case Priority.Critical { return 4 }
        case Priority.High     { return 3 }
        case Priority.Medium   { return 2 }
        case Priority.Low      { return 1 }
    }
}

// ===================================================================
// DATASET
// ===================================================================

fn buildTasks() {
    return [
        makeTask(1,  "Fix auth token expiry",          "Alice", Priority.Critical, Status.InProgress, 8),
        makeTask(2,  "Add dark mode to UI",             "Bob",   Priority.Low,      Status.Open,       3),
        makeTask(3,  "Database connection pooling",     "Alice", Priority.High,     Status.Done,       13),
        makeTask(4,  "Write API documentation",         "Carol", Priority.Medium,   Status.Open,       5),
        makeTask(5,  "Rate limiting middleware",         "Bob",   Priority.High,     Status.InProgress, 8),
        makeTask(6,  "Fix CSV export encoding",         "Carol", Priority.Medium,   Status.Blocked,    3),
        makeTask(7,  "Pagination for /users endpoint",  "Alice", Priority.High,     Status.Done,       5),
        makeTask(8,  "Upgrade dependency versions",     "Dave",  Priority.Low,      Status.Open,       2),
        makeTask(9,  "Memory leak in stream parser",    "Dave",  Priority.Critical, Status.InProgress, 13),
        makeTask(10, "Add 2FA to login flow",           "Bob",   Priority.High,     Status.Open,       8),
        makeTask(11, "Cache invalidation strategy",     "Carol", Priority.Medium,   Status.Done,       8),
        makeTask(12, "Refactor configuration loader",   "Dave",  Priority.Low,      Status.Open,       3)
    ]
}

// ===================================================================
// ANALYSIS FUNCTIONS
// ===================================================================

fn countByPriority(tasks) {
    counts = {"Critical": 0, "High": 0, "Medium": 0, "Low": 0}
    for t in tasks {
        switch t.priority {
            case Priority.Critical { counts["Critical"] = counts["Critical"] + 1 }
            case Priority.High     { counts["High"]     = counts["High"]     + 1 }
            case Priority.Medium   { counts["Medium"]   = counts["Medium"]   + 1 }
            case Priority.Low      { counts["Low"]      = counts["Low"]      + 1 }
        }
    }
    return counts
}

fn countByStatus(tasks) {
    counts = {"Open": 0, "InProgress": 0, "Done": 0, "Blocked": 0}
    for t in tasks {
        switch t.status {
            case Status.Open       { counts["Open"]       = counts["Open"]       + 1 }
            case Status.InProgress { counts["InProgress"] = counts["InProgress"] + 1 }
            case Status.Done       { counts["Done"]       = counts["Done"]       + 1 }
            case Status.Blocked    { counts["Blocked"]    = counts["Blocked"]    + 1 }
        }
    }
    return counts
}

fn pointsByOwner(tasks) {
    totals = {}
    for t in tasks {
        if !hasKey(totals, t.owner) { totals[t.owner] = 0 }
        totals[t.owner] = totals[t.owner] + t.points
    }
    return totals
}

fn wordFreq(texts) {
    stopWords = ["the", "a", "an", "to", "of", "in", "for", "is",
                 "and", "with", "or", "at", "by", "on", "be"]
    freq = {}
    for text in texts {
        words = split(lower(text), " ")
        for w in words {
            w = trim(w)
            if len(w) > 2 && !arr.contains(stopWords, w) {
                if !hasKey(freq, w) { freq[w] = 0 }
                freq[w] = freq[w] + 1
            }
        }
    }
    return freq
}

// ===================================================================
// HTTP HELPERS
// ===================================================================

fn fetchPosts(limit) {
    url = API_BASE + "/posts?_limit=" + str(limit)
    resp, err = http.get(url)
    if err != null { return null, "network: " + err }
    if !http.isOk(resp) { return null, "HTTP " + str(resp.status) }
    data, jerr = json.parse(resp.body)
    if jerr != null { return null, "JSON: " + str(jerr) }
    return data, null
}

fn fetchUsers(limit) {
    url = API_BASE + "/users?_limit=" + str(limit)
    resp, err = http.get(url)
    if err != null { return null, "network: " + err }
    if !http.isOk(resp) { return null, "HTTP " + str(resp.status) }
    data, jerr = json.parse(resp.body)
    if jerr != null { return null, "JSON: " + str(jerr) }
    return data, null
}

// ===================================================================
// DISPLAY HELPERS
// ===================================================================

fn printTaskTable(tasks) {
    println(format("  %3s  %-4s  %-32s  %-6s  %-10s  %3s",
        "ID", "PRI", "TITLE", "OWNER", "STATUS", "PTS"))
    println("  " + s.repeat("─", 68))
    for t in tasks {
        println(format("  %3d  %-4s  %-32s  %-6s  %-10s  %3d",
            t.id, t.priorityLabel(), t.title,
            t.owner, t.statusLabel(), t.points))
    }
}

fn printBarChart(title, counts, keys) {
    println("  " + title)
    maxVal = reduce(keys, fn(mx, k) {
        v = counts[k]
        if v == null { v = 0 }
        if v > mx { return v }
        return mx
    }, 0)
    for k in keys {
        v = counts[k]
        if v == null { v = 0 }
        b = bar(v, maxVal)
        println(format("  %-12s  %s  %d", k, b, v))
    }
}

// ===================================================================
// PROGRAM START
// ===================================================================

printBanner()

// Capture start time for elapsed reporting
startTime = dt.now()
println("  Started at: " + dt.format(startTime, dt.DATETIME))

// ===================================================================
// GREETING
// ===================================================================

section("WELCOME")
name = input("  What is your name? ")
if len(trim(name)) == 0 { name = "Developer" }
println("")
println("  Hello, " + name + "! Let's run the FROG Report.")
println("  Today is " + dt.format(startTime, dt.DATETIME))
println("  Weekday:  " + startTime.weekday)

// ===================================================================
// TASK BOARD — LOCAL DATA
// ===================================================================

section("PROJECT TASK BOARD")

tasks = buildTasks()
assert(len(tasks) == 12, "expected 12 tasks in dataset")

println("")
printTaskTable(tasks)
println("")
println(format("  Total tasks: %d", len(tasks)))

// --- Filter active (not done) tasks, sort by priority score ---
activeTasks = tasks
    |> filter(fn(t) { return !t.isDone() })
    |> arr.sortBy(fn(a, b) {
        return priorityScore(a.priority) > priorityScore(b.priority)
    })

println(format("  Active:      %d", len(activeTasks)))
println(format("  Completed:   %d", len(filter(tasks, fn(t) { return t.isDone() }))))
println(format("  Blocked:     %d", len(filter(tasks, fn(t) { return t.isBlocked() }))))

// --- Total story points ---
totalPts  = reduce(tasks,   fn(acc, t) { return acc + t.points }, 0)
donePts   = reduce(filter(tasks, fn(t) { return t.isDone() }),
                   fn(acc, t) { return acc + t.points }, 0)
activePts = totalPts - donePts

println("")
println(format("  Story points — total: %d  done: %d  remaining: %d",
    totalPts, donePts, activePts))

// ===================================================================
// PRIORITY & STATUS BREAKDOWN
// ===================================================================

section("WORKLOAD BREAKDOWN")

byPriority = countByPriority(tasks)
byStatus   = countByStatus(tasks)
byOwner    = pointsByOwner(tasks)

println("")
printBarChart("Tasks by Priority:", byPriority,
    ["Critical", "High", "Medium", "Low"])
println("")
printBarChart("Tasks by Status:", byStatus,
    ["Open", "InProgress", "Done", "Blocked"])

// --- Points per owner using pipeline ---
println("")
println("  Points by Owner:")
ownerNames = arr.sortBy(keys(byOwner), fn(a, b) {
    return byOwner[b] < byOwner[a]
})
maxOwnerPts = reduce(ownerNames, fn(mx, k) {
    v = byOwner[k]
    if v > mx { return v }
    return mx
}, 0)
for owner in ownerNames {
    pts = byOwner[owner]
    b = bar(pts, maxOwnerPts)
    println(format("  %-10s  %s  %d pts", owner, b, pts))
}

// ===================================================================
// HIGHER-ORDER FUNCTION SHOWCASE
// ===================================================================

section("FUNCTIONAL PROGRAMMING SHOWCASE")

// compose: build a pipeline transform using f.compose
isHighPriority = fn(t) { return priorityScore(t.priority) >= 3 }
getTitle       = fn(t) { return t.title }
toUpperTitle   = f.compose(upper, getTitle)

println("")
println("  Top-priority task titles (via compose):")
tasks
    |> filter(isHighPriority)
    |> map(toUpperTitle)
    |> map(fn(title) { return "    • " + title })
    |> map(println)

// reduce: longest task title
longestTitle = reduce(tasks, fn(best, t) {
    if len(t.title) > len(best) { return t.title }
    return best
}, "")
println("")
println("  Longest title: \"" + longestTitle + "\"")
println(format("  Length: %d characters", len(longestTitle)))

// Word frequency across task titles
println("")
println("  Word frequency in task titles (top 5):")
titles     = map(tasks, getTitle)
freq       = wordFreq(titles)
freqWords  = arr.sortBy(keys(freq), fn(a, b) { return freq[b] < freq[a] })
topN       = math.min(5, len(freqWords))
i = 0
while i < topN {
    w = freqWords[i]
    println(format("    %-16s  %d occurrence(s)", w, freq[w]))
    i = i + 1
}

// ===================================================================
// ASYNC HTTP — PARALLEL FETCH
// ===================================================================

section("LIVE DATA FETCH (async + await)")

println("")
println("  Launching parallel HTTP requests...")
println("  → GET " + API_BASE + "/posts?_limit=8")
println("  → GET " + API_BASE + "/users?_limit=5")

fetchStart = dt.now()

taskPosts = async(fn() { return fetchPosts(8) })
taskUsers = async(fn() { return fetchUsers(5) })

results    = alib.await_all([taskPosts, taskUsers])
fetchEnd   = dt.now()
fetchMs    = dt.diffSeconds(fetchEnd, fetchStart)

postsResult = results[0]
usersResult = results[1]

posts, postsErr = postsResult
users, usersErr = usersResult

println(format("  Completed in %d second(s).", fetchMs))

// Graceful degradation if network is unavailable
networkOk = postsErr == null && usersErr == null

if !networkOk {
    println("")
    println("  ⚠  Network unavailable — skipping live data sections.")
    if postsErr != null { println("     Posts error: " + str(postsErr)) }
    if usersErr != null { println("     Users error: " + str(usersErr)) }
} else {
    assert(type(posts) == "ARRAY", "posts must be an array")
    assert(type(users) == "ARRAY", "users must be an array")
    println(format("  Received %d posts and %d users.", len(posts), len(users)))
}

// ===================================================================
// POST ANALYSIS (only if network succeeded)
// ===================================================================

if networkOk {
    section("POST ANALYSIS")

    // Build user lookup map by ID
    userMap = {}
    for u in users {
        userMap[u["id"]] = u["name"]
    }

    // Print post summary table
    println("")
    println(format("  %-4s  %-7s  %-42s", "ID", "USER", "TITLE"))
    println("  " + s.repeat("─", 56))
    for post in posts {
        uid    = post["userId"]
        author = userMap[uid]
        if author == null { author = "User " + str(uid) }
        title  = post["title"]
        if len(title) > 40 { title = substr(title, 0, 37) + "..." }
        println(format("  %-4d  %-7s  %-42s", post["id"], author, title))
    }

    // Word frequency in post titles
    println("")
    println("  Top 5 words across post titles:")
    postTitles = map(posts, fn(p) { return p["title"] })
    postFreq   = wordFreq(postTitles)
    postWords  = arr.sortBy(keys(postFreq), fn(a, b) {
        return postFreq[b] < postFreq[a]
    })
    topPost = math.min(5, len(postWords))
    i = 0
    while i < topPost {
        w = postWords[i]
        b = bar(postFreq[w], postFreq[postWords[0]])
        println(format("    %-16s  %s  %d", w, b, postFreq[w]))
        i = i + 1
    }

    // Posts per user (pipeline + reduce)
    println("")
    println("  Posts per user:")
    postsPerUser = reduce(posts, fn(acc, p) {
        uid = p["userId"]
        // use let so this local does not walk the chain and overwrite the
        // outer 'name' variable that holds the analyst's name
        let pAuthor = userMap[uid]
        if pAuthor == null { pAuthor = "User " + str(uid) }
        if !hasKey(acc, pAuthor) { acc[pAuthor] = 0 }
        acc[pAuthor] = acc[pAuthor] + 1
        return acc
    }, {})
    ppuNames = arr.sortBy(keys(postsPerUser),
        fn(a, b) { return postsPerUser[b] < postsPerUser[a] })
    maxPPU = reduce(ppuNames,
        fn(mx, k) {
            v = postsPerUser[k]
            if v > mx { return v }
            return mx
        }, 0)
    for owner in ppuNames {
        n = postsPerUser[owner]
        b = bar(n, maxPPU, 10)
        println(format("    %-18s  %s  %d", owner, b, n))
    }
}

// ===================================================================
// INTERACTIVE SEARCH
// ===================================================================

section("INTERACTIVE KEYWORD SEARCH")

println("")
keyword = input("  Enter a keyword to search post titles (or press Enter to skip): ")
keyword = lower(trim(keyword))

if len(keyword) > 0 && networkOk {
    println("")
    matched = posts |> filter(fn(p) {
        return s.contains(lower(p["title"]), keyword)
    })
    if len(matched) == 0 {
        println(format("  No posts matched \"%s\".", keyword))
    } else {
        println(format("  %d post(s) matched \"%s\":", len(matched), keyword))
        for p in matched {
            uid    = p["userId"]
            author = userMap[uid]
            if author == null { author = "User " + str(uid) }
            println(format("    [%d] %s — %s", p["id"], author, p["title"]))
        }
    }
} else if len(keyword) > 0 {
    println("  (Cannot search — network was unavailable.)")
} else {
    println("  Skipping keyword search.")
}

// ===================================================================
// REPORT GENERATION
// ===================================================================

section("REPORT GENERATION")

endTime    = dt.now()
elapsedSec = dt.diffSeconds(endTime, startTime)

// Build report lines using closure accumulator pattern
reportLines = []
let addLine = fn(line = "") {
    reportLines = push(reportLines, line)
}

addLine(s.repeat("═", COL_WIDTH))
addLine("  THE FROG REPORT")
addLine("  Generated : " + dt.format(endTime, dt.DATETIME))
addLine("  Analyst   : " + name)
addLine("  Language  : kLex v" + VERSION)
addLine(s.repeat("═", COL_WIDTH))
addLine("")
addLine("  TASK SUMMARY")
addLine("  " + s.repeat("─", 40))
addLine(format("  Total tasks    : %d", len(tasks)))
addLine(format("  Active         : %d", len(activeTasks)))
addLine(format("  Completed      : %d  (%d pts)",
    len(filter(tasks, fn(t) { return t.isDone() })), donePts))
addLine(format("  Blocked        : %d", len(filter(tasks, fn(t) { return t.isBlocked() }))))
addLine(format("  Total pts      : %d  (remaining: %d)", totalPts, activePts))
addLine("")
addLine("  PRIORITY BREAKDOWN")
addLine("  " + s.repeat("─", 40))
priKeys = ["Critical", "High", "Medium", "Low"]
for k in priKeys {
    v = byPriority[k]
    if v == null { v = 0 }
    addLine(format("  %-12s : %d task(s)", k, v))
}
addLine("")
addLine("  POINTS BY OWNER")
addLine("  " + s.repeat("─", 40))
for owner in ownerNames {
    addLine(format("  %-10s : %d pts", owner, byOwner[owner]))
}
addLine("")
if networkOk {
    addLine("  LIVE DATA")
    addLine("  " + s.repeat("─", 40))
    addLine(format("  Posts fetched  : %d", len(posts)))
    addLine(format("  Users fetched  : %d", len(users)))
    addLine(format("  Fetch time     : %d second(s)", fetchMs))
    if len(keyword) > 0 {
        addLine(format("  Keyword search : \"%s\" — %d match(es)",
            keyword, len(matched)))
    }
    addLine("")
}
addLine("  SESSION")
addLine("  " + s.repeat("─", 40))
addLine(format("  Started        : %s", dt.format(startTime, dt.DATETIME)))
addLine(format("  Finished       : %s", dt.format(endTime, dt.DATETIME)))
addLine(format("  Elapsed        : %d second(s)", elapsedSec))
addLine("")
addLine(s.repeat("═", COL_WIDTH))
addLine("")

reportText = join(reportLines, "\n")

// Save report using safe() to handle any I/O failure gracefully
_, writeErr = safe(fs.write, REPORT_OUT, reportText)
if writeErr != null {
    println("  Warning: could not save report — " + writeErr.message)
} else {
    println("")
    println("  Report saved to: " + REPORT_OUT)
    info, _ = fs.stat(REPORT_OUT)
    if info != null {
        println(format("  File size: %d bytes", info.size))
    }
}

// ===================================================================
// FINAL SUMMARY
// ===================================================================

section("SUMMARY")

println("")
println(format("  %-30s %s", "Analyst:",        name))
println(format("  %-30s %s", "Tasks analysed:", str(len(tasks))))
println(format("  %-30s %s", "Story points:",   str(totalPts) + " total / " + str(donePts) + " done"))
println(format("  %-30s %s", "Active tasks:",   str(len(activeTasks))))
if networkOk {
    println(format("  %-30s %s", "Posts fetched:", str(len(posts))))
    println(format("  %-30s %s", "Users fetched:", str(len(users))))
}
println(format("  %-30s %s", "Elapsed:", str(elapsedSec) + " second(s)"))
println(format("  %-30s %s", "Report:", REPORT_OUT))
println("")

// Final pipeline demo — top 3 active tasks by priority
println("  Top 3 active tasks right now:")
top3 = activeTasks |> slice(0, math.min(3, len(activeTasks)))
i = 0
for t in top3 {
    i = i + 1
    println(format("  %d. [%s] %s (%s, %d pts)",
        i, t.priorityLabel(), t.title, t.owner, t.points))
}

println("")
println(s.repeat("═", COL_WIDTH))
println("  FROG — Functional · Reactive · Opinionated · Governed")
println("  That's a wrap. Go build something.")
println(s.repeat("═", COL_WIDTH))
println("")
