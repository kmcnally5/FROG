import "frogray.lex" as fg

// ============================================================
// CONFIGURATION — Change these values easily
// ============================================================

let CONFIG_WIDTH = 1024
let CONFIG_HEIGHT = 768
let CONFIG_WORKERS = 10
let CONFIG_OUTPUT = "render_output.ppm"

// ============================================================
// SCENE SETUP
// ============================================================

fn main() {
    println("============================================================")
    println("FROGRAY RENDER")
    println("============================================================")
    println("")

    println("Configuration:")
    println("  Resolution: " + str(CONFIG_WIDTH) + "x" + str(CONFIG_HEIGHT))
    println("  Workers: " + str(CONFIG_WORKERS))
    println("  Output: " + CONFIG_OUTPUT)
    println("")

    let red_mat = fg.create_material(1, 0.2, 0.2, 0.4, 32)
    let green_mat = fg.create_material(0.2, 1, 0.2, 0.3, 64)
    let blue_mat = fg.create_material(0.2, 0.2, 1, 0.5, 16)
    let yellow_mat = fg.create_material(1, 1, 0.2, 0.2, 128)
    let cyan_mat = fg.create_material(0.2, 1, 1, 0.6, 8)
    let white_shiny = fg.create_material(1, 1, 1, 0.8, 256)
    let gray_mat = fg.create_material(0.5, 0.5, 0.5, 0.05, 256)
    let dark_mat = fg.create_material(0.2, 0.2, 0.2, 0.1, 64)

    let objects = [
        fg.create_sphere(0, 0.5, 8, 1.2, red_mat),
        fg.create_sphere(3.5, 1.2, 10, 0.8, green_mat),
        fg.create_sphere(-3.5, 0.8, 9, 0.9, blue_mat),
        fg.create_sphere(0, -2, 12, 1.5, yellow_mat),
        fg.create_sphere(2, 2, 6, 0.6, cyan_mat),
        fg.create_sphere(-2.5, 1.5, 7, 0.7, white_shiny),
        fg.create_sphere(4, -1, 11, 0.5, dark_mat),
        fg.create_plane(0, -2, 0, 0, 1, 0, gray_mat)
    ]

    let lights = [
        fg.create_light(5, 5, 3, 1, 1, 1, 1.0),
        fg.create_light(-5, 3, 5, 0.7, 0.7, 1, 0.6)
    ]

    let camera = [0, 1, -5, 0, 0, 8, 60]

    println("Rendering...")
    let t_start = _timeNanos()
    let pixels = fg.render_parallel(CONFIG_WIDTH, CONFIG_HEIGHT, camera, objects, lights, CONFIG_WORKERS)
    let t_end = _timeNanos()
    let elapsed_ms = (t_end - t_start) / 1000000

    println("Completed in " + str(elapsed_ms) + "ms")
    println("")

    println("Writing " + CONFIG_OUTPUT + "...")
    fg.write_ppm(CONFIG_OUTPUT, CONFIG_WIDTH, CONFIG_HEIGHT, pixels)
    println("✓ Success")
    println("")

    println("=== RENDER COMPLETE ===")
}

main()
