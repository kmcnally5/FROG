// ============================================================
// PURE AST / PRATT STRESS TEST (NO ? :)
// ============================================================


// ------------------------------------------------------------
// deep expression tree (Pratt parser stress)
// ------------------------------------------------------------
fn deep_expr(n) {
    if n <= 0 {
        return 1
    }

    return (deep_expr(n - 1) + n) * (1 + (n % 3))
}


// ------------------------------------------------------------
// recursive call chain (call + env stress)
// ------------------------------------------------------------
fn chain_a(n) {
    if n <= 0 { return 1 }
    return chain_b(n - 1) + deep_expr(n % 5)
}

fn chain_b(n) {
    if n <= 0 { return 2 }
    return chain_c(n - 1) * 2 + chain_a(n % 4)
}

fn chain_c(n) {
    if n <= 0 { return 3 }
    return chain_a(n - 1) + chain_b(n - 2)
}


// ------------------------------------------------------------
// accumulator closure simulation
// ------------------------------------------------------------
fn make_accumulator(seed) {
    let state = seed

    return fn(x) {
        state = state + x * (state % 5 + 1)
        return state
    }
}


// ------------------------------------------------------------
// worker (heavy AST + branch + recursion mix)
// ------------------------------------------------------------
fn worker(id, n) {
    let acc = make_accumulator(id)
    let total = 0

    for i in range(0, n) {

        let v =
            deep_expr(i % 6)
            + chain_a(i % 5)
            * chain_b(i % 3)
            - chain_c(i % 4)

        // explicit branching only (no ternary)
        if (i % 7 == 0) {
            v = v + deep_expr(5)
        }

        if (i % 11 == 0) {
            v = v - deep_expr(3)
        }

        if (v % 2 == 0) {
            total = total + acc(v % 11)
        }

        if (v % 2 != 0) {
            total = total - acc(v % 9)
        }

        if (v % 5 == 0) {
            total = total + chain_a(4)
        }

        if (v % 7 == 0) {
            total = total + chain_b(3)
        }
    }

    return total
}


// ------------------------------------------------------------
// runner (same structure as your benchmark style)
// ------------------------------------------------------------
fn run(total, threads) {
    let chunk = total / threads
    let tasks = []

    println("--- AST STRESS TEST FIXED ---")
    println("Items:", total)
    println("Threads:", threads)

    for i in range(threads) {
        let t = async(worker, i, chunk)
        tasks = push(tasks, t)
    }

    let result = 0

    for t in tasks {
        result = result + await(t)
    }

    println("--- DONE ---")
    println("Checksum:", result)

    return result
}


// ------------------------------------------------------------
// EXECUTION
// ------------------------------------------------------------
let items = 2000000
let threads = 10

run(items, threads)

println("=== END ===")
