// Tier 1 — every literal kind reaches the VM and prints identically.
//
// Run via the differential runner:
//   go run ./vm/cmd/vmdiff tests/vm/*.lex
//
// MATCH means the VM's stdout byte-equals the tree-walker's.

println(42)
println(3.14)
println("hello")
println(true)
println(false)
println(null)
