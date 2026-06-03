# kLex Documentation Standard

How every builtin (and, by extension, every stdlib function) is documented so
that `BUILTINS.MD`, `STDLIB.MD`, the editor hover/signature help, and the
playground's searchable index all come from **one source of truth** and stay
honest.

This is the spec we write against. It exists because the older docs drifted in
three directions (Go comments, hand-kept LSP signatures, the playground index)
and rendered inconsistently — run-on summaries, `(…)` signatures with no
parameter names, missing return types, and internal maintainer notes leaking
into user-facing pages.

## Principles

FROG docs are **pure, honest, and explicit** — the same discipline as the
language:

1. **One source of truth.** A single structured doc block above each builtin
   feeds the reference docs, the LSP, and the playground search index. There is
   no second place to update.
2. **Docs can't lie.** Every `@example` is *executed* — on **both** the
   tree-walker and the bytecode VM (via the differential runner) — and checked
   against its stated result. An example that rots fails the build.
3. **Explicit types and arity.** Every parameter and the return value are typed,
   using the language's own annotation vocabulary. No `(…)`.
4. **Honest about failure.** Because kLex is strictly typed and uses the
   `(value, err)` tuple idiom, the failure modes are documented as carefully as
   the success path. This is what "hard to misuse" means in practice.
5. **No implementation noise.** Maintainer notes (audit history, VM internals,
   performance rationale) never reach user-facing docs.

## The doc block

A builtin is documented by the comment block immediately above its
`Builtins["name"] = …` registration in `eval/builtins_*.go`:

```go
// split — break a string into pieces on a separator.
//
// Returns the substrings between each occurrence of sep, in order. Use it to
// tokenise input; pair with join to round-trip. Splitting on "" yields one
// entry per Unicode code point (rune), not per byte.
//
// @sig     split(str: string, sep: string) -> array
// @param   str  the string to split
// @param   sep  the separator; "" splits into individual runes
// @returns an array of string pieces; never null, never an error
// @errors  TypeError if str or sep is not a string
// @example split("a,b,c", ",")  → ["a", "b", "c"]
// @example split("café", "")    → ["c", "a", "f", "é"]
// @since   0.1.0
// @see     join, replace, substr
//
// impl: rune path only allocates on the first non-ASCII access — see String.
Builtins["split"] = &Builtin{ … }
```

### First line — the summary

`name — one concise sentence, ending in a period.` This single line is the
listing summary and the search-index summary, so keep it short and declarative
(what it does, not how). Mirrors Rust/Elixir: the first line is harvested
everywhere a one-liner is needed.

### Body — what and why

One or more plain prose paragraphs after the summary. Say what the builtin does
and, crucially, **why you'd reach for it** — not just mechanics. Reference other
builtins/functions by bare name so they auto-link.

### Tags

| Tag | Required | Meaning |
|---|---|---|
| `@sig` | **yes** | The canonical signature — the single source for arity, parameter names, parameter types, and return type. Drives the reference docs, the LSP, and search. |
| `@param` | one per parameter | `@param name  description` — meaning, constraints, and the effect of special values (empty string, zero, negative, null). |
| `@returns` | **yes** | What comes back on success. For fallible builtins, document the `(value, err)` tuple shape and what each side holds. |
| `@errors` | **yes** | The failure modes: which `TypeError`s strict typing raises, any `RuntimeError`s, and — for the tuple idiom — when `err` is non-null. If genuinely infallible, write `@errors none`. |
| `@example` | **yes (≥1)** | A runnable line: `expr  → expected`. Executed and checked on both interpreters. |
| `@since` | recommended | Version the builtin first appeared. |
| `@see` | optional | Comma-separated related names; auto-linked. |
| `@category` | optional | Overrides the category otherwise derived from the source file. |

### `@sig` grammar

```
@sig  name(p1: type, p2: type, [optional: type], rest...: type) -> returnType
```

- **Types** use the annotation vocabulary: `int`/`integer`, `float`, `number`,
  `string`/`str`, `bool`/`boolean`, `array`, `hash`, `function`, `null`, `any`,
  `bytes`, `tuple`, `channel`, `task`, `error`, `image`, `font`, plus any struct
  or enum type name.
- **Optional** parameters are wrapped in `[ ]`.
- **Variadic** uses `...` (`rest...: type` — the type applies to each element).
- **Fallible** builtins return a tuple: `-> (value-type, error)`. Document both
  sides under `@returns`/`@errors`.

### `@example` and doctests

Each `@example` is `expr  →  expected`, where `expected` is the printed output
(for `println`-style calls) or the returned value's literal form. The doctest
runner evaluates `expr` in a fresh environment on **both** interpreters and
asserts equality; the differential runner additionally guarantees tree-walker
output == VM output. Conventions:

- **Hidden setup.** A line prefixed with `#` is executed but not shown in the
  rendered docs (for `let x = …` scaffolding). Mirrors rustdoc's hidden lines.
- **Unrunnable examples.** Side-effecting, random, time-, network-, or
  graphics-dependent examples are marked `@example no-run` — shown but not
  executed.
- Examples should demonstrate *why*, including at least one edge case
  (empty input, the special value, the error path).

### `impl:` — maintainer notes

Any line in the block beginning `impl:` (or a trailing `impl:` paragraph) is for
maintainers and is **never rendered** to user-facing docs. This is where audit
history, VM-internal rationale, and performance notes belong — out of the
reference, but still next to the code.

## Minimum bar for "documented"

A builtin counts as documented when it has: a first-line summary, `@sig`, a
`@param` for each parameter, `@returns`, `@errors` (or `@errors none`), and at
least one executable `@example`. The coverage tool reports progress against this
bar and gates a floor in CI.

## Priority

Backfill is ordered by **call count** (most-used builtins first), category by
category. Public builtins (no leading `_`) come before internal/private ones
(`_wasm*`, `_mtl*`, …), which get lighter treatment.

## stdlib

The same convention applies to per-function doc comments in `stdlib/*.lex`
(alongside the existing `@module`/`@summary` header). Because stdlib is written
in kLex, its examples doctest the most naturally of all. Lower priority than
builtins.
