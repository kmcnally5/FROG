package eval

import (
	"fmt"
	"klex/ast"
)

// builtins_iter.go — iteration-prep helpers used by the bytecode VM's
// compileForIn lowering.
//
// The tree-walker handles `for k, v in hash` directly in its ForInStmt
// arm: it snapshots the hash's pairs under the hash mutex and binds
// key+value per iteration. The VM's compileForIn uses an integer-
// indexed `coll[i]` lowering, which can't iterate hashes (integer
// indexing on a Hash returns NULL, not a pair).
//
// Rather than have the VM duplicate the hash snapshot + pair iteration
// in opcode form, we provide _iterPrep as a runtime helper:
//
//   _iterPrep(coll, twoVar)  →  (iterArray, isPairs)
//
// * For *Array / *String / *Bytes / *Tuple / *Channel / *ConcurrentHash —
//   pass-through; isPairs is FALSE; the VM's existing index-based loop
//   handles it.
// * For *Hash with twoVar=true — returns ([[k, v], …], TRUE). The VM
//   sees isPairs=true and unpacks each iter element as a 2-tuple into
//   the loop's two binding slots.
// * For *Hash with twoVar=false — returns an Error (tree-walker also
//   rejects single-var hash iteration with the same message).
// * For any other type — returns an Error.

func init() {
	Builtins["_iterPrep"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_iterPrep expects 2 arguments (collection, twoVar)", ast.Pos{})
		}
		twoVarObj, ok := args[1].(*Boolean)
		if !ok {
			return typeError(fmt.Sprintf("_iterPrep: second argument must be Bool, got %s", args[1].Type()), ast.Pos{})
		}
		twoVar := twoVarObj.Value

		switch coll := args[0].(type) {
		case *Hash:
			if !twoVar {
				return typeError("for-in over a hash requires two variables: for k, v in hash", ast.Pos{})
			}
			pairs := coll.Snapshot()
			out := make([]Object, len(pairs))
			for i, p := range pairs {
				out[i] = &Tuple{Elements: []Object{p.Key, p.Value}}
			}
			return &Tuple{Elements: []Object{
				&Array{Elements: out},
				TRUE,
			}}
		case *Array, *String, *Bytes, *Tuple:
			return &Tuple{Elements: []Object{coll, FALSE}}
		case *Channel:
			if twoVar {
				return typeError("for-in over a channel does not support two variables", ast.Pos{})
			}
			// Drain the channel into a slice. The VM's for-in
			// lowering is index-based and can't recv lazily without
			// a per-iteration opcode + dispatch — draining first is
			// the pragmatic path that matches the tree-walker's
			// observable behaviour for the common test pattern
			// (producer closes after sending its values, consumer
			// iterates to completion).
			//
			// Known limitation: a `break` inside the loop body cannot
			// cancel the producer here because we've already drained
			// the channel. The tree-walker closes `coll.done` on
			// break and lets the producer notice via `send` returning
			// false. Callers that rely on early-cancel-of-producer
			// semantics should keep using the tree-walker for now.
			out := []Object{}
			for val := range coll.ch {
				out = append(out, val)
			}
			return &Tuple{Elements: []Object{&Array{Elements: out}, FALSE}}
		default:
			return typeError(fmt.Sprintf("for-in: cannot iterate %s", args[0].Type()), ast.Pos{})
		}
	}}
}
