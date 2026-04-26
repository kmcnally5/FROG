import "fs.lex" as fs

_, err = fs.write("/tmp/test.txt", "hello\n")
if err != null { println(err)  return null }

content, err = fs.read("/tmp/test.txt")
if err != null { println(err)  return null }
println(content)

println(fs.exists("/tmp/test.txt"))   // true
println(fs.exists("/tmp/no_such_file.txt"))   // false

files, err = fs.listDir("/tmp")
if err != null { println(err)  return null }
println(len(files))

info, err = fs.stat("/tmp/test.txt")
if err != null { println(err)  return null }
println(info.isDir)   // false

info, err = fs.stat("/tmp")
if err != null { println(err)  return null }
println(info.isDir)   // true
