// multiLetTest.lex — coverage for `let a, b = ...` declarations.
//
// Locks in the parser/eval/VM wiring for the new MultiLetStmt node:
//   - single-name `let x = expr` still works
//   - `let a, b = pair()` declares two locals from a tuple RHS
//   - `let _, x = pair()` discards the first element
//   - three-name multi-let
//   - bare multi-assign still mutates outer names (no `let`)
//   - arity mismatch is a runtime error
//   - inside a function body, `let a, b` creates fn-local bindings
//     that don't leak out (function frames DO create child envs)
//
// `let` is function-scoped in kLex, not block-scoped — `if/while/for`
// bodies share the enclosing function's env. The migration rule is
// "add `let` when the name is new to the scope chain"; rebinding an
// outer-scope name stays bare. So we do not test rust-style shadowing
// inside `if {}` here — that's not the language's contract.

import "assert.lex" as t

fn pair() {
    return 1, 2
}

fn triple() {
    return 10, 20, 30
}

// single-name let still works
let solo = 42
t.assertEqual(solo, 42)

// two-name multi-let
let a, b = pair()
t.assertEqual(a, 1)
t.assertEqual(b, 2)

// discard
let _, only = pair()
t.assertEqual(only, 2)

// three-name multi-let
let r, s, u = triple()
t.assertEqual(r, 10)
t.assertEqual(s, 20)
t.assertEqual(u, 30)

// existing bare multi-assign still mutates outer-scope names
let p = 0
let q = 0
p, q = pair()
t.assertEqual(p, 1)
t.assertEqual(q, 2)

// function-body scope: `let a, b` inside fn creates fn-local bindings
// that do not leak out of the function.
let outerA = 100
let outerB = 200
fn useFnLocals() {
    let outerA, outerB = pair()
    return outerA + outerB
}
let total = useFnLocals()
t.assertEqual(total, 3)
t.assertEqual(outerA, 100)
t.assertEqual(outerB, 200)

// arity mismatch — wrap in safe() so the test continues
fn tryShortUnpack() {
    let x, y, z = pair()      // 2 values into 3 vars
    return z
}
let _result, err = safe(tryShortUnpack)
t.assertTrue(isError(err))

t.summary()
