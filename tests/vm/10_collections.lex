// Tier 10 — array / hash / tuple literals + indexing.

// Array literal + indexing
let arr = [10, 20, 30, 40, 50]
println(arr[0])
println(arr[2])
println(arr[4])
println(len(arr))

// Nested arrays
let grid = [[1, 2], [3, 4], [5, 6]]
println(grid[0][0])
println(grid[1][1])
println(grid[2][0])

// Hash literal — string keys
let person = {"name": "Karl", "age": 99}
println(person["name"])
println(person["age"])

// Hash literal — integer keys
let nums = {1: "one", 2: "two", 3: "three"}
println(nums[1])
println(nums[2])

// Missing key returns null (not an error)
println(person["nope"])

// Mixed: array of hashes
let people = [{"n": "Alice"}, {"n": "Bob"}]
println(people[0]["n"])
println(people[1]["n"])

// Empty literals
println(len([]))
println(len({}))
