import "math.lex" as m

// Standard JPEG Luminance Quantization Table
// This "Governs" how much data we throw away (Higher numbers = More compression)
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

// Step 1: RGB to Y (Luminance) - Humans see brightness better than color
fn rgb_to_y(r, g, b) {
    return 0.299*r + 0.587*g + 0.114*b - 128
}

// Step 2: Compress a single 8x8 block
fn compress_block(block8x8) {
    let dct_coeffs = m.apply_dct(block8x8)
    let quantized = []
    
    for i in range(64) {
        // Divide by Q_TABLE to drop high-frequency "noise"
        let val = round(dct_coeffs[i] / Q_TABLE[i])
        quantized = push(quantized, val)
    }
    return quantized
}