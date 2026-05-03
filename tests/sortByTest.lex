import "array.lex" as arr

// 1. sort() still works — backward compatible
println(str(sort([3, 1, 4, 1, 5, 9, 2, 6])))

// 2. sortBy ascending integers
println(str(sortBy([5, 2, 8, 1, 9], fn(a, b) { return a < b })))

// 3. sortBy descending integers
println(str(sortBy([5, 2, 8, 1, 9], fn(a, b) { return a > b })))

// 4. sortBy strings ascending
println(str(sortBy(["banana", "apple", "cherry"], fn(a, b) { return a < b })))

// 5. sortBy strings by length
println(str(sortBy(["banana", "fig", "apple", "kiwi"], fn(a, b) { return len(a) < len(b) })))

// 6. sortBy structs by field
struct Person { name, age }

people = [
    Person { name: "Charlie", age: 30 },
    Person { name: "Alice",   age: 25 },
    Person { name: "Bob",     age: 35 }
]

byAge = sortBy(people, fn(a, b) { return a.age < b.age })
for p in byAge {
    println(p.name + " " + str(p.age))
}

// 7. sortBy structs descending by field
byAgeDesc = sortBy(people, fn(a, b) { return a.age > b.age })
for p in byAgeDesc {
    println(p.name + " " + str(p.age))
}

// 8. original array is unchanged (functions transform, not mutate)
nums = [3, 1, 2]
sorted = sortBy(nums, fn(a, b) { return a < b })
println(str(nums))    // [3, 1, 2] — unchanged
println(str(sorted))  // [1, 2, 3]
