// Package vm holds kLex's bytecode virtual machine: the opcode set,
// the compiler that translates AST → bytecode, the interpreter that
// executes bytecode against a stack-machine value model, and the
// generator-tools that keep all the pieces in sync.
//
// # Why a separate package
//
// The existing tree-walking interpreter in `eval/` stays untouched
// throughout VM development. That gives us two things:
//
//   - A reference implementation to diff against during testing
//     (every existing tests/unit/*.lex becomes a regression test for
//     the VM the moment we wire the differential runner up).
//   - A clean rollback point if a VM commit goes sideways — `eval/`
//     is always a known-good state.
//
// # Sync discipline
//
// The pain of a previous VM attempt was opcode / handler / compiler
// drift: an opcode added in one place but missing the matching switch
// arm elsewhere. We attack that head-on with three generator tools:
//
//	vm/cmd/vmgen/        — opcode codegen. Reads vm/opcodes_def.go
//	                       (hand-written source of truth) and emits
//	                       vm/opcodes_gen.go (const block, name table,
//	                       stack-effect table, dispatch checklist).
//
//	vm/cmd/vmbuiltins/   — builtin registry codegen. Scans every
//	                       Builtins[X] = ... registration across
//	                       eval/builtins_*.go + eval/eval.go and emits
//	                       vm/builtins_gen.go with a stable
//	                       alphabetical index map + completeness check.
//
//	vm/cmd/vmcoverage/   — AST coverage audit. Walks ast/ast.go to
//	                       enumerate every Node-implementing type and
//	                       reports which lack a compiler clause. CI-
//	                       runnable so a new AST node without a
//	                       compiler arm breaks the build, not the user.
//
// All three are wired via `go:generate` directives. Running
// `go generate ./vm/...` regenerates every derived file from
// scratch — no manual sync, no drift.

//go:generate go run ./cmd/vmbuiltins

package vm
