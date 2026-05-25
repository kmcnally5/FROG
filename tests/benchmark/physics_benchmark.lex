import "parallel.lex" as p

// Quick benchmark: 1M particles × 10 steps = 10M updates
// Should complete in ~2-3 seconds and show real throughput

struct Particle {
    x
    y
    z
    vx
    vy
    vz
    mass
    fx
    fy
    fz
}

fn create_particle(x, y, z, vx, vy, vz, mass) {
    return Particle {
        x: x, y: y, z: z, vx: vx, vy: vy, vz: vz, mass: mass, fx: 0, fy: 0, fz: 0
    }
}

fn init_particle_pool(count) {
    let particles = makeArray(count, null)
    for i in range(count) {
        particles[i] = create_particle(
            (i * 73) % 1000,
            (i * 137) % 1000,
            (i * 191) % 1000,
            ((i * 11) % 100) - 50,
            ((i * 23) % 100) - 50,
            ((i * 37) % 100) - 50,
            ((i % 10) + 1)
        )
    }
    return particles
}

fn apply_forces(p, gx, gy, gz, friction) {
    p.fx = p.fx + (gx * p.mass)
    p.fy = p.fy + (gy * p.mass)
    p.fz = p.fz + (gz * p.mass)
    p.vx = p.vx * (1.0 - friction)
    p.vy = p.vy * (1.0 - friction)
    p.vz = p.vz * (1.0 - friction)
    return p
}

fn update_particle(p, dt) {
    let ax = p.fx / p.mass
    let ay = p.fy / p.mass
    let az = p.fz / p.mass
    p.vx = p.vx + (ax * dt)
    p.vy = p.vy + (ay * dt)
    p.vz = p.vz + (az * dt)
    p.x = p.x + (p.vx * dt)
    p.y = p.y + (p.vy * dt)
    p.z = p.z + (p.vz * dt)
    p.fx = 0
    p.fy = 0
    p.fz = 0
    return p
}

fn simulate_chunk(start_idx, end_idx, particles, gx, gy, gz, friction, dt) {
    let i = start_idx
    let count = 0

    // Process 4 particles per iteration (loop unrolling)
    while i + 3 < end_idx {
        // Particle 1
        let p1 = particles[i]
        p1 = apply_forces(p1, gx, gy, gz, friction)
        p1 = update_particle(p1, dt)
        particles[i] = p1

        // Particle 2
        let p2 = particles[i + 1]
        p2 = apply_forces(p2, gx, gy, gz, friction)
        p2 = update_particle(p2, dt)
        particles[i + 1] = p2

        // Particle 3
        let p3 = particles[i + 2]
        p3 = apply_forces(p3, gx, gy, gz, friction)
        p3 = update_particle(p3, dt)
        particles[i + 2] = p3

        // Particle 4
        let p4 = particles[i + 3]
        p4 = apply_forces(p4, gx, gy, gz, friction)
        p4 = update_particle(p4, dt)
        particles[i + 3] = p4

        i = i + 4
        count = count + 4
    }

    // Handle remainder (up to 3 particles)
    while i < end_idx {
        let p = particles[i]
        p = apply_forces(p, gx, gy, gz, friction)
        p = update_particle(p, dt)
        particles[i] = p
        i = i + 1
        count = count + 1
    }

    return count
}

fn simulate_step(particles, gx, gy, gz, friction, dt, num_workers) {
    let n = len(particles)
    let chunk_size = n / num_workers
    let tasks = makeArray(num_workers, null)

    for w in range(num_workers) {
        let start_idx = w * chunk_size
        let end_idx = (w + 1) * chunk_size
        if w == num_workers - 1 { end_idx = n }

        let task = async(fn() {
            return simulate_chunk(start_idx, end_idx, particles, gx, gy, gz, friction, dt)
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
    println("PHYSICS ENGINE - QUICK BENCHMARK")
    println("============================================================")
    println("Initializing 1M particle pool...")

    let particles = init_particle_pool(1000000)
    println("✓ Ready")
    println("")
    println("Running 10 simulation steps (8 workers)...")
    println("")

    let gravity_x = 0
    let gravity_y = -9.81
    let gravity_z = 0
    let friction = 0.01
    let dt = 0.016
    let num_workers = 8
    let total_updates = 0

    for step in range(10) {
        let updates = simulate_step(particles, gravity_x, gravity_y, gravity_z, friction, dt, num_workers)
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
