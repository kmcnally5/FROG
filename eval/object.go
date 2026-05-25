package eval

// object.go defines kLex's runtime type system.
//
// Every value that exists at runtime — integers, strings, functions, arrays,
// errors, everything — is represented as a Go struct that implements the
// Object interface. This is the "everything is an object" model used by most
// dynamic language runtimes.
//
// The ObjectType constants act as runtime type tags. The evaluator uses Go
// type assertions (val.(*Integer)) to get at the concrete type when it needs
// to do something type-specific (e.g. arithmetic on integers).
//
// Why an interface instead of a union/enum?
// Go interfaces let us add new types without changing existing code. Adding
// a new runtime type is just: write the struct, implement Type() and Inspect(),
// and handle it in eval.go.

import (
	"bufio"
	"database/sql"
	"fmt"
	"io"
	"klex/ast"
	"net"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ObjectType is a string tag naming the runtime type of a value.
type ObjectType string

const (
	INTEGER_OBJ  ObjectType = "INTEGER"
	FLOAT_OBJ    ObjectType = "FLOAT"
	BOOLEAN_OBJ  ObjectType = "BOOLEAN"
	STRING_OBJ   ObjectType = "STRING"
	BYTES_OBJ    ObjectType = "BYTES"
	NULL_OBJ     ObjectType = "NULL"
	RETURN_OBJ   ObjectType = "RETURN"  // wraps a value being returned
	ERROR_OBJ    ObjectType = "ERROR"   // runtime or type error
	FUNCTION_OBJ ObjectType = "FUNCTION"
	BUILTIN_OBJ  ObjectType = "BUILTIN" // built-in functions (println, len, etc.)
	BREAK_OBJ    ObjectType = "BREAK"   // signal that bubbles up to the while loop
	CONTINUE_OBJ ObjectType = "CONTINUE"
	ARRAY_OBJ    ObjectType = "ARRAY"
	HASH_OBJ     ObjectType = "HASH"
	TUPLE_OBJ    ObjectType = "TUPLE"
	MODULE_OBJ      ObjectType = "MODULE"
	TASK_OBJ        ObjectType = "TASK"
	STRUCT_DEF_OBJ  ObjectType = "STRUCT_DEF"
	STRUCT_INST_OBJ ObjectType = "STRUCT"
	CHANNEL_OBJ     ObjectType = "CHANNEL"
	NET_CONN_OBJ    ObjectType = "NET_CONN"
	ENUM_DEF_OBJ    ObjectType = "ENUM_DEF"
	ENUM_VARIANT_OBJ ObjectType = "ENUM_VARIANT"
	ENUM_OBJ        ObjectType = "ENUM"
	ATOMIC_INT_ARRAY_OBJ   ObjectType = "ATOMIC_INT_ARRAY"
	ATOMIC_FLOAT_ARRAY_OBJ ObjectType = "ATOMIC_FLOAT_ARRAY"
	CONCURRENT_HASH_OBJ    ObjectType = "CONCURRENT_HASH"
	IMAGE_OBJ              ObjectType = "IMAGE"
	FONT_OBJ               ObjectType = "FONT"
	DB_CONN_OBJ            ObjectType = "DB_CONN"
	DB_TX_OBJ              ObjectType = "DB_TX"
	BRIDGE_OBJ             ObjectType = "BRIDGE"
	BRIDGE_POOL_OBJ        ObjectType = "BRIDGE_POOL"
	MCP_CLIENT_OBJ         ObjectType = "MCP_CLIENT"
	MCP_SERVER_OBJ         ObjectType = "MCP_SERVER"

	// COMPILED_FUNCTION_OBJ is the type tag for vm.CompiledFunction.
	// Declared here (not in vm) so eval-side dispatchers can do
	// `fn.Type() == COMPILED_FUNCTION_OBJ` directly without going
	// through the previously-required IsExternalCallable hook
	// indirection (M2 audit fix, 2026-05-22). vm.CompiledFunction's
	// Type() returns this constant; eval can reference it without
	// importing vm (avoids the cycle).
	COMPILED_FUNCTION_OBJ ObjectType = "COMPILED_FUNCTION"
)

// Object is the interface every runtime value implements.
// Type() lets the evaluator do runtime type checks.
// Inspect() gives a human-readable representation (used by println).
type Object interface {
	Type() ObjectType
	Inspect() string
}

// -------------------- INTEGER --------------------

type Integer struct {
	Value int
}

func (i *Integer) Type() ObjectType { return INTEGER_OBJ }
func (i *Integer) Inspect() string  { return fmt.Sprintf("%d", i.Value) }

// -------------------- FLOAT --------------------

type Float struct {
	Value float64
}

func (f *Float) Type() ObjectType { return FLOAT_OBJ }
func (f *Float) Inspect() string  { return strconv.FormatFloat(f.Value, 'f', -1, 64) }

// -------------------- BOOLEAN --------------------

type Boolean struct {
	Value bool
}

func (b *Boolean) Type() ObjectType { return BOOLEAN_OBJ }
func (b *Boolean) Inspect() string  { return fmt.Sprintf("%t", b.Value) }

// -------------------- STRING --------------------

// String holds a kLex string value. Iteration and indexing operate on
// Unicode code points (runes), not bytes, so len("café") == 4.
//
// kLex strings are immutable. To avoid quadratic blow-up when scripts walk
// a long string with s[i] (e.g. json.parse over a 400KB Forge response —
// without caching, each index is O(n) for the rune conversion, total O(n²),
// 1-2 minutes for ~400K chars), the rune-level view is cached lazily on
// first rune-level access. ASCII strings skip the rune slice entirely and
// byte-index Value directly — zero extra allocation, O(1) per access.
//
// The cache is thread-safe: async-spawned goroutines share *String pointers
// (env snapshot copies bindings but not the strings they reference), so
// concurrent RuneAt/RuneLen/RuneSubstring calls are expected. sync.Once
// guarantees ensureRuneCache runs exactly once per String. After init, the
// runes/runeCount/isASCII fields are read-only and safe for concurrent reads.
type String struct {
	Value string

	runeOnce  sync.Once
	runes     []rune // nil when isASCII — byte-indexing Value is correct then
	runeCount int
	isASCII   bool
}

func (s *String) Type() ObjectType { return STRING_OBJ }
func (s *String) Inspect() string  { return s.Value }

// ensureRuneCache populates the rune-level fields exactly once per String.
// Cheap for ASCII strings — no rune slice allocation, just a byte scan.
func (s *String) ensureRuneCache() {
	s.runeOnce.Do(func() {
		ascii := true
		for i := 0; i < len(s.Value); i++ {
			if s.Value[i] >= 0x80 {
				ascii = false
				break
			}
		}
		s.isASCII = ascii
		if ascii {
			s.runeCount = len(s.Value)
		} else {
			s.runes = []rune(s.Value)
			s.runeCount = len(s.runes)
		}
	})
}

// RuneLen returns the number of Unicode code points in s.
// O(n) on the first rune-level access, O(1) thereafter.
func (s *String) RuneLen() int {
	s.ensureRuneCache()
	return s.runeCount
}

// RuneAt returns the i-th rune as (r, true), or (0, false) if i is out of
// bounds. O(1) after the first rune-level access on this string.
func (s *String) RuneAt(i int) (rune, bool) {
	s.ensureRuneCache()
	if i < 0 || i >= s.runeCount {
		return 0, false
	}
	if s.isASCII {
		return rune(s.Value[i]), true
	}
	return s.runes[i], true
}

// RuneSubstring returns the substring from rune index start (inclusive) to
// end (exclusive). Caller is responsible for bounds checking via RuneLen().
func (s *String) RuneSubstring(start, end int) string {
	s.ensureRuneCache()
	if s.isASCII {
		return s.Value[start:end]
	}
	return string(s.runes[start:end])
}

// -------------------- BYTES --------------------

// Bytes holds an arbitrary sequence of bytes — not necessarily valid utf-8.
// This is the type kLex uses for binary payloads (file contents, network
// frames, base64-decoded data, anything that doesn't have a natural text
// reading). It is a value type passed by reference like Array and Hash:
// builtins that mutate (none today) must document that explicitly.
//
// Inspect() renders as bytes(N) where N is the byte count, deliberately
// avoiding any attempt to render the raw bytes — they may be binary garbage
// when printed. Use bytesToHex() / bytesToBase64() / str() for visualisation.
type Bytes struct {
	Value []byte
}

func (b *Bytes) Type() ObjectType { return BYTES_OBJ }
func (b *Bytes) Inspect() string  { return fmt.Sprintf("bytes(%d)", len(b.Value)) }

// -------------------- NULL --------------------

// Null is a first-class value in kLex — it is not an error or the absence of
// a value, it is a deliberate "no value" that can be stored and compared.
// null == null is true. null == anything-else is false (never a TypeError).
type Null struct{}

func (n *Null) Type() ObjectType { return NULL_OBJ }
func (n *Null) Inspect() string  { return "null" }

// Singletons for immutable constant values — allocated once at startup,
// reused everywhere. Every comparison, condition, and null check returns
// one of these, eliminating the most common heap allocations in the evaluator.
var (
	TRUE  = &Boolean{Value: true}
	FALSE = &Boolean{Value: false}
	NULL  = &Null{}
)

// boolObj returns the singleton TRUE or FALSE for a Go bool, avoiding the
// per-comparison allocation that &Boolean{Value: b} would cause. Use this
// everywhere a boolean result needs to enter the kLex object graph — it is
// the difference between O(1) allocations and O(comparisons) allocations
// in any non-trivial program.
func boolObj(b bool) Object {
	if b {
		return TRUE
	}
	return FALSE
}

// Small-integer pool — kLex programs allocate Integers constantly: loop
// counters, array indices, intermediate arithmetic results. Pooling the
// most-common range eliminates the bulk of those allocations.
//
// The range -128..255 catches the overwhelming majority of cases: small
// literals, single-byte values, array indices in small arrays, and the
// early iterations of any loop. Each Integer is ~16 bytes; the whole pool
// is ~6KB of permanent memory.
//
// SAFETY: pooled Integers are SHARED — never mutate Integer.Value. kLex
// arithmetic always produces new Integers (we never overwrite an existing
// one), so this is enforced by convention, not by the type system. If you
// ever find yourself wanting to mutate Integer.Value, you're holding it
// wrong: allocate a fresh one instead.
const (
	smallIntMin = -128
	smallIntMax = 255
)

var smallInts [smallIntMax - smallIntMin + 1]*Integer

func init() {
	for i := range smallInts {
		smallInts[i] = &Integer{Value: smallIntMin + i}
	}
}

// intObj returns the pooled Integer for n if it falls in the small-int range,
// or allocates a fresh Integer otherwise. Drop-in replacement for
// &Integer{Value: n} — pointer identity differs (pooled values are reused)
// but kLex doesn't expose pointer identity for primitive types, so this is
// invisible to user code.
func intObj(n int) *Integer {
	if n >= smallIntMin && n <= smallIntMax {
		return smallInts[n-smallIntMin]
	}
	return &Integer{Value: n}
}

// NewInteger is the exported alias for intObj so the vm package (and
// other future callers outside eval) can box integers through the
// same pool path the evaluator uses. Keep the exported surface
// narrow — this is the only public Integer constructor.
func NewInteger(n int) *Integer { return intObj(n) }

// -------------------- ERROR --------------------

// ErrorKind distinguishes between type errors (wrong types for an operation)
// and runtime errors (out of bounds, division by zero, undefined variable).
// Keeping them separate gives better error messages.
type ErrorKind string

const (
	TypeError  ErrorKind = "TypeError"
	RuntimeErr ErrorKind = "RuntimeError"
)

// Frame is one entry in an error's call stack.
// It records the function name and the position of the call site —
// i.e. where in the source code this function was called from.
// Frames are appended to the Error as it bubbles up through evalCall,
// so the slice reads innermost-first (index 0 = where the error originated).
type Frame struct {
	FnName  string
	CallPos ast.Pos
}

// Error serves two roles depending on IsUserError:
//
//  false (default) — an internal propagation signal. Bubbles up through Eval
//  until it reaches the top-level loop or is caught by safe(). Users never
//  hold one of these directly.
//
//  true — a first-class user value created by error(code, message) or
//  returned by safe() when it catches a system error. Does NOT propagate;
//  isError() ignores it so it stays put in the environment.
//
// Code is only meaningful when IsUserError is true. For internal signals,
// the kind is already carried by the Kind field.
//
// Stack accumulates call frames as an internal error unwinds — each function
// boundary in evalCall appends one frame, giving a full call trace.
type Error struct {
	Kind        ErrorKind
	Pos         ast.Pos // where the error originated
	Message     string
	Stack       []Frame // call frames, innermost first (internal errors only)
	Code        string  // user-visible error code, e.g. "NOT_FOUND"
	IsUserError bool    // true = first-class value; false = propagation signal

	// Bridge-originated errors can carry structured detail from the remote
	// language: the exception class name (e.g. "ValueError", "FileNotFoundError")
	// and the full traceback. Both are empty for non-bridge errors. Exposed
	// to kLex code via err.errorType and err.traceback (see eval.go DotExpr).
	ErrorType string
	Traceback string

	// hookFired is set true by FireErrorBubbleHook on the first call
	// for this Error instance, so the agentic on_error_bubble hook
	// fires at most ONCE per logical error even when the error passes
	// through both eval-side helpers (typeError / runtimeError) AND
	// the VM's bubbleError path. Not exported — internal bookkeeping.
	hookFired bool
}

func (e *Error) Type() ObjectType { return ERROR_OBJ }
func (e *Error) Inspect() string {
	if e.IsUserError {
		return "error(" + e.Code + ": " + e.Message + ")"
	}
	out := fmt.Sprintf("%s: %s", e.Kind, e.Message)
	if e.Pos.Line > 0 {
		out += fmt.Sprintf("\n  at line %d, col %d", e.Pos.Line, e.Pos.Col)
	}
	for _, f := range e.Stack {
		name := f.FnName
		if name == "" {
			name = "<anonymous>"
		}
		if f.CallPos.Line > 0 {
			out += fmt.Sprintf("\n  in %s (called at line %d, col %d)", name, f.CallPos.Line, f.CallPos.Col)
		} else {
			out += fmt.Sprintf("\n  in %s", name)
		}
	}
	return out
}

// -------------------- ATOMIC ARRAYS --------------------

// AtomicIntArray is a fixed-size integer array supporting lock-free concurrent
// updates via the sync/atomic package. Multiple goroutines can call atomicAdd,
// atomicLoad, atomicStore, atomicCAS on different (or even the same) indices
// simultaneously without data races.
//
// Backed by []int64 because sync/atomic operates on int64. kLex Integer maps
// directly onto int64 internally.
type AtomicIntArray struct {
	Data []int64
}

func (a *AtomicIntArray) Type() ObjectType { return ATOMIC_INT_ARRAY_OBJ }
func (a *AtomicIntArray) Inspect() string {
	return fmt.Sprintf("AtomicIntArray(size=%d)", len(a.Data))
}

// AtomicFloatArray is a fixed-size float array supporting lock-free concurrent
// updates. Floats are stored as their IEEE-754 bit representation in int64
// slots so that sync/atomic.CompareAndSwapInt64 can be used. atomicAdd uses
// a CAS-loop internally; on contention it retries until the swap succeeds.
type AtomicFloatArray struct {
	Bits []int64 // each int64 holds the bits of a float64
}

func (a *AtomicFloatArray) Type() ObjectType { return ATOMIC_FLOAT_ARRAY_OBJ }
func (a *AtomicFloatArray) Inspect() string {
	return fmt.Sprintf("AtomicFloatArray(size=%d)", len(a.Bits))
}

// -------------------- CONCURRENT HASH --------------------

// ConcurrentHash is a thread-safe hash map for shared mutable state across
// goroutines. Backed by sync.Map (Go 1.20+) which provides lock-free reads
// and atomic CAS-based writes per key.
//
// Use cases:
//   - Shared event counter where keys are discovered dynamically (regular
//     atomic arrays require knowing the size up front)
//   - Cross-goroutine accumulator with arbitrary string/int keys
//   - Lock-free deduplication / set-membership checking
//
// Like regular Hash, supports string/integer/boolean keys via HashKey.
// Reads through ch[key] return the value or null. Writes via ch[key] = v
// are atomic. atomicHashIncr/atomicHashAdd provide lock-free arithmetic
// using sync.Map.CompareAndSwap.
//
// Cnt is an atomic counter so len(ch) is O(1) instead of O(n) iterating sync.Map.
// IMPORTANT: Cnt is incremented after a successful LoadOrStore / decremented
// after a successful Delete, but the two operations are not atomic together.
// Under concurrent mutation, len(ch) can briefly diverge from the actual map
// size by the number of in-flight Store/Delete calls. The map itself is always
// consistent; only the reported count is approximate during contention. For
// exact size after a known quiescent point, len() is correct.
type ConcurrentHash struct {
	M   sync.Map // HashKey → Object
	Cnt int64    // atomic count of live entries (approximate under concurrent mutation)
}

func (c *ConcurrentHash) Type() ObjectType { return CONCURRENT_HASH_OBJ }
func (c *ConcurrentHash) Inspect() string {
	return fmt.Sprintf("ConcurrentHash(size=%d)", atomic.LoadInt64(&c.Cnt))
}

// -------------------- RETURN --------------------

// ReturnValue is a wrapper that carries a value back up the call stack.
// When the evaluator sees `return expr`, it wraps the result in ReturnValue.
// Each level of eval checks for this wrapper and passes it up unchanged,
// until the function-call handler unwraps it and returns the inner value.
// This is the standard way to implement return in a tree-walking interpreter.
type ReturnValue struct {
	Value Object
}

func (r *ReturnValue) Type() ObjectType { return RETURN_OBJ }
func (r *ReturnValue) Inspect() string  { return r.Value.Inspect() }

// -------------------- BREAK / CONTINUE --------------------

// BreakSignal and ContinueSignal work the same way as ReturnValue:
// they are sentinel objects that bubble up through the eval loop
// until the while-loop handler catches them.
// This means break and continue are loop-local — they cannot cross
// function boundaries (a return wrapping them would be unwrapped first).
type BreakSignal struct{}

func (b *BreakSignal) Type() ObjectType { return BREAK_OBJ }
func (b *BreakSignal) Inspect() string  { return "break" }

type ContinueSignal struct{}

func (c *ContinueSignal) Type() ObjectType { return CONTINUE_OBJ }
func (c *ContinueSignal) Inspect() string  { return "continue" }

// -------------------- FUNCTION --------------------

// Function is a first-class value — functions can be stored in variables,
// passed as arguments, and returned from other functions.
//
// Env captures the environment at the point the function was defined (closure).
// When the function is later called, its body runs inside a new environment
// whose outer pointer is this captured Env, not the caller's environment.
// This is what gives kLex lexical (not dynamic) scoping.
//
// Name is set by the evaluator when an anonymous function is assigned to a
// variable (fn foo(x) { } → Name = "foo"). This enables recursion: foo can
// refer to itself by name because foo is in the outer env when the body runs.
type Function struct {
	Name        string // empty for anonymous functions
	Params      []string
	Defaults    []ast.Node // parallel to Params; nil entry means the param is required
	Variadic    bool       // true if the last param collects remaining args as an array
	NumRequired int        // count of leading required params; set once at construction
	Body        []ast.Node
	Env         *Environment // the closure environment captured at definition time
}

func (f *Function) Type() ObjectType { return FUNCTION_OBJ }
func (f *Function) Inspect() string {
	if f.Name != "" {
		return "fn " + f.Name
	}
	return "fn"
}

// -------------------- ARRAY --------------------

// Array is a mutable, ordered list of Objects.
// All elements are Objects, so arrays can hold mixed types: [1, "two", true].
// Arrays are passed by reference — if two variables point to the same *Array,
// mutating one mutates the other.
type Array struct {
	Elements []Object
	frozen   bool
}

func (a *Array) Type() ObjectType { return ARRAY_OBJ }
func (a *Array) Inspect() string {
	var buf strings.Builder
	buf.WriteString("[")
	for i, el := range a.Elements {
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(el.Inspect())
	}
	buf.WriteString("]")
	return buf.String()
}

// -------------------- TUPLE --------------------

// Tuple carries multiple return values from a function.
// It is produced by `return a, b` and consumed by `a, b = expr`.
// Tuples are not general-purpose values — they exist solely to transport
// multiple return values across a function boundary. If a Tuple ends up
// assigned to a single variable, it can be inspected but not indexed.
type Tuple struct {
	Elements []Object
}

func (t *Tuple) Type() ObjectType { return TUPLE_OBJ }
func (t *Tuple) Inspect() string {
	var buf strings.Builder
	buf.WriteString("(")
	for i, el := range t.Elements {
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(el.Inspect())
	}
	buf.WriteString(")")
	return buf.String()
}

// -------------------- HASH --------------------

// HashKey is the map key used internally in Go to store hash pairs.
// We can't use Object directly as a Go map key (interfaces aren't comparable
// in the way we need), so we convert each kLex key to a HashKey struct.
//
// Including the Type field means integer 1 and string "1" are different keys,
// which is correct: {"1": "a", 1: "b"} has two distinct entries.
type HashKey struct {
	Type  ObjectType
	Value string // string representation of the key value
}

// HashPair stores the original kLex Key object alongside the Value so that
// the keys() builtin can return the real kLex objects, not the internal strings.
type HashPair struct {
	Key   Object
	Value Object
}

// Hash is a mutable key-value store. Valid key types are string, integer,
// and boolean (anything that can be reliably converted to a HashKey).
// Like arrays, hashes are passed by reference.
//
// CONCURRENCY (OFI #3): kLex's async snapshot semantics pass *Hash by
// pointer across goroutine boundaries — see the cheatsheet in
// docs/ASYNC_BEST_PRACTICES.MD §2.1.1. That makes accidental
// concurrent mutation easy, and a bare Go map panics with
// "fatal error: concurrent map writes" — unrecoverable.
//
// M5 lazy mutex (audit follow-up 2026-05-22): the `mu` mutex used to
// be acquired on EVERY read/write/iteration. Profiling showed that
// for synchronous code (the vast majority — json.parse, kv_store,
// observable on a single goroutine), the mutex cost ~25ns per op
// dominated. With this change Hash starts NON-shared and skips
// locking entirely. The first time the hash crosses a goroutine
// boundary (send/async/parallelArray*/etc.) we MarkShared(h), which
// atomically flips `shared` to true. From then on every Lock /
// Unlock takes the mutex.
//
// Correctness: MarkShared MUST be called BEFORE the goroutine
// publication (the channel send / `go` statement). The atomic.Store
// in MarkShared establishes a happens-before with the read in
// Lock()/Unlock() of subsequent goroutines, so they correctly observe
// shared=true and use the mutex. The original goroutine — which may
// still hold the hash — will also observe its own atomic.Store and
// switch to mutex-acquiring access on subsequent operations.
//
// For high-throughput shared mutation, prefer ConcurrentHash — its
// sync.Map-backed implementation is lock-free for many operations.
// Hash's mutex is a correctness floor, not a performance peak.
type Hash struct {
	Pairs  map[HashKey]HashPair
	frozen bool
	shared atomic.Bool // lazy-shared flag; once true, mu is mandatory
	mu     sync.Mutex
}

func (h *Hash) Type() ObjectType { return HASH_OBJ }

// IsShared reports whether the hash has been published across
// goroutine boundaries (and thus every operation must take mu).
// Used by Lock/Unlock and the Get/Set/Del/LenSafe/Snapshot helpers.
// Public so vm-side code (in particular vm/external_callable.go's
// async snapshot) can include hashes when recursively marking.
func (h *Hash) IsShared() bool { return h.shared.Load() }

// MarkShared flips the lazy-shared flag and is idempotent. Called
// by MarkSharedRecursive at every goroutine-crossing sink. Bare
// MarkShared (without the recursive walk) is rarely what you want —
// nested hashes inside the marked one would still be lock-free —
// but is exposed for unit tests and tight wrappers.
func (h *Hash) MarkShared() { h.shared.Store(true) }

// Lock acquires the hash's mutex IF the hash has been marked shared.
// For never-shared hashes (the common case under synchronous use),
// it's a single atomic read + branch — about 1ns vs the ~25ns of
// an unconditional mu.Lock.
func (h *Hash) Lock() {
	if h.shared.Load() {
		h.mu.Lock()
	}
}

// Unlock releases the mutex IF it was acquired by a paired Lock.
// Must mirror Lock — the shared flag is single-direction
// (false → true, never back), so it's safe to test load again
// here: if Lock observed shared=true and took mu, Unlock also
// observes shared=true (it's monotone) and releases. If Lock
// observed shared=false, Unlock observes the same and skips.
//
// One subtle invariant: a goroutine can be IN the middle of a
// Lock/Unlock-bracketed section when ANOTHER goroutine calls
// MarkShared. The mid-section reader observed shared=false at
// Lock-time (no mu held) and will observe... whatever Load
// returns at Unlock-time. If shared became true mid-section,
// Unlock would try mu.Unlock() on an unlocked mutex — panic.
//
// Avoidance: MarkShared is ALWAYS called from the OWNING
// goroutine, BEFORE the value is published. The owning goroutine
// can't be inside a Lock/Unlock-bracketed section at that moment
// (single-threaded code, no preemption mid-statement). So when
// other goroutines later receive the published value, Lock sees
// shared=true and takes mu; Unlock sees the same.
func (h *Hash) Unlock() {
	if h.shared.Load() {
		h.mu.Unlock()
	}
}

// Snapshot returns a stable slice of (HashKey, HashPair) pairs taken
// under the lock. Callers iterate the snapshot WITHOUT holding the
// lock, so user-level callbacks (e.g. for-in bodies) don't block
// concurrent writers and can't deadlock by recursively locking the
// same hash.
//
// Order is non-deterministic (Go map iteration). Snapshot is O(n)
// in space — fine for typical kLex hashes, but consider locked
// direct access for very large hashes where you only need a single
// key.
func (h *Hash) Snapshot() []HashPair {
	if h.shared.Load() {
		h.mu.Lock()
	}
	out := make([]HashPair, 0, len(h.Pairs))
	for _, p := range h.Pairs {
		out = append(out, p)
	}
	if h.shared.Load() {
		h.mu.Unlock()
	}
	return out
}

// Get safely reads a single key. Returns (HashPair{}, false) on miss.
// Replaces the unsafe `pair, ok := h.Pairs[k]` pattern at any site
// that may race with writers.
func (h *Hash) Get(k HashKey) (HashPair, bool) {
	if h.shared.Load() {
		h.mu.Lock()
		p, ok := h.Pairs[k]
		h.mu.Unlock()
		return p, ok
	}
	p, ok := h.Pairs[k]
	return p, ok
}

// Set safely writes a single pair. Callers that need to preserve the
// frozen check must do so BEFORE calling Set — Set itself doesn't
// inspect frozen so it stays a tight, single-purpose primitive.
func (h *Hash) Set(k HashKey, p HashPair) {
	if h.shared.Load() {
		h.mu.Lock()
		h.Pairs[k] = p
		h.mu.Unlock()
		return
	}
	h.Pairs[k] = p
}

// Del safely removes a key. No-op on miss. Same frozen-check
// expectations as Set.
func (h *Hash) Del(k HashKey) {
	if h.shared.Load() {
		h.mu.Lock()
		delete(h.Pairs, k)
		h.mu.Unlock()
		return
	}
	delete(h.Pairs, k)
}

// LenSafe returns the current entry count under the lock. The bare
// `len(h.Pairs)` is technically racy with concurrent writers (Go's
// runtime won't panic on the read but the result could be stale).
func (h *Hash) LenSafe() int {
	if h.shared.Load() {
		h.mu.Lock()
		n := len(h.Pairs)
		h.mu.Unlock()
		return n
	}
	return len(h.Pairs)
}

func (h *Hash) Inspect() string {
	var buf strings.Builder
	buf.WriteString("{")
	pairs := h.Snapshot()
	// Sort by key.Inspect() so the rendered string is deterministic
	// across runs. Go map iteration is randomised; without sorting,
	// `println({a: 1, b: 2})` could print either order, which makes
	// tree-walker-vs-VM differential tests flaky for any test that
	// stringifies a hash. The contract that mattered was already
	// "no guaranteed order" — sorting just picks one.
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].Key.Inspect() < pairs[j].Key.Inspect()
	})
	first := true
	for _, pair := range pairs {
		if !first {
			buf.WriteString(", ")
		}
		buf.WriteString(pair.Key.Inspect())
		buf.WriteString(": ")
		buf.WriteString(pair.Value.Inspect())
		first = false
	}
	buf.WriteString("}")
	return buf.String()
}

// MarkSharedRecursive walks `v` and marks every reachable mutable
// container as goroutine-shared. Called by every goroutine-crossing
// sink (send, async args+upvalues, parallelArray*, callInParallel)
// BEFORE the goroutine publication. The atomic.Store inside Hash.
// MarkShared establishes the happens-before edge so subsequent
// goroutines observe shared=true and use mu.
//
// Currently only *Hash benefits — *Array doesn't have a mutex and
// *StructInstance has its own (always-on) RWMutex. But we walk
// Array / StructInstance / Tuple contents in case nested hashes
// are inside them. Cycles are handled via the visited set.
//
// Cheap when the value is a primitive: a single type-switch
// fall-through, no allocations. Called per goroutine-crossing —
// not on hot read/write paths — so the recursive walk cost is
// amortised against the one-time-per-crossing benefit.
func MarkSharedRecursive(v Object) {
	if v == nil {
		return
	}
	visited := make(map[Object]bool)
	markSharedRec(v, visited)
}

func markSharedRec(v Object, visited map[Object]bool) {
	if v == nil || visited[v] {
		return
	}
	visited[v] = true
	switch o := v.(type) {
	case *Hash:
		// CRITICAL: take the mutex BEFORE flipping the flag and
		// iterating Pairs. The walker may be reached for a hash
		// that's already shared and currently being mutated by
		// other goroutines (under their own Lock()). Acquiring mu
		// here serialises the walker against those writers.
		//
		// Short-circuit when the hash is ALREADY shared: nested
		// hashes were marked transitively on the original
		// publication, so re-walking would be redundant work AND
		// race-free is no longer guaranteed for any nested state
		// ADDED after publication (rare; documented limitation —
		// post-publication insertions of fresh mutable state into
		// a published container are NOT auto-marked).
		o.mu.Lock()
		if o.IsShared() {
			o.mu.Unlock()
			return
		}
		o.MarkShared()
		pairs := make([]HashPair, 0, len(o.Pairs))
		for _, p := range o.Pairs {
			pairs = append(pairs, p)
		}
		o.mu.Unlock()
		for _, p := range pairs {
			markSharedRec(p.Value, visited)
			markSharedRec(p.Key, visited)
		}
	case *Array:
		for _, el := range o.Elements {
			markSharedRec(el, visited)
		}
	case *Tuple:
		for _, el := range o.Elements {
			markSharedRec(el, visited)
		}
	case *StructInstance:
		// StructInstance already serialises Fields access via its
		// own RWMutex (always on). Walking the fields for nested
		// hashes is the win — a hash inside a struct field that
		// gets passed across goroutines also needs marking.
		o.mu.RLock()
		fields := make([]Object, 0, len(o.Fields))
		for _, fv := range o.Fields {
			fields = append(fields, fv)
		}
		o.mu.RUnlock()
		for _, fv := range fields {
			markSharedRec(fv, visited)
		}
	}
	// Primitives (Integer, Float, String, Boolean, Null), and
	// reference types without nested mutable state (Function,
	// Channel, Task, NetConn, DBConn, etc.) fall through — nothing
	// to mark.
}

// -------------------- MODULE --------------------

// Module is the runtime representation of an imported file.
// Its Env holds all top-level variables and functions defined in that file.
// Property access (math.add) looks up names directly in this Env.
type Module struct {
	Name string       // the alias used in the import statement
	Env  *Environment // the module's top-level scope after evaluation
}

func (m *Module) Type() ObjectType { return MODULE_OBJ }
func (m *Module) Inspect() string  { return "module(" + m.Name + ")" }

// -------------------- TASK --------------------

// Task represents an asynchronous computation launched by async().
// The done channel is closed when the goroutine finishes; result holds
// whatever the function returned (including an *Error if it failed).
// Reading result after <-done is safe without a mutex: the Go memory model
// guarantees that writes before close(done) are visible after <-done returns.
type Task struct {
	done   atomic.Bool
	result Object
}

func (t *Task) Type() ObjectType { return TASK_OBJ }
func (t *Task) Inspect() string  { return "task" }

// taskPool reuses Task objects to reduce allocation overhead.
var taskPool = sync.Pool{
	New: func() interface{} {
		return &Task{}
	},
}

// getTask retrieves a Task from the pool or allocates a new one.
// Caller must call returnTask() when done to return it to the pool.
func getTask() *Task {
	task := taskPool.Get().(*Task)
	task.done.Store(false)
	task.result = nil
	return task
}

// returnTask returns a Task to the pool for reuse.
func returnTask(task *Task) {
	taskPool.Put(task)
}

// -------------------- CHANNEL --------------------

// Channel is a goroutine-safe conduit for passing values between tasks.
// It wraps a Go channel of Objects so kLex tasks can communicate without
// sharing mutable state directly.
// Unbuffered (cap 0): send blocks until a receiver is ready.
// Buffered (cap n):   send blocks only when the buffer is full.
// done is closed by the consumer (via cancel() or for-in break) to signal
// that no more values should be sent. send() returns false when done is closed.
type Channel struct {
	ch   chan Object
	done chan struct{}
}

func (c *Channel) Type() ObjectType { return CHANNEL_OBJ }
func (c *Channel) Inspect() string  { return fmt.Sprintf("channel(cap=%d)", cap(c.ch)) }

// -------------------- NET_CONN --------------------

// NetConn wraps a net.Conn for use in kLex programs.
// Produced by tcpDial and tcpListen; consumed by netRead, netWrite, netClose.
type NetConn struct {
	Conn net.Conn
}

func (n *NetConn) Type() ObjectType { return NET_CONN_OBJ }
func (n *NetConn) Inspect() string {
	if n.Conn == nil {
		return "conn(closed)"
	}
	return "conn(" + n.Conn.RemoteAddr().String() + ")"
}

// -------------------- DB_CONN / DB_TX --------------------

// DBConn wraps a *sql.DB connection pool for use in kLex programs.
// Produced by dbOpen; consumed by dbQuery, dbExec, dbBegin, dbClose, dbPing.
// The pool is safe for concurrent use across goroutines.
type DBConn struct {
	DB      *sql.DB
	Driver  string        // user-facing driver name ("mssql", "postgres")
	Timeout time.Duration // 0 = no timeout (context.Background())
}

func (d *DBConn) Type() ObjectType { return DB_CONN_OBJ }
func (d *DBConn) Inspect() string  { return "dbconn(" + d.Driver + ")" }

// DBTx wraps a *sql.Tx database transaction.
// Produced by dbBegin; consumed by dbQuery, dbExec, dbCommit, dbRollback.
type DBTx struct {
	Tx      *sql.Tx
	Driver  string
	Timeout time.Duration // inherited from DBConn; overridable via dbSetTimeout
}

func (d *DBTx) Type() ObjectType { return DB_TX_OBJ }
func (d *DBTx) Inspect() string  { return "dbtx(" + d.Driver + ")" }

// -------------------- BRIDGE --------------------

// BridgeRingBuffer is a fixed-size byte buffer that drops oldest bytes
// when capacity is exceeded. Used to capture the tail of a bridge
// subprocess's stderr output so it can be surfaced in error messages
// or via bridgeStderr().
type BridgeRingBuffer struct {
	mu    sync.Mutex
	buf   []byte
	limit int
}

func NewBridgeRingBuffer(limit int) *BridgeRingBuffer {
	return &BridgeRingBuffer{limit: limit}
}

func (r *BridgeRingBuffer) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf = append(r.buf, p...)
	if len(r.buf) > r.limit {
		r.buf = r.buf[len(r.buf)-r.limit:]
	}
	return len(p), nil
}

func (r *BridgeRingBuffer) Snapshot() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]byte, len(r.buf))
	copy(out, r.buf)
	return out
}

// bridgeMetricsSampleN is the per-function circular buffer size for latency
// samples. 256 is generous enough to give stable percentile estimates while
// keeping memory predictable: 256 floats × N functions per bridge.
const bridgeMetricsSampleN = 256

// fnMetrics tracks per-function counters and a circular buffer of recent
// call latencies (milliseconds) used to compute p50/p95/p99 on read.
type fnMetrics struct {
	count    int64
	errors   int64
	samples  [bridgeMetricsSampleN]float64
	wrIdx    int  // next slot to overwrite
	wrapped  bool // true once samples has filled at least once
}

// bridgeMetrics holds the per-bridge observability state. All fields are
// mutated under mu; the bridgeMetrics() builtin reads them as a snapshot.
type bridgeMetrics struct {
	mu             sync.Mutex
	callsTotal     int64
	callsFailed    int64
	callsInflight  int64
	streamsTotal   int64
	streamsActive  int64
	streamsFailed  int64
	bytesSent      int64
	bytesReceived  int64
	errorsByCode   map[string]int64
	perFunction    map[string]*fnMetrics
}

// recordCall adds one sample for fn and updates the relevant counters.
// elapsedMs is the wall-clock duration; errCode is "" on success.
func (m *bridgeMetrics) recordCall(fn string, elapsedMs float64, errCode string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callsInflight--
	if errCode != "" {
		m.callsFailed++
		if m.errorsByCode == nil {
			m.errorsByCode = make(map[string]int64)
		}
		m.errorsByCode[errCode]++
	}
	if m.perFunction == nil {
		m.perFunction = make(map[string]*fnMetrics)
	}
	fm := m.perFunction[fn]
	if fm == nil {
		fm = &fnMetrics{}
		m.perFunction[fn] = fm
	}
	fm.count++
	if errCode != "" {
		fm.errors++
	}
	fm.samples[fm.wrIdx] = elapsedMs
	fm.wrIdx++
	if fm.wrIdx >= bridgeMetricsSampleN {
		fm.wrIdx = 0
		fm.wrapped = true
	}
}

// bridgeResponse is the parsed form of one line from the bridge subprocess's
// stdout. The reader goroutine parses each line exactly once and forwards
// this wrapper to the waiting caller's channel — callers used to receive
// raw bytes and re-parse, which doubled the JSON work and forced a
// defensive copy out of bufio.Scanner's buffer per message.
//
//   msg   — the decoded JSON object the caller would otherwise re-Unmarshal.
//   bytes — the raw line length, used by bridgeMetrics.bytesReceived. The
//           reader records this so the call site doesn't need the raw slice.
//
// A nil *bridgeResponse delivered on a pending channel is the lifecycle
// signal used by taintAllBridge ("bridge became unavailable"), matching
// the previous nil-byte-slice convention.
type bridgeResponse struct {
	msg   map[string]interface{}
	bytes int
}

// Bridge is a persistent subprocess connection for cross-language FFI.
// kLex communicates with the subprocess via line-delimited JSON over
// stdin/stdout. Any language that can read/write JSON lines can be a bridge.
//
// Phase 2 architecture: a single reader goroutine owns stdout exclusively.
// It routes response lines to per-call channels (keyed by id) and
// notification lines to notifCh. This enables:
//   - Concurrent calls: no mutex held during the wait, only during the write.
//   - Server-push notifications: bridge can emit {"notif": ...} at any time.
type Bridge struct {
	Cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    *bufio.Scanner
	timeout   time.Duration     // 0 = no timeout
	stderrBuf *BridgeRingBuffer // tail of stderr for error reporting
	stderrLog string            // path to stderr log file (if set)

	// mu protects all mutable lifecycle state: tainted, closed, pending, nextID.
	// It is held only briefly (never during the long wait for a response).
	mu      sync.Mutex
	nextID  int
	pending map[int]chan *bridgeResponse // per-call response channels; keyed by request id
	tainted   bool
	taintMsg  string
	taintCode string // error code delivered to the FIRST waiting call (BRIDGE_CLOSED, BRIDGE_TIMEOUT, etc.)
	closed    bool

	// writeMu serialises writes to stdin so concurrent calls don't interleave
	// their JSON on the wire. Held only during the Write() call (microseconds).
	writeMu sync.Mutex

	// notifCh is the kLex channel to which the reader goroutine delivers
	// unsolicited {"notif": ...} messages. Always created in nativeBridge;
	// closed by the reader when stdout closes.
	notifCh    *Channel
	notifClose sync.Once

	// schemas is the per-handler signature map populated during the
	// __schema__ handshake in nativeBridge. nil when the bridge doesn't
	// expose __schema__ (older-style bridges still work).
	schemas map[string]*FnSchema

	// protocol, capabilities, and the *Info fields are populated during the
	// __hello__ handshake in nativeBridge. A bridge that doesn't implement
	// __hello__ is treated as protocol 0 with an empty capability set; every
	// pre-handshake bridge keeps working unchanged.
	//
	// capabilities holds the negotiated intersection (features both sides
	// support). New features added to the bridge protocol that need explicit
	// opt-in are gated on entries in this map — today only "schema" and
	// "binary" are recognised; more will be added as features land.
	protocol        int
	capabilities    map[string]bool
	helperInfo      string // e.g. "klex_bridge.py/0.7.0"
	language        string // e.g. "python", "node"
	languageVersion string // e.g. "3.12.4"

	// metrics captures per-bridge counters, byte volumes, error-code
	// breakdowns, and per-function latency samples. Lazily allocated in
	// spawnBridge; mutated under its own mutex by bridgeCall/bridgeStream
	// instrumentation; read as a snapshot by the bridgeMetrics() builtin.
	metrics *bridgeMetrics

	// streams holds per-call channels for in-flight bridgeStream() calls,
	// keyed by request id. The reader goroutine delivers each {"id": N,
	// "stream": item} message to streams[N], and closes + deletes the entry
	// when a {"id": N, "stream_end": true} arrives (or on a streaming error,
	// after delivering the error through the channel).
	streams map[int]*Channel

	// streamIdleReset is signalled by dispatchBridgeLine on every stream item
	// arrival so the per-stream watchdog goroutine can reset its idle timer.
	// Buffered(1) per entry — a second item arriving before the watcher has
	// drained the previous signal is harmless (one reset covers any number of
	// items that arrived since the last drain).
	streamIdleReset map[int]chan struct{}

	// streamUnackedCount tracks items delivered to the kLex consumer channel
	// since the last ack we sent the bridge. Increments on each stream item
	// in dispatchBridgeLine; when it reaches streamAckThreshold[id] we emit
	// an {"ack": K, "id": M} and reset to zero. Per-stream because each
	// stream has its own ack cadence determined by its window.
	streamUnackedCount map[int]int

	// streamAckThreshold is window/2 for each stream — the kLex side acks
	// at half-window so the bridge never blocks waiting for an ack we are
	// still batching up. Zero entry means "no backpressure on this stream";
	// dispatchBridgeLine then never sends an ack for it.
	streamAckThreshold map[int]int
}

func (b *Bridge) Type() ObjectType { return BRIDGE_OBJ }
func (b *Bridge) Inspect() string {
	if len(b.Cmd.Args) > 0 {
		return "bridge(" + strings.Join(b.Cmd.Args, " ") + ")"
	}
	return "bridge"
}

// Accessors used by builtins_bridge.go (same package, so unexported fields are
// visible directly — these exist for readability and test use).
func (b *Bridge) Stdin() io.WriteCloser    { return b.stdin }
func (b *Bridge) Stdout() *bufio.Scanner   { return b.stdout }
func (b *Bridge) Timeout() time.Duration   { return b.timeout }
func (b *Bridge) StderrBuf() *BridgeRingBuffer { return b.stderrBuf }
func (b *Bridge) StderrLog() string        { return b.stderrLog }
func (b *Bridge) IsClosed() bool           { return b.closed }
func (b *Bridge) IsTainted() bool          { return b.tainted }
func (b *Bridge) TaintMsg() string         { return b.taintMsg }
func (b *Bridge) NotifCh() *Channel        { return b.notifCh }

// -------------------- ENUM --------------------

// EnumDef is the runtime type definition — bound in the environment as e.g. Shape.
// Variants maps each variant name to its ordered field names.
type EnumDef struct {
	Name     string
	Variants map[string][]string // variant name → field names (nil = zero-field)
}

func (e *EnumDef) Type() ObjectType { return ENUM_DEF_OBJ }
func (e *EnumDef) Inspect() string  { return "enum " + e.Name }

// EnumVariant is the descriptor for a data-carrying variant, produced when you
// evaluate Shape.Circle without calling it. Calling it produces an EnumInstance.
type EnumVariant struct {
	TypeName    string
	VariantName string
	Fields      []string // field names in declaration order
}

func (e *EnumVariant) Type() ObjectType { return ENUM_VARIANT_OBJ }
func (e *EnumVariant) Inspect() string  { return e.TypeName + "." + e.VariantName }

// EnumInstance is a concrete enum value — the result of constructing a variant.
// Zero-field variants are instances directly (no call required).
type EnumInstance struct {
	TypeName    string
	VariantName string
	FieldNames  []string          // declaration order, used by Inspect
	Fields      map[string]Object // field name → value
}

func (e *EnumInstance) Type() ObjectType { return ENUM_OBJ }
func (e *EnumInstance) Inspect() string {
	if len(e.FieldNames) == 0 {
		return e.TypeName + "." + e.VariantName
	}
	var buf strings.Builder
	buf.WriteString(e.TypeName)
	buf.WriteString(".")
	buf.WriteString(e.VariantName)
	buf.WriteString("(")
	for i, name := range e.FieldNames {
		if i > 0 {
			buf.WriteString(", ")
		}
		val := e.Fields[name]
		if val == nil {
			val = NULL
		}
		buf.WriteString(name)
		buf.WriteString(": ")
		buf.WriteString(val.Inspect())
	}
	buf.WriteString(")")
	return buf.String()
}

// -------------------- STRUCT DEF --------------------

// StructDef is the runtime representation of a struct type declaration.
// It is stored in the environment under the struct's name, like a function.
// Methods are stored as Functions with an empty Env; self is injected at call time.
type StructDef struct {
	Name    string
	Fields  []string             // declared field names, in order
	Methods map[string]*Function // method name → tree-walker function

	// MethodsAny is a polymorphic method table populated by alternative
	// front-ends (currently the bytecode VM, which stores
	// *vm.CompiledFunction values here). It lets the same StructDef
	// hold methods compiled by either interpreter without forcing
	// either one to know about the other's callable type. Method
	// dispatch sites should consult MethodsAny first, and only fall
	// back to Methods when MethodsAny is empty or missing the name.
	// nil-safe — empty / absent means "no VM-compiled methods."
	MethodsAny map[string]Object

	// H2 (audit fix, 2026-05-22): cache of tree-walker methods with
	// `self` prepended to their parameter list. The bytecode VM's
	// OpCallMethod fallback (when MethodsAny misses and we hit the
	// tree-walker-compiled Methods map) used to allocate a fresh
	// *Function + two appended slices per call. With this cache,
	// the wrapper is built once on first dispatch and reused
	// forever. methodsWithSelfMu serialises lazy population only;
	// the read path after first init is lock-free via the existing
	// map-read semantics (no concurrent writes once populated).
	methodsWithSelf   map[string]*Function
	methodsWithSelfMu sync.Mutex
}

// MethodWithSelf returns the tree-walker method `name` with `self`
// prepended as the first parameter. Lazily populated and cached.
// Returns nil if the method isn't in the tree-walker Methods map —
// caller should fall through to the VM-compiled MethodsAny lookup.
func (s *StructDef) MethodWithSelf(name string) *Function {
	s.methodsWithSelfMu.Lock()
	defer s.methodsWithSelfMu.Unlock()
	if s.methodsWithSelf != nil {
		if w, ok := s.methodsWithSelf[name]; ok {
			return w
		}
	}
	fn, ok := s.Methods[name]
	if !ok {
		return nil
	}
	if s.methodsWithSelf == nil {
		s.methodsWithSelf = make(map[string]*Function, len(s.Methods))
	}
	// Build wrapper once. Append produces a fresh backing array each
	// invocation in the wild — caching the wrapper means subsequent
	// calls share the same Params / Defaults slices too.
	wrapped := &Function{
		Name:        fn.Name,
		Params:      append([]string{"self"}, fn.Params...),
		Defaults:    append([]ast.Node{nil}, fn.Defaults...),
		Variadic:    fn.Variadic,
		NumRequired: fn.NumRequired + 1,
		Body:        fn.Body,
		Env:         fn.Env,
	}
	s.methodsWithSelf[name] = wrapped
	return wrapped
}

func (s *StructDef) Type() ObjectType { return STRUCT_DEF_OBJ }
func (s *StructDef) Inspect() string  { return "struct " + s.Name }

// -------------------- STRUCT INSTANCE --------------------

// StructInstance is one concrete value of a struct type.
// Fields holds the current values of all declared fields.
//
// CONCURRENCY: kLex's async snapshot model passes *StructInstance by
// pointer, so multiple goroutines can legitimately call methods that
// mutate the same instance's Fields map. Go's runtime panics on
// concurrent map mutations even if logically benign — so every
// read or write of Fields MUST go through GetField / SetField, which
// take the mutex. Direct `inst.Fields[k]` access is a footgun and
// will panic under concurrent load. (See observableTest's pattern
// of multi-goroutine subscribers updating self.value.)
type StructInstance struct {
	mu     sync.RWMutex
	Def    *StructDef
	Fields map[string]Object
	frozen bool
}

// GetField is the locked accessor for instance fields. Use this in
// every site that reads Fields — direct `inst.Fields[k]` access
// races against any concurrent SetField from another goroutine.
func (s *StructInstance) GetField(name string) (Object, bool) {
	s.mu.RLock()
	v, ok := s.Fields[name]
	s.mu.RUnlock()
	return v, ok
}

// SetField is the locked mutator for instance fields. Use this in
// every site that writes Fields. Skips its own frozen check —
// callers (EvalSetField) already do that under their own error
// reporting path before calling here.
func (s *StructInstance) SetField(name string, val Object) {
	s.mu.Lock()
	s.Fields[name] = val
	s.mu.Unlock()
}

func (s *StructInstance) Type() ObjectType { return STRUCT_INST_OBJ }
func (s *StructInstance) Inspect() string {
	var buf strings.Builder
	buf.WriteString(s.Def.Name)
	buf.WriteString(" {")
	first := true
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, name := range s.Def.Fields {
		if !first {
			buf.WriteString(", ")
		}
		val := s.Fields[name]
		if val == nil {
			val = NULL
		}
		buf.WriteString(name)
		buf.WriteString(": ")
		buf.WriteString(val.Inspect())
		first = false
	}
	buf.WriteString("}")
	return buf.String()
}

// -------------------- IMAGE --------------------

// Image holds image data for use with drawImage().
// Texture upload is deferred to the first drawImage() call so that
// loadImage() can safely be called before window() opens an OpenGL context.
type Image struct {
	TextureID uint32
	W, H      int
	pixels    []byte // non-nil until first drawImage() uploads to GPU
}

func (img *Image) Type() ObjectType { return IMAGE_OBJ }
func (img *Image) Inspect() string  { return fmt.Sprintf("image(%dx%d)", img.W, img.H) }

// -------------------- FONT --------------------

// glyphMetric stores the UV coordinates and advance width for one glyph
// in a Font's SDF atlas texture.
type glyphMetric struct {
	u0, u1  float32 // horizontal UV extent in the atlas (0..1)
	advance float32 // display-scale horizontal pen advance after this glyph
}

// Font holds a loaded TrueType/OpenType font as an SDF texture atlas.
// GPU upload is deferred to the first textFont() call, matching Image.
// All metric values are at scale 1 (display pixels); multiply by scale when drawing.
// glyphs is keyed by Unicode codepoint; fallback is rendered for missing codepoints.
type Font struct {
	TextureID uint32
	LineH     float32             // line height at scale 1
	glyphs    map[rune]glyphMetric
	fallback  glyphMetric         // rendered for codepoints not in glyphs (· middle dot)
	atlasW    int32               // atlas pixel width for GPU upload
	atlasHpx  int32               // atlas pixel height for GPU upload
	pixels    []byte              // non-nil until first textFont() uploads to GPU
}

func (f *Font) Type() ObjectType { return FONT_OBJ }
func (f *Font) Inspect() string  { return fmt.Sprintf("font(lineH=%.1f)", f.LineH) }

// -------------------- BUILTIN --------------------

// BuiltinFunction is the Go function signature for built-in functions.
// Built-ins receive already-evaluated arguments and return an Object.
type BuiltinFunction func(args []Object) Object

// Builtin wraps a Go function so it can live in the environment alongside
// user-defined functions and be called with the same call syntax.
//
// RetainsArgs (M4 audit fix, 2026-05-22): true if the Fn captures the
// args slice in any way that outlives the call — e.g. async() hands
// args[1:] to a goroutine that runs after the Fn returns. When true,
// the VM's OpCallBuiltin allocates a fresh args slice per call so the
// retained reference can't see another caller's args. When false
// (the default), OpCallBuiltin reuses a pooled args buffer — Fn
// reads the args during its own execution but doesn't keep a
// reference beyond return.
//
// Default false. Most builtins return after consuming args (println,
// len, arithmetic helpers, etc.); the few retainers (async, anything
// that fires-and-forgets a goroutine) must opt in explicitly.
// Mismarking a true retainer as false → silent data corruption on
// next call. Mismarking a non-retainer as true → harmless allocation.
type Builtin struct {
	Fn          BuiltinFunction
	RetainsArgs bool
}

func (b *Builtin) Type() ObjectType { return BUILTIN_OBJ }
func (b *Builtin) Inspect() string  { return "builtin" }
