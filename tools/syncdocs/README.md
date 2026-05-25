# syncdocs

kLex documentation sync tool. Audits every gap between **Go source**, **LSP
docs**, **VS Code syntax**, and the **language/grammar reference files** —
then optionally fixes the mechanical ones.

## The problem

A kLex builtin is described in five places:

1. Its Go registration (`Builtins["foo"] = &Builtin{...}` in `eval/*.go`).
2. Its arity guard (`if len(args) != N { return typeError(...) }`).
3. Its LSP entry in `snowball/froglsp/builtins.go` (for editor completion / hover).
4. Its syntax highlight regex in the VS Code grammar
   (`editors/vscode_froglsp/.../syntaxes/klex.tmLanguage.json`).
5. Its reference entry in `docs/KLEX_LANGUAGE.MD` (and the grammar doc).

Drift between any pair makes the language feel inconsistent. syncdocs
detects every name-presence, arity, and signature drift in one pass.

## Build & run

```bash
go build -o bin/syncdocs ./tools/syncdocs
./bin/syncdocs                            # full audit report
./bin/syncdocs --fix-vscode               # rewrite VS Code syntax regex in place
./bin/syncdocs --gen-lsp-stubs            # print LSP entries for undocumented builtins
./bin/syncdocs --gen-lang-blocks          # print paste-ready markdown for missing
                                          # KLEX_LANGUAGE.MD entries, grouped by source file
./bin/syncdocs --refresh-thin-lsp         # rewrite present-but-thin LSP entries
                                          # in place from Go-source comments
                                          # (add --dry-run to preview)
```

## What it checks

| Check | What it catches |
|---|---|
| **Name presence** | Builtin registered in Go but missing from LSP, VS Code, or docs |
| **Arity equality** | Go arity guard says 2-3 args, LSP signature says 2, docs say 1-2 — drift |
| **Discrete arity** | `drawImage` accepts `{3, 5}` (no 4-arg form) — must match everywhere |
| **Variadic flag** | `len(args) >= 1` (variadic) vs LSP fixed-arity entries |
| **Signature shape** | LSP `Signature:` string vs Go comment `// foo(x, y) → type` |

## Auto-generated stubs

`--gen-lsp-stubs` reads the `//` comment block above each Go registration,
extracts the signature, and prints a paste-ready LSP entry:

```text
"yourBuiltin": {
    Signature:   "yourBuiltin(x: int, y: string) -> bool",
    Description: "First line of the doc comment block.",
    Detail:      "Subsequent lines, joined.",
},
```

`--gen-lang-blocks` does the same for `KLEX_LANGUAGE.MD`, grouping output
by source file so you can paste a whole domain's missing entries in one
edit (e.g. all of `builtins_image_fx.go` at once).

## Per-builtin doc comments

syncdocs extracts the doc block by walking contiguous `//` lines upward
from each `Builtins["…"] = …` line, stopping at blank lines. **Each
registration needs its own comment block immediately above** — a section
header alone is not picked up. Arrow-form signature line plus prose:

```go
// foo(x: int, y: string) → bool
// Returns true when x is positive AND y is non-empty.
Builtins["foo"] = &Builtin{Fn: func(args []Object) Object { ... }}
```

## Companion

Sibling tools that catch the *other* classes of drift:
- [erraudit](stuff/tools/erraudit/) — error-message quality
- [hookaudit](../hookaudit/) — agentic-hook completeness
- [doclinks](../doclinks/) — inter-document link rot
- [doclint](stuff/tools/doclint/) — semantic doc-vs-code audit (Claude API, paid)
