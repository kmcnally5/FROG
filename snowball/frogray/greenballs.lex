// ============================================================================
// FROGSCENE — A FROG RENDERED IN FROGRAY RAY TRACING
// ============================================================================
// A cute frog modeled with geometric primitives and rendered with full
// ray tracing: shadows, reflections, and Phong shading.

import "frogray.lex" as frogray

// Create materials
mat_head = frogray.create_material(0.2, 0.7, 0.3, 0.1, 64)      // Green head
mat_body = frogray.create_material(0.15, 0.65, 0.25, 0.15, 64)  // Darker green body
mat_eye = frogray.create_material(0.9, 0.9, 0.95, 0.3, 128)      // Shiny white eyes
mat_pupil = frogray.create_material(0.1, 0.1, 0.1, 0.2, 64)      // Black pupils
mat_belly = frogray.create_material(0.5, 0.85, 0.4, 0.05, 32)    // Lighter belly
mat_leg = frogray.create_material(0.18, 0.62, 0.28, 0.1, 48)     // Leg green
mat_ground = frogray.create_material(0.4, 0.5, 0.3, 0.0, 16)     // Ground (no reflection)
mat_sky = frogray.create_material(0.5, 0.7, 1.0, 0.2, 8)         // Sky (mild reflection)

// Build the frog
objects = [
    // ===== HEAD =====
    frogray.create_sphere(0, 1.5, 0, 0.6, mat_head),

    // ===== LEFT EYE =====
    frogray.create_sphere(-0.35, 2.1, -0.35, 0.2, mat_eye),
    frogray.create_sphere(-0.35, 2.1, -0.35, 0.12, mat_pupil),

    // ===== RIGHT EYE =====
    frogray.create_sphere(0.35, 2.1, -0.35, 0.2, mat_eye),
    frogray.create_sphere(0.35, 2.1, -0.35, 0.12, mat_pupil),

    // ===== MOUTH (two small spheres for cute expression) =====
    frogray.create_sphere(-0.15, 1.15, -0.55, 0.1, mat_belly),
    frogray.create_sphere(0.15, 1.15, -0.55, 0.1, mat_belly),

    // ===== BODY =====
    frogray.create_sphere(0, 0.5, 0, 0.75, mat_body),

    // ===== BELLY (lighter colored, slightly overlapping) =====
    frogray.create_sphere(0, 0.4, -0.4, 0.6, mat_belly),

    // ===== FRONT LEFT LEG =====
    frogray.create_sphere(-0.7, -0.1, -0.4, 0.25, mat_leg),

    // ===== FRONT RIGHT LEG =====
    frogray.create_sphere(0.7, -0.1, -0.4, 0.25, mat_leg),

    // ===== BACK LEFT LEG =====
    frogray.create_sphere(-0.8, -0.2, 0.5, 0.3, mat_leg),

    // ===== BACK RIGHT LEG =====
    frogray.create_sphere(0.8, -0.2, 0.5, 0.3, mat_leg),

    // ===== GROUND PLANE =====
    frogray.create_plane(0, -0.5, 0, 0, 1, 0, mat_ground),

    // ===== BACK WALL (subtle sky reflection) =====
    frogray.create_plane(0, 0, 3, 0, 0, -1, mat_sky)
]

// Lights
lights = [
    // Main key light (warm, from upper left front)
    frogray.create_light(2, 2.5, -1, 1.0, 0.95, 0.85, 1.2),

    // Fill light (cool, from upper right back)
    frogray.create_light(-1.5, 1.8, 2, 0.7, 0.8, 1.0, 0.6),

    // Rim light (from behind, adds depth)
    frogray.create_light(0, 2, 3, 1.0, 0.9, 0.8, 0.5)
]

// Camera: positioned to see the frog sitting, slightly above and front
// Position: where the camera is (eye position)
// Target: where the camera is looking (target point)
// FOV: field of view (45 degrees is standard)
camera = [
    0.5, 0.8, -1.8,    // position (x, y, z)
    0, 0.7, 0,         // target (x, y, z)
    45                 // FOV in degrees
]

println("🐸 Building the frog scene...")
println("   Head, eyes, body, legs, and environment ready!")
println("")

// Render settings
width = 1024
height = 768
num_workers = 10

println("🎨 Rendering frog with frogray ray tracing engine...")
println("   Resolution: " + str(width) + "x" + str(height))
println("   Workers: " + str(num_workers))
println("   Features: shadows, reflections, Phong shading, MSAA")
println("")

// Render the scene
pixels = frogray.render_parallel(width, height, camera, objects, lights, num_workers)

println("✓ Rendering complete!")
println("")

// Write to file
output_file = "frog_scene.ppm"
println("💾 Writing to " + output_file)
frogray.write_ppm(output_file, width, height, pixels)

println("✓ Done! Open " + output_file + " in an image viewer to see the frog.")
println("")
println("The frog was rendered using FROGRAY:")
println("  - BVH acceleration for fast ray intersection")
println("  - 4-sample MSAA for smooth edges")
println("  - Shadow rays for realistic lighting")
println("  - Reflection rays for material realism")
println("  - Parallel tile rendering across " + str(num_workers) + " workers")
