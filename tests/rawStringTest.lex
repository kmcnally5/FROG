// 1. Basic multi-line raw string
query = `
    SELECT *
    FROM users
    WHERE age > 18
`
println(query)

// 2. Raw string preserves quotes and braces literally — no interpolation
name = "Karl"
tmpl = `Hello {name}, you owe $10.00`
println(tmpl)

// 3. Backslash is literal — no escape processing
path = `C:\Users\karl\documents\file.txt`
println(path)

// 4. Raw string in an expression
println(`one` + ` ` + `two`)

// 5. len() works on raw strings
println(len(`hello`))

// 6. Raw string assigned and reused
help = `Usage: klex [options] file.lex

  -h    show this help
  -v    verbose output
`
println(help)

// 7. Verify no interpolation — { expr } is not evaluated
x = 99
println(`x = {x}`)
