#!/usr/bin/env python3
"""
kLex native bridge example — Python side.

Demonstrates the schema-aware bridge style using the klex_bridge helper:
  - @handler decorator declares each function's args and return type
  - serve() runs the standard dispatch loop and exposes __schema__ to kLex
"""
import math

from klex_bridge import handler, serve


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


serve()
