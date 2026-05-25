# doclinks

Detect and repair stale inter-document links across kLex docs. Self-contained,
no API calls, no cost.

## The problem

Long-lived projects accumulate cross-document references: READMEs pointing at
examples, `KLEX_LANGUAGE.MD` citing `eval/builtins_*.go`, stdlib modules
name-dropping each other in doc comments. When a file is renamed or moved,
those references rot silently — nobody notices until a click fails or a
search misses.

## What it does

Three resolvers run in order of confidence:

1. **Git rename history.** `git log --diff-filter=R --name-status` gives a
   deterministic old-path → current-path mapping for any file Git knows was
   renamed. Zero ambiguity in the common case.
2. **Basename match.** Walks the repo, builds a `basename → path` index. A
   broken link to `auth.md` resolves to any `auth.md` that exists, scored
   by path-similarity to the link's original location so `docs/auth.md`
   doesn't collide with `vendor/foo/auth.md`.
3. **Content fingerprint.** Each tracked `.md` / `.txt` file may carry an
   anchor like `<!-- doclink: a3f7c9 -->` near the top. The hash is computed
   from the first non-blank meaningful lines, so it stays stable across
   renames AND mild edits but changes when the file's role changes. Broken
   links can carry the same anchor hint inline; we resolve by hash lookup
   when path + git + basename all fail.

## Build & run

```bash
go build -o bin/doclinks ./tools/doclinks
./bin/doclinks                            # audit, no changes
./bin/doclinks --apply                    # rewrite trivially-fixable links
./bin/doclinks --add-anchors <path>       # add doclink anchor(s) to file(s)
./bin/doclinks --add-anchors all          # to every tracked .md/.txt
./bin/doclinks --update-anchors           # refresh hashes in all anchored files
./bin/doclinks --file <path>              # audit one file only
```

`--apply` only rewrites when a broken link has a **single high-confidence
resolution**. Ambiguous cases stay in the report for human review.

## What's scanned

| Extensions | `.md`, `.MD`, `.txt`, `.TXT` |
| Skipped dirs | `.git`, `node_modules`, `vendor`, `build`, `dist` |
| External schemes (ignored) | `http://`, `https://`, `mailto:`, `ftp://`, `ftps://`, `ssh://`, `git://`, `data:`, `tel:` |

## Fingerprint anchor format

```markdown
<!-- doclink: a3f7c9 -->
```

8-hex SHA-1 of the file's first non-blank meaningful lines. Used as a
resilience hint: a link can carry the same anchor inline so it survives
even if the target gets renamed AND moved AND the basename changes.

`--add-anchors` stamps the anchor at the top of a file; `--update-anchors`
refreshes them when content drifts.

## Companion

Pairs with [syncdocs](../syncdocs/) (name/arity drift) and
[doclint](stuff/tools/doclint/) (semantic doc-vs-code drift, paid). doclinks
catches the path-rot class that the other two don't see.
