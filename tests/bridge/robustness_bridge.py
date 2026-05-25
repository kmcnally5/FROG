#!/usr/bin/env python3
"""
Robustness test bridge for kLex Phase 1 bridge improvements.

Each handler deliberately exercises a different failure mode so the kLex
test script can verify the bridge layer reports the correct error code.

Handlers:
  echo(x)               — sanity check; returns x unchanged
  hang_forever()        — sleeps 60s; used to test BRIDGE_TIMEOUT
  produce_huge(size_mb) — returns a string of size_mb megabytes
                          (forces a buffer-overflow read error)
  crash_with_traceback()— writes a fake traceback to stderr, then raises
  kill_self()           — exits the subprocess via os._exit(1)
                          (used to test BRIDGE_CLOSED)
"""
import json
import os
import sys
import time
import traceback


HANDLERS = {}


def _handler(name):
    def decorator(fn):
        HANDLERS[name] = fn
        return fn
    return decorator


@_handler("echo")
def echo(x):
    return x


@_handler("hang_forever")
def hang_forever():
    time.sleep(60)
    return "should not reach here"


@_handler("produce_huge")
def produce_huge(size_mb):
    # Build a string of approximately size_mb megabytes.
    chunk = "x" * (1024 * 1024)
    return chunk * int(size_mb)


@_handler("crash_with_traceback")
def crash_with_traceback():
    # Write a recognisable traceback to stderr first so the kLex side
    # can verify it shows up via bridgeStderr() and in error tails.
    try:
        raise RuntimeError("deliberate failure for robustness test")
    except RuntimeError:
        traceback.print_exc(file=sys.stderr)
        sys.stderr.flush()
        raise


@_handler("kill_self")
def kill_self():
    # Hard exit — bypasses the bridge loop's response, so the kLex side
    # should see BRIDGE_CLOSED on its read.
    sys.stdout.flush()
    os._exit(1)


# ── Phase 2 handlers ─────────────────────────────────────────────────────────

def notify(data):
    """Emit a server-push notification to the kLex notification channel."""
    print(json.dumps({"notif": data}), flush=True)


@_handler("health_check")
def health_check():
    return {"ok": True, "version": "robustness_bridge/1.0"}


@_handler("slow_with_notifications")
def slow_with_notifications(steps):
    """
    Emits `steps` progress notifications at 20ms intervals, then returns.
    Used to verify that notifications stream while bridgeCall is in flight.
    """
    for i in range(int(steps)):
        notify({"phase": "progress", "done": i + 1, "total": steps})
        time.sleep(0.02)
    return {"result": "done", "steps": steps}


@_handler("echo_concurrent")
def echo_concurrent(value, delay_ms):
    """
    Returns value after delay_ms milliseconds.
    Multiple concurrent calls with different delays verify id-based routing.
    """
    time.sleep(delay_ms / 1000.0)
    return {"value": value, "delay_ms": delay_ms}


# ── Bridge loop ──────────────────────────────────────────────────────────────
for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    req = None
    try:
        req     = json.loads(line)
        req_id  = req["id"]
        fn_name = req["fn"]
        args    = req.get("args", [])
        if fn_name not in HANDLERS:
            resp = {"id": req_id, "error": f"unknown function: {fn_name}"}
        else:
            result = HANDLERS[fn_name](*args)
            resp   = {"id": req_id, "result": result}
    except Exception as e:
        resp = {"id": req.get("id", 0) if req else 0, "error": str(e)}
    print(json.dumps(resp), flush=True)
