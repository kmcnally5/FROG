#!/usr/bin/env python3
"""
klex_bridge.py — helper for writing kLex native bridges.

A bridge is a subprocess that speaks the kLex bridge protocol: line-delimited
JSON over stdin/stdout. This module provides the boilerplate so bridge authors
write only the actual functions they expose.

Two equivalent ways to register handlers:

    # Decorator
    from klex_bridge import handler, serve

    @handler(args=[("path", "string")], returns="hash")
    def load(path):
        return {"loaded": True, "path": path}

    serve()

    # Imperative
    from klex_bridge import register, serve

    def load(path):
        return {"loaded": True, "path": path}

    register("load", load, args=[("path", "string")], returns="hash")
    serve()

Both populate the same internal registry. Mix them freely.

Schema mini-language (used in args and returns):

    "int", "float", "string", "bool", "array", "hash", "null", "any"
    Trailing "?" makes the type nullable: "string?" accepts string or None.

kLex auto-fetches every handler's schema via the special __schema__ call
(registered automatically by serve()), and validates arguments before they
hit the wire. This module also validates inside serve() as defence in depth,
so the bridge gives the same error whether or not kLex did its check first.
"""
import json
import sys
import traceback


# Internal handler registry. Populated by handler() and register().
# Keyed by handler name; values are {"fn": callable, "args": [...], "returns": str}.
_HANDLERS = {}


def handler(*, name=None, args=None, returns="any"):
    """Decorator. Register the wrapped function as a bridge handler.

    name:    explicit handler name; defaults to fn.__name__
    args:    list of (param_name, schema_type) tuples
    returns: schema_type string ("hash", "array", "int", ...)
    """
    def wrap(fn):
        register(name or fn.__name__, fn, args=args, returns=returns)
        return fn
    return wrap


def register(name, fn, *, args=None, returns="any"):
    """Imperative handler registration. Same registry as @handler."""
    _HANDLERS[name] = {
        "fn":      fn,
        "args":    list(args) if args else [],
        "returns": returns,
    }


def schema():
    """Return the schema for every registered user handler.

    The dispatch loop exposes this as the special __schema__ call so kLex
    can pull the bridge's signature during handshake. Internal names that
    start with __ are excluded so __schema__ itself doesn't show up.
    """
    return {
        name: {"args": h["args"], "returns": h["returns"]}
        for name, h in _HANDLERS.items()
        if not name.startswith("__")
    }


def notify(payload):
    """Send an unsolicited notification to the kLex side.

    Use during long-running handlers to stream progress. The kLex bridge
    delivers these on the bridge's notification channel — bridges receive
    them via bridgeNotifications(bridge).

    Payload may be any JSON-serialisable value.
    """
    sys.stdout.write(json.dumps({"notif": payload}) + "\n")
    sys.stdout.flush()


# ── Schema validation ────────────────────────────────────────────────────────

def _matches(value, schema_type):
    """True iff value satisfies schema_type.

    Schema mini-language is documented at the top of this module. Unknown
    schema strings are accepted permissively rather than rejected, so new
    types added on the kLex side don't break older Python helpers.
    """
    if schema_type == "any":
        return True

    nullable = schema_type.endswith("?")
    base = schema_type[:-1] if nullable else schema_type

    if value is None:
        return nullable or base == "null"

    if base == "int":
        # bool is a subclass of int in Python — exclude explicitly.
        return isinstance(value, int) and not isinstance(value, bool)
    if base == "float":
        # JSON has only "number" — accept Python ints in float slots.
        return isinstance(value, (int, float)) and not isinstance(value, bool)
    if base == "string":
        return isinstance(value, str)
    if base == "bool":
        return isinstance(value, bool)
    if base == "array":
        return isinstance(value, list)
    if base == "hash":
        return isinstance(value, dict)
    if base == "null":
        return value is None

    # Unknown — be permissive.
    return True


def _validate_args(fn_name, declared, actual):
    """Raise ValueError if actual args don't match declared schema."""
    if len(actual) != len(declared):
        raise ValueError(
            f"{fn_name}: expected {len(declared)} arg(s), got {len(actual)}"
        )
    for i, (pname, ptype) in enumerate(declared):
        if not _matches(actual[i], ptype):
            got = type(actual[i]).__name__
            raise ValueError(
                f"{fn_name}: arg {i} '{pname}': expected {ptype}, got {got}"
            )


# ── Dispatch loop ────────────────────────────────────────────────────────────

def serve():
    """Run the dispatch loop. Blocks until stdin closes (kLex closed the bridge).

    Registers __schema__ automatically so kLex can introspect the bridge.
    Validates incoming args against each handler's declared schema before
    invoking the handler. Exceptions become {"id": N, "error": "..."} responses
    with the traceback written to stderr for bridgeStderr() capture.
    """
    register("__schema__", schema, args=[], returns="hash")

    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue

        req_id = -1
        try:
            req     = json.loads(line)
            req_id  = req.get("id", -1)
            fn_name = req["fn"]
            actual  = req.get("args", [])

            h = _HANDLERS.get(fn_name)
            if h is None:
                resp = {"id": req_id, "error": f"unknown function: {fn_name}"}
            else:
                _validate_args(fn_name, h["args"], actual)
                resp = {"id": req_id, "result": h["fn"](*actual)}
        except Exception as e:
            # Traceback to stderr so kLex can surface it via bridgeStderr().
            traceback.print_exc(file=sys.stderr)
            resp = {"id": req_id, "error": str(e)}

        sys.stdout.write(json.dumps(resp) + "\n")
        sys.stdout.flush()
