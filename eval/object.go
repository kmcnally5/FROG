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
	"fmt"
	"klex/ast"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// ObjectType is a string tag naming the runtime type of a value.
type ObjectType string

const (
	INTEGER_OBJ  ObjectType = "INTEGER"
	FLOAT_OBJ    ObjectType = "FLOAT"
	BOOLEAN_OBJ  ObjectType = "BOOLEAN"
	STRING_OBJ   ObjectType = "STRING"
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

type String struct {
	Value string
}

func (s *String) Type() ObjectType { return STRING_OBJ }
func (s *String) Inspect() string  { return s.Value }

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
	Name     string // empty for anonymous functions
	Params   []string
	Defaults []ast.Node   // parallel to Params; nil entry means the param is required
	Variadic bool         // true if the last param collects remaining args as an array
	Body     []ast.Node
	Env      *Environment // the closure environment captured at definition time
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
type Hash struct {
	Pairs map[HashKey]HashPair
}

func (h *Hash) Type() ObjectType { return HASH_OBJ }
func (h *Hash) Inspect() string {
	var buf strings.Builder
	buf.WriteString("{")
	first := true
	for _, pair := range h.Pairs {
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
	Methods map[string]*Function // method name → function
}

func (s *StructDef) Type() ObjectType { return STRUCT_DEF_OBJ }
func (s *StructDef) Inspect() string  { return "struct " + s.Name }

// -------------------- STRUCT INSTANCE --------------------

// StructInstance is one concrete value of a struct type.
// Fields holds the current values of all declared fields.
type StructInstance struct {
	Def    *StructDef
	Fields map[string]Object
}

func (s *StructInstance) Type() ObjectType { return STRUCT_INST_OBJ }
func (s *StructInstance) Inspect() string {
	var buf strings.Builder
	buf.WriteString(s.Def.Name)
	buf.WriteString(" {")
	first := true
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

// -------------------- BUILTIN --------------------

// BuiltinFunction is the Go function signature for built-in functions.
// Built-ins receive already-evaluated arguments and return an Object.
type BuiltinFunction func(args []Object) Object

// Builtin wraps a Go function so it can live in the environment alongside
// user-defined functions and be called with the same call syntax.
type Builtin struct {
	Fn BuiltinFunction
}

func (b *Builtin) Type() ObjectType { return BUILTIN_OBJ }
func (b *Builtin) Inspect() string  { return "builtin" }
