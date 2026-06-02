# kLex FORMAL GRAMMAR (EBNF)

The kLex language is nicknamed **FROG**: Functional, Reactive, Opinionated, Governed

This document is the authoritative source of truth for kLex syntax. Before adding any new syntax to the language, write it here first and check it for ambiguities. If you cannot express the new construct unambiguously in this grammar, it is not yet ready to implement.

**Derived from:** `lexer/lexer.go` and `parser/parser.go`  
**Keep in sync:** whenever either file is changed.

**Execution backends.** kLex programs run on one of two backends:

* **Tree-walker (default).** Reference implementation. Every construct
  in this grammar is supported; semantics here describe the tree-walker.
* **Bytecode VM (`--vm`, experimental).** A second backend that compiles
  the AST to bytecode for tighter dispatch loops. Most of the language
  works under `--vm`, but some advanced constructs (e.g. enum patterns
  require a switch subject and exactly one value per case; certain
  async + select patterns) are still being brought up — they fall back
  to a clear `"VM unimplemented: ..."` error rather than misbehaving.
  Treat `--vm` as a performance opt-in, not the canonical path. If a
  program runs on the tree-walker but fails under `--vm`, that is a VM
  bring-up gap, not a language ambiguity.

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
switch  case  default  struct  enum  select  let  const  import  as
true  false  null
```

Source of truth: `keywords` map in `lexer/lexer.go`. Identifiers that match any
of these tokenise to the corresponding keyword instead of `IDENT`.

**Reserved keywords cannot be used as parameter names.** This applies to every
keyword in the table above (`fn`, `if`, `else`, `case`, `let`, etc.). The parser
rejects them at function-definition time with a clear error pointing at the
offending name — e.g. `invalid parameter name 'fn': reserved keywords cannot be
used as parameter names`. Pick an alternative: `func`, `f`, `callback`,
`item`, `tc`, etc.

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
                  "unterminated string literal".
                  A bare "}" outside an interp_expr is a ParseError:
                  "bare '}' is reserved inside a string" — use \} or }} *)

str_char     = escape_seq | brace_double
             | (* any character except '"', bare "{", bare "}", or "\" *)
escape_seq   = '\"'         (* literal double quote          *)
             | '\n'         (* newline                        *)
             | '\t'         (* tab                            *)
             | '\\'         (* literal backslash              *)
             | '\r'         (* carriage return                *)
             | '\b'         (* backspace                      *)
             | '\{'         (* literal "{" — NOT interpolation *)
             | '\}'         (* literal "}" — symmetric with \{ *)
             | '\x' HEX HEX (* byte with hex value (e.g. \x1b for ESC) *)
brace_double = '{{'         (* literal "{" (Python f-string style) *)
             | '}}'         (* literal "}" *)
interp_expr  = "{" expression "}"
               (* Inside the {…}, contents are real kLex code — no escape
                  processing. Inner string literals are recognised by the
                  recursive lexer, so `"<{f("a")}>"` is the natural form;
                  there is no need to escape inner quotes. *)

RAW_STR      = '`' { any_char_except_backtick } '`'
               (* No escape processing. No interpolation. Every character
                  including newlines and backslashes is taken literally.
                  Use for multi-line text, file paths, SQL, HTML, etc.
                  A backtick cannot appear inside a raw string.
                  A raw string not closed before end-of-file is a ParseError:
                  "unterminated raw string literal" *)

BYTES        = 'b' '"' { bytes_char } '"'
               (* binary literal — the `b` prefix is only consumed when
                  immediately followed by `"`. Bare `b` remains a normal
                  identifier so existing code using `b` as a variable name
                  keeps working.

                  Escape sequences supported: \"  \\  \n  \r  \t  \0  \xHH
                  No {expr} interpolation — `{` is literal inside bytes.
                  Indexing returns an integer in [0,255], len() returns the
                  byte count. See LANGUAGE.TXT "BYTES" for full semantics. *)
```

**Three delimiter rules — no overlap:**
* `"..."`  — escape sequences and `{expr}` interpolation are active
* `` `...` `` — raw: all characters literal, nothing processed
* `b"..."` — bytes: escape sequences active, NO interpolation, indexing is byte-numeric

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
                  second IDENT: value — only meaningful when iterating a hash

                  kLex deliberately has NO C-style numeric form
                  (for (i = 0; i < n; i++) { ... }) and NO ++ / --
                  operators. Numeric loops are written explicitly:

                      let i = 0
                      while i < n {
                          ...
                          i = i + 1
                      }

                  This is the same "explicit over implicit" line that
                  rules out `i++` (hides the mutation) and rules out
                  a C-style for header (interleaves three different
                  statement kinds — init, condition, step — into one
                  syntactic slot). The for/in form is reserved for
                  iterating real collections. *)

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

enum_pattern = ( dot_expr | IDENT ) "(" IDENT { "," IDENT } ")"
             (* Destructuring pattern. Two forms:
                  full:  case Shape.Circle(r) { ... }   -- dot_expr function
                  short: case Circle(r) { ... }         -- bare Ident function
                Pattern target must resolve to an enum variant at runtime.
                Each IDENT is a new local variable bound positionally to the
                variant's declared fields for the duration of the case body.
                Binding count must exactly match the variant's field count;
                a mismatch is a RuntimeError.
                Bindings are local to the case arm and do not affect the outer scope.
                Zero-field variants use the plain expression form — no parentheses.

                Parse-time rewriting: a CallExpr whose function is either a
                DotExpr or a bare Ident, AND whose arguments are all bare
                Idents, is rewritten to enum_pattern. This means the parser
                CANNOT distinguish an enum-variant pattern from an ordinary
                function-call pattern at the syntactic level — e.g.

                    case foo(r) { ... }

                where `foo` happens to be a function (not an enum variant)
                parses identically to a real enum pattern. The variant
                check happens at match time and surfaces a clean
                RuntimeError ("X is not an enum variant"), not a silent
                failure. Catching this at parse time would require a
                semantic-analysis pass kLex deliberately does not have;
                the runtime catch is the explicit tradeoff. *)

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
               (* bare "return"     yields null
                  return a          yields a single value
                  return a, b       yields a 2-element Tuple
                  return a, b, c    yields a 3-element Tuple, etc.

                  Multi-element returns must be received with a
                  multi_assign_stmt; assigning the whole Tuple to one
                  name with `t = fn()` leaves t as a *Tuple value
                  whose elements can only be accessed by destructuring.

                  TUPLE SEMANTICS — kLex has NO tuple literal syntax.
                  `(a, b)` is just grouping (a single expression).
                  Tuple VALUES are produced only by:
                    * multi-element return statements
                    * builtins that explicitly document a tuple return
                      (e.g. recv, safe, parallel_reduce, dbQuery)
                  Tuples are a runtime multi-return mechanism, not a
                  user-constructable composite type. If you want a
                  user-constructable ordered tuple-like value, use an
                  Array; for a named record, use a struct. *)

break_stmt   = "break"
continue_stmt= "continue"
```

### Declarations

```
import_stmt  = "import" STRING "as" IDENT
               (* Path is resolved against five locations in order, first
                  existing file wins: (1) path as given (CWD-relative),
                  (2) directory of the importing .lex file, (3) $KLEX_PATH,
                  (4) directory of the klex binary, (5) one level above the
                  binary. See LANGUAGE.TXT "MODULES" for full rationale.

                  Imports are deduplicated by absolute file path for the
                  lifetime of the process — the first import evaluates the
                  module's top-level code, and subsequent imports (under any
                  alias, from anywhere) return a module bound to the SAME
                  underlying environment. Module-level mutable state is
                  therefore process-global. See LANGUAGE.TXT "MODULE CACHING".
                  Import cycles are detected and reported as a RuntimeError. *)

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
                    (* Right-hand side must evaluate to a Tuple with EXACTLY
                       the same number of elements as the left-hand side. Any
                       mismatch (too few or too many) is a RuntimeError:
                       "multi-assign expected N values, got M". kLex does not
                       silently truncate or pad — be explicit about how many
                       elements the producer returns.

                       Discard slots: any of the IDENTs may be `_` to discard
                       the corresponding tuple element. Multiple `_` in the
                       same statement are independent (they do NOT shadow
                       each other). E.g. `_, err = safe(fn)` keeps only the
                       error; `val, _, _ = threeReturns()` keeps only the
                       first.

                       `let` form: `let a, b = expr` declares both names in
                       the current scope; the bare form `a, b = expr` walks
                       the scope chain via Assign() to update existing
                       bindings where they live. *)

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
params       = (* empty *) | param { "," param }

param        = [ type_name ] IDENT [ "..." ] [ "=" expression ]
             (* Order: optional type → name → optional variadic marker → optional default.

                Type annotation:
                  Optional, but ALL-OR-NOTHING — if any parameter is annotated,
                  every parameter must be annotated, or it is a ParseError.

                Variadic ("..."):
                  Must appear on the LAST parameter only. The parameter
                  receives any extra arguments as an array; zero extra
                  arguments yields an empty array, never null. The variadic
                  parameter cannot itself carry a default value.

                Default value ("= expression"):
                  Evaluated at call time in the function's closure environment,
                  not at definition time. Parameters with defaults must follow
                  all required parameters — a required param after a defaulted
                  one is a ParseError. *)
```

`fn_literal` accepts the same `params` block plus an optional return-type
annotation after the closing paren — `fn(integer a): integer { ... }`. See
`fn_literal` in the Primary Expressions section.

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
| 9 | `?` | left (postfix) | error-propagation unwrap |

Source of truth: `precedences` map and `LOWEST..INDEX` constants in
`parser/parser.go`. Higher number = binds tighter.

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
             | "?"                    (* error-propagation unwrap;
                                         the preceding expression must
                                         evaluate to a 2-element tuple
                                         (value, err) — see LANGUAGE.TXT
                                         "ERROR PROPAGATION — THE ? OPERATOR" *)

arg_list     = (* empty *) | expression { "," expression }
```

### Primary Expressions

```
primary      = INT
             | FLOAT
             | STRING
             | RAW_STR
             | INTERP_STR
             | BYTES
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
             (* anonymous function; use named_fn_stmt for top-level named fns.
                The `params` and `param` productions are defined in the
                "Parameters" section above. *)

type_name    = IDENT
             (* Built-in type names (with aliases): int/integer, float,
                number/num, string/str, bool/boolean, null, bytes, array,
                hash/map, function/fn, channel, task, tuple, error, image,
                font, any. A struct or enum type name is also valid and
                matches an instance of that exact type.
                Type names are plain identifiers (not reserved keywords);
                any identifier is syntactically accepted. Where present,
                annotations ARE enforced at runtime — see the semantic
                rules below. *)
```

### String Interpolation

An interpolated string (`INTERP_STR`) contains embedded kLex expressions:

```
"Hello, {name}! Result: {1 + 2}"
```

The lexer builds the alternating text/expression segments in one pass and pre-tokenizes each expression so its tokens carry accurate source line/col. The parser walks the structured segments — there is no raw-byte re-scanning anywhere downstream.

#### Braces are special everywhere

Both `{` and `}` are reserved inside `"..."` strings:

* Bare `{` starts an interpolation expression and must have a matching `}`.
* Bare `}` outside an interpolation is a **parse error** (`bare '}' is reserved inside a string`). No silent "literal close-brace" fallback.

#### Two ways to write a literal brace

```
"path = \{home\}"        -- backslash escape:   path = {home}
"path = {{home}}"        -- brace doubling:     path = {home}
```

Both styles are equivalent. Use whichever reads better in context. For longer JSON / regex / template text, the raw-string syntax avoids both styles entirely:

```
json = `{"users": [], "count": 0}`   -- backtick: no escapes, no interp
```

#### Inner strings inside expressions

Inside `{…}`, you write real kLex code. The recursive lexer handles inner string literals so you do **not** need to escape inner quotes:

```
"hash[\"a\"]"            -- error: \ is illegal in expression position
"<{ h["a"] }>"           -- correct: inner string is recognised naturally
"<{ format("%d", n) }>"  -- correct
```

#### Mixed examples

```
"value: {fn(x) { x + 1 }(5)}"        -- expression contains a function literal
"sum is {{ {1+2} }}"                  -- prints: sum is { 3 }
"score: \{base\}+{bonus}"             -- prints: score: {base}+<bonus value>
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

### 8. Compound Assignment Is Not Valid

The lexer tokenises `+=`, `-=`, `*=`, `/=`, `%=` so the parser can emit a clear
diagnostic, but no compound-assignment construct exists in the grammar:
```
x += 1    -- parse error: operator += is not valid in kLex — use x = x + 1
```
Use plain `x = x + 1` (or for collections, `arr[i] = arr[i] + 1`).

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

* **Type Annotation Rules** (optional annotations, enforced where present):
  * Type annotations on function parameters and return types are optional.
  * If ANY parameter in a function has a type annotation, ALL parameters must have type annotations (all-or-nothing rule). Mixing annotated and unannotated parameters is a `ParseError`.
  * Return type annotations are optional even when parameters are annotated.
  * Where present, annotations are **enforced at runtime** (tree-walker and bytecode VM, identically): a mismatched argument, defaulted value, variadic element, or return value raises a `TypeError` at the call boundary. **Strict null** — a typed parameter rejects `null` unless annotated `any` or `null`.
  * Valid type names (with aliases): `int`/`integer`, `float`, `number`/`num`, `string`/`str`, `bool`/`boolean`, `null`, `bytes`, `array`, `hash`/`map`, `function`/`fn`, `channel`, `task`, `tuple`, `error`, `image`, `font`, `any`, plus any `struct`/`enum` type name.
  * Annotations also drive IDE hover tooltips and signature help.

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

**Extracted from source.** The builtin list below is generated from the Go
source and kept in sync with the language server (LSP) and the VS Code syntax
definition, so it always reflects the builtins the interpreter actually
registers.

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
* `replaceAll(str, old, new)` — replace every occurrence (alias of `replace` with explicit "all" naming for code that prefers it)

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
* `concatAll(arrayOfArrays)` — single-allocation flatten of an array-of-arrays. Use instead of looping `concat()` (which is O(n²) on the growing accumulator).
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
* `apply(fn, args)` — spread an `ARRAY` of args into a positional call; `apply(println, ["hello", "world"]) ≡ println("hello", "world")`

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

### Math — Interpolation & Remapping

* `lerp(a, b, t)` → `FLOAT` — linear interpolation `a + (b-a)*t`; t=0 → a, t=1 → b; not clamped
* `constrain(val, lo, hi)` — clamp `val` to `[lo, hi]`; returns `INTEGER` if `val` is `INTEGER`, `FLOAT` otherwise
* `remap(val, inLow, inHigh, outLow, outHigh)` → `FLOAT` — re-map from one range to another; not clamped (chain with `constrain()` if needed); named `remap` rather than `map` to avoid collision with the higher-order `map(arr, fn)`

### Bitwise (named functions; not operators)

All arguments must be `INTEGER`; any other type is a `TypeError`. Hex/binary/
octal integer literals (`0xFF`, `0b1101`, `0o755`) are integers at runtime.

* `bitAnd(a, b)` → `INTEGER` — bitwise AND
* `bitOr(a, b)` → `INTEGER` — bitwise OR
* `bitXor(a, b)` → `INTEGER` — bitwise XOR
* `bitNot(x)` → `INTEGER` — ones'-complement
* `bitShiftLeft(x, n)` → `INTEGER` — `x * 2^n`; `n` must be ≥ 0
* `bitShiftRight(x, n)` → `INTEGER` — arithmetic right shift; `n` must be ≥ 0

### Bytes (binary values)

* `bytes(x)` — construct a `BYTES` value (`STRING`→utf-8 / `ARRAY`→pack 0..255 / `INTEGER` N→N zero bytes / `BYTES`→defensive copy)
* `strToBytes(s)` → `BYTES` — utf-8 encode a string
* `bytesToStr(b)` → `(STRING, error)` — utf-8 decode; err.code `BYTES_INVALID_UTF8`
* `bytesToBase64(b)` → `STRING` — RFC 4648, padded
* `base64ToBytes(s)` → `(BYTES, error)` — err.code `BASE64_INVALID`
* `bytesToHex(b)` → `STRING` — lowercase hex, no separator
* `hexToBytes(s)` → `(BYTES, error)` — accepts upper or lower; err.code `HEX_INVALID`
* `floatsToBytes(arr)` → `BYTES` — pack a number array as little-endian IEEE-754 float32 bytes (len = 4 × len(arr))
* `bytesToFloats(b)` → `(ARRAY, error)` — unpack little-endian float32 bytes; err.code `BYTES_LEN_INVALID` if len not a multiple of 4
* `bytesConcat(arrayOfBytes)` → `BYTES` — merge an array of `BYTES` into a single value in one Go-level allocation (avoids round-tripping through an int array).
* `_randomBytes(n)` → `BYTES` — n cryptographically-strong random bytes
* `chr(n)` → `STRING` — single-character string from Unicode codepoint
* `ord(c)` → `INTEGER` — codepoint of the first character of `c`

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
* `_fsReadBytes(path)` → `(BYTES, error)` — like `_fsRead` but returns raw bytes (required for binary data)
* `_fsAppendBytesSync(path, bytes)` → `(null, error)` — atomic append + fsync; bytes are durably on disk on return
* `_fsMap(path)` → `(STRING, error)` — memory-map file; content backed by mmap, zero-copy; perfect for parallel analysis of files up to ~1GB
* `_fsCountLines(path)` → `(INTEGER, error)` — stream-count `\n` bytes (matches `wc -l` semantics). Works on multi-GB files without loading into memory.
* `_fsTruncate(path, newSize)` → `error` — truncate to exactly `newSize` bytes (grows with zero-padding if larger; POSIX truncate(2) semantics).

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
* `_osName()` → `STRING` — Go's `runtime.GOOS` verbatim (`"darwin"`, `"linux"`, `"windows"`, ...). Use for cross-platform branching when path-probing would be brittle.
* `_scriptDir()` → `STRING` — absolute directory containing the currently-running .lex script

### Process Execution (Low-level)

* `_processExec(cmd, args)` → `(stdout, stderr, exitCode, error)`
* `_processRun(cmd, args)` → `(stdout, stderr, exitCode, error)`
* `_processShell(cmd)` → `(stdout, stderr, exitCode, error)`
* `_processSpawnDetached(cmd, args, opts?)` → `(pid, error)` — start a child detached from the parent (survives parent exit). Returns immediately; do NOT block on the child's exit. `opts` (all optional): `logFile`, `env`, `dir`. Child is reaped in a background goroutine — no zombie leak.

**Simple process helpers (RuntimeError on failure):**

* `exec(cmd: STRING, args: ARRAY)` → `STRING` — run command, return stdout
* `env(name: STRING)` → `STRING | null` — get environment variable

### Networking (Low-level)

* `tcpDial(host: STRING, port: INTEGER)` → `(CONN, error)` — connect to TCP server
* `tcpListen(host: STRING, port: INTEGER)` → `CHANNEL` of accepted connections — start TCP server; break from `for-in` to stop listening
* `netRead(conn, maxBytes: INTEGER)` → `(STRING, error)` — read up to `maxBytes` from connection
* `netWrite(conn, data: STRING)` → `(null, error)` — write to connection
* `netClose(conn)` → `null` — close connection
* `_httpDo(method, url, headers, body)` → `(response, error)` — HTTP request
* `_httpServe(addr, handler)` → `(null, error)` — HTTP server
* `dnsLookup(hostname: STRING)` → `(ARRAY, error)` — DNS resolution → array of IP strings

### Native Bridge (cross-language FFI)

kLex communicates with subprocesses via line-delimited JSON over stdin/stdout. Any language that can read/write JSON lines can be a bridge — Python, Rust, C, Node, anything.

**Protocol** (subprocess implements this):
- Receive on stdin: `{"id": N, "fn": "name", "args": [...]}`
- Send on stdout:   `{"id": N, "result": value}` or `{"id": N, "error": "message"}`

* `bridgeOpen(transport: HASH)` → `(BRIDGE, error)` — start a bridge from a transport hash. Subprocess transport requires `"kind": "subprocess"` plus `"cmd"` (and optional `"args"`); see KLEX_LANGUAGE.MD for the full schema
* `bridgeCall(bridge: BRIDGE, fn: STRING, args: ARRAY)` → `(result, error)` — call a function; all kLex types marshal to JSON automatically; calls are serialised (safe from async tasks)
* `bridgeClose(bridge: BRIDGE)` → `null` — send EOF to subprocess and wait for it to exit

**Type mapping** (kLex ↔ JSON): `INTEGER`/`FLOAT` ↔ number · `STRING` ↔ string · `BOOLEAN` ↔ boolean · `NULL` ↔ null · `ARRAY` ↔ array · `HASH` ↔ object

### JSON (Low-level)

* `_jsonParse(s: STRING)` → `(value, errStr)` — decode a JSON document into kLex values. `(value, null)` on success, `(null, "<message>")` on parse failure.
* `_jsonStringify(v: any)` → `(STRING, errStr)` — encode a kLex value as JSON. Hash keys are emitted sorted (deterministic output). Errors when a value isn't JSON-representable (Bytes, cycles, NaN/Inf floats).

### Agentic Runtime Hooks (Low-level)

The runtime exposes structured semantic events that an external observer (an LLM, a debugger, a telemetry sink) can subscribe to. Each `_set*Hook` registers a single callable (or `null` to clear). Hooks are zero-cost when no handler is registered and synchronous when invoked. Observer-only — a hook cannot alter the event or interfere with the host program. **Prefer the friendlier `agent.lex` wrappers** (`agent.onErrorBubble` / `onAsyncSpawn` / `onAsyncDone`) over these primitives.

* `_setErrorHook(hookFn: fn | null)` → `null` — Phase 1. Hook fires once per internal `*Error` at the moment it begins propagating up the call stack — BEFORE any `safe()` / `?` / top-level handler sees it. Event hash: `{kind, message, code, line, stack}`. User errors from `error()` do NOT fire the hook.
* `_setAsyncSpawnHook(hookFn: fn | null)` → `null` — Phase 2. Invoked synchronously in the spawning goroutine BEFORE the task's goroutine starts. Event hash: `{task_id, fn, argc, spawned_at}`.
* `_setAsyncDoneHook(hookFn: fn | null)` → `null` — Phase 2. Invoked from the task's OWN goroutine immediately after the body returns. Event hash: `{task_id, duration_ms, ok, error}`. Use `task_id` to pair with the earlier spawn event.
* `_setUiEventHook(hookFn: fn | null)` → `null` — Phase 3. Fires SYNCHRONOUSLY from interactive widgets in `stdlib/ui.lex` (button / slider / checkbox / dropdown / textField / toggle / radio) when the user causes a state change. Pure-display widgets do NOT fire; hover is not emitted. Event hash: `{kind, widget, label, value, x, y}`.
* `_setBridgeCallHook(hookFn: fn | null)` → `null` — Phase 3. Fires once per `_bridgeCall` AFTER the round-trip completes. Event hash: `{fn, argc, duration_ms, ok, error}`. A remote function returning a `(null, errorValue)` tuple is NOT a bridge-call failure — the hook fires with `ok=true`.
* `_uiEventActive()` → `BOOLEAN` — fast gate used by widget instrumentation to skip event-hash construction when no consumer is registered. Single atomic pointer load on the no-hook path.
* `_uiEvent(kind, widget, label: STRING, value: any, x, y: INTEGER)` → `null` — producer-side primitive called by instrumented widgets to emit a UI event. See `_setUiEventHook` for the documented kind / widget vocabulary. Custom widgets may call this directly.

Re-entry guard: if a hook itself errors, the runtime skips the recursive call — no infinite loop.

### Tensors — FrogPy (Low-level)

FrogPy is kLex's NumPy-equivalent: contiguous N-D numeric arrays backed by Go + cgo C kernels. Prefer the `tensor.lex` stdlib wrapper. dtypes: `"f32"` / `"float32"`, `"f64"` / `"float64"`, `"i64"` / `"int64"`.

**Construction & introspection**

* `_tensor_from_array(data: ARRAY, dtype: STRING)` → `TENSOR` — build a 1-D tensor from a flat number array; element conversion is strict (rejects values that don't fit dtype).
* `_tensor_zeros(shape: ARRAY, dtype: STRING)` → `TENSOR` — fresh contiguous zero-filled tensor with the given shape.
* `_tensor_full(shape: ARRAY, value: NUMBER, dtype: STRING)` → `TENSOR` — fresh tensor with every element set to `value`. NumPy parallel: `np.full`. i64 rejects Float values.
* `_tensor_random(shape: ARRAY, dtype: STRING, seed: INTEGER)` → `TENSOR` — pseudo-random tensor. Floats: uniform [0,1) (NumPy `np.random.rand`). i64: full int63 range. `seed > 0` deterministic; `seed == 0` time-seeded. Go `math/rand`; not cryptographic.
* `_tensor_linspace(start: NUMBER, stop: NUMBER, n: INTEGER, dtype: STRING)` → `TENSOR` — 1-D tensor of n evenly spaced values from start to stop inclusive. n must be ≥ 1. NumPy parallel: `np.linspace`.
* `_tensor_arange(start: NUMBER, stop: NUMBER, step: NUMBER, dtype: STRING)` → `TENSOR` — 1-D tensor of values from start up to (but not including) stop, separated by step. step must be non-zero. NumPy parallel: `np.arange`.
* `_tensor_eye(n: INTEGER, dtype: STRING)` → `TENSOR` — n×n identity matrix: 1 on the diagonal, 0 elsewhere. NumPy parallel: `np.eye`.
* `_tensor_dtype(t: TENSOR)` → `STRING` — returns `"f32"`, `"f64"`, or `"i64"`.
* `_tensor_shape(t: TENSOR)` → `ARRAY` — fresh kLex array of integers describing t's shape.
* `_tensor_numel(t: TENSOR)` → `INTEGER` — total element count (product of shape).
* `_tensor_get(t: TENSOR, idx: INTEGER)` → number — linear element access (treats t as a flat 1-D view in row-major logical order); bounds-checked. Works on strided views — the flat index is translated through Strides.
* `_tensor_to_array(t: TENSOR)` → `ARRAY` — extract all elements into a flat kLex array, row-major logical order. f32/f64 elements come back as Float; i64 as Integer. Works on strided views.

**Materialisation & conversion**

* `_tensor_clone(t: TENSOR)` → `TENSOR` — fresh independent contiguous copy. Mutations to the result do not affect t. On a strided view, materialises through stride math (same as `_tensor_contiguous`).
* `_tensor_cast(t: TENSOR, dtype: STRING)` → `TENSOR` — fresh tensor with every element converted to the target dtype. Same conversion rules as `_tensor_from_array`: f32↔f64 lossless widening / truncating; i64 → float is best-effort (exact up to 2^24 / 2^53); float → i64 truncates toward zero.

**Shape operations** (view-based — share backing storage with input)

* `_tensor_reshape(t: TENSOR, newShape: ARRAY)` → `TENSOR` — NumPy-style reshape view. `product(newShape)` must equal `numel(t)`. Contiguous inputs only in v1.
* `_tensor_squeeze(t: TENSOR)` → `TENSOR` — remove all size-1 dimensions; all-1s shape collapses to 0-D scalar. NumPy parallel: `np.squeeze`.
* `_tensor_expand_dims(t: TENSOR, axis: INTEGER)` → `TENSOR` — insert a new size-1 dimension at `axis`. Negative axis OK (counts from end). NumPy parallel: `np.expand_dims`.

**Slicing** (view-based)

* `_tensor_slice(t: TENSOR, specs: ARRAY)` → `TENSOR` — strided view sharing t's backing data. `specs` is an array with one entry per axis of t; each entry is either `null` (full axis) or `[start, stop, step]` (any of start/stop/step may itself be `null` for the default 0/dim/1). Negative start/stop count from end. step must be positive in v1. Output Shape/Strides reflect the slice; mutations through the view affect t and vice versa. Source must itself be contiguous in v1 (slice-of-slice requires `_tensor_contiguous` in between).
* `_tensor_contiguous(t: TENSOR)` → `TENSOR` — if t is contiguous, returns t unchanged (no-op fast path, NumPy `ascontiguousarray` semantics). If t is a strided view, materialises into a fresh contiguous tensor by walking the multi-index through Strides. Required before passing a slice view to any kernel-based op (add, matmul, reductions, etc.).

**Pure element-wise ops** (allocate a fresh result tensor — NumPy semantics with full broadcasting)

* `_tensor_abs(a)` `_tensor_neg(a)` `_tensor_exp(a)` `_tensor_log(a)` `_tensor_sqrt(a)` `_tensor_sin(a)` `_tensor_cos(a)` → `TENSOR` — unary element-wise; float-only for `exp`/`log`/`sqrt`/`sin`/`cos`.
* `_tensor_pow(a, b)` → `TENSOR` — element-wise pow with broadcasting.
* `_tensor_add(a, b)` `_tensor_sub(a, b)` `_tensor_mul(a, b)` `_tensor_div(a, b)` → `TENSOR` — binary element-wise with **NumPy-style broadcasting**: either operand may be a scalar (Integer or Float) or a tensor with a broadcast-compatible shape (e.g. `(m,n) op (n,)` for per-column op; `(m,1) op (1,n)` for outer-style). Dtype must match across tensor operands. `_tensor_div` follows IEEE 754 for f32/f64 (n/0 = ±Inf, 0/0 = NaN); i64 rejects zero divisors pre-kernel with the offending index.
* `_tensor_clip(t: TENSOR, lo: NUMBER, hi: NUMBER)` → `TENSOR` — clamp every element to [lo, hi]. lo and hi must be numbers compatible with t's dtype. `lo <= hi` is required.

**In-place ops** (mutate the LHS tensor, return it; use when the caller owns the LHS)

* `_tensor_abs_inplace(a)` `_tensor_neg_inplace(a)` `_tensor_exp_inplace(a)` `_tensor_log_inplace(a)` `_tensor_sqrt_inplace(a)` `_tensor_sin_inplace(a)` `_tensor_cos_inplace(a)` → `TENSOR` — in-place unary.
* `_tensor_pow_inplace(a, exp)` → `TENSOR` — in-place pow.
* `_tensor_add_inplace(a, b)` `_tensor_sub_inplace(a, b)` `_tensor_mul_inplace(a, b)` `_tensor_div_inplace(a, b)` → `TENSOR` — in-place binary. **Strict shape match — no broadcasting** (so the LHS isn't silently resized).

**Comparisons** (output is always i64 — a 0/1 mask)

* `_tensor_eq(a, b)` `_tensor_ne(a, b)` `_tensor_lt(a, b)` `_tensor_le(a, b)` `_tensor_gt(a, b)` `_tensor_ge(a, b)` → `TENSOR` — element-wise comparison with same broadcasting + scalar promotion as binary element-wise ops. Output dtype is always i64 (0 or 1) regardless of input dtype. NaN comparisons follow IEEE 754: any comparison with NaN returns 0 except `ne` which returns 1.

**Selection**

* `_tensor_where(mask: TENSOR, x: TENSOR | NUMBER, y: TENSOR | NUMBER)` → `TENSOR` — element-wise conditional: `out[i] = x[i] if mask[i] != 0 else y[i]`. mask must be i64 (typically from a comparison op). x and y must have the same dtype; either may be a scalar (broadcast to mask's shape). Shapes must match mask's shape after scalar promotion. NumPy parallel: `np.where`.

**Joining**

* `_tensor_concatenate(tensors: ARRAY, axis: INTEGER)` → `TENSOR` — join an array of tensors along an existing axis. All tensors must have the same dtype, same rank, and identical shapes on every axis except `axis`. NumPy parallel: `np.concatenate`.
* `_tensor_stack(tensors: ARRAY, axis: INTEGER)` → `TENSOR` — join along a NEW axis. All tensors must have identical shapes. Output rank = input rank + 1, with the new axis of size `len(tensors)` inserted at position `axis`. NumPy parallel: `np.stack`.

**Full reductions** (produce a scalar from a tensor)

* `_tensor_sum(t)` → number — accumulator type matches t's dtype.
* `_tensor_mean(t)` → `FLOAT` — `sum/n`; `i64` widens to `f64` for the divide (matches `np.mean` on integer arrays).
* `_tensor_max(t)` `_tensor_min(t)` → number — extreme values; return type matches t's dtype.
* `_tensor_argmax(t)` `_tensor_argmin(t)` → `INTEGER` — index of the extreme element in the flat 1-D view; first occurrence wins on ties.

**Axis-aware reductions** (collapse one axis of an N-D tensor → result of rank N−1)

* `_tensor_sum_axis(t: TENSOR, axis: INTEGER)` → `TENSOR` — sum along one axis. Output dtype matches input. Empty axis returns zeros (sum's identity). NumPy parallel: `np.sum(t, axis=N)`.
* `_tensor_mean_axis(t: TENSOR, axis: INTEGER)` → `TENSOR` — mean along one axis. Output is always f64 dtype. Empty axis errors.
* `_tensor_min_axis(t: TENSOR, axis: INTEGER)` `_tensor_max_axis(t: TENSOR, axis: INTEGER)` → `TENSOR` — extreme values along one axis. Output dtype matches input. Empty axis errors.
* `_tensor_argmin_axis(t: TENSOR, axis: INTEGER)` `_tensor_argmax_axis(t: TENSOR, axis: INTEGER)` → `TENSOR` — index of extreme element along one axis (indices refer to positions within the reduced axis). Output is i64 regardless of input dtype. First-occurrence on ties. Empty axis errors.

axis may be negative (-1 is the last axis). For input shape `[d0, ..., dk, ..., dn-1]` reducing axis k: drop-variant output shape is `[d0, ..., dk-1, dk+1, ..., dn-1]`.

**Keepdims reductions** (same as axis reductions, but the reduced axis is kept at size 1 in the output instead of dropped — composes cleanly with broadcasting)

* `_tensor_sum_axis_keepdims(t, axis)` `_tensor_mean_axis_keepdims(t, axis)` `_tensor_min_axis_keepdims(t, axis)` `_tensor_max_axis_keepdims(t, axis)` `_tensor_argmin_axis_keepdims(t, axis)` `_tensor_argmax_axis_keepdims(t, axis)` → `TENSOR` — for input shape `[d0, ..., dk, ..., dn-1]` the output shape is `[d0, ..., 1, ..., dn-1]`. Empty-axis policy identical to the drop variants. NumPy parallel: `keepdims=True`.

**Linear algebra**

* `_tensor_matmul(a: TENSOR, b: TENSOR)` → `TENSOR` — 2-D matrix multiply C := A · B. Shapes: a `[m, k]`, b `[k, n]`, result `[m, n]`. Both inputs must be 2-D, same dtype, contiguous. Dispatch: f32 routes through Apple `MPSMatrixMultiplication` on macOS (GPU; ~1 TFLOPS @ 1024³ on M4); f32 on Linux + f64/i64 always use the CPU autovec kernel with goroutine row-strip parallelism above `m >= 128` (f64 ~74 GFLOPS @ 1024³ on M4 4× P-cores).
* `_tensor_transpose(t: TENSOR)` → `TENSOR` — 2-D matrix transpose. `[m, n] → [n, m]`, `out[i, j] = in[j, i]`. Materialising (fresh contiguous tensor, does NOT share storage). v1 is 2-D only.
* `_tensor_dot(a: TENSOR, b: TENSOR)` → number — 1-D inner product `sum(a[i] * b[i])`. Both inputs must be 1-D, same dtype, same length, contiguous. Returns a scalar matching the dtype. Fused mul+sum C kernel — no temporary tensor.

### Store Recovery (Low-level)

* `_replaySeenFile(path: STRING)` → `HASH` — read a JSONL file where each line is `{"path": ..., "mtime": ..., ...}`; returns a hash keyed by `path` with the latest `mtime` value (last write wins). Missing file → empty hash. Used by `stdlib/store.lex`'s crash-recovery path to populate `s.seen` without the interpreted-JSON-parser bottleneck.

### VM Compiler Helpers (not user APIs)

* `_iterPrep(coll, twoVar: BOOLEAN)` → `(iterArray, isPairs)` — internal helper emitted by the VM's `compileForIn` lowering. Normalises any iterable into a `(iterArray, isPairs)` tuple the index-based loop can consume. Not intended for direct kLex use.

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

### Hashing & Password / HMAC

* `_md5(data: STRING | BYTES)` → `STRING` — MD5 lowercase hex (32 chars); legacy interop only — NOT for security
* `_sha256(data: STRING | BYTES)` → `STRING` — SHA-256 lowercase hex (64 chars)
* `_sha512(data: STRING | BYTES)` → `STRING` — SHA-512 lowercase hex (128 chars)
* `_hmacSha256(key: STRING | BYTES, data: STRING | BYTES)` → `STRING` — HMAC-SHA256 lowercase hex
* `_hmacSha512(key: STRING | BYTES, data: STRING | BYTES)` → `STRING` — HMAC-SHA512 lowercase hex
* `_bcryptHash(password: STRING [, cost: INTEGER])` → `(STRING, error)` — bcrypt hash (cost default 10)
* `_bcryptVerify(password: STRING, hash: STRING)` → `BOOLEAN` — constant-time bcrypt verify
* `_constantTimeEquals(a: STRING, b: STRING)` → `BOOLEAN` — timing-safe string equality (use when comparing secrets / signatures)

### Compression

* `_gzipCompress(data: STRING | BYTES [, level: INTEGER])` → `(BYTES, error)` — RFC 1952 gzip
* `_gzipDecompress(data: BYTES)` → `(BYTES, error)`
* `_deflateCompress(data: STRING | BYTES [, level: INTEGER])` → `(BYTES, error)` — raw RFC 1951 deflate (no header)
* `_deflateDecompress(data: BYTES)` → `(BYTES, error)`

### CSV (Low-level)

* `_csvParse(s: STRING)` → `(ARRAY<ARRAY<STRING>>, error)` — parse RFC 4180 CSV
* `_csvParseDelim(s: STRING, delim: STRING)` → `(ARRAY<ARRAY<STRING>>, error)` — custom delimiter
* `_csvParseHeaders(s: STRING)` → `(ARRAY<HASH>, error)` — first row becomes hash keys
* `_csvHasRows(s: STRING)` → `BOOLEAN` — quick non-empty test
* `_csvFirstRowCols(s: STRING)` → `(ARRAY<STRING>, error)` — header row only
* `_csvFormat(rows: ARRAY<ARRAY>)` → `STRING` — serialize back to CSV
* `_csvFormatDelim(rows, delim: STRING)` → `STRING` — custom delimiter
* `_csvStream(path: STRING)` → `(CHANNEL, error)` — streaming row-by-row reader for huge files

### HTTP — Streaming

* `_httpStream(method, url, headers, body [, mode])` → `(CHANNEL, error)` — server-sent events / chunked transfer; one channel item per yielded chunk

### MCP Client (Model Context Protocol)

* `_mcpSpawn(cmd: STRING, args: ARRAY [, opts: HASH])` → `(MCP_CLIENT, error)` — start an MCP subprocess
* `_mcpCall(client, method: STRING, params, timeoutSec)` → `(result, error)` — JSON-RPC request
* `_mcpNotify(client, method: STRING, params)` → `(null, error)` — fire-and-forget notification
* `_mcpClose(client)` → `null` — terminate the MCP subprocess
* `_mcpInfo(client)` → `HASH` — handshake info (protocol version, capabilities, tools, …)
* `_mcpNotifications(client)` → `CHANNEL` — server-initiated notifications stream

### MCP Server (Expose kLex As an MCP Server)

* `_mcpServeHTTP(spec: HASH)` → `(MCP_SERVER, error)` — start an HTTP+SSE MCP server, returns immediately
* `_mcpStopServer(server: MCP_SERVER)` → `null` — graceful shutdown, idempotent
* `_mcpServerInfo(server: MCP_SERVER)` → `HASH` — snapshot of name/version/port/tools/stopped

### Build / Version

* `__version__` → `STRING` — kLex runtime version (e.g. "0.3.35"); accessible as a special identifier without parentheses

### PDF (text extraction)

* `_pdfExtractText(path)` → `(STRING, error)` — concatenated text from every page
* `_pdfExtractByPage(path)` → `(ARRAY, error)` — array of per-page text strings
* `_pdfPageCount(path)` → `(INTEGER, error)` — page count without loading text

### TSV (tab-separated values)

* `_tsvParse(s: STRING)` → `ARRAY` of arrays of strings
* `_tsvFormat(rows: ARRAY)` → `STRING`

### Vector / Similarity Math

* `cosineSim(v1, v2: ARRAY)` → `FLOAT` — cosine similarity in [-1, 1]; zero-magnitude vectors return 0
* `dotProduct(v1, v2: ARRAY)` → `FLOAT` — sum of element-wise products
* `vecNorm(vec: ARRAY)` → `FLOAT` — L2 (Euclidean) norm

### Concurrent Hash — additional helper

* `quiesceLen(ch: CONCURRENT_HASH)` → `INTEGER` — exact length AFTER waiting for in-flight mutations to settle; use over `len()` when an exact snapshot is needed under heavy concurrent writes

### Native Bridge — introspection + pool

* `bridgeInfo(bridge)` → `HASH` — protocol version, capabilities, function list
* `bridgeSchema(bridge [, fn])` → `HASH | null` — JSON Schema for a function (or all functions)
* `bridgeStderr(bridge)` → `ARRAY<STRING>` — drained recent stderr ring buffer
* `bridgeStream(bridge, fn, args [, timeout])` → `(CHANNEL, error)` — streaming call; one channel item per yielded message
* `bridgeNotifications(bridge)` → `CHANNEL` — unsolicited notifications from the subprocess
* `bridgeMetrics(bridge)` → `HASH` — counters + per-function p50/p95/p99 latency
* `bridgePool(n: INTEGER, transport: HASH [, opts: HASH])` → `(POOL, error)` — spawn N identical workers from a transport hash; `opts` holds pool-only keys (e.g. `init`)
* `bridgePoolCall(pool, fn, args [, timeout])` → `(any, error)` — call via idle worker
* `bridgePoolStream(pool, fn, args [, timeout])` → `(CHANNEL, error)` — streaming variant
* `bridgePoolHealth(pool)` → `HASH` — per-worker health + restart history
* `bridgePoolStderr(pool)` → `ARRAY<STRING>` — combined stderr across workers
* `bridgePoolClose(pool)` → `null` — shut down every worker

### Image (low-level pixel access)

* `imageFromRgba(bytes: BYTES, w: INTEGER, h: INTEGER)` → `IMAGE` — wrap raw RGBA8 pixels (w*h*4 bytes) into an Image without PNG decode
* `imageSize(img: IMAGE)` → `(INTEGER, INTEGER)` — (width, height) tuple
* `imageToRgba(img: IMAGE)` → `BYTES` — raw RGBA8 pixels; reads pre-upload CPU buffer if available, else GL readback
* `saveImage(img: IMAGE, path: STRING)` → `null` — encode + write; extension picks format (.png / .jpg / .gif)
* `image(img, x, y, w, h [, mode])` — draw an Image at a rect; mode = "stretch" (default) | "fit" | "fill"
* `droppedFiles()` → `ARRAY<STRING>` — drained list of paths dragged onto the window since last call

### Image FX (Go-backed cross-platform pixel filters)

All take `(bytes: BYTES, w: INTEGER, h: INTEGER, ...)` and return `(BYTES, error)`. Alpha preserved. sRGB-encoded byte math (matches Photoshop / Lightroom defaults).

* `_imgExposure(bytes, w, h, stops: FLOAT)` — multiply RGB by 2^stops
* `_imgBrightness(bytes, w, h, amount: FLOAT)` — additive in [-1, +1]
* `_imgContrast(bytes, w, h, amount: FLOAT)` — scale around 0.5
* `_imgSaturation(bytes, w, h, amount: FLOAT)` — pull toward / away from luma
* `_imgHueShift(bytes, w, h, degrees: FLOAT)` — HSV rotate (wraps mod 360)
* `_imgGamma(bytes, w, h, gamma: FLOAT)` — `c ^= 1/gamma` per channel
* `_imgLevels(bytes, w, h, inB, inW, gamma, outB, outW: FLOAT)` — Photoshop levels
* `_imgChannelMixer(bytes, w, h, matrix9: ARRAY)` — 3×3 RGB matrix
* `_imgInvert(bytes, w, h)` — photo negative
* `_imgDesaturate(bytes, w, h)` — every channel → Rec.709 luma
* `_imgSepia(bytes, w, h, strength: FLOAT)` — mix sepia-toned by strength
* `_imgVignette(bytes, w, h, strength: FLOAT, radius: FLOAT)` — centre-darkening

### Metal / GPU (macOS only — error tuple elsewhere)

Most users will reach for `stdlib/mtl.lex` which wraps these. Cross-platform scripts should branch on `mtl.isAvailable()` first.

**Device:**

* `_mtlInfo()` → `(HASH, error)` — name, registry_id, has_unified_memory, supports_raytracing, supports_apple7, max_threads_per_group, max_bind_slots

**Surfaces:**

* `_mtlSurface(w, h: INTEGER)` → `(handle, error)` — blank RGBA8 texture
* `_mtlSurfaceFromBytes(rgba: BYTES, w, h: INTEGER)` → `(handle, error)` — upload pixels (w*h*4 bytes)
* `_mtlSurfaceReadRgba(handle)` → `(BYTES, error)` — readback
* `_mtlSurfaceSavePng(handle, path)` → `CHANNEL(null, error)` — async PNG write
* `_mtlClear(handle, color: ARRAY)` → `CHANNEL(null, error)` — async clear-to-color (4-element RGBA in [0,1])
* `_mtlSurfaceRelease(handle)` — free

**Kernels:**

* `_mtlKernel(msl: STRING, fnName: STRING)` → `(handle, error)` — compile MSL pipeline
* `_mtlKernelRelease(handle)` — free

**Buffers:**

* `_mtlBuffer(arr: ARRAY)` → `(handle, error)` — float32 buffer from array
* `_mtlBufferFromTensor(t: TENSOR)` → `(handle, error)` — zero-marshalling upload from a FrogPy f32 tensor's `[]float32` slice. ~5× faster than `_mtlBuffer` for large arrays (skips the per-element kLex-Float→float32 conversion loop). Tensor must be DType f32 and contiguous.
* `_mtlBufferU32(arr: ARRAY)` → `(handle, error)` — uint32 buffer
* `_mtlBufferAllocF32(count: INTEGER)` → `(handle, error)` — N zeroed float32
* `_mtlBufferAllocU32(count: INTEGER)` → `(handle, error)` — N zeroed uint32
* `_mtlReadBuffer(handle)` → `(ARRAY<FLOAT>, error)`
* `_mtlReadBufferU32(handle)` → `(ARRAY<INTEGER>, error)`
* `_mtlReadBufferIntoTensor(handle, t: TENSOR)` → `(null, error)` — zero-allocation readback: copies an f32 MTLBuffer's contents directly into a pre-allocated f32 contiguous tensor's `[]float32` slice. Skips the per-element `*Float` construction that `_mtlReadBuffer` pays. Tensor must be DType f32, contiguous, and sized exactly to the buffer's element count.
* `_mtlBufferRelease(handle)` — free

**Dispatch:**

* `_mtlDispatch(kernel, bindings: HASH, grid: ARRAY)` → `CHANNEL(null, error)` — async; bindings has `textures`, `buffers`, `accels` arrays of handles

**Batching:**

* `_mtlBatchBegin()` → `(batch, error)`
* `_mtlBatchDispatch(batch, kernel, bindings, grid)` → `(null, error)` — encode into open batch
* `_mtlBatchCommit(batch)` → `CHANNEL(null, error)` — async commit + wait
* `_mtlBatchRelease(batch)` — abandon batch (error path)

**MPS primitives:**

* `_mtlMatmulMPS(a, b, c, m, k, n)` → `CHANNEL(null, error)` — C = A·B
* `_mtlFFT(buf, n, inverse)` → `CHANNEL(null, error)` — 1D complex-to-complex
* `_mtlHistogram(buf, count, numBins, minVal, maxVal)` → `CHANNEL(ARRAY<INTEGER>, error)`

**Acceleration structures:**

* `_mtlAccel(vertex_buffer, vertex_count)` → `CHANNEL(handle, error)` — flat triangle list
* `_mtlAccelIndexed(vertex_buffer, vertex_count, index_buffer, index_count)` → `CHANNEL(handle, error)`
* `_mtlAccelRelease(handle)`

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
* `arc(cx, cy, radius, startAngle, endAngle: FLOAT [, segments: INTEGER])` — arc / partial circle; angles in radians, drawn anticlockwise from `startAngle` to `endAngle`; `segments` defaults to a count chosen for smooth-looking edges

### Path API (Canvas-style)

For curves and shapes that don't fit the primitive shapes. Build a path with move/line/curve commands then fill or stroke it.

* `beginPath()` — start a fresh empty path
* `moveTo(x, y: FLOAT)` — move cursor without drawing
* `lineTo(x, y: FLOAT)` — straight line from cursor to (x, y)
* `quadTo(cpx, cpy, x, y: FLOAT)` — quadratic Bézier through control point
* `bezierTo(c1x, c1y, c2x, c2y, x, y: FLOAT)` — cubic Bézier with two control points
* `closePath()` — line back to path start
* `fillPath()` — fill current path with current fill colour
* `strokePath()` — stroke current path with current stroke colour + weight

### Gradients & Blending

* `gradient(x, y, w, h: FLOAT, color1, color2: ARRAY, dir: STRING)` → `null` — GPU linear gradient fill; `color1`/`color2` are `[r,g,b,a]` float arrays; `dir`: `"h"` = horizontal (left→right), `"v"` = vertical (top→bottom); can be called inside or outside `uiBegin()/uiEnd()`
* `blendMode(mode: STRING)` — set blend mode: `"normal"` | `"add"` | `"multiply"` | `"screen"`
* `hsl(h, s, l [, a]: FLOAT)` → `ARRAY` — construct an `[r,g,b,a]` colour from HSL (hue 0-360, saturation/lightness 0-1)
* `noShadow()` — disable any previously-set drop shadow

### Charts (high-level data visualisation)

* `barChart(data: ARRAY, x, y, w, h: FLOAT [, min, max: FLOAT])` — bar chart
* `lineChart(data: ARRAY, x, y, w, h: FLOAT [, min, max: FLOAT])` — line chart connecting points
* `pieChart(data: ARRAY, colors: ARRAY, cx, cy, radius: FLOAT [, innerRadius: FLOAT])` — pie or donut
* `sparkline(data: ARRAY, x, y, w, h: FLOAT)` — minimal one-line trend, no axes

### Images

* `loadImage(path: STRING)` → `IMAGE` — load PNG or JPEG; safe to call before `window()`
* `loadImage(bytes: BYTES)` → `IMAGE` — same but from in-memory encoded bytes
* `drawImage(img: IMAGE, x, y: FLOAT)` — draw at natural size
* `drawImage(img: IMAGE, x, y, w, h: FLOAT)` — draw scaled to w×h
* `image(img: IMAGE, x, y, w, h: INTEGER [, mode: STRING])` — draw at integer rect; mode = `"stretch"` (default) | `"fit"` | `"fill"`
* `saveImage(img: IMAGE, path: STRING)` → `null` — encode + write; extension picks format
* `droppedFiles()` → `ARRAY<STRING>` — drained list of paths dragged onto the window since last call

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
* `progressBar(x, y, w, h: INTEGER, value, min, max: FLOAT)` — display-only filled bar; fill fraction = (value − min) / (max − min), clamped [0, 1]; no interaction
* `lineHeight([scale: FLOAT])` → `INTEGER` — pixel height of one line of widget text at scale (default 0.5); useful for vertical centering: `y + (h - lineHeight()) / 2`

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
* `listMulti(label: STRING, items: ARRAY, selected: ARRAY, x, y, w, h: INTEGER [, size: FLOAT])` → `ARRAY` — multi-select variant; `selected` is the array of currently-selected items; returns the updated selection
* `dropdown(label: STRING, items: ARRAY, x, y, w: INTEGER [, size: FLOAT])` → `STRING` — header bar + pop-down menu; call after other widgets so open menu renders on top
* `tabs(x, y, w: INTEGER, items: ARRAY, activeIdx: INTEGER [, size: FLOAT])` → `INTEGER` — horizontal tab bar; returns new active index; active tab shown with accent underline

### Multi-line Input

* `textArea(label: STRING, text: STRING, x, y, w, h: INTEGER [, size: FLOAT] [, syntax: STRING])` → `STRING` — multiline editable text box; click to focus; Enter for newline; mouse wheel scrolls; label drawn above box. The optional trailing args are order-independent: a number sets the font scale, the string `"klex"` enables kLex syntax highlighting (colours adapt to the theme background)

### Data Grid

* `table(headers: ARRAY, rows: ARRAY, x, y, w, h: INTEGER [, size: FLOAT])` → `INTEGER` — scrollable data grid; headers: string array; rows: array of string arrays; click to select row; returns selected row index (-1 = none); set `fill()` to a bright colour before calling for readable text

### Collapsible Sections

* `accordion(x, y, w: INTEGER, sections: ARRAY, openIdx: INTEGER [, size: FLOAT])` → `INTEGER` — stacked clickable headers; returns open section index (-1 = all closed)

### Context Menu

* `contextMenu(x, y: INTEGER, items: ARRAY, visible: BOOL [, size: FLOAT])` → `INTEGER` — floating popup menu; call after all other widgets; returns index of clicked item, -1 if none, -2 if clicked outside (dismiss)

### Disabled State

* `pushDisabled(disabled: BOOL)` → `null` — push disabled state; all interactive widgets between `pushDisabled(true)` and `popDisabled()` render at half opacity and ignore hover/click; stack-based so nested regions can selectively re-enable
* `popDisabled()` → `null` — pop the top disabled frame; every `pushDisabled` must have a matching `popDisabled`

### Notifications

* `toast(message: STRING [, style: STRING] [, duration: FLOAT])` — non-modal notification toast; style: `"info"` (default) | `"success"` | `"warning"` | `"error"`; duration in seconds (default ~3); auto-dismisses
* `tooltip(text: STRING)` — show hover text for the widget drawn immediately before this call. Appears after the cursor rests on the widget for ~0.5s.

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
* `setTheme(name: STRING)` → `null` — install a preset theme by name; valid: `"nebula"` (default), `"light"`, `"dark"`, `"highContrast"`; sets colour palette and style tokens (radii, spacing, font); call after `window()` opens

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

### WASM / Browser-Only Builtins

These two builtins are registered **only** in the WebAssembly build (`//go:build js && wasm`) and have no desktop equivalent. They support the browser-hosted kLex Playground IDE. See [WASM.MD](WASM.MD) for the full WASM reference.

* `runScript(src: STRING)` → `HASH` — evaluate `src` as a complete, isolated kLex program in a fresh environment; returns a hash with keys `output` (printed text), `error` (message, empty on success) and `isError` (bool); definitions do not persist between calls; scripts calling `window()` are rejected
* `openURL(url: STRING)` → `null` — open `url` in a new browser tab via `window.open(url, "_blank")`

---

*End of Grammar*
