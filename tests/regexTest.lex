import "regex.lex" as regex

// --- match ---
ok, err = regex.match("[0-9]+", "abc123")
if err != null { println(err)  return null }
if ok != true { println("FAIL: match digits") } else { println("match digits: ok") }

ok, err = regex.match("[0-9]+", "abcdef")
if err != null { println(err)  return null }
if ok != false { println("FAIL: match no digits") } else { println("match no digits: ok") }

// invalid pattern returns an error
_, err = regex.match("[invalid", "abc")
if err == null { println("FAIL: bad pattern should error") } else { println("bad pattern error: ok") }

// --- find ---
m, err = regex.find("[0-9]+", "abc123def456")
if err != null { println(err)  return null }
if m != "123" { println("FAIL: find first") } else { println("find first: ok") }

m, err = regex.find("[0-9]+", "abcdef")
if err != null { println(err)  return null }
if m != null { println("FAIL: find no match should be null") } else { println("find no match: ok") }

// --- findAll ---
all, err = regex.findAll("[0-9]+", "abc123def456ghi789")
if err != null { println(err)  return null }
if len(all) != 3 { println("FAIL: findAll count") } else { println("findAll count: ok") }
if all[0] != "123" { println("FAIL: findAll[0]") } else { println("findAll[0]: ok") }

// --- replace ---
result, err = regex.replace("[0-9]+", "abc123def456", "NUM")
if err != null { println(err)  return null }
if result != "abcNUMdef456" { println("FAIL: replace first") } else { println("replace first: ok") }

// --- replaceAll ---
result, err = regex.replaceAll("[0-9]+", "abc123def456", "NUM")
if err != null { println(err)  return null }
if result != "abcNUMdefNUM" { println("FAIL: replaceAll") } else { println("replaceAll: ok") }

// --- split ---
parts, err = regex.split(",\\s*", "a, b,c,  d")
if err != null { println(err)  return null }
if len(parts) != 4 { println("FAIL: split count") } else { println("split count: ok") }
if parts[1] != "b" { println("FAIL: split[1]") } else { println("split[1]: ok") }

// --- groups ---
g, err = regex.groups("([a-z]+)([0-9]+)", "abc123")
if err != null { println(err)  return null }
if g == null { println("FAIL: groups null") } else { println("groups match: ok") }
if g[0] != "abc123" { println("FAIL: groups[0] full match") } else { println("groups[0]: ok") }
if g[1] != "abc"    { println("FAIL: groups[1]") } else { println("groups[1]: ok") }
if g[2] != "123"    { println("FAIL: groups[2]") } else { println("groups[2]: ok") }

g, err = regex.groups("([0-9]+)", "no digits here")
if err != null { println(err)  return null }
if g != null { println("FAIL: groups no match should be null") } else { println("groups no match: ok") }

// --- groupsAll ---
all, err = regex.groupsAll("([a-z]+)([0-9]+)", "abc123 def456")
if err != null { println(err)  return null }
if len(all) != 2 { println("FAIL: groupsAll count") } else { println("groupsAll count: ok") }
if all[0][1] != "abc" { println("FAIL: groupsAll[0][1]") } else { println("groupsAll[0][1]: ok") }
if all[1][2] != "456" { println("FAIL: groupsAll[1][2]") } else { println("groupsAll[1][2]: ok") }
