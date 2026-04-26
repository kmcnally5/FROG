# kLex
FROG is a Go-native, tree-walking interpreter I designed for developers who value absolute clarity over cleverness. It is built on the kLex engine and follows a core philosophy of being Functional, Reactive, Opinionated, and Governed.

In FROG, types are strict, coercion is non-existent, and errors are handled as first-class values. I’ve prioritized a "Governed" structure where the language's behavior is predictable and explicit, rather than hidden behind shorthand or magic.

I wrote FROG in an effort to explore a development environment better suited for AI-assisted coding. In my experience, the "looseness" of many modern languages can occasionally lead even the most capable LLMs into cycles of ambiguity and hallucination.

It is my hope that by enforcing a more rigid, opinionated syntax, I can improve LLM efficiency and make the human-AI collaboration process more reliable. To put this theory to the test, I developed the kLex interpreter almost entirely using AI, treating the creation of the engine as my first real-world trial of this workflow.

FROG demonstrates the complete pipeline of a programming language: Lexer → Parser → AST → Evaluator → Environment.  For indepth language reading please find the KLEX_GRAMMER.TXT and KLEX_LANGUAGE.TXT files in the /docs/ folder.

## Try it Online

**[Launch the kLex REPL](https://kmcnally5.github.io/FROG/playground/)** — run kLex code directly in your browser, no installation required.

The REPL supports multi-line input (automatically detects when blocks are complete) and maintains session state — define variables and functions, then use them in subsequent lines.

## Quick Start

### Prerequisites
- Go 1.16 or later

### Running kLex Programs

```bash
go run . <file.lex>
```

Or build and run directly:

```bash
go build -o klex .
./klex <file.lex>
```

## Language Features

kLex supports a rich set of features:

- **Types:** integer, boolean, string, null, array, function
- **Operators:** arithmetic (`+`, `-`, `*`, `/`), comparison (`==`, `!=`, `<`, `>`, `<=`, `>=`), logical (`&&`, `||`, `!`)
- **Control flow:** `if`/`else`, `while`, `break`, `continue`, `return`
- **Functions:** named and anonymous, closures, strict arity checking
- **Arrays:** literals, indexing, and builtins (`len`, `push`)
- **Null:** first-class keyword with explicit null semantics

## Architecture

The interpreter is organized into focused packages:

```
lexer/lexer.go     — Tokenization with Line/Col stamping
ast/ast.go         — All AST node types and position tracking
parser/parser.go   — Pratt parser producing *ast.Program
eval/object.go     — Runtime object interface and concrete types
eval/env.go        — Environment (lexical scope chain)
eval/typecheck.go  — Type compatibility and error constructors
eval/eval.go       — Main evaluation engine and builtins
main.go            — Entry point (reads .lex file and evaluates)
```

**No external dependencies** — the interpreter is built entirely with the Go standard library.

## Design Principles

kLex enforces a strict, explicit type system:

- **No implicit type coercion** — `1 == "1"` is a type error
- **Explicit null semantics** — `null == null` is true; `null == T` is false for any other type
- **Strict boolean conditions** — only `bool` types are valid in conditionals; integers are not truthy
- **Lexical scoping** — full closure support with proper environment chaining

## Examples

### Recursion and Control Flow
```klex
fn fibonacci(n) {
  if (n <= 1) {
    return n;
  }
  return fibonacci(n - 1) + fibonacci(n - 2);
}

println(fibonacci(10));  // 55
```

### Pattern Matching with Enums
```klex
enum Status { Pending Active Completed }

fn label(status) {
  switch status {
    case Status.Pending { return "⏳ Pending" }
    case Status.Active { return "🔄 Active" }
    case Status.Completed { return "✓ Completed" }
  }
}

println(label(Status.Active));  // 🔄 Active
```

### Higher-Order Functions and Pipelines
```klex
numbers = [1, 2, 3, 4, 5]

result = numbers
  |> map(fn(x) { x * 2 })        // [2, 4, 6, 8, 10]
  |> filter(fn(x) { x > 5 })     // [6, 8, 10]
  |> reduce(fn(acc, x) { acc + x }, 0)  // 24

println(result);
```

### Structs with Methods
```klex
struct Point {
  x, y

  fn distance() {
    return math.sqrt(self.x * self.x + self.y * self.y)
  }

  fn scale(factor) {
    return Point { x: self.x * factor, y: self.y * factor }
  }
}

p = Point { x: 3, y: 4 }
println(p.distance())      // 5
println(p.scale(2).x)      // 6
```

### Closures and First-Class Functions
```klex
fn makeAdder(n) {
  return fn(x) { x + n }
}

add5 = makeAdder(5)
add10 = makeAdder(10)

println(add5(3))   // 8
println(add10(3))  // 13
```

### Strict Type System (No Implicit Coercion)
```klex
// This is strict — no "truthy" integers
if (1) { println("truthy") }  // TypeError: condition must be bool

// Proper boolean checks
if (1 > 0) { println("correct") }  // Works

// Type safety in comparisons
result = 1 == "1"  // TypeError: cannot compare int and string
```

## Full Examples in /tests/

These snippets are excerpted from the test suite. For comprehensive examples and full feature exploration, check out:

- **[`tests/showcaseTest.lex`](tests/showcaseTest.lex)** — The complete FROG Report: enums, structs, pipelines, closures, async/await, HTTP requests, file I/O, JSON parsing, and interactive input. This is the most comprehensive demonstration of the language.
- **[`tests/functionalTest.lex`](tests/functionalTest.lex)** — Higher-order functions: `compose`, `pipe`, `tap`, `partial`, `flip`, and real-world pipeline examples.
- **[`tests/patternTest.lex`](tests/patternTest.lex)** — Enum pattern matching with data binding and exhaustive switch statements.
- **[`tests/httpTest.lex`](tests/httpTest.lex)** — HTTP client usage and async requests.
- **[`tests/fsTest.lex`](tests/fsTest.lex)** — File system operations (read, write, stat).
- **[`tests/arrayTest.lex`](tests/arrayTest.lex)** — Array operations and built-in functions.
- **[`tests/assertTest.lex`](tests/assertTest.lex)** — Assertion and error handling patterns.

## Testing

Test your changes against the included test suite:

```bash
go run . test1.lex
```

## License

This is a learning project. Feel free to explore and learn from the implementation.
