// moduleDedupTest.lex — proves that two imports of the same file share
// module-level state. Prior to the module cache, each `import` rebuilt
// the module from scratch, silently giving every importer its own copy
// of the file's globals. That broke any library trying to act as a
// process-wide ledger / registry / pool (notably the AI cost ledger in
// stdlib/ai/ai_common.lex).
//
// We import the same library twice under two different aliases. If the
// dedup works, mutations through alias `a` are visible through alias
// `b` and vice versa.

import "stdlib/ai/ai_common.lex" as a
import "stdlib/ai/ai_common.lex" as b


fn assertEq(label, got, want) {
    if got != want {
        println("FAIL " + label + ": got " + str(got) + " want " + str(want))
        _osExit(1)
    }
}


// Start from a clean slate via one alias.
a.resetUsage()
a.budget(null)

// Recording through `a` should be visible through `b`.
a._record("test-model", 1000, 500, 3.0, 15.0)
assertEq("b sees spend recorded via a", b.spent(), a.spent())
assertEq("b agrees a saw the call",     len(keys(b.usage())), 1)

// Setting a budget through `b` should constrain a future call checked
// via `a` — exactly the cross-module coordination the dedup unlocks.
b.budget(0.0001)
assertEq("a sees budget set via b", a.budgetExceeded(), true)

// And clearing through `a` is visible to `b`.
a.resetUsage()
a.budget(null)
assertEq("b sees reset via a", b.spent(), 0.0)
assertEq("b sees cap cleared via a", b.budgetExceeded(), false)


println("PASS moduleDedupTest")
