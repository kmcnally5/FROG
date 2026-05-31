# fsdispatchgen

Generates `eval/builtins_fs_dispatch_gen.go` — the URL-scheme dispatcher
file that registers every `_fs*` builtin exactly once and routes calls
to per-scheme native helpers based on the path argument's URI scheme.

Part of the Phase 5 I/O scheme refactor — see the project memory
`project-phase5-io-scheme-plan` for full context.

## Build

```
go build -o ./bin/fsdispatchgen ./tools/fsdispatchgen
```

(or just `go run ./tools/fsdispatchgen`)

## Usage

From the repo root:

```
go run ./tools/fsdispatchgen                 # write eval/builtins_fs_dispatch_gen.go
go run ./tools/fsdispatchgen -stdout         # print to stdout (no file write)
go run ./tools/fsdispatchgen -out other.go   # custom output path
```

## What it generates

For each `_fs*` builtin in the schema, a dispatcher of the shape:

```go
Builtins["_fsRead"] = &Builtin{Fn: func(args []Object) Object {
    if len(args) != 1 {
        return runtimeError("_fsRead expects 1 argument (path)", ast.Pos{})
    }
    pathObj, ok := args[0].(*String)
    if !ok {
        return typeError("_fsRead: path must be string, got "+args[0].Type(), ast.Pos{})
    }
    pathParsed := ParseIOPath(pathObj.Value)
    switch pathParsed.Scheme {
    case SchemeFile:
        return nativeFsRead(pathParsed.Remainder)
    case SchemeOPFS:
        return opfsFsRead(pathParsed.Remainder)
    }
    return runtimeError("_fsRead: scheme "+pathParsed.Scheme.String()+" not supported (file, opfs)", ast.Pos{})
}}
```

Path args are extracted, type-checked, scheme-parsed, and the helper
receives `parsed.Remainder` as a primitive string. Passthrough args
(e.g. `content` for `_fsWrite`) flow through as raw `Object` so the
helper can do its own type-checking — matches the existing builtin
convention.

For two-path ops (`_fsCopy`, `_fsRename`, `_fsSymlink`) the dispatcher
also enforces that both paths share the same scheme; cross-scheme
operations return a clear error rather than silently mishandling them.

## Helper naming convention

| Scheme | Helper prefix | Example |
|---|---|---|
| `file://` (or bare path) | `native` | `nativeFsRead` |
| `opfs://` | `opfs` | `opfsFsRead` |

The dispatcher generator does not generate the helpers themselves — those
live in hand-written files alongside the existing platform-specific fs
code. Adding a new scheme means adding a constant to the schema *and*
implementing the matching helpers.

## The schema

The single source of truth is the `ops` slice in `main.go`. Each entry:

```go
{"_fsRead", []string{"path"}, nil, []scheme{schFile, schOPFS}},
//  name      path args         passthrough  supported schemes
```

| Field | Meaning |
|---|---|
| Name | Builtin name |
| PathArgs | Names of leading path-typed arguments — these get scheme-parsed |
| PassthroughArgs | Names of remaining args — forwarded as raw `Object` to the helper |
| Schemes | Which URI schemes this operation accepts |

To add a new fs builtin: add a row, regenerate, implement
`nativeFsXxx` and `opfsFsXxx` helpers, run tests.

## Why deferred: HTTP scheme support

The Phase 5 plan also names `http://` / `https://` as fs.read targets
(so `fs.read("https://example.com/x.json")` becomes a `fetch()` GET).
v1 of this generator deliberately omits HTTP from the schema — file +
opfs is the core unlock, and HTTP fs.read can land in a follow-up
without changing the dispatcher's generated shape (just add `schHTTP`
to the schema and write `httpFsRead`, `httpFsExists`, `httpFsStat`).
