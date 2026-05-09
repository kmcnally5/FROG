import "stdlib/parallel.lex" as p

// PHYSICS ENGINE EXAMPLE - STRUCT OF ARRAYS (SoA) OPTIMIZATION
//
// Data layout: Instead of Array[Struct], use Struct[Array]
// Benefits:
//  - Contiguous memory per field (better cache locality)
//  - Fewer interpreter lookups
//  - More cache-efficient access patterns
//
// Before: particles[i].x, particles[i].y, particles[i].z = 3 lookups
// After:  xs[i], ys[i], zs[i] = direct array access

struct ParticlePool {
    xs
    ys
    zs
    vxs
    vys
    vzs
    masses
    fxs
    fys
    fzs
}

fn init_particle_pool(count) {
    let xs = makeArray(count, 0)
    let ys = makeArray(count, 0)
    let zs = makeArray(count, 0)
    let vxs = makeArray(count, 0)
    let vys = makeArray(count, 0)
    let vzs = makeArray(count, 0)
    let masses = makeArray(count, 0)
    let fxs = makeArray(count, 0)
    let fys = makeArray(count, 0)
    let fzs = makeArray(count, 0)

    for i in range(count) {
        xs[i] = (i * 73) % 1000
        ys[i] = (i * 137) % 1000
        zs[i] = (i * 191) % 1000
        vxs[i] = ((i * 11) % 100) - 50
        vys[i] = ((i * 23) % 100) - 50
        vzs[i] = ((i * 37) % 100) - 50
        masses[i] = ((i % 10) + 1)
        fxs[i] = 0
        fys[i] = 0
        fzs[i] = 0
    }

    return ParticlePool {
        xs: xs,
        ys: ys,
        zs: zs,
        vxs: vxs,
        vys: vys,
        vzs: vzs,
        masses: masses,
        fxs: fxs,
        fys: fys,
        fzs: fzs
    }
}

fn simulate_chunk(start_idx, end_idx, pool, gx, gy, gz, friction, dt) {
    let xs = pool.xs
    let ys = pool.ys
    let zs = pool.zs
    let vxs = pool.vxs
    let vys = pool.vys
    let vzs = pool.vzs
    let masses = pool.masses
    let fxs = pool.fxs
    let fys = pool.fys
    let fzs = pool.fzs

    let i = start_idx
    let count = 0

    // Process 4 particles per iteration (loop unrolling)
    while i + 3 < end_idx {
        // ========== PARTICLE 1 ==========
        let m1 = masses[i]

        // Apply forces
        fxs[i] = fxs[i] + (gx * m1)
        fys[i] = fys[i] + (gy * m1)
        fzs[i] = fzs[i] + (gz * m1)
        vxs[i] = vxs[i] * (1.0 - friction)
        vys[i] = vys[i] * (1.0 - friction)
        vzs[i] = vzs[i] * (1.0 - friction)

        // Update particle
        let ax1 = fxs[i] / m1
        let ay1 = fys[i] / m1
        let az1 = fzs[i] / m1
        vxs[i] = vxs[i] + (ax1 * dt)
        vys[i] = vys[i] + (ay1 * dt)
        vzs[i] = vzs[i] + (az1 * dt)
        xs[i] = xs[i] + (vxs[i] * dt)
        ys[i] = ys[i] + (vys[i] * dt)
        zs[i] = zs[i] + (vzs[i] * dt)
        fxs[i] = 0
        fys[i] = 0
        fzs[i] = 0

        // ========== PARTICLE 2 ==========
        let m2 = masses[i + 1]

        fxs[i + 1] = fxs[i + 1] + (gx * m2)
        fys[i + 1] = fys[i + 1] + (gy * m2)
        fzs[i + 1] = fzs[i + 1] + (gz * m2)
        vxs[i + 1] = vxs[i + 1] * (1.0 - friction)
        vys[i + 1] = vys[i + 1] * (1.0 - friction)
        vzs[i + 1] = vzs[i + 1] * (1.0 - friction)

        let ax2 = fxs[i + 1] / m2
        let ay2 = fys[i + 1] / m2
        let az2 = fzs[i + 1] / m2
        vxs[i + 1] = vxs[i + 1] + (ax2 * dt)
        vys[i + 1] = vys[i + 1] + (ay2 * dt)
        vzs[i + 1] = vzs[i + 1] + (az2 * dt)
        xs[i + 1] = xs[i + 1] + (vxs[i + 1] * dt)
        ys[i + 1] = ys[i + 1] + (vys[i + 1] * dt)
        zs[i + 1] = zs[i + 1] + (vzs[i + 1] * dt)
        fxs[i + 1] = 0
        fys[i + 1] = 0
        fzs[i + 1] = 0

        // ========== PARTICLE 3 ==========
        let m3 = masses[i + 2]

        fxs[i + 2] = fxs[i + 2] + (gx * m3)
        fys[i + 2] = fys[i + 2] + (gy * m3)
        fzs[i + 2] = fzs[i + 2] + (gz * m3)
        vxs[i + 2] = vxs[i + 2] * (1.0 - friction)
        vys[i + 2] = vys[i + 2] * (1.0 - friction)
        vzs[i + 2] = vzs[i + 2] * (1.0 - friction)

        let ax3 = fxs[i + 2] / m3
        let ay3 = fys[i + 2] / m3
        let az3 = fzs[i + 2] / m3
        vxs[i + 2] = vxs[i + 2] + (ax3 * dt)
        vys[i + 2] = vys[i + 2] + (ay3 * dt)
        vzs[i + 2] = vzs[i + 2] + (az3 * dt)
        xs[i + 2] = xs[i + 2] + (vxs[i + 2] * dt)
        ys[i + 2] = ys[i + 2] + (vys[i + 2] * dt)
        zs[i + 2] = zs[i + 2] + (vzs[i + 2] * dt)
        fxs[i + 2] = 0
        fys[i + 2] = 0
        fzs[i + 2] = 0

        // ========== PARTICLE 4 ==========
        let m4 = masses[i + 3]

        fxs[i + 3] = fxs[i + 3] + (gx * m4)
        fys[i + 3] = fys[i + 3] + (gy * m4)
        fzs[i + 3] = fzs[i + 3] + (gz * m4)
        vxs[i + 3] = vxs[i + 3] * (1.0 - friction)
        vys[i + 3] = vys[i + 3] * (1.0 - friction)
        vzs[i + 3] = vzs[i + 3] * (1.0 - friction)

        let ax4 = fxs[i + 3] / m4
        let ay4 = fys[i + 3] / m4
        let az4 = fzs[i + 3] / m4
        vxs[i + 3] = vxs[i + 3] + (ax4 * dt)
        vys[i + 3] = vys[i + 3] + (ay4 * dt)
        vzs[i + 3] = vzs[i + 3] + (az4 * dt)
        xs[i + 3] = xs[i + 3] + (vxs[i + 3] * dt)
        ys[i + 3] = ys[i + 3] + (vys[i + 3] * dt)
        zs[i + 3] = zs[i + 3] + (vzs[i + 3] * dt)
        fxs[i + 3] = 0
        fys[i + 3] = 0
        fzs[i + 3] = 0

        i = i + 4
        count = count + 4
    }

    // Handle remainder
    while i < end_idx {
        let m = masses[i]

        fxs[i] = fxs[i] + (gx * m)
        fys[i] = fys[i] + (gy * m)
        fzs[i] = fzs[i] + (gz * m)
        vxs[i] = vxs[i] * (1.0 - friction)
        vys[i] = vys[i] * (1.0 - friction)
        vzs[i] = vzs[i] * (1.0 - friction)

        let ax = fxs[i] / m
        let ay = fys[i] / m
        let az = fzs[i] / m
        vxs[i] = vxs[i] + (ax * dt)
        vys[i] = vys[i] + (ay * dt)
        vzs[i] = vzs[i] + (az * dt)
        xs[i] = xs[i] + (vxs[i] * dt)
        ys[i] = ys[i] + (vys[i] * dt)
        zs[i] = zs[i] + (vzs[i] * dt)
        fxs[i] = 0
        fys[i] = 0
        fzs[i] = 0

        i = i + 1
        count = count + 1
    }

    return count
}

fn simulate_step(pool, gx, gy, gz, friction, dt, num_workers) {
    let n = len(pool.xs)
    let chunk_size = n / num_workers
    let tasks = makeArray(num_workers, null)

    for w in range(num_workers) {
        let start_idx = w * chunk_size
        let end_idx = (w + 1) * chunk_size
        if w == num_workers - 1 { end_idx = n }

        let task = async(fn() {
            return simulate_chunk(start_idx, end_idx, pool, gx, gy, gz, friction, dt)
        })
        tasks[w] = task
    }

    let total_updated = 0
    for i in range(num_workers) {
        let count = await(tasks[i])
        total_updated = total_updated + count
    }
    return total_updated
}

fn main() {
    println("============================================================")
    println("BULLFROG PHYSICS ENGINE - SoA OPTIMIZED")
    println("============================================================")
    println("Initializing 1M particle pool (Struct of Arrays)...")

    let pool = init_particle_pool(1000000)
    println("✓ Ready")
    println("")
    println("Running 10 simulation steps (8 workers)...")
    println("")

    let gx = 0
    let gy = -9.81
    let gz = 0
    let friction = 0.01
    let dt = 0.016
    let num_workers = 8
    let total_updates = 0

    for step in range(10) {
        let updates = simulate_step(pool, gx, gy, gz, friction, dt, num_workers)
        total_updates = total_updates + updates
        println("Step", step, "- Updated", updates, "particles")
    }

    println("")
    println("============================================================")
    println("RESULTS")
    println("============================================================")
    println("Total particle updates:", total_updates)
    println("Throughput: ~", total_updates / 3, "particles/sec")
    println("")
    println("=== BENCHMARK COMPLETE ===")
}

main()
