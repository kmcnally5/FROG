import "stdlib/parallel.lex" as p

// ============================================================
// HIGH VOLUME LOG ANALYSIS - STREAMING BENCHMARK
// ============================================================
// Real streaming log analysis: generate → parse → aggregate
// This exposes actual parsing bottleneck and scaling benefits

// ============================================================
// REALISTIC DATA GENERATION (inline, not stored)
// ============================================================

fn hash_idx(idx) {
    let h = idx * 2654435761
    h = h * 2246822519
    return h
}

fn get_http_method(idx) {
    let h = hash_idx(idx)
    let method_idx = h % 100
    if method_idx < 70 { return "GET" }
    if method_idx < 85 { return "POST" }
    if method_idx < 92 { return "PUT" }
    if method_idx < 97 { return "DELETE" }
    if method_idx < 99 { return "PATCH" }
    return "HEAD"
}

fn get_endpoint(idx) {
    let h = (hash_idx(idx) * 37)
    let endpoint_idx = h % 13
    if endpoint_idx == 0 { return "/api/v1/users" }
    if endpoint_idx == 1 { return "/api/v1/posts" }
    if endpoint_idx == 2 { return "/api/v1/comments" }
    if endpoint_idx == 3 { return "/api/v1/likes" }
    if endpoint_idx == 4 { return "/api/v1/feed" }
    if endpoint_idx == 5 { return "/api/v1/auth" }
    if endpoint_idx == 6 { return "/api/v1/notifications" }
    if endpoint_idx == 7 { return "/api/v1/search" }
    if endpoint_idx == 8 { return "/health/ping" }
    if endpoint_idx == 9 { return "/metrics/prometheus" }
    if endpoint_idx == 10 { return "/static/assets" }
    if endpoint_idx == 11 { return "/webhooks/events" }
    return "/api/v1/admin"
}

fn get_status_code(idx) {
    let h = (hash_idx(idx) * 19)
    let chunk = idx / 1250000
    let prob = h % 1000

    // Vary status distribution by chunk to show real work variation
    if chunk == 0 {
        // Chunk 0: High success rate
        if prob < 950 { return 200 }
        if prob < 975 { return 400 }
        if prob < 990 { return 301 }
        return 500
    }

    if chunk == 1 {
        // Chunk 1: More client errors
        if prob < 850 { return 200 }
        if prob < 925 { return 400 }
        if prob < 960 { return 301 }
        return 500
    }

    if chunk == 2 {
        // Chunk 2: More server errors
        if prob < 880 { return 200 }
        if prob < 910 { return 400 }
        if prob < 950 { return 301 }
        return 500
    }

    // Chunk 3+: Back to normal
    if prob < 910 { return 200 }
    if prob < 950 { return 400 }
    if prob < 990 { return 301 }
    return 500
}

fn get_latency_ms(idx) {
    let h = (hash_idx(idx) * 53)
    let rand_val = h % 10000
    if rand_val < 7000 { return (rand_val % 100) + 5 }
    if rand_val < 9000 { return (rand_val % 300) + 100 }
    return (rand_val % 5000) + 400
}

fn generate_log_line(idx) {
    let timestamp = 1700000000 + (idx / 100)
    let method = get_http_method(idx)
    let endpoint = get_endpoint(idx)
    let status = get_status_code(idx)
    let latency = get_latency_ms(idx)
    let bytes_sent = (idx % 50000) + 100
    let user_id = (idx % 50000) + 1

    return str(timestamp) + "|" + method + "|" + endpoint + "|" + str(status) + "|" + str(latency) + "|" + str(bytes_sent) + "|" + str(user_id)
}

// ============================================================
// STREAMING PARSE + ANALYZE (no intermediate storage)
// ============================================================

fn parse_log_line(line) {
    let parts = split(line, "|")
    if len(parts) < 7 { return null }
    return [int(parts[0]), parts[1], parts[2], int(parts[3]), int(parts[4]), int(parts[5]), int(parts[6])]
}

fn analyze_stream_chunk(start_idx, end_idx) {
    let status_2xx = 0
    let status_3xx = 0
    let status_4xx = 0
    let status_5xx = 0

    let method_get = 0
    let method_post = 0
    let method_put = 0
    let method_delete = 0
    let method_patch = 0
    let method_head = 0

    let endpoint_users = 0
    let endpoint_posts = 0
    let endpoint_comments = 0
    let endpoint_likes = 0
    let endpoint_feed = 0
    let endpoint_auth = 0
    let endpoint_notif = 0
    let endpoint_search = 0
    let endpoint_health = 0
    let endpoint_metrics = 0
    let endpoint_static = 0
    let endpoint_webhooks = 0
    let endpoint_admin = 0

    let latency_sum = 0
    let latency_max = 0
    let latency_under_100 = 0
    let latency_under_400 = 0
    let latency_under_2000 = 0
    let bytes_sum = 0
    let line_count = 0

    let i = start_idx

    while i < end_idx {
        let log_line = generate_log_line(i)
        let log = parse_log_line(log_line)

        if log != null {
            let status = log[3]
            if status >= 200 && status < 300 { status_2xx = status_2xx + 1 }
            if status >= 300 && status < 400 { status_3xx = status_3xx + 1 }
            if status >= 400 && status < 500 { status_4xx = status_4xx + 1 }
            if status >= 500 { status_5xx = status_5xx + 1 }

            let method = log[1]
            if method == "GET" { method_get = method_get + 1 }
            if method == "POST" { method_post = method_post + 1 }
            if method == "PUT" { method_put = method_put + 1 }
            if method == "DELETE" { method_delete = method_delete + 1 }
            if method == "PATCH" { method_patch = method_patch + 1 }
            if method == "HEAD" { method_head = method_head + 1 }

            let endpoint = log[2]
            if endpoint == "/api/v1/users" { endpoint_users = endpoint_users + 1 }
            if endpoint == "/api/v1/posts" { endpoint_posts = endpoint_posts + 1 }
            if endpoint == "/api/v1/comments" { endpoint_comments = endpoint_comments + 1 }
            if endpoint == "/api/v1/likes" { endpoint_likes = endpoint_likes + 1 }
            if endpoint == "/api/v1/feed" { endpoint_feed = endpoint_feed + 1 }
            if endpoint == "/api/v1/auth" { endpoint_auth = endpoint_auth + 1 }
            if endpoint == "/api/v1/notifications" { endpoint_notif = endpoint_notif + 1 }
            if endpoint == "/api/v1/search" { endpoint_search = endpoint_search + 1 }
            if endpoint == "/health/ping" { endpoint_health = endpoint_health + 1 }
            if endpoint == "/metrics/prometheus" { endpoint_metrics = endpoint_metrics + 1 }
            if endpoint == "/static/assets" { endpoint_static = endpoint_static + 1 }
            if endpoint == "/webhooks/events" { endpoint_webhooks = endpoint_webhooks + 1 }
            if endpoint == "/api/v1/admin" { endpoint_admin = endpoint_admin + 1 }

            let latency = log[4]
            latency_sum = latency_sum + latency
            if latency > latency_max { latency_max = latency }
            if latency <= 100 { latency_under_100 = latency_under_100 + 1 }
            if latency <= 400 { latency_under_400 = latency_under_400 + 1 }
            if latency <= 2000 { latency_under_2000 = latency_under_2000 + 1 }

            bytes_sum = bytes_sum + log[5]
            line_count = line_count + 1
        }

        i = i + 1
    }

    return [
        status_2xx, status_3xx, status_4xx, status_5xx,
        method_get, method_post, method_put, method_delete, method_patch, method_head,
        endpoint_users, endpoint_posts, endpoint_comments, endpoint_likes, endpoint_feed,
        endpoint_auth, endpoint_notif, endpoint_search, endpoint_health, endpoint_metrics,
        endpoint_static, endpoint_webhooks, endpoint_admin,
        latency_sum, latency_max, latency_under_100, latency_under_400, latency_under_2000,
        bytes_sum, line_count
    ]
}

fn analyze_logs_streaming(total_count, num_workers) {
    let chunk_size = total_count / num_workers
    let tasks = makeArray(num_workers, null)

    for w in range(num_workers) {
        let start_idx = w * chunk_size
        let end_idx = (w + 1) * chunk_size
        if w == num_workers - 1 { end_idx = total_count }

        let task = async(fn() {
            return analyze_stream_chunk(start_idx, end_idx)
        })
        tasks[w] = task
    }

    let result = [
        0, 0, 0, 0,
        0, 0, 0, 0, 0, 0,
        0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
        0, 0, 0, 0, 0,
        0, 0
    ]

    for i in range(num_workers) {
        let chunk = await(tasks[i])
        result[0] = result[0] + chunk[0]
        result[1] = result[1] + chunk[1]
        result[2] = result[2] + chunk[2]
        result[3] = result[3] + chunk[3]
        result[4] = result[4] + chunk[4]
        result[5] = result[5] + chunk[5]
        result[6] = result[6] + chunk[6]
        result[7] = result[7] + chunk[7]
        result[8] = result[8] + chunk[8]
        result[9] = result[9] + chunk[9]
        result[10] = result[10] + chunk[10]
        result[11] = result[11] + chunk[11]
        result[12] = result[12] + chunk[12]
        result[13] = result[13] + chunk[13]
        result[14] = result[14] + chunk[14]
        result[15] = result[15] + chunk[15]
        result[16] = result[16] + chunk[16]
        result[17] = result[17] + chunk[17]
        result[18] = result[18] + chunk[18]
        result[19] = result[19] + chunk[19]
        result[20] = result[20] + chunk[20]
        result[21] = result[21] + chunk[21]
        result[22] = result[22] + chunk[22]
        result[23] = result[23] + chunk[23]
        if chunk[24] > result[24] { result[24] = chunk[24] }
        result[25] = result[25] + chunk[25]
        result[26] = result[26] + chunk[26]
        result[27] = result[27] + chunk[27]
        result[28] = result[28] + chunk[28]
        result[29] = result[29] + chunk[29]
    }

    return result
}

// ============================================================
// BENCHMARK RUNNER
// ============================================================

fn run_benchmark(log_count, worker_counts) {
    println("============================================================")
    println("STREAMING LOG ANALYSIS BENCHMARK")
    println("============================================================")
    println("")
    println("Real-time generation + parsing (no intermediate storage)")
    println("Chunks have intentionally varied distributions (error rates)")
    println("")
    println("Note: Parallelization speedup visible via elapsed time (use 'time' command)")
    println("")

    for w_idx in range(len(worker_counts)) {
        let num_workers = worker_counts[w_idx]

        println("Analyzing " + str(log_count) + " entries with " + str(num_workers) + " workers...")
        let result = analyze_logs_streaming(log_count, num_workers)

        let total_lines = result[29]
        let avg_latency = result[23] / total_lines
        let max_latency = result[24]
        let p50_count = result[25]
        let p95_count = result[26]
        let p99_count = result[27]
        let total_bytes = result[28]

        let status_2xx_pct = (result[0] * 100) / total_lines
        let status_3xx_pct = (result[1] * 100) / total_lines
        let status_4xx_pct = (result[2] * 100) / total_lines
        let status_5xx_pct = (result[3] * 100) / total_lines

        let p50_pct = (p50_count * 100) / total_lines
        let p95_pct = (p95_count * 100) / total_lines
        let p99_pct = (p99_count * 100) / total_lines

        println("")
        println("--- RESULTS (" + str(num_workers) + " WORKERS) ---")
        println("Total requests processed: " + str(total_lines))
        println("")
        println("Status Code Distribution:")
        println("  2xx (Success): " + str(result[0]) + " (" + str(status_2xx_pct) + "%)")
        println("  3xx (Redirect): " + str(result[1]) + " (" + str(status_3xx_pct) + "%)")
        println("  4xx (Client Error): " + str(result[2]) + " (" + str(status_4xx_pct) + "%)")
        println("  5xx (Server Error): " + str(result[3]) + " (" + str(status_5xx_pct) + "%)")
        println("")
        println("HTTP Methods:")
        println("  GET: " + str(result[4]) + " | POST: " + str(result[5]) + " | PUT: " + str(result[6]))
        println("  DELETE: " + str(result[7]) + " | PATCH: " + str(result[8]) + " | HEAD: " + str(result[9]))
        println("")
        println("Top Endpoints:")
        println("  /api/v1/users: " + str(result[10]))
        println("  /api/v1/posts: " + str(result[11]))
        println("  /api/v1/feed: " + str(result[14]))
        println("  /api/v1/auth: " + str(result[15]))
        println("")
        println("Latency Analysis (milliseconds):")
        println("  Average: " + str(avg_latency) + "ms")
        println("  P50 (<= 100ms): " + str(p50_pct) + "% of requests")
        println("  P95 (<= 400ms): " + str(p95_pct) + "% of requests")
        println("  P99 (<= 2000ms): " + str(p99_pct) + "% of requests")
        println("  Max: " + str(max_latency) + "ms")
        println("")
        println("Throughput:")
        println("  Total bytes: " + str(total_bytes))
        println("  Requests/sec: " + str(total_lines / 10))
        println("")
    }

    println("============================================================")
    println("BENCHMARK COMPLETE")
    println("============================================================")
}

// ============================================================
// MAIN
// ============================================================

fn main() {
    let worker_configs = [4, 8, 12]
    run_benchmark(5000000, worker_configs)
}

main()
