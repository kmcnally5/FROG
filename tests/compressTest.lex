import "compress.lex" as c
import "fs.lex" as fs

println("=== COMPRESS TEST ===")
println("")

// Test 1: Basic gzip compression
println("Test 1: Basic gzip compression")
data = "hello world hello world hello world hello world"
compressed, err = c.compress(data)
println("Original length:", len(data))
println("Compressed length:", len(compressed))
println("Error:", err)
println("Compression ratio:", c.compressionRatio(data, compressed))
println("")

// Test 2: Decompression
println("Test 2: Decompression")
decompressed, err = c.decompress(compressed)
println("Decompressed matches original:", decompressed == data)
println("Error:", err)
println("")

// Test 3: Deflate compression
println("Test 3: Deflate compression")
deflated, err = c.deflate(data)
println("Deflate compressed length:", len(deflated))
println("Error:", err)
println("")

// Test 4: Deflate decompression
println("Test 4: Deflate decompression")
inflated, err = c.inflate(deflated)
println("Inflated matches original:", inflated == data)
println("Error:", err)
println("")

// Test 5: Large text compression
println("Test 5: Large text compression (JSON-like data)")
json = `{"id": 1, "name": "Alice", "email": "alice@example.com"}`
json = json + json + json + json + json + json + json + json + json + json
compressed, err = c.compress(json)
println("Original:", len(json), "bytes")
println("Compressed:", len(compressed), "bytes")
println("Ratio:", c.compressionRatio(json, compressed))
println("Savings:", c.savings(json, compressed))
println("")

// Test 6: compressedSize utility
println("Test 6: compressedSize utility")
testData = "test data test data test data"
size = c.compressedSize(testData)
println("Original size:", len(testData), "bytes")
println("Compressed size:", size, "bytes")
println("")

// Test 7: Batch compression
println("Test 7: Batch compression")
items = ["item1 " + "data", "item2 " + "data", "item3 " + "data"]
compressed_items, err = c.compressMany(items)
println("Compressed array length:", len(compressed_items))
println("Error:", err)
println("")

// Test 8: Batch decompression
println("Test 8: Batch decompression")
decompressed_items, err = c.decompressMany(compressed_items)
println("Match items[0]:", decompressed_items[0] == items[0])
println("Match items[1]:", decompressed_items[1] == items[1])
println("Match items[2]:", decompressed_items[2] == items[2])
println("Error:", err)
println("")

// Test 9: Error handling - wrong type
println("Test 9: Error handling (wrong type)")
result, err = c.compress(123)
println("Compress(123) error:", err)
println("Result is null:", result == null)
println("")

// Test 10: File compression (if test file exists)
println("Test 10: File operations")
// Create a test file
testFile = "test_compress_file.txt"
testFileCompressed = "test_compress_file.txt.gz"
content = "This is test content for compression. " + "This is test content for compression. " + "This is test content for compression."

// Write test file
_, err = fs.write(testFile, content)
if err == null {
    // Compress the file
    compressed, err = c.compressFile(testFile)
    println("compressFile succeeded:", err == null)

    // Write the compressed data to a file
    _, err = fs.write(testFileCompressed, compressed)
    if err == null {
        // Read the compressed file and decompress it
        compressedData, err = fs.read(testFileCompressed)
        decompressed, err = c.decompress(compressedData)
        println("decompressFile succeeded:", err == null)
        println("Content matches:", decompressed == content)

        // Clean up
        _, _ = fs.remove(testFile)
        _, _ = fs.remove(testFileCompressed)
    }
} else {
    println("Could not create test file:", err)
}

println("")
println("=== COMPRESS TEST COMPLETE ===")
