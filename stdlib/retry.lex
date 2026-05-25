// retry.lex
// @module    retry
// @version   1.0.0
// @since     klex 0.3.35
// @author    karl
// @summary   Retry logic for kLex — three entry points spanning simple to full control.
// Retry logic for kLex — three entry points spanning simple to full control.
//
// All three accept a no-arg function f that must return a (result, err)
// tuple and stop on the first call where err is null.
//
//   do(f, maxAttempts)
//     Simplest form — N attempts, no delay between them. err can be any
//     value (string or typed); every non-null err triggers a retry.
//
//   doWithBackoff(f, maxAttempts, initialDelayMs)
//     Same retry-everything semantics as do(), but sleeps between attempts.
//     Delay doubles each retry (initialDelayMs, 2x, 4x, …).
//
//   doWith(f, opts)
//     Full-control form — opts is a hash with all the knobs. Adds:
//       - a classifier that decides which errors are transient
//       - per-attempt delay cap
//       - total-time deadline
//       - jitter to break retry synchronisation
//       - onRetry callback for logging/progress
//     Designed to wrap typed (error-struct) calls — particularly the
//     stdlib/ai/* library set — but works fine with any error shape.
//
// Usage:
//   import "retry.lex" as retry
//
//   // simple — up to 3 attempts, no delay
//   result, err = retry.do(fn() { return http.get(url) }, 3)
//
//   // exponential backoff — 100ms, 200ms, 400ms between failures
//   result, err = retry.doWithBackoff(fn() { return http.get(url) }, 4, 100)
//
//   // AI-aware — only retry transient errors, with deadline + jitter
//   import "stdlib/ai/anthropic.lex" as claude
//   result, err = retry.doWith(fn() { return claude.messages(c, opts) }, {
//       "maxAttempts": 5,
//       "baseDelay":   500,
//       "deadline":    120000,
//   })


// ── Simple retry ──────────────────────────────────────────────────────────

// do calls f up to maxAttempts times, returning immediately on success.
// Every non-null err triggers another attempt — see doWith() for a
// classifier-aware variant that distinguishes transient from fatal.
fn do(f, maxAttempts) {
    return doWithBackoff(f, maxAttempts, 0)
}

// doWithBackoff calls f up to maxAttempts times with exponential backoff
// between failures. initialDelayMs is the delay before the second attempt;
// each subsequent delay doubles. Pass 0 for no delay.
fn doWithBackoff(f, maxAttempts, initialDelayMs) {
    let i = 0
    let delay = initialDelayMs
    let lastErr = null

    while i < maxAttempts {
        let result, err = f()
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


// ── Full-control retry: classifier, jitter, deadline ─────────────────────

const _DEFAULT_MAX_ATTEMPTS = 3
const _DEFAULT_BASE_DELAY   = 500       // ms
const _DEFAULT_MAX_DELAY    = 30000     // 30s cap per single backoff
const _DEFAULT_JITTER       = true

// Error-code suffixes the default classifier treats as transient.
// Matches the SCREAMING_SNAKE provider-prefix convention used across the
// stdlib/ai/* library set (e.g. ANTHROPIC_RATE_LIMIT, OLLAMA_SERVER).
const _RETRYABLE_SUFFIXES = [
    "_RATE_LIMIT",
    "_SERVER",
    "_TIMEOUT",
    "_OVERLOADED",
    "_NETWORK",
    "_CONNECTION",
]

// Error-code suffixes the default classifier treats as permanent.
const _FATAL_SUFFIXES = [
    "_AUTH",
    "_FORBIDDEN",
    "_NOT_FOUND",
    "_BAD_REQUEST",
    "_INVALID",
]


// _endsWith returns true if s ends with suffix.
fn _endsWith(s, suffix) {
    let sl = len(s)
    let el = len(suffix)
    if sl < el { return false }
    return substr(s, sl - el, sl) == suffix
}

fn _optInt(opts, key, fallback) {
    if opts == null         { return fallback }
    if !hasKey(opts, key)   { return fallback }
    let v = opts[key]
    if v == null            { return fallback }
    if type(v) == "FLOAT"   { return int(v) }
    return v
}

fn _optBool(opts, key, fallback) {
    if opts == null         { return fallback }
    if !hasKey(opts, key)   { return fallback }
    let v = opts[key]
    if v == null            { return fallback }
    return v
}

fn _optFn(opts, key) {
    if opts == null         { return null }
    if !hasKey(opts, key)   { return null }
    return opts[key]
}

// _backoffMs returns the delay (ms) for retry index n (0 = first retry).
// Exponential growth capped at maxDelay, optionally jittered by a random
// factor in [0.5, 1.0) so concurrent callers don't synchronise their retries.
fn _backoffMs(n, baseDelay, maxDelay, jitter) {
    let d   = float(baseDelay) * pow(2, n)
    let cap = float(maxDelay)
    if d > cap { d = cap }
    if jitter {
        d = d * (0.5 + rand() * 0.5)
    }
    return int(d)
}

fn _elapsedMs(startNs) {
    return (_timeNanos() - startNs) / 1000000
}


// defaultClassifier returns true if err looks like a transient failure
// worth retrying, using the SCREAMING_SNAKE error-code convention.
//
// Policy:
//   - Codes ending in a known fatal suffix → false
//   - Codes ending in a known retryable suffix → true
//   - Anything else (untyped errors, unknown codes) → false. Be conservative
//     so unfamiliar shapes don't silently burn API credits. Pass a custom
//     classifier to doWith() if you need broader retry coverage.
fn defaultClassifier(err) {
    if err == null          { return false }
    if !isError(err)        { return false }
    let code = err.code
    if type(code) != "STRING" { return false }

    for fs in _FATAL_SUFFIXES {
        if _endsWith(code, fs) { return false }
    }
    for rs in _RETRYABLE_SUFFIXES {
        if _endsWith(code, rs) { return true }
    }
    return false
}


// doWith(f, [opts]) → (result, err)
//
// Full-control retry. Calls f() repeatedly with exponential backoff until
// it returns a non-error result, the classifier judges the error fatal,
// max attempts is reached, or the total deadline elapses.
//
// f must be a zero-argument function returning (value, err).
//
// Options (all optional, hash):
//   maxAttempts : int  — default 3
//   baseDelay   : int  — initial backoff in ms (default 500)
//   maxDelay    : int  — cap on any single backoff in ms (default 30000)
//   deadline    : int  — total ms budget across all attempts (default 0 = none)
//   jitter      : bool — randomise backoffs by [0.5, 1.0) (default true)
//   onRetry     : fn(attempt, err, delayMs)  — called before each sleep
//   isRetryable : fn(err) → bool             — overrides defaultClassifier
//
// Returns:
//   (result, null)                        on success
//   (null,   <original err>)              on fatal classification
//   (null,   error("RETRY_EXHAUSTED",…))  when maxAttempts is hit
//   (null,   error("RETRY_DEADLINE",…))   when the total deadline elapses
fn doWith(f, opts = null) {
    let maxAttempts = _optInt(opts, "maxAttempts", _DEFAULT_MAX_ATTEMPTS)
    let baseDelay   = _optInt(opts, "baseDelay",   _DEFAULT_BASE_DELAY)
    let maxDelay    = _optInt(opts, "maxDelay",    _DEFAULT_MAX_DELAY)
    let deadline    = _optInt(opts, "deadline",    0)
    let jitter      = _optBool(opts, "jitter",     _DEFAULT_JITTER)
    let onRetry     = _optFn(opts, "onRetry")
    let classifier  = _optFn(opts, "isRetryable")

    if maxAttempts < 1 { maxAttempts = 1 }

    let startNs = _timeNanos()
    let attempt = 1
    let lastErr = null

    while attempt <= maxAttempts {
        let result, err = f()
        if err == null { return result, null }
        lastErr = err

        let retryable = false
        if classifier != null {
            retryable = classifier(err)
        } else {
            retryable = defaultClassifier(err)
        }
        if !retryable { return null, err }

        if attempt >= maxAttempts {
            return null, error("RETRY_EXHAUSTED",
                "exhausted " + str(maxAttempts) + " attempts: " + lastErr.message)
        }

        let delayMs = _backoffMs(attempt - 1, baseDelay, maxDelay, jitter)
        if deadline > 0 {
            let remainMs = deadline - _elapsedMs(startNs)
            if remainMs <= 0 {
                return null, error("RETRY_DEADLINE",
                    "deadline " + str(deadline) + "ms exceeded after " +
                    str(attempt) + " attempts: " + lastErr.message)
            }
            if delayMs > remainMs { delayMs = remainMs }
        }

        if onRetry != null { onRetry(attempt, err, delayMs) }

        sleep(delayMs)
        attempt = attempt + 1
    }

    return null, error("RETRY_EXHAUSTED",
        "exhausted " + str(maxAttempts) + " attempts")
}
