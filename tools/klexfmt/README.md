# klexfmt

kLex source code formatter CLI. Thin wrapper around the `klex/formatter`
package — all the logic lives there; this tool just handles arg parsing,
file I/O, and atomic in-place writes.

## Two front-ends, one engine

There are two ways to format kLex source. Both call into the same library:

| Front-end | Use case |
|---|---|
| `klexfmt` (this tool) | CLI / pre-commit / CI |
| `froglsp` (`snowball/froglsp/`) | Editor format-on-save via LSP |

They produce **byte-identical output** for any given input, by construction.
`klexfmt -w foo.lex` from the terminal and an editor "Format Document"
command in VS Code / Neovim / Zed both route to the same `Format()`
function — no second implementation to drift out of sync with.

Format spec lives in [formatter/format.go](../../formatter/format.go).

## Build & run

```bash
go build -o bin/klexfmt ./tools/klexfmt

./bin/klexfmt path.lex                # print formatted output to stdout
./bin/klexfmt -w path.lex             # rewrite the file in place
cat src.lex | ./bin/klexfmt           # stdin → stdout (editor-pipe mode)
./bin/klexfmt -w file1.lex file2.lex  # batch in-place format
./bin/klexfmt -l path.lex             # list-only: print paths whose
                                      # formatted output differs from disk
```

## Modes

| Mode | Flag | Output | Exit |
|---|---|---|---|
| stdout (default) | — | formatted to stdout | 0 |
| write in-place | `-w` | rewrites file (atomic temp+rename) | 0, or 1 on I/O error |
| list-only (CI) | `-l` | prints paths needing reformat | 0 if clean, 1 if any |
| stdin → stdout | (no args) | formatted from stdin | 0 |

## Atomic writes

`-w` writes to a sibling temp file (`.<basename>.klexfmt-*`) and renames
into place. Crashes during write leave the original intact; readers never
see a half-formatted file.

## CI usage

```bash
# Fail the build if any .lex file in stdlib/ isn't formatted.
./bin/klexfmt -l $(find stdlib -name '*.lex')
```

Exits 1 with the list of unformatted paths on stdout.

## Pre-commit hook

```bash
# .git/hooks/pre-commit (excerpt)
files=$(git diff --cached --name-only --diff-filter=ACM '*.lex')
[ -z "$files" ] && exit 0
./bin/klexfmt -l $files && exit 0
echo "kLex files need formatting:"
./bin/klexfmt -l $files
exit 1
```
