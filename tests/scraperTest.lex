
import "http.lex" as http

// COMENT - This test demonstrates how to perform parallel HTTP requests using kLex's concurrency features.
fn fetch_and_analyze(id, url, results_bus) {
    // Adding a 'Start' and 'End' log helps us see if the worker actually completes
    println("Worker " + str(id) + " START: " + url)
    
    // CRITICAL: Use 'let' for the tuple assignment
    resp, err = http.get(url)
    
    if err != null {
        send(results_bus, { "url": url, "error": err, "done": true })
        return null
    }

    let body_len = len(resp.body)
    
    send(results_bus, {
        "url": url,
        "status": resp.status,
        "size": body_len,
        "server": http.header(resp, "server"),
        "done": true
    })
    println("Worker " + str(id) + " FINISHED")
}

fn main() {
    let targets = [
        "https://google.com",
        "https://github.com",
        "https://wikipedia.org"
    ]
    
    // We make the channel slightly larger than needed to prevent blocking
    let bus = channel(10) 
    println("--- kLex Parallel Scraper ---")

    for i in range(len(targets)) {
        async(fetch_and_analyze, i, targets[i], bus)
    }

    let completed = 0
    while completed < len(targets) {
        // CRITICAL: Use 'let' here too
        result, ok = recv(bus)
        
        if ok {
            completed = completed + 1
            if hasKey(result, "error") {
                println("FAILED: " + result["url"] + " -> " + result["error"])
            } else {
                println("DONE: " + result["url"] + " (" + str(result["size"]) + " bytes)")
            }
        }
    }
    println("--- ALL TARGETS PROCESSED ---")
}

main()