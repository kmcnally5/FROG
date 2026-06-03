//go:build !js

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
//
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
	// bridgePool — start N identical bridges as one round-robin pool.
	//
	// The pattern for fanning the same job across many subprocesses (e.g. 16
	// Python YARA workers). Starts `n` bridges from a transport hash — same shape
	// as bridgeOpen ("kind", "cmd", "args", optional timeout_seconds /
	// max_response_mb / stderr_log) — and hands back a pool that routes
	// bridgePoolCall/Stream to the next alive member. The optional opts hash takes
	// "init": a callable run once per bridge after spawn (receives the bridge); if
	// it returns an error — or a (result, err) tuple with a non-null err — that
	// bridge is marked dead. Use it to load rules, open connections, etc. If any
	// spawn fails the whole pool is rolled back so no subprocesses leak.
	//
	// @sig     bridgePool(n: int, transport: hash, [opts: hash]) -> (BridgePool, error)
	// @param   n          how many bridges to start (must be >= 1)
	// @param   transport  the bridge transport hash (as for bridgeOpen)
	// @param   opts        pool options, currently {"init": fn(bridge)} (optional)
	// @returns a (pool, null) tuple on success, or (null, error) on failure
	// @errors  TypeError on bad argument types; returns BRIDGE_TRANSPORT_* / spawn errors in the tuple's second slot
	// @example no-run pool, err = bridgePool(16, {"kind": "subprocess", "cmd": "python3", "args": ["yara_bridge.py"]})
	// @since   0.1.0
	// @see     bridgePoolCall, bridgePoolStream, bridgePoolClose, bridgeOpen
	Builtins["bridgePool"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 2 || len(args) > 3 {
			return runtimeError("bridgePool expects 2 or 3 arguments (n, transport, opts?)", ast.Pos{})
		}
		nArg, ok := args[0].(*Integer)
		if !ok || nArg.Value < 1 {
			return typeError(fmt.Sprintf("bridgePool: n must be a positive integer, got %s", args[0].Type()), ast.Pos{})
		}
		transport, ok := args[1].(*Hash)
		if !ok {
			return typeError(fmt.Sprintf("bridgePool: transport must be a hash, got %s", args[1].Type()), ast.Pos{})
		}

		// Dispatch on kind. Only subprocess is implemented today.
		kindObj := hashLookup(transport, "kind")
		if kindObj == nil {
			return bridgeError("BRIDGE_TRANSPORT_MISCONFIGURED",
				"bridgePool: transport hash missing required key 'kind'")
		}
		kindStr, ok := kindObj.(*String)
		if !ok {
			return bridgeError("BRIDGE_TRANSPORT_MISCONFIGURED",
				fmt.Sprintf("bridgePool: 'kind' must be a string, got %s", kindObj.Type()))
		}
		switch kindStr.Value {
		case "subprocess":
			// fall through
		case "worker", "remote":
			return bridgeError("BRIDGE_TRANSPORT_UNAVAILABLE",
				fmt.Sprintf("bridgePool: transport kind %q is not yet implemented", kindStr.Value))
		default:
			return bridgeError("BRIDGE_TRANSPORT_UNKNOWN",
				fmt.Sprintf("bridgePool: unknown transport kind %q (known: 'subprocess')", kindStr.Value))
		}

		cmd, cmdArgs, opts, perr := parseSubprocessTransport(transport)
		if perr != nil {
			return perr
		}

		// Pool-only opts (init).
		var initFn Object
		if len(args) == 3 {
			if h, ok := args[2].(*Hash); ok {
				initKey := HashKey{Type: STRING_OBJ, Value: "init"}
				if pair, found := h.Pairs[initKey]; found {
					if _, isNull := pair.Value.(*Null); !isNull {
						initFn = pair.Value
					}
				}
			} else if _, isNull := args[2].(*Null); !isNull {
				return typeError(fmt.Sprintf("bridgePool: opts must be a hash or null, got %s", args[2].Type()), ast.Pos{})
			}
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
			b, errObj := spawnBridge(cmd, cmdArgs, opts)
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

	// bridgePoolCall — call a function on the next alive bridge in a pool.
	//
	// Picks a member round-robin (via an atomic counter) and forwards to
	// bridgeCall, so it behaves identically — same timeouts, schema validation,
	// error codes — just load-balanced. N concurrent async tasks hit N different
	// bridges with no contention. Returns BRIDGE_POOL_EMPTY when every member is
	// dead, or BRIDGE_CLOSED if the pool was closed.
	//
	// @sig     bridgePoolCall(pool: BridgePool, fn: string, args: array, [timeoutSec: number]) -> (any, error)
	// @param   pool        a pool from bridgePool
	// @param   fn          the remote function name to invoke
	// @param   args        an array of arguments to pass
	// @param   timeoutSec  per-call timeout override in seconds (optional)
	// @returns a (result, null) tuple on success, or (null, error) on failure
	// @errors  TypeError on bad argument types; returns BRIDGE_POOL_EMPTY / BRIDGE_CLOSED / per-call errors in the tuple's second slot
	// @example no-run result, err = bridgePoolCall(pool, "scan_batch", [files])
	// @since   0.1.0
	// @see     bridgePool, bridgePoolStream, bridgeCall
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

	// bridgePoolStream — call a streaming handler on the next alive bridge in a pool.
	//
	// The streaming counterpart to bridgePoolCall: picks a member round-robin and
	// forwards to bridgeStream, returning a channel of items. Same semantics as
	// bridgeStream (channel closes on end-of-stream; a mid-stream Error arrives as
	// the final item), just load-balanced across the pool.
	//
	// @sig     bridgePoolStream(pool: BridgePool, fn: string, args: array, [timeout: number]) -> (channel, error)
	// @param   pool     a pool from bridgePool
	// @param   fn       the streaming handler name to invoke
	// @param   args     an array of arguments to pass
	// @param   timeout  per-call timeout override in seconds (optional)
	// @returns a (channel, null) tuple on success, or (null, error) on failure
	// @errors  TypeError on bad argument types; returns BRIDGE_POOL_EMPTY / BRIDGE_CLOSED / stream errors in the tuple's second slot
	// @example no-run ch, err = bridgePoolStream(pool, "tail", [path])
	// @since   0.1.0
	// @see     bridgePoolCall, bridgeStream, bridgePool
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

	// bridgePoolClose — close every bridge in a pool.
	//
	// Calls bridgeClose on each member, reaping all the subprocesses at once.
	// Idempotent — closing an already-closed pool is a no-op. Always close pools
	// you open.
	//
	// @sig     bridgePoolClose(pool: BridgePool) -> null
	// @param   pool  the pool to close
	// @returns null
	// @errors  TypeError if the argument isn't a bridge pool
	// @example no-run bridgePoolClose(pool)
	// @since   0.1.0
	// @see     bridgePool, bridgeClose
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

	// bridgePoolHealth — how many pool members are alive vs dead.
	//
	// Returns a hash {"size", "alive", "dead"} — the total, the count routing
	// will use, and the count tainted out of service (init failure or a fatal
	// error). Poll it to decide whether to recreate the pool or alert.
	//
	// @sig     bridgePoolHealth(pool: BridgePool) -> hash
	// @param   pool  the pool to inspect
	// @returns a hash with "size", "alive", and "dead" counts
	// @errors  TypeError if the argument isn't a bridge pool
	// @example no-run bridgePoolHealth(pool)
	// @since   0.1.0
	// @see     bridgePoolStderr, bridgeMetrics, bridgePool
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

	// bridgePoolStderr — the merged stderr of every pool member, tagged by index.
	//
	// Concatenates each member's stderr tail into one array, prefixing every line
	// with "[N] " so you can tell which subprocess emitted it. The first place to
	// look when a pooled job misbehaves.
	//
	// @sig     bridgePoolStderr(pool: BridgePool) -> array
	// @param   pool  the pool to inspect
	// @returns an array of "[index] line" strings (empty if none)
	// @errors  TypeError if the argument isn't a bridge pool
	// @example no-run bridgePoolStderr(pool)
	// @since   0.1.0
	// @see     bridgePoolHealth, bridgeStderr
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
//
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
