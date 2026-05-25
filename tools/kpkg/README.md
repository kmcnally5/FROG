# kpkg

kLex package manager. Inventory + integrity tool for the standard library.

## Scope

kpkg does **not** (yet) fetch third-party packages, resolve dependency
graphs, or manage a registry. kLex's ecosystem is one author and one
bundled stdlib, and any tooling that pretends otherwise is over-engineering.

What kpkg does today:

| Command | Purpose |
|---|---|
| `list` | one-line-per-module inventory of `stdlib/` |
| `info <module>` | full metadata, imports, exports for one module |
| `lock` | write `klex.lock` with current versions + content hashes |
| `verify` | recompute hashes, fail on drift vs lockfile (CI-friendly) |

The v2 proposal (TOML manifests, true dependency resolution via MVS,
`klex_modules/`) is documented at [docs/kpkgv2Proposal.md](projects/kpkgv2Proposal.md)
but deferred until 3rd-party demand exists.

## Build & run

```bash
go build -o bin/kpkg ./tools/kpkg

./bin/kpkg list                           # one-line per stdlib module
./bin/kpkg info rest                      # deep view of one module
./bin/kpkg lock                           # write klex.lock at repo root
./bin/kpkg verify                         # exit non-zero on hash drift
./bin/kpkg version                        # print kpkg semver
```

Per-command flags:

```
list / lock / verify
  --stdlib   directory to scan (default "stdlib")
lock / verify
  --lockfile path to klex.lock (default "klex.lock" at repo root)
list
  --format   table | json (default "table")
```

## Metadata header convention

kpkg reads optional `// @tag value` lines from each `.lex` file's leading
comment block. All tags are optional; missing tags fall back to sensible
defaults (module name = filename, version = `"unversioned"`).

```lex
// @module    rest          canonical module name (else: filename)
// @version   1.0.0         semver string (else: "unversioned")
// @since     klex 0.3.35   first kLex release that shipped this module
// @author    karl          maintainer (free text)
// @summary   one-liner     short description (else: first prose line)
```

The headers are plain `//` comments — the kLex interpreter ignores them,
so **adoption is incremental**: stamp a header into one module at a time
as you touch it; the others continue to work without any.

## klex.lock format

`kpkg lock` writes a JSON file at the repo root:

```json
{
  "kpkg": "0.1.0",
  "klex": "0.3.35",
  "generated_at": "2026-05-24T20:00:00Z",
  "modules": {
    "rest":     { "version": "1.0.0",       "sha256": "a3f7c9..." },
    "json":     { "version": "1.2.1",       "sha256": "b8e2d1..." },
    "graphics": { "version": "unversioned", "sha256": "c5a4f7..." }
  }
}
```

`kpkg verify` recomputes every hash and exits non-zero on any mismatch.
Wire this into CI to detect accidental stdlib drift.

## Version bumping (the binary)

`kpkgVersion` in `main.go` is stamped into every `klex.lock` under the
`kpkg` field so a future kpkg can detect — and migrate or reject —
lockfiles written by an older release.

Bump rules:

| Bump | Reason |
|---|---|
| patch (0.1.X) | bug fix, no behaviour or schema change |
| minor (0.X.0) | new subcommand, new flag, additive lockfile field |
| major (X.0.0) | lockfile schema break, removed/renamed subcommand |

## Source layout

```
tools/kpkg/
  main.go     CLI dispatcher, version, common helpers (findRoot, fatal)
  list.go     `kpkg list` command
  info.go     `kpkg info <module>` command
  lock.go     `kpkg lock` command
  verify.go   `kpkg verify` command
  scan.go     stdlib walker, header parser, exported-fn extractor
```

Pure Go, no external dependencies.
