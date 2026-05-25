#!/usr/bin/env python3
"""
old_bridge.py — a deliberately legacy bridge that does NOT use klex_bridge
and does NOT implement __hello__ or __schema__. Exists solely to verify
that fetchBridgeHello() / fetchBridgeSchemas() degrade gracefully when
talking to a bridge that predates negotiation.

The shape mirrors the very first hand-rolled bridges before the helper
modules existed: read a JSON line, dispatch on `fn`, write a JSON line.
Every unknown function gets an explicit "unknown function" error — which
is exactly what kLex's handshake code treats as the "this is protocol 0"
signal.
"""
import json
import sys


def echo(value):
    return value


def add(a, b):
    return a + b


_HANDLERS = {
    "echo": echo,
    "add":  add,
}


def main():
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            req = json.loads(line)
        except json.JSONDecodeError as e:
            sys.stderr.write("bad JSON: " + str(e) + "\n")
            continue

        req_id = req.get("id", -1)
        fn_name = req.get("fn", "")
        args = req.get("args", [])

        if fn_name in _HANDLERS:
            try:
                result = _HANDLERS[fn_name](*args)
                sys.stdout.write(json.dumps({"id": req_id, "result": result}) + "\n")
            except Exception as e:
                sys.stdout.write(json.dumps({
                    "id":    req_id,
                    "error": str(e),
                }) + "\n")
        else:
            sys.stdout.write(json.dumps({
                "id":    req_id,
                "error": "unknown function: " + fn_name,
            }) + "\n")
        sys.stdout.flush()


if __name__ == "__main__":
    main()
