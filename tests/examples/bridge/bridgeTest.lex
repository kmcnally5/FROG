// bridgeTest.lex — demonstrates calling Python from kLex via nativeBridge

bridge, err = nativeBridge("python3", ["tests/examples/bridge/python_bridge.py"])
if err != null {
    println("Failed to start bridge: {err.message}")
    return
}
println("✓ Bridge started")

// Basic arithmetic
result, err = bridgeCall(bridge, "add", [10, 20])
if err != null { println("add error: {err.message}") } else {
    println("add(10, 20)         = {result}")
}

result, err = bridgeCall(bridge, "multiply", [6, 7])
if err != null { println("multiply error: {err.message}") } else {
    println("multiply(6, 7)      = {result}")
}

// String handling
result, err = bridgeCall(bridge, "greet", ["Karl"])
if err != null { println("greet error: {err.message}") } else {
    println("greet('Karl')       = {result}")
}

result, err = bridgeCall(bridge, "reverse_words", ["the quick brown fox"])
if err != null { println("reverse error: {err.message}") } else {
    println("reverse_words(...)  = {result}")
}

// Array in, hash out
result, err = bridgeCall(bridge, "stats", [[4, 8, 15, 16, 23, 42]])
if err != null { println("stats error: {err.message}") } else {
    count = result["count"]
    mean  = result["mean"]
    mx    = result["max"]
    println("stats([...])        = count={count} mean={mean} max={mx}")
}

// Boolean return
result, err = bridgeCall(bridge, "is_prime", [97])
if err != null { println("is_prime error: {err.message}") } else {
    println("is_prime(97)        = {result}")
}

result, err = bridgeCall(bridge, "is_prime", [100])
if err != null { println("is_prime error: {err.message}") } else {
    println("is_prime(100)       = {result}")
}

// Array return
result, err = bridgeCall(bridge, "primes_up_to", [30])
if err != null { println("primes error: {err.message}") } else {
    println("primes_up_to(30)    = {result}")
}

// Error handling — unknown function
result, err = bridgeCall(bridge, "does_not_exist", [])
if err != null {
    println("✓ Error caught:     {err.message}")
} else {
    println("ERROR: expected error but got {result}")
}

bridgeClose(bridge)
println("\n✓ Bridge closed cleanly")
