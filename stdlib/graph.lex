// stdlib/graph.lex — reactive computation graph
//
// Graph is an instantiable dependency graph. Multiple independent graphs
// can coexist in the same program.
//
// Usage:
//   import "graph.lex" as gmod
//   g = gmod.newGraph()
//   g.node("a", fn() { return 10 })
//   g.node("b", fn() { return 20 })
//   g.node("sum", fn() { return g.compute("a") + g.compute("b") })
//   g.depends("sum", "a")
//   g.depends("sum", "b")
//   println(g.compute("sum"))   // 30

struct Graph {
    nodes
    values
    deps
    reverse

    // node(name, fnRef) — declare a computed or constant node.
    fn node(name, fnRef) {
        self.nodes[name] = fnRef
        self.deps[name] = []
        self.values[name] = null
        return null
    }

    // depends(target, source) — declare that target depends on source.
    fn depends(target, source) {
        if !hasKey(self.reverse, source) {
            self.reverse[source] = []
        }
        self.reverse[source] = push(self.reverse[source], target)
        self.deps[target] = push(self.deps[target], source)
        return null
    }

    // compute(name) — evaluate a node, using cached value if available.
    // Returns (null, message) if the node is unknown.
    fn compute(name) {
        if !hasKey(self.nodes, name) {
            return null, "unknown node: " + name
        }
        if hasKey(self.values, name) && self.values[name] != null {
            return self.values[name]
        }
        fnRef = self.nodes[name]
        result = fnRef()
        self.values[name] = result
        return result
    }

    // set(name, value) — assign a constant value and propagate to dependents.
    fn set(name, value) {
        self.nodes[name] = fn() { return value }
        self.values[name] = value
        self.invalidate(name)
        self.propagate(name)
        return null
    }

    // invalidate(name) — clear the cached value for a node.
    fn invalidate(name) {
        self.values[name] = null
        return null
    }

    // propagate(name) — recompute all transitive dependents of name.
    fn propagate(name) {
        if !hasKey(self.reverse, name) {
            return null
        }
        dependents = self.reverse[name]
        i = 0
        while i < len(dependents) {
            d = dependents[i]
            self.invalidate(d)
            self.compute(d)
            self.propagate(d)
            i = i + 1
        }
        return null
    }

    // recomputeAll() — force recomputation of every node.
    fn recomputeAll() {
        ks = keys(self.nodes)
        i = 0
        while i < len(ks) {
            self.compute(ks[i])
            i = i + 1
        }
        return null
    }

    // debug() — print the full graph state to stdout.
    fn debug() {
        println("=== GRAPH STATE ===")
        g = self
        ks = keys(self.nodes)
        i = 0
        while i < len(ks) {
            k = ks[i]
            println(k + " = " + str(g.compute(k)) + " | deps=" + str(g.deps[k]))
            i = i + 1
        }
        return null
    }
}

// newGraph() — returns a fresh empty Graph.
fn newGraph() {
    return Graph { nodes: {}, values: {}, deps: {}, reverse: {} }
}
