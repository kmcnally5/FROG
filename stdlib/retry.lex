// retry.lex
// Retry logic with optional exponential backoff for kLex.
//
// The function passed to do/doWithBackoff must return a (result, err) tuple.
// Retry stops on the first call where err is null.
//
// Usage:
//   import "retry.lex" as retry
//
//   // simple retry — up to 3 attempts, no delay
//   result, err = retry.do(fn() { return http.get(url) }, 3)
//
//   // with exponential backoff — 100ms, 200ms, 400ms between retries
//   result, err = retry.doWithBackoff(fn() { return http.get(url) }, 4, 100)

// do calls f up to maxAttempts times, returning immediately on success.
// Returns (result, err) where err is null on success or the last error
// seen after all attempts are exhausted.
fn do(f, maxAttempts) {
    return doWithBackoff(f, maxAttempts, 0)
}

// doWithBackoff calls f up to maxAttempts times with exponential backoff
// between failures. initialDelayMs is the delay before the second attempt;
// each subsequent delay doubles. Pass 0 for no delay.
fn doWithBackoff(f, maxAttempts, initialDelayMs) {
    i = 0
    delay = initialDelayMs
    lastErr = null

    while i < maxAttempts {
        result, err = f()
        if err == null { return result, null }

        lastErr = err
        i = i + 1

        if i < maxAttempts && delay > 0 {
            sleep(delay)
            delay = delay * 2
        }
    }

    return null, lastErr
}
