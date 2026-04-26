import "uuid.lex" as uuid

// --- basic shape ---
id = uuid.v4()
println(type(id) == "STRING")     // true
println(len(id) == 36)            // true

// format: 8-4-4-4-12 hex chars separated by hyphens
parts = split(id, "-")
println(len(parts) == 5)          // true
println(len(parts[0]) == 8)       // true
println(len(parts[1]) == 4)       // true
println(len(parts[2]) == 4)       // true
println(len(parts[3]) == 4)       // true
println(len(parts[4]) == 12)      // true

// version nibble at position 0 of the third group must be '4'
println(parts[2][0] == "4")       // true

// --- uniqueness ---
a = uuid.v4()
b = uuid.v4()
println(a != b)                   // true
