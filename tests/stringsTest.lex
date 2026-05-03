import "strings.lex" as s

println("== indexOf (builtin) ==")
println(indexOf("hello", "ll"))    // 2
println(indexOf("hello", "x"))     // -1
println(indexOf("hello", ""))      // 0

println("== startsWith / endsWith (builtins) ==")
println(startsWith("hello", "he"))    // true
println(startsWith("hello", "lo"))    // false
println(endsWith("hello", "lo"))      // true
println(endsWith("hello", "he"))      // false

println("== repeat ==")
println(s.repeat("ab", 3))    // ababab
println(s.repeat("x", 0))     // (empty)

println("== replace (builtin) ==")
println(replace("a-b-c", "-", "."))    // a.b.c
println(replace("aaa", "a", "b"))      // bbb

println("== count ==")
println(s.count("banana", "an"))    // 2
println(s.count("hello", "x"))      // 0

println("== trimLeft ==")
println(s.trimLeft("   hello"))     // hello
println(s.trimLeft("hello"))        // hello

println("== trimRight ==")
println(s.trimRight("hello   "))    // hello
println(s.trimRight("hello"))       // hello

println("== trim (builtin) ==")
println(trim("  hello  "))   // hello

println("== padLeft / padRight ==")
println(s.padLeft("42", 5, "0"))      // 00042
println(s.padRight("hi", 5, "."))     // hi...

println("== upper / lower (builtins) ==")
println(upper("hello"))    // HELLO
println(lower("HELLO"))    // hello

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
