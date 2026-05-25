# hookaudit

kLex agentic-hook completeness audit tool. Catches UI widgets that read
input but forget to fire the appropriate agentic-hook event.

## The motivating bug

Phase 3 of the agentic hooks rollout wired UI hooks into the `stdlib/ui.lex`
widgets. But every real application uses the **Go-side widget builtins** in
`eval/builtins_ui.go`, which were missed entirely. Whole class of bug:
widget reads `gfx.mouseJustClicked`, never calls `FireUiEventHook`, and
the agentic tape misses the event.

hookaudit catches that class automatically by AST-walking every
`Builtins["..."]` registration and classifying it.

## What it does

For each `Builtins["…"] = ...` registration:

1. Scans the body for **input signals** — references to `gfx.mouseJustClicked`,
   `gfx.charBuf`, `gfx.keys`, scroll deltas, key counts, etc.
2. Scans for **fire calls** — `FireUiEventHook`, `FireBridgeCallHook`,
   `FireAsyncSpawnHook`, `FireAsyncDoneHook`, `FireErrorBubbleHook`.
3. Checks for the **widget marker** — `uiRegisterElement` calls indicate
   this is a real widget, not just a primitive input reader.

Classification:

| Status | Meaning |
|---|---|
| `ok` | Widget reads input and fires a hook ✓ |
| `missing` | Widget reads input but does NOT fire any hook — bug |
| `non_interactive` | No input reads — not a candidate |
| `infra` | Reads input but not a widget (e.g. `mouseX` primitive) — fine |

## Build & run

```bash
go build -o bin/hookaudit ./tools/hookaudit
./bin/hookaudit                           # human report
./bin/hookaudit --json                    # JSON inventory
./bin/hookaudit --missing                 # only MISSING sites
./bin/hookaudit --json --out=sites.json   # write JSON to file
```

## Exit codes

| Code | Meaning |
|---|---|
| `0` | Clean — no MISSING sites |
| `2` | At least one MISSING site found — wire into CI to gate landings |

## Heuristics

Conservative — false positives are fine, false negatives are not. Treat
each `MISSING` as "look at this," not "this is broken." The current
baseline (2026-05-23) is 19/19 instrumented widgets `ok`, 0 missing.

## Signals detected

**Input reads** (any `gfx.X` access where X is in this set):
```
mouseJustClicked, mouseDown, mouseRightClicked, mouseRightDown,
mouseScrollY, mouseScrollX, uiScrollDelta, uiScrollX,
charBuf, uiBackspaceCount, uiDeleteCount,
uiLeftCount, uiRightCount, uiUpCount, uiDownCount, keys
```

**Hook fires** (calls to any of):
```
FireUiEventHook, FireBridgeCallHook,
FireAsyncSpawnHook, FireAsyncDoneHook, FireErrorBubbleHook
```

**Widget marker:** any call to `uiRegisterElement` — distinguishes real
widgets from input primitives.

## Companion

Sibling audit tools:
- [erraudit](stuff/tools/erraudit/) — error-message quality across `eval/`/`parser/`/`lexer/`
- [syncdocs](../syncdocs/) — name/arity drift across Go ↔ LSP ↔ docs
- [doclinks](../doclinks/) — inter-document link rot
