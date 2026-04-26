import "graph.lex" as gmod

g = gmod.newGraph()

g.node("a", fn() { return 40 })
g.node("b", fn() { return 60 })

g.node("sum", fn() {
    return g.compute("a") + g.compute("b")
})

x = g.depends("sum", "a")
y = g.depends("sum", "b")
println(x)
println(y)

println(g.compute("sum"))   // 100
