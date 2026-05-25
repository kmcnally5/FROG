// stdlib/graph.lex — reactive computation graph
// @module    graph
// @version   1.0.0
// @since     klex 0.3.35
// @author    karl
// @summary   reactive computation graph
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
//
// CACHE SEMANTICS
//   Each node's computed result is cached until invalidated. The cache is
//   tracked by a separate `computed` flag rather than by the value itself,
//   so a node that legitimately returns null is cached correctly and not
//   recomputed on every read.
//
// PROPAGATION SEMANTICS
//   propagate(name) invalidates every transitive dependent ONCE (visited
//   set), then recomputes each once. Diamond dependencies do not double
//   compute, and cycles in `reverse` do not cause infinite recursion.

struct Graph {
    nodes
    values
    deps
    reverse
    computed

    // node(name, fnRef) — declare a computed or constant node.
    fn node(name, fnRef) {
        self.nodes[name] = fnRef
        self.deps[name] = []
        self.values[name] = null
        if hasKey(self.computed, name) {
            delete(self.computed, name)
        }
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
        if hasKey(self.computed, name) {
            return self.values[name]
        }
        let fnRef = self.nodes[name]
        let result = fnRef()
        self.values[name] = result
        self.computed[name] = true
        return result
    }

    // set(name, value) — assign a constant value and propagate to dependents.
    // The new value IS the cached value; no recompute is needed for `name` itself.
    fn set(name, value) {
        self.nodes[name] = fn() { return value }
        self.values[name] = value
        self.computed[name] = true
        self.propagate(name)
        return null
    }

    // invalidate(name) — clear the cached value for a node so the next
    // compute() will re-run its function. Idempotent; safe to call on an
    // already-invalidated node.
    fn invalidate(name) {
        if hasKey(self.computed, name) {
            delete(self.computed, name)
        }
        return null
    }

    // propagate(name) — recompute all transitive dependents of name exactly
    // once each. Uses a visited set so diamonds are deduplicated and cycles
    // in `reverse` terminate cleanly.
    fn propagate(name) {
        if !hasKey(self.reverse, name) {
            return null
        }
        let visited = {}
        self._collectDependents(name, visited)
        let ks = keys(visited)
        let i = 0
        while i < len(ks) {
            self.compute(ks[i])
            i = i + 1
        }
        return null
    }

    // _collectDependents — DFS over the reverse adjacency list. For each
    // unvisited dependent: mark visited, invalidate cache, recurse. This
    // both collects the set to recompute and clears stale cached values
    // before recomputation begins.
    fn _collectDependents(name, visited) {
        if !hasKey(self.reverse, name) {
            return null
        }
        let dependents = self.reverse[name]
        let i = 0
        while i < len(dependents) {
            let d = dependents[i]
            if !hasKey(visited, d) {
                visited[d] = true
                self.invalidate(d)
                self._collectDependents(d, visited)
            }
            i = i + 1
        }
        return null
    }

    // recomputeAll() — force recomputation of every node.
    fn recomputeAll() {
        let ks = keys(self.nodes)
        let i = 0
        while i < len(ks) {
            self.invalidate(ks[i])
            i = i + 1
        }
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
        let g = self
        let ks = keys(self.nodes)
        let i = 0
        while i < len(ks) {
            let k = ks[i]
            println(k + " = " + str(g.compute(k)) + " | deps=" + str(g.deps[k]))
            i = i + 1
        }
        return null
    }
}

// newGraph() — returns a fresh empty Graph.
fn newGraph() {
    return Graph { nodes: {}, values: {}, deps: {}, reverse: {}, computed: {} }
}
