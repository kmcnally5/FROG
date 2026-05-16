// >_ FROGSPAWN — Parallel Particle Engine
//
// Demonstrates FROG's raw parallel speed applied to graphics.
// Physics runs across async worker goroutines each frame.
// ALL particles rendered in a SINGLE GPU draw call via drawParticles().
//
// Run: KLEX_PATH=. ./klex tests/examples/frogspawn.lex

import "stdlib/ui.lex" as ui

// ── Particle pool (SoA) ───────────────────────────────────────────────────────

const MAX_PARTICLES = 60000
const NUM_WORKERS   = 10
const DT            = 0.016   // target 60fps timestep
const GRAVITY       = 280.0   // pixels/sec²
const FRICTION      = 0.012

xs        = makeArray(MAX_PARTICLES, 0.0)
ys        = makeArray(MAX_PARTICLES, 0.0)
vxs       = makeArray(MAX_PARTICLES, 0.0)
vys       = makeArray(MAX_PARTICLES, 0.0)
ages      = makeArray(MAX_PARTICLES, 999.0)  // start dead
lifetimes = makeArray(MAX_PARTICLES, 1.0)
rs        = makeArray(MAX_PARTICLES, 1.0)
gs        = makeArray(MAX_PARTICLES, 1.0)
bs        = makeArray(MAX_PARTICLES, 1.0)
alphas    = makeArray(MAX_PARTICLES, 0.0)

nextSlot    = 0
activeCount = 0

// ── Emit a single particle ────────────────────────────────────────────────────

fn emit(x, y, vx, vy, lifetime, r, g, b) {
    idx = nextSlot
    xs[idx]        = x
    ys[idx]        = y
    vxs[idx]       = vx
    vys[idx]       = vy
    ages[idx]      = 0.0
    lifetimes[idx] = lifetime
    rs[idx]        = r
    gs[idx]        = g
    bs[idx]        = b
    alphas[idx]    = 1.0
    nextSlot = mod(nextSlot + 1, MAX_PARTICLES)
}

// ── Parallel physics step ─────────────────────────────────────────────────────
// Spawns NUM_WORKERS async goroutines — saturates all cores.

fn stepPhysics(windX) {
    chunk = MAX_PARTICLES / NUM_WORKERS
    tasks = makeArray(NUM_WORKERS, null)

    for w in range(0, NUM_WORKERS) {
        let start = w * chunk
        let end   = start + chunk
        if w == NUM_WORKERS - 1 { end = MAX_PARTICLES }

        let task = async(fn() {
            i = start
            while i < end {
                age = ages[i]
                lt  = lifetimes[i]
                if age < lt {
                    vys[i]    = vys[i] + GRAVITY * DT
                    vxs[i]    = vxs[i] + windX * DT
                    vxs[i]    = vxs[i] * (1.0 - FRICTION)
                    vys[i]    = vys[i] * (1.0 - FRICTION)
                    xs[i]     = xs[i] + vxs[i] * DT
                    ys[i]     = ys[i] + vys[i] * DT
                    ages[i]   = age + DT
                    alphas[i] = 1.0 - (age / lt)
                } else {
                    alphas[i] = 0.0
                }
                i = i + 1
            }
        })
        tasks[w] = task
    }

    for w in range(0, NUM_WORKERS) {
        await(tasks[w])
    }
}

// ── Emitter functions ─────────────────────────────────────────────────────────

fn emitFountain(cx, cy, n) {
    for i in range(0, n) {
        angle = (rand() - 0.5) * 1.2        // ±0.6 rad spread
        speed = 180.0 + rand() * 220.0
        vx    = sin(angle) * speed
        vy    = -cos(angle) * speed          // up = negative y
        life  = 1.2 + rand() * 1.4
        // Cyan/teal/white palette
        t     = rand()
        emit(cx + (rand()-0.5)*8.0, cy, vx, vy, life,
             0.2 + t*0.6, 0.7 + t*0.3, 1.0)
    }
}

fn emitExplosion(cx, cy, n) {
    for i in range(0, n) {
        angle = rand() * 6.2832
        speed = 60.0 + rand() * 340.0
        vx    = cos(angle) * speed
        vy    = sin(angle) * speed
        life  = 0.4 + rand() * 0.9
        // Fire palette: orange/yellow/white
        t     = rand()
        emit(cx, cy, vx, vy, life,
             1.0, 0.3 + t*0.6, t*0.3)
    }
}

fn emitRain(ww, n) {
    for i in range(0, n) {
        x     = rand() * ww
        speed = 200.0 + rand() * 150.0
        life  = 1.5 + rand() * 1.0
        t     = rand() * 0.5
        emit(x, -5.0, (rand()-0.5)*15.0, speed, life,
             0.3 + t, 0.5 + t, 0.9 + t*0.1)
    }
}

fn emitSparks(cx, cy, n) {
    for i in range(0, n) {
        angle = rand() * 6.2832
        speed = 30.0 + rand() * 200.0
        life  = 0.3 + rand() * 0.6
        t     = rand()
        emit(cx, cy, cos(angle)*speed, sin(angle)*speed, life,
             0.8 + t*0.2, 0.6 + t*0.3, 0.1)
    }
}

// ── App state ─────────────────────────────────────────────────────────────────

windX     = 0.0
showInfo  = true

window(1100, 740, ">_ FROGSPAWN — Parallel Particle Engine", fn(frame) {
    ww = float(winWidth())
    wh = float(winHeight())

    // Clear — deep space black
    fill(0.00, 0.00, 0.02, 1.0)
    noStroke()
    rect(0.0, 0.0, ww, wh)

    // ── Physics — parallel on all cores ──────────────────────────────────────
    windX = sin(elapsedTime() * 0.4) * 35.0
    stepPhysics(windX)

    // ── Emit particles ────────────────────────────────────────────────────────

    // Fountain — centre bottom
    emitFountain(ww * 0.5, wh - 10.0, 80)

    // Rain — top of screen
    emitRain(ww, 20)

    // Sparks — trailing mouse movement
    if mouseDown() {
        emitExplosion(mouseX(), mouseY(), 60)
    } else {
        emitSparks(mouseX(), mouseY(), 6)
    }

    // ── Render — ALL particles in ONE draw call ───────────────────────────────
    drawParticles(xs, ys, rs, gs, bs, alphas, MAX_PARTICLES, 3.0)

    // ── UI overlay ───────────────────────────────────────────────────────────
    if showInfo {
        panW = 280.0
        panH = 180.0
        ui.panelTitle(ww - panW - 10.0, 10.0, panW, panH, "FROGSPAWN")
        ui.beginLayout(ww - panW + 2.0, 50.0, panW - 24.0, 6.0)

        ui.labelColored(ww - panW + 2.0, ui.layoutY(),
            "Particles: " + str(MAX_PARTICLES), ui.UI_ACCENT)
        ui.advanceCursor(18.0)
        ui.labelColored(ww - panW + 2.0, ui.layoutY(),
            "Workers:   " + str(NUM_WORKERS) + " goroutines", ui.UI_ACCENT)
        ui.advanceCursor(18.0)
        ui.labelColored(ww - panW + 2.0, ui.layoutY(),
            "Draw calls: 1  (batched)", ui.UI_ACCENT)
        ui.advanceCursor(18.0)
        ui.layoutSeparator()
        ui.layoutLabelDim("Hold mouse  — explosion")
        ui.layoutLabelDim("Move mouse  — sparks")
        ui.layoutLabelDim("H           — hide UI")
    }

    if keyPressed("H") {
        showInfo = !showInfo
    }
})
