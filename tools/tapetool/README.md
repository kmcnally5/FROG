# tapetool

kLex `.lextape` utility — record, inspect, and (eventually) replay agentic-hook
event tapes.

## What's a .lextape?

A JSON-Lines record of every agentic-hook event a kLex program produced
during a run. The runtime writes one when invoked with
`--record-tape=PATH`; tapetool wraps that flow and gives you ways to
inspect, replay, diff, and mutate tapes after the fact.

## Build & run

```bash
go build -o bin/tapetool ./tools/tapetool

./bin/tapetool record [--output FILE] [--klex BIN] [--vm] <program.lex> [args...]
./bin/tapetool show   <FILE> [--filter KIND] [--from N] [--to M] [--json]
./bin/tapetool help
```

## Subcommands

### `record` — capture a tape

Invokes the kLex binary with `--record-tape=FILE` wrapping it. The wrapped
program runs normally (stdin/stdout/stderr pass through); the tape is
written alongside.

```bash
./bin/tapetool record myapp.lex
# → ./myapp-20260524-150400.lextape

./bin/tapetool record --output /tmp/run.lextape myapp.lex arg1 arg2
./bin/tapetool record --klex ./klex-debug myapp.lex
./bin/tapetool record --vm myapp.lex             # use the bytecode VM
```

Defaults:
- `--output` → `<program>-<timestamp>.lextape` next to the program
- `--klex` → `./klex`

### `show` — pretty-print a tape

```bash
./bin/tapetool show myapp-20260524-150400.lextape
./bin/tapetool show tape.lextape --filter ui_event
./bin/tapetool show tape.lextape --from 100 --to 200
./bin/tapetool show tape.lextape --json
```

Flags:
- `--filter KIND` — limit to one event kind (`ui_event`, `async_spawn`, etc.)
- `--from N --to M` — restrict the event ID range
- `--json` — emit a parseable summary instead of the human report

## Tape format (overview)

Each line is one JSON object. Three line shapes:

```json
{"type":"header","tape_version":1,"klex_version":"0.3.35","program":"...","program_sha256":"...","started_at":"...","args":[...]}
{"type":"event","id":42,"caused_by":7,"t_ms":1230,"kind":"ui_event","data":{...}}
{"type":"footer","ended_at":"...","duration_ms":12345,"event_count":210,"counts_by_kind":{...}}
```

Every event carries `id` and `caused_by` for causal-graph reconstruction.
The runtime stamps `caused_by` by walking the per-goroutine event stack at
fire time, so the chain is correct **by construction** — no post-hoc
inference needed.

## Roadmap (not yet implemented)

| Subcommand | Purpose |
|---|---|
| `play <tape>` | re-drive a kLex program using the recorded inputs as a deterministic input source |
| `diff <a> <b>` | structural diff between two tapes |
| `mutate <tape>` | scripted mutations for fuzz/property testing |

## When tapes earn their keep

Use a tape FIRST for hangs/glitches, before adding print statements.

Real example (2026-05-24): tadPole appeared to hang on a UI redraw. A
tape recorded for ~5s caught `_MD_PROSE_SCALE × 2310` — a silent
mid-frame error storm that print debugging would have buried. Tape gave
the answer in one read.

## Companion

Tapes are produced by the runtime's agentic-hook surfaces (5 of them:
error, async-spawn, async-done, UI, bridge). Coverage of those is audited
by [hookaudit](stuff/tools/hookaudit/).
