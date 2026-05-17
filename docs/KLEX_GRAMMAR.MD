# kLex FORMAL GRAMMAR (EBNF)

The kLex language is nicknamed **FROG**: Functional, Reactive, Opinionated, Governed

This document is the authoritative source of truth for kLex syntax. Before adding any new syntax to the language, write it here first and check it for ambiguities. If you cannot express the new construct unambiguously in this grammar, it is not yet ready to implement.

**Derived from:** `lexer/lexer.go` and `parser/parser.go`  
**Keep in sync:** whenever either file is changed.

---

## NOTATION

```
rule = ...          production definition
A B                 A followed by B (concatenation)
A | B               A or B (alternation)
[ A ]               A is optional (zero or one)
{ A }               A repeated zero or more times
A+                  A repeated one or more times  (shorthand for A { A })
"text"              literal terminal (exact characters)
UPPER_CASE          lexical token (defined in the Lexical Grammar section)
(* note *)          comment — not part of the grammar
```

---

## LEXICAL GRAMMAR

The lexer converts raw source text into a flat stream of tokens. Whitespace (spaces, tabs, newlines) and line comments are stripped here; they are not visible to the parser.

### Comments

```
comment      = "//" { any_char } newline
               (* line comment only; no block comments in kLex *)
```

### Identifiers and Keywords

```
letter       = (* any Unicode letter *) | "_"
digit        = "0" | "1" | "2" | "3" | "4" | "5" | "6" | "7" | "8" | "9"

IDENT        = letter { letter | digit }
               (* if the result matches a keyword below, the keyword token
                  is produced instead of IDENT *)
```

**Keywords** (reserved; cannot be used as identifiers):
```
if  else  fn  return  while  for  in  break  continue
switch  case  default  struct  enum  select  let  import  as
true  false  null
```

**IMPORTANT:** `fn` is a reserved keyword and cannot be used as a parameter name
or variable name. Using `fn` as a parameter causes the parser to miscount the
function's arity, leading to "expects N argument(s), got M" errors. Use alternative
names: `func`, `f`, `callback`, `fn_arg`, etc.

### Numeric Literals

```
INT          = digit+
               (* value must fit in a signed 64-bit integer
                  (−9,223,372,036,854,775,808 to 9,223,372,036,854,775,807);
                  a literal that overflows this range is a ParseError —
                  silent wrap is never permitted *)

FLOAT        = digit+ "." digit+
               (* the decimal point must be followed by at least one digit;
                  bare "3." is not a valid float *)
```

### String Literals

```
STRING       = '"' { str_char } '"'
INTERP_STR   = '"' { str_char | interp_expr } '"'
               (* INTERP_STR is produced whenever the string contains at
                  least one bare "{"; STRING otherwise.
                  A string not closed before end-of-file is a ParseError:
                  "unterminated string literal" *)

str_char     = escape_seq | (* any character except '"' and unescaped "{" *)
escape_seq   = '\"'         (* literal double quote          *)
             | '\n'         (* newline                        *)
             | '\t'         (* tab                            *)
             | '\\'         (* literal backslash              *)
             | '\r'         (* carriage return                *)
             | '\b'         (* backspace                      *)
             | '\{'         (* literal "{" — NOT interpolation *)
             | '\x' HEX HEX (* byte with hex value (e.g. \x1b for ESC) *)
interp_expr  = "{" expression "}"

RAW_STR      = '`' { any_char_except_backtick } '`'
               (* No escape processing. No interpolation. Every character
                  including newlines and backslashes is taken literally.
                  Use for multi-line text, file paths, SQL, HTML, etc.
                  A backtick cannot appear inside a raw string.
                  A raw string not closed before end-of-file is a ParseError:
                  "unterminated raw string literal" *)
```

**Two delimiter rules — no overlap:**
* `"..."` — escape sequences and `{expr}` interpolation are active
* `` `...` `` — all characters are literal; nothing is processed

### Operators and Punctuation

```
"+"   "-"   "*"   "/"   "%"       arithmetic
"="                               assignment
"=="  "!="                        equality
"<"   ">"   "<="  ">="            comparison
"&&"  "||"  "!"                   logical
"("   ")"                         grouping / call
"{"   "}"                         block / hash / struct body
"["   "]"                         array / index
","                               separator
":"                               key-value separator
"."                               property / method access
"..."                             variadic marker (on last parameter only)
```

---

## SYNTACTIC GRAMMAR

### Top Level

```
program      = { statement }
```

### Statements

```
statement    = if_stmt
             | while_stmt
             | for_stmt
             | switch_stmt
             | select_stmt
             | return_stmt
             | break_stmt
             | continue_stmt
             | import_stmt
             | struct_decl
             | enum_decl
             | named_fn_stmt
             | let_stmt
             | multi_assign_stmt
             | assign_stmt
             | index_assign_stmt
             | dot_assign_stmt
             | expression             (* expression used as a statement *)
```

### Control Flow

```
if_stmt      = "if" expression block
                 [ "else" ( if_stmt | block ) ]

while_stmt   = "while" expression block

for_stmt     = "for" IDENT [ "," IDENT ] "in" expression block
               (* first IDENT: element (arrays/channels) or key (hashes)
                  second IDENT: value — only meaningful when iterating a hash *)

switch_stmt  = "switch" [ expression ] "{"
                 { "case" case_pattern { "," case_pattern } block }
                 [ "default" block ]
               "}"
               (* no subject → expression switch: each case value is a bool expr
                  with subject → value switch: each case value is compared with ==
                  no fallthrough between cases *)

case_pattern = enum_pattern
             | expression
             (* enum_pattern is only valid in a value switch (with subject);
                expression switch cases must be boolean expressions *)

enum_pattern = dot_expr "(" IDENT { "," IDENT } ")"
             (* Destructuring pattern: case Shape.Circle(r) { } or Shape.Rect(w, h) { }
                dot_expr must resolve to an enum variant at runtime.
                Each IDENT is a new local variable bound positionally to the
                variant's declared fields for the duration of the case body.
                Binding count must exactly match the variant's field count;
                a mismatch is a RuntimeError.
                Bindings are local to the case arm and do not affect the outer scope.
                Zero-field variants use the plain expression form — no parentheses.
                Recognised at parse time: a CallExpr where the function is a DotExpr
                and every argument is a bare Ident is rewritten to enum_pattern. *)

select_stmt  = "select" "{"
                 { "case" select_case block }
                 [ "default" block ]
               "}"
               (* blocks until one channel operation can proceed;
                  uses reflect.Select internally so all cases are evaluated simultaneously;
                  optional default makes the select non-blocking *)

select_case  = "send" "(" expression "," expression ")"
             (* send a value to a channel *)

             | IDENT "," IDENT "=" "recv" "(" expression ")"
             (* receive with two bindings: val, ok = recv(ch)
                ok is false when the channel is closed *)

             | IDENT "=" "recv" "(" expression ")"
             (* receive with one binding: val = recv(ch) *)

             | "recv" "(" expression ")"
             (* receive, discarding the value *)

             (* recv bindings are resolved via Assign() — they update existing
                outer-scope variables rather than creating new local ones *)

return_stmt  = "return" [ expression { "," expression } ]
               (* bare "return" yields null
                  "return a, b" yields a Tuple — must be received with multi-assign *)

break_stmt   = "break"
continue_stmt= "continue"
```

### Declarations

```
import_stmt  = "import" STRING "as" IDENT
               (* path is resolved against KLEX_PATH env var when not absolute;
                  each import evaluates the file independently — no caching;
                  import cycles are detected and reported as a RuntimeError *)

named_fn_stmt= "fn" IDENT "(" [ params ] ")" [ ":" type_name ] block
               (* syntactic sugar — desugared at parse time to:
                  IDENT = fn( params ): type_name { body }
                  Type annotation is optional. *)

struct_decl  = "struct" IDENT "{"
                 { field_list | method_decl }
               "}"

field_list   = IDENT { "," IDENT }
               (* one or more field names on the same "line";
                  fields and methods may be interleaved freely *)

method_decl  = "fn" IDENT "(" [ params ] ")" [ ":" type_name ] block
               (* methods have access to the instance via the implicit
                  identifier `self`; `self` is injected at call time.
                  Type annotations follow the same rules as functions. *)

enum_decl    = "enum" IDENT "{"
                 { variant_decl }
               "}"

variant_decl = IDENT [ "(" IDENT { "," IDENT } ")" ]
               (* zero-field variant: just a name,  e.g. None
                  tuple variant:      name + fields, e.g. Point(x, y) *)
```

### Assignment

```
let_stmt          = "let" IDENT "=" expression
                    (* always creates the binding in the current scope via Set();
                       never walks the scope chain — existing outer bindings are
                       unaffected. Use this to declare a variable that must be
                       local regardless of outer scope contents. *)

const_stmt        = "const" IDENT "=" expression
                    (* creates an immutable binding in the current scope.
                       Any subsequent assignment to this name from any scope
                       that can see it is a RuntimeError:
                       "cannot reassign constant <name>".
                       An inner scope may shadow a const with let or bare
                       assignment without triggering the error. *)

assign_stmt       = IDENT "=" expression
                    (* creates the variable in the current scope if absent;
                       walks the scope chain via Assign() if it already exists *)

multi_assign_stmt = IDENT "," IDENT { "," IDENT } "=" expression
                    (* right-hand side must evaluate to a Tuple with exactly
                       the same number of elements as the left-hand side *)

index_assign_stmt = expression "[" expression "]" "=" expression
                    (* mutates the array or hash in place *)

dot_assign_stmt   = expression "." IDENT "=" expression
                    (* mutates the hash entry or struct field in place *)
```

### Block

```
block        = "{" { statement } "}"
```

### Parameters

```
params       = (* empty *)
             | param { "," param } [ "," IDENT "..." ]
             (* "..." must appear on the last parameter only;
                that parameter receives any extra arguments as an array;
                zero extra arguments yields an empty array, never null *)

param        = IDENT [ "=" expression ]
             (* optional default value; evaluated at call time in the
                function's closure environment, not at definition time.
                Parameters with defaults must follow all required parameters —
                a required param after a defaulted one is a ParseError. *)
```

---

## EXPRESSION GRAMMAR

Expressions are parsed with a Pratt (top-down operator precedence) parser. The grammar below is layered by precedence from lowest to highest. All binary operators are left-associative at the same precedence level.

### Operator Precedence Table

| Level | Operators | Associativity | Name |
|-------|-----------|---------------|------|
| 2 | `\|>` | left | pipeline |
| 3 | `&&` `\|\|` | left | logical |
| 4 | `==` `!=` `<` `>` `<=` `>=` | left | comparison |
| 5 | `+` `-` | left | additive |
| 6 | `*` `/` `%` | left | multiplicative |
| 7 | `-` `!` | right (prefix) | unary |
| 8 | `( args )` | left | call |
| 9 | `[ index ]` | left | subscript |
| 9 | `. ident` | left | dot access |

### Expression Rules

```
pipe_expr    = logic_expr { "|>" ( call_expr | postfix_expr ) }
               (* Pipes the left value as the first argument of the right
                  callable. If the right side is a call_expr, the piped
                  value is prepended to its argument list.
                  If the right side is a bare reference (ident, dot_expr),
                  it is called with the left value as the only argument.
                  Left-associative: a |> f() |> g() ≡ g(f(a)) *)

expression   = pipe_expr

logic_expr   = compare_expr { ( "&&" | "||" ) compare_expr }

compare_expr = add_expr { ( "==" | "!=" | "<" | ">" | "<=" | ">=" ) add_expr }

add_expr     = mul_expr { ( "+" | "-" ) mul_expr }

mul_expr     = unary_expr { ( "*" | "/" | "%" ) unary_expr }

unary_expr   = ( "-" | "!" ) unary_expr
             | postfix_expr

postfix_expr = primary { postfix_op }

postfix_op   = "[" expression "]"     (* subscript / index      *)
             | "(" arg_list ")"       (* call                   *)
             | "." IDENT              (* property or method name *)

arg_list     = (* empty *) | expression { "," expression }
```

### Primary Expressions

```
primary      = INT
             | FLOAT
             | STRING
             | RAW_STR
             | INTERP_STR
             | "true"
             | "false"
             | "null"
             | ident_or_struct_lit
             | array_literal
             | hash_literal
             | fn_literal
             | "(" expression ")"

ident_or_struct_lit
             = IDENT                           (* plain identifier *)
             | IDENT "{" field_init_list "}"   (* struct literal   *)

             (* IMPORTANT: struct literals are SUPPRESSED in condition
                positions — see "Disambiguation Rules" below.
                In those positions, IDENT "{" is always an identifier
                followed by a block, never a struct literal. *)

field_init_list
             = (* empty *)
             | field_init { "," field_init }

field_init   = IDENT ":" expression

array_literal= "[" [ expression { "," expression } ] "]"

hash_literal = "{" [ pair { "," pair } ] "}"
             (* "{" in expression position is always a hash literal.
                "{" after a keyword (if/while/fn/etc.) is always a block.
                There is no ambiguity because the parser context is always known. *)

pair         = expression ":" expression

fn_literal   = "fn" "(" [ params ] ")" [ ":" type_name ] block
             (* anonymous function; use named_fn_stmt for top-level named fns *)

params       = param { "," param }

param        = [ type_name ] IDENT [ "?" ] [ "..." ]
             (* Type annotation is optional, but if ANY param has a type,
                ALL params must have types (all-or-nothing rule).
                "?" indicates a default value follows (not shown here for brevity).
                "..." makes this param variadic (must be the last param).
                Order: [type] name [?] [...] *)

type_name    = IDENT
             (* Valid type names: integer, float, string, boolean, array, hash,
                function, null, task, channel, error, tuple, struct.
                Type names are plain identifiers (not reserved keywords);
                any identifier can be used as a type. *)
```

### String Interpolation

An interpolated string (`INTERP_STR`) contains embedded kLex expressions:

```
"Hello, {name}! Result: {1 + 2}"
```

Each `{…}` is parsed as a full expression using a nested parser instance. Nested braces are tracked by depth, so function bodies and hashes inside interpolations work correctly:

```
"value: {fn(x) { x + 1 }(5)}"   -- valid
```

Use `\{` to include a literal `{` without triggering interpolation:

```
"not interpolated: \{name}"      -- prints: not interpolated: {name}
```

---

## DISAMBIGUATION RULES

### 1. Struct Literals in Condition Position

The parser suppresses struct literal parsing after the following keywords:
* `if`
* `while`
* `for` (in the collection expression)
* `switch` (subject)
* `case` (each match expression)

This prevents the opening brace of a block from being misread as the start of a struct literal:

```
if x != handler {         -- "handler {" is NOT a struct literal here
    ...
}
```

Outside these positions, `IDENT` immediately followed by `{` is always parsed as a struct literal.

### 2. Named Function vs Anonymous Function

* `"fn"` followed immediately by `IDENT` at statement level:
  ```
  fn foo(x) { x + 1 }      -- named function; desugars to: foo = fn(x) { x + 1 }
  ```

* `"fn"` followed by `(` anywhere:
  ```
  fn(x) { x + 1 }          -- anonymous function literal
  ```

### 3. Block vs Hash Literal

* `{` after a keyword or in statement position → block
* `{` in expression position (right of `=`, inside another expression, etc.) → hash literal

Because the parser always knows its context, these cases never overlap.

### 4. Dot Access vs Float Literal

`"."` is only a property accessor when it appears after a `postfix_expr`. `"."` followed by digits is not a valid token — floats must have a leading digit: `"3.14"` is valid, `".14"` is not.

### 5. Comparison Chaining

Comparison operators are left-associative and can chain syntactically:
```
a == b == c   parses as   (a == b) == c
```

This is a syntax-level property; whether the result is well-typed is a runtime concern (`bool == bool` is valid; `bool == int` is a `TypeError`).

### 6. Index Assign vs Index Read

Syntactically, `expr[i] = val` is an `index_assign_stmt` only at the statement level — the `=` is detected after the full index expression is parsed. Inside a larger expression, `expr[i]` is always a read.

### 7. Assignment to Reserved Literals

Assigning to `"true"`, `"false"`, or `"null"` is a parse-time error:
```
null = 3    -- parse error: cannot assign to reserved literal
```

---

## SEMANTIC CONSTRAINTS

*(enforced by the evaluator, not the parser)*

These are not grammar rules but are listed here because they shape what programs are meaningful, and they affect language design decisions.

* Conditions for `"if"` and `"while"` must evaluate to exactly `BOOL`. Integer, null, or any other type in condition position is a `TypeError`.

* Cross-type equality (other than null comparisons) is a `TypeError`. `"1 == true"` is a `TypeError`. `"null == anything"` is always safe.
  * **Exception:** an `ENUM` instance may be compared with an `ENUM` variant using `==` or `!=` in either direction — both operand orderings are valid.
    ```
    shape == Shape.Circle   (* instance on left *)
    Shape.Circle == shape   (* variant on left — identical semantics *)
    ```

* Arithmetic on two integers yields integer; integer OP float (or vice versa) yields float. All other type combinations are `TypeError`s.

* Ordering operators (`<`, `>`, `<=`, `>=`) are defined for `(int, int)`, `(float, float)`, `(int, float)`, `(float, int)`, and `(string, string)`. String comparison is lexicographic by Unicode code point order. Any other type combination is a `TypeError`.

* Function arity is strict for non-variadic functions. Calling `foo(a, b)` with one or three arguments is a `RuntimeError`.

* Struct literals must initialise every declared field exactly once. Missing fields and unknown fields are both `RuntimeError`s.

* **Type Annotation Rules** (optional annotations for documentation):
  * Type annotations on function parameters and return types are optional.
  * If ANY parameter in a function has a type annotation, ALL parameters must have type annotations (all-or-nothing rule). Mixing annotated and unannotated parameters is a `ParseError`.
  * Return type annotations are optional even when parameters are annotated.
  * Type annotations are not enforced at runtime and do not affect runtime behavior.
  * Valid type names are plain identifiers: `integer`, `float`, `string`, `boolean`, `array`, `hash`, `function`, `null`, `task`, `channel`, `error`, `tuple`, `struct`.
  * Type annotations are available for IDE hover tooltips and code documentation.

* Multi-assign requires the right-hand side to be a `Tuple` with exactly as many elements as there are names on the left.

* `"break"` and `"continue"` are loop-local. They do not propagate through function call boundaries. Using either outside a loop is a `RuntimeError`.

* `_` is the discard identifier. It is valid on the left-hand side of any assignment form (`assign_stmt`, `let_stmt`, `multi_assign_stmt`, `for_stmt` loop variables, `select` recv bindings). The right-hand side is evaluated for side effects but the value is not stored. Reading `_` in an expression is always a `RuntimeError`: `"_ is a discard — its value cannot be read"`. Multiple `_` in the same scope are independent — they do not shadow each other.

* **Enum switch exhaustiveness:** when a switch has an enum subject and no case matches, a `RuntimeError` is raised if there is no default clause.
  ```
  switch: Shape.Rect not handled — add a case or a default
  ```
  An explicit empty `default {}` suppresses the check — use it to express intentional partial matching. This check does not apply to non-enum subjects (integer, string, etc.) where the domain is open.

* `"return"` inside a struct method or function closes only that function. It never exits an outer function.

* `error()` creates a first-class `Error` value (type `"ERROR"`). It is NOT a propagation signal. `isError()` ignores it; it stays in the environment like any other value. Only errors produced internally by the evaluator propagate automatically.

* `format()` argument count must match the number of format specifiers exactly. Too few or too many arguments is a `RuntimeError`. Verb/type mismatches (e.g. `%d` given a string) are also `RuntimeError`s.

* **Functions transform; assignments mutate.** No function in kLex mutates its arguments. Functions that operate on collections (`push`, `pop`, `slice`, `sort`, `reverse`, `map`, `filter`, `reduce`) return a new value and leave the original unchanged. The caller must capture the return value or it is discarded.

  ```
  arr = push(arr, x)   (* correct — result captured *)
  push(arr, x)         (* discard — arr is unchanged; the language permits
                          this syntactically but it is always a logic error
                          when the intent was to modify arr *)
  ```

  Assignment forms (`index_assign_stmt`, `dot_assign_stmt`) always mutate the target in place. There is no function in the core language or stdlib that violates this rule on either side.

---

## BUILT-IN FUNCTIONS

*(always in scope — no import required)*

**Extracted from source:** use `go run tools/docwalk/extract_builtins.go` to verify completeness.

### Output & Input

* `println(val, ...)` — prints each argument on its own line
* `print(val, ...)` — prints each argument with no newline
* `input(prompt)` — reads a line from stdin

### Console Output (Text Colorization)

* `color_red()` `color_green()` `color_blue()` `color_yellow()` `color_magenta()` `color_cyan()` `color_white()` `color_black()` — foreground color codes
* `color_bold()` `color_dim()` `color_underline()` `color_reset()` — text style codes
* `color_bg_red()` `color_bg_green()` `color_bg_blue()` `color_bg_yellow()` `color_bg_magenta()` `color_bg_cyan()` `color_bg_white()` `color_bg_black()` — background color codes
* `colorize(text, colorCode)` — wraps text with color code and auto-resets

### Type Conversion

* `type(val)` — returns type name as `STRING`
* `str(val)` — any value → `STRING`
* `int(val)` — `STRING` | `FLOAT` → `INTEGER`; crashes on bad input — use `parseInt()` for untrusted strings
* `float(val)` — `STRING` | `INTEGER` → `FLOAT`; crashes on bad input — use `parseFloat()` for untrusted strings

### Safe Number Parsing

Use when input is untrusted (CSV, HTTP, databases). Returns `(value, null)` on success or `(null, error)` on failure. Whitespace is trimmed automatically.

* `parseInt(str: STRING)` → `(INTEGER, error)` — parse string as integer; gives a helpful error if the string looks like a float
* `parseFloat(str: STRING)` → `(FLOAT, error)` — parse string as float; also accepts integer-formatted strings

### String Operations

* `len(string)` — character count (Unicode code points)
* `split(str, sep)` — `STRING` → `ARRAY` of `STRING`s
* `join(arr, sep)` — `ARRAY` of `STRING`s → `STRING`
* `upper(str)` `lower(str)` — case conversion
* `trim(str)` — strip leading/trailing whitespace
* `replace(str, old, new)` — replace every occurrence
* `indexOf(str, sub)` — first index or -1
* `startsWith(str, pre)` — `BOOLEAN`
* `endsWith(str, suf)` — `BOOLEAN`
* `substr(str, start [, end])` — substring by 0-based index; end exclusive
* `format(template, args...)` — printf-style formatting

### String Encoding (Low-level)

* `_base64Encode(str)` — encode string to base64
* `_base64Decode(str)` — decode base64 to string
* `_base64UrlEncode(str)` — URL-safe base64 encoding
* `_base64UrlDecode(str)` — URL-safe base64 decoding
* `_urlEncode(str)` — URL percent-encoding
* `_urlDecode(str)` — URL percent-decoding

### Array & Collection Operations

* `len(array)` — element count
* `push(arr, val)` — new array with val appended
* `pop(arr)` — new array without last element
* `concat(arr1, arr2)` — new array containing all elements of arr1 followed by arr2; single allocation
* `makeArray(size)` — new `ARRAY` with `size` null elements; O(n) pre-allocation
* `makeArray(size, default)` — new `ARRAY` with `size` elements, each initialized to `default`
  * **CRITICAL:** Use instead of `push()` in loops (avoids O(n²))
* `range(stop)` `range(start, stop)` `range(start, stop, step)` — `INTEGER` arrays
* `slice(arr, start [, end])` — sub-array by 0-based index; end exclusive
* `sort(arr)` — new sorted array (generic sort; elements must be comparable)
* `sortBy(arr, fn)` — new sorted array using `fn(a, b) -> BOOLEAN`; true = a before b
* `shuffle(arr)` — new array with elements in random order

### Hash Operations

* `keys(h)` `values(h)` — `ARRAY`s of keys / values
* `hasKey(h, key)` — `BOOLEAN`
* `delete(h, key)` — removes key in place; returns null

### Higher-Order Functions

* `map(arr, fn)` — apply `fn(el)` → new `ARRAY`
* `filter(arr, fn)` — keep elements where `fn(el)` is true
* `reduce(arr, fn, init)` — fold to single value; `fn` may be user-defined or built-in

### Math Functions

* `abs(x)` → same type — absolute value
* `acos(x)` → `FLOAT` — arc cosine; x in [-1, 1]
* `asin(x)` → `FLOAT` — arc sine; x in [-1, 1]
* `atan(x)` → `FLOAT` — arc tangent
* `atan2(y, x)` → `FLOAT` — arc tangent of y/x, correct quadrant
* `ceil(x)` → `INTEGER` — round up
* `cos(x)` → `FLOAT` — cosine (radians)
* `e()` → `FLOAT` — Euler's number (2.718…)
* `exp(x)` → `FLOAT` — e raised to x
* `floor(x)` → `INTEGER` — round down
* `fmod(a, b)` → `FLOAT` — floating-point remainder; both args must be `FLOAT`
* `log(x)` → `FLOAT` — natural logarithm; x must be positive
* `log2(x)` → `FLOAT` — base-2 logarithm; x must be positive
* `log10(x)` → `FLOAT` — base-10 logarithm; x must be positive
* `max(a, b)` — maximum of two numbers; returns original type
* `min(a, b)` — minimum of two numbers; returns original type
* `mod(a, b)` → `INTEGER` — integer remainder (a % b); both args must be `INTEGER`
* `pi()` → `FLOAT` — π (3.14159…)
* `pow(base, exp)` → `FLOAT` — base raised to exp
* `rand()` → `FLOAT` — random float in [0, 1)
* `randInt(min, max)` → `INTEGER` — random integer in [min, max] inclusive
* `round(x)` → `INTEGER` — round to nearest integer
* `shuffle(arr)` → `ARRAY` — new array with elements in random order
* `sin(x)` → `FLOAT` — sine (radians)
* `sqrt(x)` → `FLOAT` — square root; x must be non-negative
* `tan(x)` → `FLOAT` — tangent (radians)

### Error Handling

* `error(code, message)` — create user `ERROR` value
* `isError(val)` — `BOOLEAN` — true for any Error
* `safe(fn, args...)` — returns `(result, null)` or `(null, userError)`
* `assert(condition [, message])` — `RuntimeError` if condition is false

### Concurrency & Channels

* `channel([n])` — new `CHANNEL`; unbuffered if n omitted
* `send(ch, val)` — null on success; false if cancelled; `RuntimeError` if closed
* `recv(ch)` — `TUPLE` `(val, BOOLEAN ok)`
* `recvNonBlock(ch)` — returns value or `null` immediately if channel is empty (non-blocking)
* `close(ch)` — signal no more sends
* `cancel(ch)` — signal producer to stop
* `async(fn, args...)` — returns `TASK`
* `await(task)` — blocks; returns task result
* `sleep(ms)` — pause ms milliseconds

### Parallel Processing

From `parallel.lex` (stdlib, built on `async()` and channels):

* `pmap(arr, func, numWorkers)` — `STREAM` of transformed elements (parallel)
* `parallel_filter(arr, func, numWorkers)` — `STREAM` of filtered elements (parallel)
* `parallel_reduce(arr, chunkFn, mergeFn, numWorkers, init)` — `TUPLE` `(value, null)` or `(null, error)`
* `parallel_every(arr, func, numWorkers)` — `BOOLEAN` — true iff `func(el)` true for all
* `parallel_some(arr, func, numWorkers)` — `BOOLEAN` — true iff `func(el)` true for any
* `parallel_find(arr, func, numWorkers)` — `TUPLE` `(element, null)` or `(null, null)`

### Parallel Array Primitives (built-in)

Native Go builtins that partition an array across `runtime.NumCPU()` goroutines.
No channels, no `Task` wrappers — workers run in lock-free env snapshots so user
callbacks do not contend on the global env mutex. Use these for dense data-parallel
numeric kernels (vector scaling, element-wise transforms, reductions).

* `parallelArrayUpdate(arr, fn)` — mutates `arr` in place; `fn(value, index)` return value replaces element. Returns the same array.
* `parallelArrayMap(arr, fn)` — returns new `ARRAY`; source unchanged. `fn(value, index) → new value`.
* `parallelArrayReduce(arr, fn, init)` — parallel fold; `fn(acc, element)` MUST be associative. Each worker reduces a chunk from `init`, then a final serial pass combines partials.
* `parallelArrayForEach(arr, fn)` — parallel iteration with no return value collection. Pairs naturally with atomic ops for lock-free shared-state updates.

### Atomic Arrays (lock-free shared state)

Fixed-size integer/float arrays whose elements can be read and written safely
from multiple goroutines without locks. Backed by `sync/atomic` (int64 ops, with
float64 stored as int64 bits and updated via CAS-loop).

These are the primitive that lets workers running under `parallelArray*` or
`async()` write to shared mutable state without channel overhead or serial
merge phases.

* `atomicIntArray(size, [initial])` — new `AtomicIntArray`
* `atomicFloatArray(size, [initial])` — new `AtomicFloatArray`
* `atomicLoad(arr, idx)` — atomic read; returns `INTEGER` or `FLOAT`
* `atomicStore(arr, idx, value)` — atomic write; returns `null`
* `atomicAdd(arr, idx, delta)` — atomic add; returns the new value
* `atomicCAS(arr, idx, old, new)` — compare-and-swap; returns `BOOLEAN`
* `len(arr)` — returns the array's size

### Concurrent Hash (lock-free shared key/value)

A thread-safe hash map backed by Go's `sync.Map`. Use when keys are dynamic
(not known up front) and multiple goroutines need to share state. Integrates
with normal `[]` indexing syntax — reads and writes are atomic.

* `concurrentHash()` — new `ConcurrentHash`
* `ch[key]` — atomic read; returns the value or `null`
* `ch[key] = value` — atomic write (Swap semantics)
* `delete(ch, key)` — atomic delete
* `len(ch)` — O(1) atomic-counter-backed size
* `keys(ch)` — snapshot `ARRAY` of current keys
* `atomicHashIncr(ch, key, delta)` — atomic integer increment; returns new value
* `atomicHashAdd(ch, key, delta)` — atomic float add; returns new value
* `atomicHashCAS(ch, key, old, new)` — compare-and-swap; returns `BOOLEAN`

### File System (Low-level)

* `_fsExists(path)` — `BOOLEAN`
* `_fsRead(path)` → `(STRING, error)` — entire file; load all into memory
* `_fsReadChunk(path, offset, byteCount)` → `(STRING, BOOLEAN, error)` — stream read; (content, isEOF, err)
* `_fsWrite(path, content)` → `(null, error)`
* `_fsAppend(path, content)` → `(null, error)`
* `_fsRemove(path)` → `(null, error)`
* `_fsRemoveAll(path)` → `(null, error)`
* `_fsMkdir(path)` → `(null, error)`
* `_fsMkdirAll(path)` → `(null, error)`
* `_fsRename(oldPath, newPath)` → `(null, error)`
* `_fsCopy(src, dst)` → `(null, error)`
* `_fsListDir(path)` → `(ARRAY, error)`
* `_fsReadDir(path)` → `(ARRAY, error)` — detailed file info
* `_fsStat(path)` → `(HASH, error)` — file metadata
* `_fsLstat(path)` → `(HASH, error)` — metadata without following symlinks
* `_fsSymlink(target, linkPath)` → `(null, error)`
* `_fsReadlink(path)` → `(STRING, error)`
* `_fsChmod(path, mode)` → `(null, error)`
* `_fsTmpDir()` → `(STRING, error)` — system temp directory
* `_fsTmpFile([prefix])` → `(STRING, error)` — create temp file

**Simple file helpers (no error tuple — RuntimeError on failure):**

* `readFile(path: STRING)` → `STRING` — read entire file contents
* `writeFile(path: STRING, content: STRING)` → `null` — write/overwrite file
* `appendFile(path: STRING, content: STRING)` → `null` — append to file

### Operating System (Low-level)

* `_osGetenv(key)` → `STRING` or null
* `_osSetenv(key, val)` → `(null, error)`
* `_osCwd()` → `(STRING, error)` — current working directory
* `_osHostname()` → `(STRING, error)` — machine hostname
* `_osPid()` → `INTEGER` — process ID
* `_osArgs()` → `ARRAY` — command-line arguments
* `_osExit(code)` — terminate process (does not return)

### Process Execution (Low-level)

* `_processExec(cmd, args)` → `(stdout, stderr, exitCode, error)`
* `_processRun(cmd, args)` → `(stdout, stderr, exitCode, error)`
* `_processShell(cmd)` → `(stdout, stderr, exitCode, error)`

**Simple process helpers (RuntimeError on failure):**

* `exec(cmd: STRING, args: ARRAY)` → `STRING` — run command, return stdout
* `env(name: STRING)` → `STRING | null` — get environment variable

### Networking (Low-level)

* `tcpDial(addr)` → `(CONN, error)` — connect to TCP server
* `tcpListen(addr)` → `(LISTENER, error)` — listen on TCP port
* `netRead(conn)` → `(STRING, error)` — read from connection
* `netWrite(conn, data)` → `(null, error)` — write to connection
* `netClose(conn)` → `(null, error)` — close connection
* `_httpDo(method, url, headers, body)` → `(response, error)` — HTTP request
* `_httpServe(addr, handler)` → `(null, error)` — HTTP server
* `dnsLookup(hostname)` → `(ARRAY, error)` — DNS resolution

### Native Bridge (cross-language FFI)

kLex communicates with subprocesses via line-delimited JSON over stdin/stdout. Any language that can read/write JSON lines can be a bridge — Python, Rust, C, Node, anything.

**Protocol** (subprocess implements this):
- Receive on stdin: `{"id": N, "fn": "name", "args": [...]}`
- Send on stdout:   `{"id": N, "result": value}` or `{"id": N, "error": "message"}`

* `nativeBridge(cmd: STRING, args: ARRAY)` → `(BRIDGE, error)` — start subprocess; `args` is an array of string arguments
* `bridgeCall(bridge: BRIDGE, fn: STRING, args: ARRAY)` → `(result, error)` — call a function; all kLex types marshal to JSON automatically; calls are serialised (safe from async tasks)
* `bridgeClose(bridge: BRIDGE)` → `null` — send EOF to subprocess and wait for it to exit

**Type mapping** (kLex ↔ JSON): `INTEGER`/`FLOAT` ↔ number · `STRING` ↔ string · `BOOLEAN` ↔ boolean · `NULL` ↔ null · `ARRAY` ↔ array · `HASH` ↔ object

### Database (SQL Server and PostgreSQL)

All database operations return `(value, null)` on success or `(null, ERROR)` on failure — the standard kLex two-path error pattern. All db builtins accept either a `DB_CONN` or `DB_TX` as their first argument unless otherwise noted.

**Connection**

* `dbOpen(driver: STRING, dsn: STRING)` → `(DB_CONN, error)` — open connection pool; driver is `"mssql"` or `"postgres"`; verifies connectivity with a ping
* `dbOpenWithPool(driver: STRING, dsn: STRING, options: HASH)` → `(DB_CONN, error)` — like `dbOpen` with explicit pool settings; options keys: `"maxIdle"` (INTEGER), `"maxOpen"` (INTEGER), `"idleTimeout"` (INTEGER seconds), `"lifetime"` (INTEGER seconds); all optional
* `dbClose(conn: DB_CONN)` → `null` — close the connection pool
* `dbPing(conn: DB_CONN)` → `(null, error)` — verify connection is still alive

**Querying**

* `dbQuery(conn, sql: STRING [, args: ARRAY])` → `(ARRAY, error)` — SELECT; returns array of hashes; column names are hash keys; SQL NULLs → kLex `null`; loads all rows into memory
* `dbQueryOne(conn, sql: STRING [, args: ARRAY])` → `(HASH|null, error)` — SELECT; returns first row as hash or `null` if no rows; use for primary-key lookups
* `dbQueryStream(conn, sql: STRING [, args: ARRAY])` → `(CHANNEL, error)` — SELECT; returns a channel yielding one hash per row; rows never all in memory at once; `break` from `for-in` cancels the stream early

**Executing**

* `dbExec(conn, sql: STRING [, args: ARRAY])` → `(INTEGER, error)` — INSERT / UPDATE / DELETE / DDL; returns rows affected (`-1` if unavailable)
* `dbBulkInsert(conn, table: STRING, columns: ARRAY, rows: ARRAY)` → `(INTEGER, error)` — multi-row INSERT in a single round trip; auto-batches within driver parameter limits (MSSQL 2000 / Postgres 60000 params per batch); returns total rows affected; **WARNING**: `table` and `columns` are interpolated into SQL — never pass user-controlled values

**Timeouts**

* `dbSetTimeout(conn: DB_CONN | DB_TX, ms: INTEGER)` → `null` — set a per-operation timeout; all subsequent query/exec calls fail with `DB_TIMEOUT_ERROR` if they exceed `ms` milliseconds; pass `0` to remove; `dbBegin` propagates the conn's timeout to the new `DB_TX`

**Returning rows from DML**

* `dbExecReturning(conn, sql: STRING [, args: ARRAY])` → `(ARRAY, error)` — executes INSERT/UPDATE/DELETE with `RETURNING` (Postgres) or `OUTPUT` (MSSQL) and returns result rows as an array of hashes; use when you need the auto-generated key or affected data back from the driver

**Transactions**

* `dbBegin(conn: DB_CONN)` → `(DB_TX, error)` — start a transaction
* `dbCommit(tx: DB_TX)` → `(null, error)` — commit
* `dbRollback(tx: DB_TX)` → `null` — roll back; safe to call even after commit or prior rollback

All query and exec builtins accept either `DB_CONN` or `DB_TX` as the connection argument so the same code works inside or outside a transaction.

**Transaction helper (`stdlib/db.lex`)**

```
import "stdlib/db.lex" as db

result, err = db.withTx(conn, fn(tx) {
    ...
    return value, null    // success → auto-commit
    return null,  err     // failure → auto-rollback
})
```

`withTx(conn: DB_CONN, body: FUNCTION)` → `(value, error)` — runs `body(tx)` inside a transaction; commits on `(value, null)` return; rolls back on `(value, error)` return or if `dbCommit` fails; `body` must return a two-element tuple.

**Parameter placeholders**

Always pass values as an `ARRAY` argument — never interpolate into the SQL string. Placeholder style is driver-dependent:
* MSSQL / SQL Server — `?`
* PostgreSQL — `$1`, `$2`, `$3`, ...

### Regular Expressions (Low-level)

* `_regexMatch(pattern, str)` → `(BOOLEAN, error)`
* `_regexFind(pattern, str)` → `(STRING|null, error)`
* `_regexFindAll(pattern, str)` → `(ARRAY, error)`
* `_regexReplace(pattern, str, repl)` → `(STRING, error)`
* `_regexReplaceAll(pattern, str, repl)` → `(STRING, error)`
* `_regexSplit(pattern, str)` → `(ARRAY, error)`
* `_regexGroups(pattern, str)` → `(ARRAY, error)` — capture groups from first match
* `_regexGroupsAll(pattern, str)` → `(ARRAY, error)` — all capture group matches

### Time (Low-level)

* `_timeNow()` → `INTEGER` — current Unix timestamp in nanoseconds
* `_timeParse(layout, str)` → `(INTEGER, error)` — parse time string to timestamp
* `_timeFormat(layout, t)` → `(STRING, error)` — format timestamp to string
* `_timeFields(t)` → `(HASH, error)` — break timestamp into year/month/day/hour/min/sec/nsec/weekday
* `_timeNanos()` → `INTEGER` — current time in nanoseconds

### UUID

* `_uuid()` → `STRING` — generate random UUID v4

### Encryption (AES-256-GCM)

* `_aesEncrypt(plaintext: STRING, key: STRING)` → `(STRING, STRING|NULL)` — encrypts with AES-256-GCM; returns `(ciphertext_hex, null)` on success or `(null, error)` on failure; key shorter than 32 bytes is PBKDF2-SHA256 derived; every call produces a unique ciphertext (random 12-byte nonce prepended)
* `_aesDecrypt(ciphertext_hex: STRING, key: STRING)` → `(STRING, STRING|NULL)` — decrypts output from `_aesEncrypt`; returns `(plaintext, null)` on success or `(null, error)` if key is wrong or data has been tampered with (GCM authentication tag enforced)

---

## GRAPHICS BUILTINS (macOS / Linux — OpenGL 4.1)

Graphics builtins are only available when an OpenGL window is open. All colour values are `FLOAT` in [0.0, 1.0]. Coordinates are pixel-space with origin at top-left.

### Window

* `window(width: INTEGER, height: INTEGER, title: STRING, drawFn: FUNCTION)` — open resizable window; calls `drawFn(frameCount)` every frame; blocks until closed
* `frameRate(fps: FLOAT)` — cap render loop to fps frames per second; `0` reverts to vsync; safe to call before `window()`

### State

* `background(r, g, b)` | `background(gray)` | `background(r, g, b, a)` — clear screen
* `fill(r, g, b)` | `fill(gray)` | `fill(r, g, b, a)` — set fill colour
* `noFill()` — disable fill
* `stroke(r, g, b)` | `stroke(gray)` | `stroke(r, g, b, a)` — set stroke colour
* `noStroke()` — disable stroke
* `strokeWeight(w: FLOAT)` — stroke line width in pixels

### Shapes

* `rect(x, y, w, h: FLOAT)` — rectangle, top-left origin
* `circle(x, y, radius: FLOAT)` — circle (64-segment approximation)
* `ellipse(x, y, rx, ry: FLOAT)` — ellipse with separate x/y radii
* `line(x1, y1, x2, y2: FLOAT)` — line using current stroke colour
* `triangle(x1, y1, x2, y2, x3, y3: FLOAT)` — triangle
* `polygon(points: ARRAY)` — arbitrary polygon; flat array `[x1,y1, x2,y2, ...]`; at least 3 points
* `point(x, y: FLOAT)` — single pixel at (x, y); colour = current stroke; size = `strokeWeight`
* `roundedRect(x, y, w, h, r: FLOAT)` — SDF rounded rectangle with corner radius `r`; mathematically perfect AA edges

### Gradients

* `gradient(x, y, w, h: FLOAT, color1, color2: ARRAY, dir: STRING)` → `null` — GPU linear gradient fill; `color1`/`color2` are `[r,g,b,a]` float arrays; `dir`: `"h"` = horizontal (left→right), `"v"` = vertical (top→bottom); can be called inside or outside `uiBegin()/uiEnd()`

### Images

* `loadImage(path: STRING)` → `IMAGE` — load PNG or JPEG; safe to call before `window()`
* `drawImage(img: IMAGE, x, y: FLOAT)` — draw at natural size
* `drawImage(img: IMAGE, x, y, w, h: FLOAT)` — draw scaled to w×h

### Text

* `text(str: STRING, x, y: FLOAT [, scale: FLOAT])` — embedded 8×8 monospace bitmap font; scale multiplies character size (default 1 = 8px); tinted with current fill colour

### Custom Fonts

* `loadFont(path: STRING [, ptSize: INTEGER])` → `FONT` — load `.ttf`/`.otf`; default 16pt SDF atlas; GPU upload deferred to first draw
* `textFont(font: FONT, str: STRING, x, y: FLOAT [, scale: FLOAT])` → `null` — draw string with proportional SDF font; respects current fill colour
* `textWidth(font: FONT, str: STRING [, scale: FLOAT])` → `FLOAT` — pixel width of string at given scale; use for right-align or centering

### Transforms

* `translate(x, y: FLOAT)` — move origin
* `rotate(angle: FLOAT)` — rotate axes by angle radians
* `scale(sx, sy: FLOAT)` — scale axes
* `pushMatrix()` — push current transform onto stack
* `popMatrix()` — pop and restore saved transform

### Input

* `keyDown(key: STRING)` → `BOOL` — true while key is held
* `keyPressed(key: STRING)` → `BOOL` — true on the single frame key was first pressed
* `mouseX()` → `FLOAT` — cursor x in window pixels
* `mouseY()` → `FLOAT` — cursor y in window pixels
* `mouseDown()` → `BOOL` — true if left mouse button is held
* `mouseClicked()` → `BOOL` — true only on the frame left button was first pressed (one-shot)
* `mouseRightDown()` → `BOOL` — true while right mouse button is held
* `mouseRightClicked()` → `BOOL` — true once on the frame right button was first pressed (one-shot)
* `mouseScrollY()` → `FLOAT` — vertical wheel delta this frame; positive = up, negative = down; resets each frame
* `mouseScrollX()` → `FLOAT` — horizontal wheel/trackpad delta this frame; resets each frame

### Query

* `frameCount()` → `INTEGER` — frames rendered since window opened
* `elapsedTime()` → `FLOAT` — seconds since window opened
* `winWidth()` → `INTEGER` — window width in pixels
* `winHeight()` → `INTEGER` — window height in pixels
* `fontCharWidth()` → `INTEGER` — pixel width of one font character at scale 1
* `fontCharHeight()` → `INTEGER` — pixel height of one font character at scale 1

### SDF Shapes

* `roundedRect(x, y, w, h, r: FLOAT)` — SDF-rendered rounded rectangle with corner radius r; mathematically perfect AA edges; respects fill and stroke state

### Particle Batch

* `drawParticles(xs, ys, rs, gs, bs, alphas: ARRAY, count: INTEGER, pointSize: FLOAT)` — render up to count particles in a single GPU draw call; SoA colour arrays; alpha < 0.01 skipped

### Key Name Strings

`"A"`–`"Z"`, `"0"`–`"9"`, `"SPACE"`, `"ENTER"`, `"ESC"`, `"LEFT"`, `"RIGHT"`, `"UP"`, `"DOWN"`, `"BACKSPACE"`, `"SHIFT"`, `"CTRL"`, `"TAB"`

---

## UI WIDGETS (immediate-mode, OpenGL — macOS / Linux)

Lightweight immediate-mode UI built on top of the graphics layer. Every draw frame must call `uiBegin()` first and `uiEnd()` last. Hover detection is one frame deferred; at 60fps this is imperceptible.

### Frame Lifecycle

* `uiBegin()` — reset UI element registry; call once at start of draw callback
* `uiEnd()` — update hover state for the next frame; call once at end

### Focus Management

* `uiNextFieldID()` → `STRING` — returns the ID the next `textInput` will claim; call immediately before `textInput()` to capture it for Tab navigation
* `uiGetFocus()` → `STRING` — returns the currently focused widget ID; `""` if nothing focused
* `uiSetFocus(id: STRING)` — programmatically set focus to widget with given ID; use with `uiNextFieldID()` and `keyPressed("TAB")` to implement Tab key cycling between fields
* `uiSetFont(font: FONT)` — set the active font for all widget text (button labels, tabs, etc.); call after `loadFont()`; reverts on `uiResetFont()`
* `uiResetFont()` — revert widget text back to the embedded monospace font

Tab navigation pattern:
```
fid1 = uiNextFieldID()
f1 = textInput("", f1, x, y, w, h)
fid2 = uiNextFieldID()
f2 = textInput("", f2, x, y, w, h)
if keyPressed("TAB") {
  cur = uiGetFocus()
  if cur == fid1 { uiSetFocus(fid2) } else { uiSetFocus(fid1) }
}
```

### Display Widgets

* `label(text: STRING, x, y: INTEGER [, size: FLOAT])` — static text label; size = text scale (default 0.5)
* `progressBar(x, y, w, h: INTEGER, value, max: FLOAT)` — display-only filled bar; fill fraction = value / max; no interaction

### Interactive Widgets

* `button(label: STRING, x, y, w, h: INTEGER [, size: FLOAT])` → `BOOL` — returns true on the click frame
* `checkbox(label: STRING, x, y: INTEGER, checked: BOOL [, size: FLOAT])` → `BOOL` — returns new checked state
* `toggle(label: STRING, x, y: INTEGER, on: BOOL [, size: FLOAT])` → `BOOL` — pill-shaped on/off switch; returns new state
* `radio(label: STRING, x, y: INTEGER, value: STRING, groupValue: STRING [, size: FLOAT])` → `STRING` — one option in a radio group; returns value if clicked else groupValue unchanged; chain calls to build a group
* `textInput(label: STRING, text: STRING, x, y, w, h: INTEGER [, size: FLOAT])` → `STRING` — full cursor and selection support; click to position cursor; shift+click extends selection; Left/Right move by char; Ctrl+Left/Right jump by word; Home/End; Shift+arrow extends selection; Ctrl+A select all; Backspace/Delete delete before/after cursor or selection; text scrolls horizontally to track cursor; Ctrl/Cmd+Z undo (word-boundary, 50 steps), Ctrl/Cmd+Shift+Z or Y redo, Ctrl/Cmd+V/C/X paste/copy/cut; label drawn 80px left of field (use `""` to suppress)
* `slider(label: STRING, x, y, w: INTEGER, value, min, max: FLOAT [, size: FLOAT])` → `FLOAT` — drag handle on capsule track; label + current value displayed above; returns clamped value
* `numericStepper(label: STRING, x, y, w: INTEGER, value, min, max: INTEGER [, size: FLOAT])` → `INTEGER` — [−] N [+] stepper; better than slider for discrete integers; clamped to [min, max]
* `getTypedChars()` → `STRING` — printable characters typed this frame; use inside custom focus blocks; returns `""` when nothing typed

### Selection Widgets

* `list(label: STRING, items: ARRAY, x, y, w, h: INTEGER [, size: FLOAT])` → `STRING` — scrollable list; mouse wheel to scroll; click to select; returns selected item
* `dropdown(label: STRING, items: ARRAY, x, y, w: INTEGER [, size: FLOAT])` → `STRING` — header bar + pop-down menu; call after other widgets so open menu renders on top
* `tabs(x, y, w: INTEGER, items: ARRAY, activeIdx: INTEGER [, size: FLOAT])` → `INTEGER` — horizontal tab bar; returns new active index; active tab shown with accent underline

### Multi-line Input

* `textArea(label: STRING, text: STRING, x, y, w, h: INTEGER [, size: FLOAT])` → `STRING` — multiline editable text box; click to focus; Enter for newline; mouse wheel scrolls; label drawn above box

### Data Grid

* `table(headers: ARRAY, rows: ARRAY, x, y, w, h: INTEGER [, size: FLOAT])` → `INTEGER` — scrollable data grid; headers: string array; rows: array of string arrays; click to select row; returns selected row index (-1 = none); set `fill()` to a bright colour before calling for readable text

### Collapsible Sections

* `accordion(x, y, w: INTEGER, sections: ARRAY, openIdx: INTEGER [, size: FLOAT])` → `INTEGER` — stacked clickable headers; returns open section index (-1 = all closed)

### Context Menu

* `contextMenu(x, y: INTEGER, items: ARRAY, visible: BOOL [, size: FLOAT])` → `INTEGER` — floating popup menu; call after all other widgets; returns index of clicked item, -1 if none, -2 if clicked outside (dismiss)

### Advanced Widgets

* `colorPicker(x, y, w: INTEGER, r, g, b, a: FLOAT)` → `ARRAY` — four RGBA sliders with live preview swatch; returns [r, g, b, a]; values in [0.0, 1.0]
* `treeView(x, y, w, h: INTEGER, labels: ARRAY, levels: ARRAY, expanded: ARRAY [, size: FLOAT])` → `ARRAY` — hierarchical tree; `labels[i]` = display text, `levels[i]` = indent depth (0=root), `expanded[i]` = bool; returns [selectedIdx, newExpanded]
* `scrollArea(x, y, w, h: INTEGER, contentH: FLOAT)` → `FLOAT` — draws panel+scrollbar; returns scroll offset in pixels; use with `pushClip`/`popClip` to clip content
* `splitter(pos, x, y, length: INTEGER, orient: STRING, min, max: INTEGER [, thickness: INTEGER])` → `INTEGER` — interactive resize handle between two panels; `orient`: `"v"` = vertical bar (drags left/right), `"h"` = horizontal bar (drags up/down); `pos` is the current bar position — pass it in and reassign the return value each frame; `thickness` defaults to 6; cursor changes to a resize arrow on hover automatically
* `modal(title: STRING, message: STRING, buttons: ARRAY)` → `STRING` — full-screen dimmed overlay with a centred dialog; returns the clicked button label or `""` each frame until clicked; call after all other widgets and before `uiEnd()`

### Layout Cursors

Layout cursors eliminate manual x/y arithmetic. Two are provided — a horizontal row cursor and a vertical column cursor — both backed by the Go graphics state so values are reliable across frames.

**Row cursor (horizontal layout)**

* `uiBeginRow(x, y, h, gap: FLOAT)` → `null` — initialise row cursor at (x, y); `h` = item height, `gap` = spacing between items
* `uiRowX()` → `INTEGER` — current X position; place next widget here
* `uiRowY()` → `INTEGER` — row Y (constant)
* `uiRowH()` → `INTEGER` — row height (constant); pass as `h` to each widget
* `uiRowAdvance(w: FLOAT)` → `null` — advance cursor right by `w + gap`

**Column cursor (vertical layout)**

* `uiBeginCol(x, y, w, gap: FLOAT)` → `null` — initialise col cursor at (x, y); `w` = column width, `gap` = spacing between items
* `uiColX()` → `INTEGER` — col X (constant)
* `uiColY()` → `INTEGER` — current Y position; place next widget here
* `uiColW()` → `INTEGER` — col width (constant); pass as `w` to full-width widgets
* `uiColAdvance(h: FLOAT)` → `null` — advance cursor down by `h + gap`

Both cursors can be composed — start a row inside a col by calling `uiBeginRow(uiColX(), uiColY(), rowH, gap)` then `uiColAdvance(rowH)` afterward.

All layout cursor builtins must be called between `uiBegin()` and `uiEnd()`.

### Theming

* `makeTheme()` → `ARRAY` — return the 13-slot default palette; each slot is a `[r, g, b, a]` float array; modify slots then call `uiTheme()`
* `uiTheme(palette: ARRAY)` → `null` — install a 13-slot palette as the global widget color theme; all widgets read from this palette each frame

Slot index reference:

| Index | Name | Controls |
|-------|------|----------|
| 0 | widgetBg | button / widget normal background |
| 1 | widgetBgHover | hovered state |
| 2 | widgetBgActive | pressed / checked state |
| 3 | widgetText | text on interactive widgets |
| 4 | labelText | label text above/beside widgets |
| 5 | dimText | secondary / unselected text |
| 6 | accent | accent line / stripe |
| 7 | accentBg | selected item / active tab background |
| 8 | track | slider / progress track |
| 9 | trackFill | slider / progress fill |
| 10 | handle | slider handle, scrollbar thumb, checkmark |
| 11 | inputBg | text input / list container background |
| 12 | inputFocusBg | focused text input background |

---

*End of Grammar*
