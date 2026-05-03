// StressTest_Structs.lex
struct Threat {
    id,
    severity,
    metadata
}

enum Status {
    Active(code, message),
    Resolved,
    Ignored
}

fn generate_report(limit) {
    let collection = []
    for i in range(limit) {
        // Nested struct with an Enum variant
        let t = Threat {
            id: i,
            severity: i % 10,
            metadata: Status.Active(403, "Forbidden Access Attempt")
        }
        collection = push(collection, t)
    }
    return collection
}

let startTime = 1000 // Placeholder for actual timer if you have one
let results = generate_report(100000)
println("Processed", len(results), "complex structs.")