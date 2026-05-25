#!/usr/bin/env python3
"""
kLex native bridge example — Python side.

Demonstrates the schema-aware bridge style using the klex_bridge helper:
  - @handler decorator declares each function's args and return type
  - serve() runs the standard dispatch loop and exposes __schema__ to kLex
"""
import math
import time

from klex_bridge import handler, stream_handler, serve


# Tracks how many items the cancel_demo stream actually produced before it was
# interrupted. The kLex side reads this back via the get_cancel_count() handler
# to prove the bridge stopped yielding on cancel.
_cancel_count = 0


@handler(args=[("a", "int"), ("b", "int")], returns="int")
def add(a, b):
    return a + b


@handler(args=[("a", "int"), ("b", "int")], returns="int")
def multiply(a, b):
    return a * b


@handler(args=[("name", "string")], returns="string")
def greet(name):
    return f"Hello from Python, {name}!"


@handler(args=[("numbers", "array")], returns="hash")
def stats(numbers):
    n = len(numbers)
    if n == 0:
        return {"count": 0, "sum": 0, "mean": 0, "min": 0, "max": 0}
    total = sum(numbers)
    return {
        "count": n,
        "sum":   total,
        "mean":  total / n,
        "min":   min(numbers),
        "max":   max(numbers),
    }


@handler(args=[("sentence", "string")], returns="string")
def reverse_words(sentence):
    return " ".join(sentence.split()[::-1])


@handler(args=[("n", "int")], returns="bool")
def is_prime(n):
    if n < 2:
        return False
    for i in range(2, int(math.sqrt(n)) + 1):
        if n % i == 0:
            return False
    return True


@handler(args=[("limit", "int")], returns="array")
def primes_up_to(limit):
    return [n for n in range(2, limit + 1) if is_prime(n)]


# Deliberately misbehaving handler — declared returns="int" but returns a string.
# Used by schemaTest.lex to verify return-type validation catches type drift.
@handler(args=[], returns="int")
def lies_about_return():
    return "I claim to return int but actually return a string"


# Deliberately-raising handler. Used by schemaTest.lex to verify the structured
# error fields (error_type + traceback) make it across the bridge intact.
@handler(args=[("path", "string")], returns="string")
def open_missing(path):
    with open(path) as f:
        return f.read()


# Streaming handler — yields one int at a time. Exercises bridgeStream().
@stream_handler(args=[("start", "int"), ("count", "int")], yields="int")
def count_from(start, count):
    for i in range(count):
        yield start + i


# Streaming handler that raises mid-stream. Verifies that errors during a
# stream are surfaced as a final *Error item on the kLex channel.
@stream_handler(args=[], yields="int")
def broken_stream():
    yield 1
    yield 2
    raise ValueError("intentional mid-stream failure")


# Slow streaming handler used by cancelTest.lex. Yields up to `count` ints,
# sleeping `delay_ms` milliseconds between each so the kLex consumer has time
# to break out of the for-in loop. The module-level _cancel_count is bumped
# each time we successfully yield — kLex reads it back after the break to
# verify the bridge stopped producing well before reaching `count`.
@stream_handler(args=[("count", "int"), ("delay_ms", "int")], yields="int")
def cancel_demo(count, delay_ms):
    global _cancel_count
    _cancel_count = 0
    delay = delay_ms / 1000.0
    for i in range(count):
        _cancel_count = i + 1   # bump BEFORE yield so the count reflects items
        yield i                 # the consumer actually received before cancel
        if delay > 0:
            time.sleep(delay)


@handler(args=[], returns="int")
def get_cancel_count():
    return _cancel_count


# Deliberately tears down the bridge subprocess. Used by poolTest to verify
# that bridgePoolHealth and pool.pick() notice a member that's gone tainted
# mid-session and route around it. os._exit() skips Python's normal shutdown
# so the bridge dies abruptly — exactly what we'd see in a real crash.
@handler(args=[], returns="string")
def suicide():
    import os
    os._exit(7)


# Backpressure regression: an unbounded fast producer that records how many
# items it actually yielded before being asked to stop. With no window,
# yields all `count` items before the consumer ever reads a second one. With
# a window of W, blocks after W items in flight — yielded count tracks the
# consumer's progress instead of running ahead.
_produced_count = 0

@stream_handler(args=[("count", "int")], yields="int")
def fast_producer(count):
    global _produced_count
    _produced_count = 0
    for i in range(count):
        _produced_count = i + 1
        yield i

@handler(args=[], returns="int")
def get_produced_count():
    return _produced_count


# ── Binary payload round-trip handlers (binaryTest.lex) ────────────────────

# Echo back the bytes unchanged — used to verify the wire round-trip preserves
# every byte value, including 0x00 and high-bit bytes.
@handler(args=[("data", "bytes")], returns="bytes")
def echo_bytes(data):
    assert isinstance(data, bytes), f"expected bytes, got {type(data).__name__}"
    return data


# Generate N bytes of deterministic content (n & 0xff per index). Used by
# kLex side to check large-payload round-trips without having to ship a
# matching MB-sized literal.
@handler(args=[("n", "int")], returns="bytes")
def make_bytes(n):
    return bytes(i & 0xff for i in range(n))


# Accept a hash with a bytes field and return one with an enlarged bytes
# field. Verifies that nested bytes round-trip through both directions.
@handler(args=[("h", "hash")], returns="hash")
def grow_bytes_in_hash(h):
    data = h["data"]
    assert isinstance(data, bytes), f"expected bytes, got {type(data).__name__}"
    return {"data": data + data, "name": h.get("name", "")}


# Streaming bytes — yields N chunks, each `size` bytes long, with the chunk
# index encoded in the first byte for verification.
@stream_handler(args=[("count", "int"), ("size", "int")], yields="bytes")
def stream_bytes(count, size):
    for i in range(count):
        chunk = bytes([i & 0xff]) + bytes(size - 1) if size > 0 else b""
        yield chunk


serve()
