// pipelineTest.lex — tests for the |> pipeline operator

// Bare function reference — value passed as sole argument
println("  hello  " |> trim)

// Function call with extra args — value prepended
println("a,b,c" |> split(","))

// Chained pipeline — left-associative
input = "  hello , world , kLex  "
result = input |> trim |> split(",") |> map(trim)
println(result)

// Pipeline with filter
nums = [1, -2, 3, -4, 5]
result = nums |> filter(fn(x) { return x > 0 }) |> map(fn(x) { return x * 2 })
println(result)

// Pipeline with user-defined functions
fn double(x) { return x * 2 }
fn addOne(x) { return x + 1 }
println(10 |> double |> addOne)

// Pipeline with builtin str
println(42 |> str)

// Left-associativity: (a |> f) |> g
println("hello world" |> split(" ") |> len)

// Precedence: whole left expression is piped, not just last operand
println((1 + 2) |> str)       // "3"
println(1 + 2 |> str |> len)  // str(3) = "3", len("3") = 1

// Pipeline with indexOf and comparison
import "strings.lex" as s
println(indexOf("hello", "ell") != -1)

// Pipeline with range
println(range(5) |> len)

// Empty call form vs bare form — both work identically
println("  hi  " |> trim)
println("  hi  " |> trim())
