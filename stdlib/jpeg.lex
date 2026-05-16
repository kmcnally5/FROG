const PI = pi()

// DCT basis function for an 8x8 block
fn dct_basis(u, v, x, y) {
    let cu = 1.0
    if u == 0 { cu = 0.7071 }

    let cv = 1.0
    if v == 0 { cv = 0.7071 }

    return cu * cv * cos((2 * x + 1) * u * PI / 16) * cos((2 * y + 1) * v * PI / 16)
}

// Apply Discrete Cosine Transform to a flattened 8x8 block
fn apply_dct(block8x8) {
    let output = range(64)
    for u in range(8) {
        for v in range(8) {
            let sum = 0.0
            for x in range(8) {
                for y in range(8) {
                    sum = sum + block8x8[x*8 + y] * dct_basis(u, v, x, y)
                }
            }
            output[u*8 + v] = sum / 4
        }
    }
    return output
}

// Standard JPEG Luminance Quantization Table
// Higher numbers = more compression = more data thrown away
Q_TABLE = [
    16, 11, 10, 16, 24, 40, 51, 61,
    12, 12, 14, 19, 26, 58, 60, 55,
    14, 13, 16, 24, 40, 57, 69, 56,
    14, 17, 22, 29, 51, 87, 80, 62,
    18, 22, 37, 56, 68, 109, 103, 77,
    24, 35, 55, 64, 81, 104, 113, 92,
    49, 64, 78, 87, 103, 121, 120, 101,
    72, 92, 95, 98, 112, 100, 103, 99
]

// Step 1: RGB to Y (Luminance) — humans see brightness better than colour
fn rgb_to_y(r, g, b) {
    return 0.299*r + 0.587*g + 0.114*b - 128
}

// Step 2: Compress a single 8x8 block
fn compress_block(block8x8) {
    let dct_coeffs = apply_dct(block8x8)
    let quantized = makeArray(64)
    for i in range(64) {
        quantized[i] = round(dct_coeffs[i] / Q_TABLE[i])
    }
    return quantized
}
