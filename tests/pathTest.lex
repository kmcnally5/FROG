import "path.lex" as path

println("== normalize ==")
println(path.normalize("a/b/c"))     // a/b/c
println(path.normalize("a\\b\\c"))   // a/b/c

println("== parts ==")
ps = path.parts("/a/b/c")
println(len(ps))    // 4 (includes empty string before leading /)
println(ps[1])      // a
println(ps[3])      // c

println("== join ==")
println(path.join("a", "b"))         // a/b
println(path.join("a/", "b"))        // a/b
println(path.join("", "b"))          // b

println("== joinAll ==")
println(path.joinAll("a", "b", "c"))    // a/b/c
println(path.joinAll("x"))              // x

println("== basename ==")
println(path.basename("/a/b/file.txt"))    // file.txt
println(path.basename("file.txt"))         // file.txt

println("== dirname ==")
println(path.dirname("/a/b/file.txt"))    // /a/b
println(path.dirname("file.txt"))         // .

println("== ext ==")
println(path.ext("file.txt"))      // txt
println(path.ext("file.tar.gz"))   // gz
println(path.ext("noext"))         // (empty)

println("== stripExt ==")
println(path.stripExt("file.txt"))        // file
println(path.stripExt("/a/b/file.txt"))   // /a/b/file

println("== isAbsolute / isRelative ==")
println(path.isAbsolute("/a/b"))     // true
println(path.isAbsolute("a/b"))      // false
println(path.isRelative("a/b"))      // true
println(path.isRelative("/a/b"))     // false

println("== clean ==")
println(path.clean("a/b/../c"))      // a/c
println(path.clean("a/./b/./c"))     // a/b/c
println(path.clean("a//b"))          // a/b
