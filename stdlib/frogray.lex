// ============================================================
// FROGRAY — RAY TRACING LIBRARY
// ============================================================
// High-performance ray tracing with reflections, shadows, MSAA.
// Renders to PPM format.

// ============================================================
// VECTOR MATH (Vec3 as [x, y, z])
// ============================================================

fn vec3_add(a, b) {
    return [a[0] + b[0], a[1] + b[1], a[2] + b[2]]
}

fn vec3_sub(a, b) {
    return [a[0] - b[0], a[1] - b[1], a[2] - b[2]]
}

fn vec3_scale(v, s) {
    return [v[0] * s, v[1] * s, v[2] * s]
}

fn vec3_dot(a, b) {
    return a[0] * b[0] + a[1] * b[1] + a[2] * b[2]
}

fn vec3_cross(a, b) {
    return [
        a[1] * b[2] - a[2] * b[1],
        a[2] * b[0] - a[0] * b[2],
        a[0] * b[1] - a[1] * b[0]
    ]
}

fn vec3_length(v) {
    return sqrt(vec3_dot(v, v))
}

fn vec3_normalize(v) {
    let len = vec3_length(v)
    if len < 0.0001 { return [0, 0, 0] }
    return [v[0] / len, v[1] / len, v[2] / len]
}

fn vec3_reflect(v, n) {
    let d = vec3_dot(v, n)
    return vec3_sub(v, vec3_scale(n, 2 * d))
}

fn pow_fast(base, exp) {
    if exp == 0 { return 1 }
    if exp == 1 { return base }
    if exp == 2 { return base * base }
    let result = base
    let i = 1
    while i < exp {
        result = result * base
        i = i + 1
    }
    return result
}

// ============================================================
// MATERIAL & LIGHT & SHAPES
// ============================================================

fn create_material(r, g, b, reflectance, shininess) {
    return [r, g, b, reflectance, shininess]
}

fn create_light(x, y, z, r, g, b, intensity) {
    return [x, y, z, r, g, b, intensity]
}

fn create_sphere(cx, cy, cz, radius, material) {
    return ["sphere", cx, cy, cz, radius, material]
}

fn create_plane(px, py, pz, nx, ny, nz, material) {
    return ["plane", px, py, pz, nx, ny, nz, material]
}

// ============================================================
// BVH ACCELERATION STRUCTURE
// ============================================================

fn compute_aabb(objects, indices) {
    let min_x = 999999
    let min_y = 999999
    let min_z = 999999
    let max_x = -999999
    let max_y = -999999
    let max_z = -999999

    let i = 0
    while i < len(indices) {
        let obj = objects[indices[i]]
        let cx = obj[1]
        let cy = obj[2]
        let cz = obj[3]
        let rad = obj[4]

        if cx - rad < min_x { min_x = cx - rad }
        if cy - rad < min_y { min_y = cy - rad }
        if cz - rad < min_z { min_z = cz - rad }
        if cx + rad > max_x { max_x = cx + rad }
        if cy + rad > max_y { max_y = cy + rad }
        if cz + rad > max_z { max_z = cz + rad }

        i = i + 1
    }

    return [min_x, min_y, min_z, max_x, max_y, max_z]
}

fn build_bvh_recursive(objects, indices, nodes, counter) {
    if len(indices) <= 2 {
        let aabb = compute_aabb(objects, indices)
        let idx = counter[0]
        nodes[idx] = [aabb[0], aabb[1], aabb[2], aabb[3], aabb[4], aabb[5], -1, indices[0], len(indices)]
        counter[0] = counter[0] + 1
        return idx
    }

    let aabb = compute_aabb(objects, indices)

    let extent_x = aabb[3] - aabb[0]
    let extent_y = aabb[4] - aabb[1]
    let extent_z = aabb[5] - aabb[2]

    let split_axis = 0
    if extent_y > extent_x { split_axis = 1 }
    if extent_z > extent_y { split_axis = 2 }

    let split_pos = 0
    if split_axis == 0 { split_pos = (aabb[0] + aabb[3]) / 2 }
    if split_axis == 1 { split_pos = (aabb[1] + aabb[4]) / 2 }
    if split_axis == 2 { split_pos = (aabb[2] + aabb[5]) / 2 }

    let left_indices = makeArray(len(indices), -1)
    let right_indices = makeArray(len(indices), -1)
    let left_count = 0
    let right_count = 0

    let i = 0
    while i < len(indices) {
        let obj = objects[indices[i]]
        let centroid = obj[1]
        if split_axis == 1 { centroid = obj[2] }
        if split_axis == 2 { centroid = obj[3] }

        if centroid < split_pos {
            left_indices[left_count] = indices[i]
            left_count = left_count + 1
        } else {
            right_indices[right_count] = indices[i]
            right_count = right_count + 1
        }
        i = i + 1
    }

    if left_count == 0 { left_count = 1 }
    if right_count == 0 { right_count = len(indices) - 1 }

    let node_idx = counter[0]
    counter[0] = counter[0] + 1
    nodes[node_idx] = [aabb[0], aabb[1], aabb[2], aabb[3], aabb[4], aabb[5], split_axis, 0, 0]

    let left_trimmed = makeArray(left_count, -1)
    let j = 0
    while j < left_count {
        left_trimmed[j] = left_indices[j]
        j = j + 1
    }

    let right_trimmed = makeArray(right_count, -1)
    let j = 0
    while j < right_count {
        right_trimmed[j] = right_indices[j]
        j = j + 1
    }

    let left_child = build_bvh_recursive(objects, left_trimmed, nodes, counter)
    let right_child = build_bvh_recursive(objects, right_trimmed, nodes, counter)

    nodes[node_idx][7] = right_child

    return node_idx
}

fn build_bvh(objects) {
    let indices = makeArray(len(objects), -1)
    let i = 0
    while i < len(objects) {
        indices[i] = i
        i = i + 1
    }

    let nodes = makeArray(len(objects) * 4, null)
    let counter = makeArray(1, 0)
    build_bvh_recursive(objects, indices, nodes, counter)

    let final_count = counter[0]
    let final_nodes = makeArray(final_count, null)
    let i = 0
    while i < final_count {
        final_nodes[i] = nodes[i]
        i = i + 1
    }

    return final_nodes
}

fn intersect_aabb(ro_x, ro_y, ro_z, rd_x, rd_y, rd_z, min_x, min_y, min_z, max_x, max_y, max_z) {
    let dx = 1.0 / (rd_x + 0.0001)
    let dy = 1.0 / (rd_y + 0.0001)
    let dz = 1.0 / (rd_z + 0.0001)

    let t1_x = (min_x - ro_x) * dx
    let t2_x = (max_x - ro_x) * dx
    let t1_y = (min_y - ro_y) * dy
    let t2_y = (max_y - ro_y) * dy
    let t1_z = (min_z - ro_z) * dz
    let t2_z = (max_z - ro_z) * dz

    if t1_x > t2_x {
        let tmp = t1_x
        t1_x = t2_x
        t2_x = tmp
    }
    if t1_y > t2_y {
        let tmp = t1_y
        t1_y = t2_y
        t2_y = tmp
    }
    if t1_z > t2_z {
        let tmp = t1_z
        t1_z = t2_z
        t2_z = tmp
    }

    let t_min = t1_x
    if t1_y > t_min { t_min = t1_y }
    if t1_z > t_min { t_min = t1_z }

    let t_max = t2_x
    if t2_y < t_max { t_max = t2_y }
    if t2_z < t_max { t_max = t2_z }

    if t_min > t_max { return false }
    if t_max < 0.001 { return false }

    return true
}

// ============================================================
// RAY-OBJECT INTERSECTION
// ============================================================

fn ray_sphere_hit(ray_orig, ray_dir, sphere) {
    let cx = sphere[1]
    let cy = sphere[2]
    let cz = sphere[3]
    let r = sphere[4]

    let oc_x = ray_orig[0] - cx
    let oc_y = ray_orig[1] - cy
    let oc_z = ray_orig[2] - cz

    let a = vec3_dot(ray_dir, ray_dir)
    let b = 2 * (oc_x * ray_dir[0] + oc_y * ray_dir[1] + oc_z * ray_dir[2])
    let c = oc_x * oc_x + oc_y * oc_y + oc_z * oc_z - r * r

    let disc = b * b - 4 * a * c
    if disc < 0 { return null }

    let sqrt_disc = sqrt(disc)
    let t1 = (0 - b - sqrt_disc) / (2 * a)
    let t2 = (0 - b + sqrt_disc) / (2 * a)

    let t = null
    if t1 > 0.001 { t = t1 }
    if t2 > 0.001 { if t == null { t = t2 } }
    if t == null { return null }

    let hp_x = ray_orig[0] + ray_dir[0] * t
    let hp_y = ray_orig[1] + ray_dir[1] * t
    let hp_z = ray_orig[2] + ray_dir[2] * t

    let n_x = hp_x - cx
    let n_y = hp_y - cy
    let n_z = hp_z - cz
    let norm = vec3_normalize([n_x, n_y, n_z])

    return [t, hp_x, hp_y, hp_z, norm[0], norm[1], norm[2], sphere[5]]
}

fn ray_plane_hit(ray_orig, ray_dir, plane) {
    let px = plane[1]
    let py = plane[2]
    let pz = plane[3]
    let nx = plane[4]
    let ny = plane[5]
    let nz = plane[6]

    let denom = ray_dir[0] * nx + ray_dir[1] * ny + ray_dir[2] * nz
    if denom < 0.001 { if denom > 0 - 0.001 { return null } }

    let t = ((px - ray_orig[0]) * nx + (py - ray_orig[1]) * ny + (pz - ray_orig[2]) * nz) / denom
    if t <= 0.001 { return null }

    let hp_x = ray_orig[0] + ray_dir[0] * t
    let hp_y = ray_orig[1] + ray_dir[1] * t
    let hp_z = ray_orig[2] + ray_dir[2] * t

    return [t, hp_x, hp_y, hp_z, nx, ny, nz, plane[7]]
}

fn find_nearest_hit(ray_orig, ray_dir, objects, bvh) {
    let nearest = null
    let nearest_t = 999999

    let stack = makeArray(64, -1)
    let stack_ptr = 0

    stack[stack_ptr] = 0
    stack_ptr = stack_ptr + 1

    while stack_ptr > 0 {
        stack_ptr = stack_ptr - 1
        let node_idx = stack[stack_ptr]

        if node_idx < 0 { break }
        if node_idx >= len(bvh) { break }

        let node = bvh[node_idx]
        if node == null { break }

        if intersect_aabb(ray_orig[0], ray_orig[1], ray_orig[2], ray_dir[0], ray_dir[1], ray_dir[2], node[0], node[1], node[2], node[3], node[4], node[5]) == false { continue }

        let split_axis = node[6]

        if split_axis < 0 {
            let obj_idx = node[7]
            let obj_count = node[8]
            let i = 0
            while i < obj_count {
                let obj = objects[obj_idx + i]
                let hit = null

                if obj[0] == "sphere" {
                    hit = ray_sphere_hit(ray_orig, ray_dir, obj)
                }
                if obj[0] == "plane" {
                    hit = ray_plane_hit(ray_orig, ray_dir, obj)
                }

                if hit != null {
                    if hit[0] < nearest_t {
                        nearest = hit
                        nearest_t = hit[0]
                    }
                }
                i = i + 1
            }
        } else {
            let right_child = node[7]
            let left_child = node_idx + 1

            stack[stack_ptr] = right_child
            stack_ptr = stack_ptr + 1
            stack[stack_ptr] = left_child
            stack_ptr = stack_ptr + 1
        }
    }

    return nearest
}

// ============================================================
// SHADOW TEST
// ============================================================

fn in_shadow(hit_point, light_pos, objects, bvh) {
    let dx = light_pos[0] - hit_point[0]
    let dy = light_pos[1] - hit_point[1]
    let dz = light_pos[2] - hit_point[2]
    let dist_sq = dx * dx + dy * dy + dz * dz
    let dist = sqrt(dist_sq)
    let light_dir = [dx / dist, dy / dist, dz / dist]
    let shadow_hit = find_nearest_hit(hit_point, light_dir, objects, bvh)

    if shadow_hit == null { return false }
    if shadow_hit[0] < dist { return true }

    return false
}

// ============================================================
// LIGHTING: PHONG MODEL
// ============================================================

fn shade(hit_point, normal, ray_dir, material, lights, objects, depth, bvh) {
    let color_r = material[0]
    let color_g = material[1]
    let color_b = material[2]
    let reflectance = material[3]
    let shininess = material[4]

    let ambient_r = color_r * 0.1
    let ambient_g = color_g * 0.1
    let ambient_b = color_b * 0.1

    let diffuse_r = 0
    let diffuse_g = 0
    let diffuse_b = 0

    let specular_r = 0
    let specular_g = 0
    let specular_b = 0

    let i = 0
    while i < len(lights) {
        let light = lights[i]
        let lx = light[0]
        let ly = light[1]
        let lz = light[2]
        let lr = light[3]
        let lg = light[4]
        let lb = light[5]
        let intensity = light[6]

        let light_array = [lx, ly, lz]

        if in_shadow(hit_point, light_array, objects, bvh) == false {
            let ldx = lx - hit_point[0]
            let ldy = ly - hit_point[1]
            let ldz = lz - hit_point[2]
            let ld_len = sqrt(ldx * ldx + ldy * ldy + ldz * ldz)
            let light_dir = [ldx / ld_len, ldy / ld_len, ldz / ld_len]

            let diff_factor = normal[0] * light_dir[0] + normal[1] * light_dir[1] + normal[2] * light_dir[2]
            if diff_factor > 0 {
                diffuse_r = diffuse_r + (color_r * intensity * diff_factor)
                diffuse_g = diffuse_g + (color_g * intensity * diff_factor)
                diffuse_b = diffuse_b + (color_b * intensity * diff_factor)
            }

            let reflect_dir = vec3_reflect(vec3_scale(light_dir, -1), normal)
            let view_dir = vec3_normalize([0 - ray_dir[0], 0 - ray_dir[1], 0 - ray_dir[2]])
            let spec_factor = reflect_dir[0] * view_dir[0] + reflect_dir[1] * view_dir[1] + reflect_dir[2] * view_dir[2]
            if spec_factor > 0 {
                spec_factor = pow_fast(spec_factor, int(shininess / 16))
                specular_r = specular_r + (spec_factor * intensity * 0.5)
                specular_g = specular_g + (spec_factor * intensity * 0.5)
                specular_b = specular_b + (spec_factor * intensity * 0.5)
            }
        }
        i = i + 1
    }

    let lit_r = ambient_r + diffuse_r + specular_r
    let lit_g = ambient_g + diffuse_g + specular_g
    let lit_b = ambient_b + diffuse_b + specular_b

    if reflectance > 0 { if depth > 0 {
        let reflect_dir = vec3_reflect(ray_dir, normal)
        let reflect_color = cast_ray(hit_point, reflect_dir, objects, lights, depth - 1, bvh)
        lit_r = lit_r * (1 - reflectance) + reflect_color[0] * reflectance
        lit_g = lit_g * (1 - reflectance) + reflect_color[1] * reflectance
        lit_b = lit_b * (1 - reflectance) + reflect_color[2] * reflectance
    } }

    return [lit_r, lit_g, lit_b]
}

// ============================================================
// RAY CASTING
// ============================================================

fn cast_ray(ray_orig, ray_dir, objects, lights, depth, bvh) {
    let hit = find_nearest_hit(ray_orig, ray_dir, objects, bvh)

    if hit == null {
        return [0.5, 0.7, 1.0]
    }

    let normal = [hit[4], hit[5], hit[6]]
    return shade(hit, normal, ray_dir, hit[7], lights, objects, depth, bvh)
}

// ============================================================
// CAMERA & TILE RENDERING
// ============================================================

fn compute_camera_rays(camera, width, height) {
    let forward = vec3_normalize([camera[3] - camera[0], camera[4] - camera[1], camera[5] - camera[2]])
    let right = vec3_normalize(vec3_cross(forward, [0, 1, 0]))
    let up = vec3_normalize(vec3_cross(right, forward))

    let fov_factor = camera[6] / 45
    let frustum_h = fov_factor
    let frustum_w = frustum_h * width / height

    return [forward[0], forward[1], forward[2], right[0], right[1], right[2], up[0], up[1], up[2], frustum_w, frustum_h]
}

fn render_tile(start_y, end_y, width, height, cam_basis, camera, objects, lights, bvh) {
    let pixels = makeArray((end_y - start_y) * width, null)
    let idx = 0

    let max_depth = 4
    let samples = 4

    let y = start_y
    while y < end_y {
        let x = 0
        while x < width {
            let color_r = 0
            let color_g = 0
            let color_b = 0

            let s = 0
            while s < samples {
                let sx = (x + ((s % 2) * 0.5)) / width - 0.5
                let sy = 0.5 - (y + ((s / 2) * 0.5)) / height

                let px = sx * cam_basis[9]
                let py = sy * cam_basis[10]

                let ray_dir_x = cam_basis[0] + cam_basis[3] * px + cam_basis[6] * py
                let ray_dir_y = cam_basis[1] + cam_basis[4] * px + cam_basis[7] * py
                let ray_dir_z = cam_basis[2] + cam_basis[5] * px + cam_basis[8] * py
                let ray_dir = vec3_normalize([ray_dir_x, ray_dir_y, ray_dir_z])

                let sample_color = cast_ray(camera, ray_dir, objects, lights, max_depth, bvh)
                color_r = color_r + sample_color[0]
                color_g = color_g + sample_color[1]
                color_b = color_b + sample_color[2]

                s = s + 1
            }

            color_r = color_r / samples
            color_g = color_g / samples
            color_b = color_b / samples

            if color_r > 1 { color_r = 1 }
            if color_g > 1 { color_g = 1 }
            if color_b > 1 { color_b = 1 }
            if color_r < 0 { color_r = 0 }
            if color_g < 0 { color_g = 0 }
            if color_b < 0 { color_b = 0 }

            pixels[idx] = [color_r, color_g, color_b]
            idx = idx + 1
            x = x + 1
        }
        y = y + 1
    }

    return pixels
}

// ============================================================
// PARALLEL RENDERING (Coarse-grained tiles, fixed workers)
// ============================================================

fn render_parallel(width, height, camera, objects, lights, num_workers) {
    let tile_h = height / num_workers
    let tasks = makeArray(num_workers, null)

    let cam_basis = compute_camera_rays(camera, width, height)
    let bvh = build_bvh(objects)

    // Helper to capture loop variables correctly
    fn make_render_task(sy, ey, w, h, cb, cam, objs, lts, bv) {
        return async(fn() {
            return render_tile(sy, ey, w, h, cb, cam, objs, lts, bv)
        })
    }

    let w = 0
    while w < num_workers {
        let start_y = w * tile_h
        let end_y = (w + 1) * tile_h
        if w == num_workers - 1 { end_y = height }

        let task = make_render_task(start_y, end_y, width, height, cam_basis, camera, objects, lights, bvh)
        tasks[w] = task
        w = w + 1
    }

    let all_pixels = makeArray(width * height, null)
    let offset = 0

    let w = 0
    while w < num_workers {
        let tile = await(tasks[w])
        let tile_size = len(tile)

        let i = 0
        while i < tile_size {
            all_pixels[offset + i] = tile[i]
            i = i + 1
        }
        offset = offset + tile_size
        w = w + 1
    }

    return all_pixels
}

// ============================================================
// PPM OUTPUT
// ============================================================

fn write_ppm(filename, width, height, pixels) {
    let lines = makeArray(height, "")
    let idx = 0

    let y = 0
    while y < height {
        let line = ""
        let x = 0
        while x < width {
            let color = pixels[idx]
            let r = int(color[0] * 255)
            let g = int(color[1] * 255)
            let b = int(color[2] * 255)

            if r > 255 { r = 255 }
            if g > 255 { g = 255 }
            if b > 255 { b = 255 }
            if r < 0 { r = 0 }
            if g < 0 { g = 0 }
            if b < 0 { b = 0 }

            line = line + str(r) + " " + str(g) + " " + str(b) + " "
            idx = idx + 1
            x = x + 1
        }
        lines[y] = line
        y = y + 1
    }

    let header = "P3\n" + str(width) + " " + str(height) + "\n255\n"
    let data = header + join(lines, "\n")
    writeFile(filename, data)
    return true
}
