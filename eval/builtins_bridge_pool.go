package eval

import (
	"fmt"
	"klex/ast"
	"strings"
	"sync"
	"sync/atomic"
)

// builtins_bridge_pool.go — round-robin pool of native bridges.
//
// Many bridge workloads fan out the same job across N identical subprocesses
// (SecretHunter spawns 16 Python workers for YARA + entropy). The pool wraps
// that pattern: bridgePool(n, cmd, args) gives you N pre-started bridges and a
// round-robin counter; bridgePoolCall/Stream pick the next alive bridge and
// forward to the existing bridgeCall/bridgeStream contracts.
//
// Each bridge inside the pool is a regular Bridge — same schema validation,
// stderr capture, cancel-on-stream-break, taint behaviour. The pool just owns
// their lifetime and routes calls.

// BridgePool holds N bridges plus a parallel `dead` array marking members that
// failed init or have been tainted out of service. Routing skips dead members.
type BridgePool struct {
	members []*Bridge
	dead    []bool
	mu      sync.Mutex // guards dead[]
	next    uint64     // atomic round-robin counter
	closed  bool
}

func (p *BridgePool) Type() ObjectType { return BRIDGE_POOL_OBJ }
func (p *BridgePool) Inspect() string {
	alive, dead := p.countLocked()
	return fmt.Sprintf("BridgePool(size=%d, alive=%d, dead=%d)", len(p.members), alive, dead)
}

// memberAliveLocked is the single source of truth for "can this pool slot
// take work?". Caller must hold p.mu. It folds two states together:
//   - p.dead[i]      → init failed, has been marked dead since pool creation
//   - bridge.tainted → subprocess crashed or hit a fatal error mid-session
// When it observes a tainted bridge it latches the result into p.dead[i] so
// subsequent picks skip the b.mu lock entirely. Taint is permanent (the
// docs say so — you must close and rebuild), so caching the verdict is safe.
func (p *BridgePool) memberAliveLocked(i int) bool {
	if p.dead[i] {
		return false
	}
	b := p.members[i]
	if b == nil {
		p.dead[i] = true
		return false
	}
	b.mu.Lock()
	tainted := b.tainted
	closed := b.closed
	b.mu.Unlock()
	if tainted || closed {
		p.dead[i] = true
		return false
	}
	return true
}

// countLocked snapshots how many members are alive vs dead. Walks every slot
// through memberAliveLocked so tainted-mid-session members get latched dead
// at the same time we report them — keeps bridgePoolHealth honest.
func (p *BridgePool) countLocked() (alive, dead int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := range p.members {
		if p.memberAliveLocked(i) {
			alive++
		} else {
			dead++
		}
	}
	return
}

// pick returns the next alive bridge using a round-robin counter and a
// linear scan for liveness. Returns nil if every member is dead. O(N) worst
// case but N is small (pool sizes are typically CPU count, ~8–32).
func (p *BridgePool) pick() *Bridge {
	n := len(p.members)
	if n == 0 {
		return nil
	}
	start := atomic.AddUint64(&p.next, 1)
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := 0; i < n; i++ {
		idx := int((start + uint64(i)) % uint64(n))
		if p.memberAliveLocked(idx) {
			return p.members[idx]
		}
	}
	return nil
}

func init() {
	// ── bridgePool(n, cmd, args, opts?) → (pool, err) ────────────────────────
	//
	// Starts `n` identical bridges and returns them as a pool. Optional opts
	// hash supports every bridge-level option (timeout, stderr_log, maxBytes)
	// plus an "init" callable that runs once on each bridge — handy for
	// per-bridge setup like loading YARA rules. If init returns an Error, that
	// bridge is marked dead in the pool; callers can inspect bridgePoolHealth().
	Builtins["bridgePool"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 3 || len(args) > 4 {
			return runtimeError("bridgePool expects 3 or 4 arguments (n, cmd, args, opts?)", ast.Pos{})
		}
		nArg, ok := args[0].(*Integer)
		if !ok || nArg.Value < 1 {
			return typeError(fmt.Sprintf("bridgePool: n must be a positive integer, got %s", args[0].Type()), ast.Pos{})
		}
		cmdArg, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("bridgePool: cmd must be string, got %s", args[1].Type()), ast.Pos{})
		}
		argsArr, ok := args[2].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("bridgePool: args must be array, got %s", args[2].Type()), ast.Pos{})
		}
		cmdArgs := make([]string, len(argsArr.Elements))
		for i, el := range argsArr.Elements {
			s, ok := el.(*String)
			if !ok {
				return typeError(fmt.Sprintf("bridgePool: args[%d] must be string, got %s", i, el.Type()), ast.Pos{})
			}
			cmdArgs[i] = s.Value
		}

		var opts bridgeOpts
		var initFn Object
		if len(args) == 4 {
			// Pull the init callable out before handing the rest to the shared
			// option parser — parseBridgeOpts doesn't know about pool-only keys.
			if h, ok := args[3].(*Hash); ok {
				initKey := HashKey{Type: STRING_OBJ, Value: "init"}
				if pair, found := h.Pairs[initKey]; found {
					if _, isNull := pair.Value.(*Null); !isNull {
						initFn = pair.Value
					}
				}
			}
			parsed, perr := parseBridgeOpts(args[3])
			if perr != nil {
				return perr
			}
			opts = parsed
		} else {
			parsed, _ := parseBridgeOpts(nil)
			opts = parsed
		}

		n := int(nArg.Value)
		pool := &BridgePool{
			members: make([]*Bridge, n),
			dead:    make([]bool, n),
		}

		// Spawn sequentially — most of the cost is the subprocess fork+exec,
		// which the OS already parallelises; sequential keeps error reporting
		// predictable (first failure aborts the rest).
		for i := 0; i < n; i++ {
			b, errObj := spawnBridge(cmdArg.Value, cmdArgs, opts)
			if errObj != nil {
				// Roll back any bridges we already started so we don't leak
				// subprocesses on a partial failure.
				for j := 0; j < i; j++ {
					if pool.members[j] != nil {
						Builtins["bridgeClose"].Fn([]Object{pool.members[j]})
					}
				}
				return errObj
			}
			pool.members[i] = b
		}

		// Run the optional init callable on each bridge. A returned Error
		// (or a Tuple whose second element is an Error — matches the
		// bridgeCall return shape so users can `return bridgeCall(...)`)
		// marks that bridge dead.
		if initFn != nil {
			for i, b := range pool.members {
				res, callErr := callCallable(initFn, []Object{b})
				if isPoolInitFailure(res, callErr) {
					pool.dead[i] = true
				}
			}
		}

		return &Tuple{Elements: []Object{pool, NULL}}
	}}

	// ── bridgePoolCall(pool, fn, args, timeoutSec?) → (result, err) ──────────
	Builtins["bridgePoolCall"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 3 && len(args) != 4 {
			return runtimeError("bridgePoolCall expects 3 or 4 arguments (pool, fn, args, timeoutSec?)", ast.Pos{})
		}
		pool, ok := args[0].(*BridgePool)
		if !ok {
			return typeError(fmt.Sprintf("bridgePoolCall: first argument must be a bridge pool, got %s", args[0].Type()), ast.Pos{})
		}
		if pool.closed {
			return bridgeError("BRIDGE_CLOSED", "bridgePoolCall: pool has been closed")
		}
		b := pool.pick()
		if b == nil {
			return bridgeError("BRIDGE_POOL_EMPTY", "bridgePoolCall: every pool member is dead")
		}
		// Delegate to the existing bridgeCall builtin so we inherit timeout,
		// validation, taint propagation, etc. without duplicating any of it.
		forwarded := append([]Object{b}, args[1:]...)
		return Builtins["bridgeCall"].Fn(forwarded)
	}}

	// ── bridgePoolStream(pool, fn, args, timeout?) → (channel, err) ──────────
	Builtins["bridgePoolStream"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 3 || len(args) > 4 {
			return runtimeError("bridgePoolStream expects 3 or 4 arguments (pool, fn, args, timeout?)", ast.Pos{})
		}
		pool, ok := args[0].(*BridgePool)
		if !ok {
			return typeError(fmt.Sprintf("bridgePoolStream: first argument must be a bridge pool, got %s", args[0].Type()), ast.Pos{})
		}
		if pool.closed {
			return bridgeError("BRIDGE_CLOSED", "bridgePoolStream: pool has been closed")
		}
		b := pool.pick()
		if b == nil {
			return bridgeError("BRIDGE_POOL_EMPTY", "bridgePoolStream: every pool member is dead")
		}
		forwarded := append([]Object{b}, args[1:]...)
		return Builtins["bridgeStream"].Fn(forwarded)
	}}

	// ── bridgePoolClose(pool) → null ─────────────────────────────────────────
	Builtins["bridgePoolClose"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("bridgePoolClose expects 1 argument (pool)", ast.Pos{})
		}
		pool, ok := args[0].(*BridgePool)
		if !ok {
			return typeError(fmt.Sprintf("bridgePoolClose: argument must be a bridge pool, got %s", args[0].Type()), ast.Pos{})
		}
		pool.mu.Lock()
		if pool.closed {
			pool.mu.Unlock()
			return NULL
		}
		pool.closed = true
		members := pool.members
		pool.mu.Unlock()

		for _, b := range members {
			if b != nil {
				Builtins["bridgeClose"].Fn([]Object{b})
			}
		}
		return NULL
	}}

	// ── bridgePoolHealth(pool) → {size, alive, dead} ─────────────────────────
	Builtins["bridgePoolHealth"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("bridgePoolHealth expects 1 argument (pool)", ast.Pos{})
		}
		pool, ok := args[0].(*BridgePool)
		if !ok {
			return typeError(fmt.Sprintf("bridgePoolHealth: argument must be a bridge pool, got %s", args[0].Type()), ast.Pos{})
		}
		alive, dead := pool.countLocked()
		return makeHash(
			"size", &Integer{Value: len(pool.members)},
			"alive", &Integer{Value: alive},
			"dead", &Integer{Value: dead},
		)
	}}

	// ── bridgePoolStderr(pool) → array ───────────────────────────────────────
	// Concatenates the stderr tail of every member, prefixed with "[N] " so
	// you can tell which subprocess emitted which line.
	Builtins["bridgePoolStderr"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("bridgePoolStderr expects 1 argument (pool)", ast.Pos{})
		}
		pool, ok := args[0].(*BridgePool)
		if !ok {
			return typeError(fmt.Sprintf("bridgePoolStderr: argument must be a bridge pool, got %s", args[0].Type()), ast.Pos{})
		}
		var allLines []Object
		for i, b := range pool.members {
			if b == nil || b.stderrBuf == nil {
				continue
			}
			tail := b.stderrBuf.Snapshot()
			if len(tail) == 0 {
				continue
			}
			for _, line := range strings.Split(strings.TrimRight(string(tail), "\n"), "\n") {
				allLines = append(allLines, &String{Value: fmt.Sprintf("[%d] %s", i, line)})
			}
		}
		return &Array{Elements: allLines}
	}}
}

// isPoolInitFailure decides whether an init callable's outcome should mark
// the bridge dead. We accept three return shapes a kLex author might use:
//   - the call itself errored (callErr != nil)
//   - the callable returned an Error directly
//   - the callable returned a (result, err) Tuple — the bridgeCall shape —
//     and err is non-null
// Anything else counts as success.
func isPoolInitFailure(res, callErr Object) bool {
	if callErr != nil {
		return true
	}
	if res == nil {
		return false
	}
	if _, isErr := res.(*Error); isErr {
		return true
	}
	if t, ok := res.(*Tuple); ok && len(t.Elements) >= 2 {
		if _, isErr := t.Elements[1].(*Error); isErr {
			return true
		}
	}
	return false
}

// makeHash is a small constructor for fixed-shape hashes returned from
// bridgePool* builtins. Keys must be strings; values are arbitrary Objects.
// Pairs are passed as alternating key/value strings to keep call sites tidy.
func makeHash(kv ...interface{}) *Hash {
	h := &Hash{Pairs: make(map[HashKey]HashPair, len(kv)/2)}
	for i := 0; i+1 < len(kv); i += 2 {
		k, ok := kv[i].(string)
		if !ok {
			continue
		}
		v, ok := kv[i+1].(Object)
		if !ok {
			continue
		}
		hk := HashKey{Type: STRING_OBJ, Value: k}
		h.Pairs[hk] = HashPair{Key: &String{Value: k}, Value: v}
	}
	return h
}
