// stdlib/bridge.lex — ergonomic wrapper for the native bridge system.
//
// Provides a struct-based API with:
//   - Dependency checking on startup (check_deps convention)
//   - Automatic restart on BRIDGE_CLOSED / BRIDGE_TAINTED
//   - Circuit breaker: opens after N consecutive failures
//   - Notification channel forwarding
//   - Graceful close helpers
//
// Usage:
//   import "stdlib/bridge.lex" as br
//
//   b, err = br.newResilient("python3", ["my_bridge.py"], {
//       "timeout_seconds": 30,
//       "max_retries":     3,
//       "require_deps":    true,
//   })
//   if err != null { println(err.message)  return }
//
//   result, err = b.call("my_function", [arg])
//   if err != null && err.code == "CIRCUIT_OPEN" {
//       println("bridge failed too many times — manual intervention required")
//       return
//   }
//
//   b.close()

// ── ResilientBridge struct ────────────────────────────────────────────────────

struct ResilientBridge {
    _cmd, _args, _opts, _maxRetries, _failures, _circuitOpen, _bridge

    // call(fn, callArgs) → (result, err)
    // Calls fn with callArgs. On BRIDGE_CLOSED or BRIDGE_TAINTED, attempts up
    // to _maxRetries automatic restarts. Opens the circuit after too many
    // consecutive failures. Resets the failure counter on success.
    fn call(fnName, callArgs) {
        return self._dispatch(fnName, callArgs, 0)
    }

    // callTimed(fnName, callArgs, timeoutSec) → (result, err)
    // Same as call() with a per-call timeout override.
    fn callTimed(fnName, callArgs, timeoutSec) {
        return self._dispatch(fnName, callArgs, timeoutSec)
    }

    // notifications() → channel
    // Returns the bridge's notification channel. Messages arrive whenever
    // the bridge subprocess emits {"notif": ...}. Drain this in an async
    // task during long-running calls for streaming progress.
    fn notifications() {
        return bridgeNotifications(self._bridge)
    }

    // reset() — resets the circuit breaker and failure counter.
    // Does NOT restart the bridge subprocess. If the bridge is tainted, call
    // close() first, then create a new bridge with newResilient().
    fn reset() {
        self._circuitOpen = false
        self._failures    = 0
    }

    // close() — gracefully closes the bridge subprocess.
    fn close() {
        if self._bridge != null {
            bridgeClose(self._bridge)
            self._bridge = null
        }
    }

    // ── Internal ─────────────────────────────────────────────────────────────

    fn _dispatch(fnName, callArgs, timeoutSec) {
        if self._circuitOpen == true {
            return null, error("CIRCUIT_OPEN",
                "bridge circuit is open after " + str(self._maxRetries) +
                " consecutive failures — call reset() to try again")
        }
        if self._bridge == null {
            return null, error("BRIDGE_CLOSED", "bridge has been closed")
        }

        if timeoutSec > 0 {
            result, err = bridgeCall(self._bridge, fnName, callArgs, timeoutSec)
        } else {
            result, err = bridgeCall(self._bridge, fnName, callArgs)
        }

        if err != null {
            if err.code == "BRIDGE_CLOSED" || err.code == "BRIDGE_TAINTED" {
                // Attempt automatic restart.
                self._failures = self._failures + 1
                if self._failures >= self._maxRetries {
                    self._circuitOpen = true
                    return null, error("CIRCUIT_OPEN",
                        "bridge circuit opened: " + str(self._failures) +
                        " consecutive failures (last: " + err.message + ")")
                }
                rerr = self._restart()
                if rerr != null {
                    return null, rerr
                }
                // One retry after successful restart.
                if timeoutSec > 0 {
                    retryResult, retryErr = bridgeCall(self._bridge, fnName, callArgs, timeoutSec)
                } else {
                    retryResult, retryErr = bridgeCall(self._bridge, fnName, callArgs)
                }
                if retryErr == null { self._failures = 0 }
                return retryResult, retryErr
            }
            // Non-retriable error (BRIDGE_ERROR, BRIDGE_TIMEOUT, etc.) —
            // return as-is.  BRIDGE_TIMEOUT also taints, but let the caller
            // decide whether to restart or not.
            return null, err
        }

        self._failures = 0
        return result, null
    }

    fn _restart() {
        bridgeClose(self._bridge)
        newBridge, err = nativeBridge(self._cmd, self._args, self._opts)
        if err != null { return err }

        // Health check — bridges that implement health_check() confirm they
        // are alive. BRIDGE_ERROR with "unknown function" is acceptable (bridge
        // simply doesn't implement the convention). Any other error = restart failed.
        _, hcErr = bridgeCall(newBridge, "health_check", [], 5)
        if hcErr != null && hcErr.code != "BRIDGE_ERROR" {
            bridgeClose(newBridge)
            return hcErr
        }

        self._bridge = newBridge
        return null
    }
}

// ── Constructor ───────────────────────────────────────────────────────────────

// newResilient(cmd, args, opts?) → (ResilientBridge, err)
//
// Creates a bridge and wraps it in a ResilientBridge. Options:
//   All nativeBridge opts are forwarded (timeout_seconds, max_response_mb, stderr_log).
//   max_retries     — how many consecutive BRIDGE_CLOSED/BRIDGE_TAINTED failures
//                     before the circuit opens (default: 3)
//   require_deps    — if true, calls check_deps() on startup and returns an error
//                     if the bridge reports missing dependencies (default: false)
fn newResilient(cmd, args, opts) {
    maxRetries = 3
    requireDeps = false
    if opts != null {
        if opts["max_retries"] != null  { maxRetries  = opts["max_retries"]  }
        if opts["require_deps"] != null { requireDeps = opts["require_deps"] }
    }

    b, err = nativeBridge(cmd, args, opts)
    if err != null { return null, err }

    if requireDeps == true {
        depsResult, derr = bridgeCall(b, "check_deps", [], 10)
        if derr != null {
            bridgeClose(b)
            return null, derr
        }
        if depsResult["ok"] == false {
            missing = depsResult["missing"]
            msg = "missing dependencies"
            if missing != null && len(missing) > 0 {
                i = 0
                parts = makeArray(len(missing), "")
                while i < len(missing) { parts[i] = str(missing[i])  i = i + 1 }
                msg = "missing dependencies: " + join(parts, ", ")
            }
            bridgeClose(b)
            return null, error("MISSING_DEPS", msg)
        }
    }

    rb = ResilientBridge {
        _cmd:         cmd,
        _args:        args,
        _opts:        opts,
        _maxRetries:  maxRetries,
        _failures:    0,
        _circuitOpen: false,
        _bridge:      b,
    }
    return rb, null
}

// ── Utility helpers ───────────────────────────────────────────────────────────

// withBridge(cmd, args, opts, callback) → (result, err)
//
// Creates a plain (non-resilient) bridge, passes it to callback, then closes
// it — even if callback throws. Useful for one-shot bridge operations.
//
// Example:
//   result, err = br.withBridge("python3", ["tool.py"], null, fn(b) {
//       return bridgeCall(b, "do_work", [input])
//   })
fn withBridge(cmd, args, opts, callback) {
    b, err = nativeBridge(cmd, args, opts)
    if err != null { return null, err }
    result, cerr = safe(fn() { return callback(b) })
    bridgeClose(b)
    if cerr != null { return null, cerr }
    return result, null
}

// isRetriable(err) → bool
// Returns true for errors that indicate a bridge restart may help.
fn isRetriable(err) {
    if err == null { return false }
    return err.code == "BRIDGE_CLOSED" || err.code == "BRIDGE_TAINTED"
}

// isCircuitOpen(err) → bool
// Returns true when the circuit breaker has opened (too many failures).
fn isCircuitOpen(err) {
    if err == null { return false }
    return err.code == "CIRCUIT_OPEN"
}
