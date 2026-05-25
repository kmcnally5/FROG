package eval

// builtins_jsonl.go — Go-side helpers for fast JSONL processing.
//
// The kLex stdlib json parser is itself written in kLex (stdlib/json.lex),
// which means parsing N lines costs ~N × M interpreted eval steps where
// M is the per-line evaluator overhead. On the frogLight cataloger's
// path, scanning catalog.jsonl (millions of lines) through stdlib/json.lex
// is unworkable — every reindex would block for tens of seconds before
// the walk could even start.
//
// _replaySeenFile takes that exact hot path out of the interpreter:
// streaming bufio scan + encoding/json struct decode pulls JUST the
// fields the freshness check needs (path, mtime, deleted), and returns
// a {path: mtime} hash. ~1–2 s for a 4M-line catalog vs 30+ s through
// the kLex parser, and the result shape is exactly what the cataloger's
// isFileFresh wants.

import (
	"bufio"
	"encoding/json"
	"klex/ast"
	"os"
)

func init() {
	// _replaySeenFile(path: string) -> hash
	//
	// Reads a JSONL file where each line is a {"path": str, "mtime": int,
	// ...} record. Returns a hash {path: mtime} keyed by path; latest line
	// wins on duplicate paths (matching the "last write" semantics any
	// in-flight crash-recovery would expect).
	//
	// Missing file → empty hash, no error. Malformed lines are skipped
	// silently (same best-effort policy stdlib/json.lex callers already
	// expect when iterating a JSONL).
	//
	// Used by store._replaySeen to populate s.seen from catalog.jsonl or
	// library.json without the interpreted-JSON-parser bottleneck. The
	// downstream isFileFresh(s, path, mtime) just compares scalars —
	// O(1) hash lookup, no nested chunk-array iteration.
	Builtins["_replaySeenFile"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_replaySeenFile expects 1 argument (path)", ast.Pos{})
		}
		pathArg, ok := args[0].(*String)
		if !ok {
			return typeError(
				"_replaySeenFile: path must be a string, got "+string(args[0].Type()),
				ast.Pos{})
		}

		out := &Hash{Pairs: make(map[HashKey]HashPair)}

		f, err := os.Open(pathArg.Value)
		if err != nil {
			if os.IsNotExist(err) {
				return out
			}
			return runtimeError("_replaySeenFile: open "+pathArg.Value+": "+err.Error(), ast.Pos{})
		}
		defer f.Close()

		// Buffer sized for very long JSONL lines (snippet field in
		// library.json can include ~600 chars; catalog.jsonl lines are
		// smaller). 16 MB ceiling matches the bridge max-line setting
		// elsewhere in eval/ and is well above anything we expect to see.
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)

		var rec struct {
			Path    string `json:"path"`
			Mtime   int64  `json:"mtime"`
			Deleted bool   `json:"deleted,omitempty"`
		}

		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}
			rec.Path = ""
			rec.Mtime = 0
			rec.Deleted = false
			if err := json.Unmarshal(line, &rec); err != nil {
				continue
			}
			if rec.Path == "" || rec.Deleted {
				continue
			}
			hk := HashKey{Type: STRING_OBJ, Value: rec.Path}
			out.Pairs[hk] = HashPair{
				Key:   &String{Value: rec.Path},
				Value: &Integer{Value: int(rec.Mtime)},
			}
		}
		if err := scanner.Err(); err != nil {
			return runtimeError("_replaySeenFile: scan "+pathArg.Value+": "+err.Error(), ast.Pos{})
		}

		return out
	}}
}
