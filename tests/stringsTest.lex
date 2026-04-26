import "strings.lex" as s

println("== contains ==")
println(s.contains("hello world", "world"))   // true
println(s.contains("hello world", "xyz"))     // false

println("== indexOf ==")
println(s.indexOf("hello", "ll"))    // 2
println(s.indexOf("hello", "x"))     // -1
println(s.indexOf("hello", ""))      // 0

println("== startsWith / endsWith ==")
println(s.startsWith("hello", "he"))    // true
println(s.startsWith("hello", "lo"))    // false
println(s.endsWith("hello", "lo"))      // true
println(s.endsWith("hello", "he"))      // false

println("== repeat ==")
println(s.repeat("ab", 3))    // ababab
println(s.repeat("x", 0))     // (empty)

println("== replace ==")
println(s.replace("a-b-c", "-", "."))    // a.b.c
println(s.replace("aaa", "a", "b"))      // bbb

println("== count ==")
println(s.count("banana", "an"))    // 2
println(s.count("hello", "x"))      // 0

println("== trimLeft ==")
println(s.trimLeft("   hello"))     // hello
println(s.trimLeft("hello"))        // hello

println("== trimRight ==")
println(s.trimRight("hello   "))    // hello
println(s.trimRight("hello"))       // hello

println("== trimSpace ==")
println(s.trimSpace("  hello  "))   // hello

println("== padLeft / padRight ==")
println(s.padLeft("42", 5, "0"))      // 00042
println(s.padRight("hi", 5, "."))     // hi...

println("== toUpper / toLower ==")
println(s.toUpper("hello"))    // HELLO
println(s.toLower("HELLO"))    // hello

println("== lines ==")
ls = s.lines("a\nb\nc")
println(len(ls))    // 3
println(ls[0])      // a
println(ls[2])      // c

println("== words ==")
ws = s.words("hello world foo")
println(len(ws))    // 3
println(ws[0])      // hello
println(ws[2])      // foo
