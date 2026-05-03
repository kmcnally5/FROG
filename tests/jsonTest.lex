import "json.lex" as json

// --- parse object ---
data, err = json.parse("\{\"name\":\"Frog\",\"age\":42}")
println(err == null)          // true
println(data["name"])         // Frog
println(data["age"])          // 42

// --- stringify ---
s = json.stringify(data)
println(type(s) == "STRING")  // true

// --- parse array ---
arr, err = json.parse("[1,2,3]")
println(err == null)          // true
println(arr[0])               // 1
println(arr[2])               // 3

// --- parse nested ---
nested, err = json.parse("\{\"user\":\{\"name\":\"Alice\",\"scores\":[10,20,30]}}")
println(err == null)                         // true
println(nested["user"]["name"])              // Alice
println(nested["user"]["scores"][1])         // 20

// --- parse booleans and null ---
bools, err = json.parse("[true,false,null]")
println(err == null)          // true
println(bools[0])             // true
println(bools[1])             // false
println(bools[2] == null)     // true

// --- parse float ---
fdata, err = json.parse("3.14")
println(err == null)          // true
println(fdata)                // 3.14

// --- parse error ---
bad, err = json.parse("\{bad}")
println(err != null)          // true

// --- stringify primitives ---
println(json.stringify(null))     // null
println(json.stringify(true))     // true
println(json.stringify(false))    // false
println(json.stringify(42))       // 42
println(json.stringify("hello"))  // "hello"
println(json.stringify([1,2,3]))  // [1,2,3]
