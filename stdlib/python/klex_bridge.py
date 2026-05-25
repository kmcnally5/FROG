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
import base64
import json
import queue
import sys
import threading
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


def register(name, fn, *, args=None, returns="any", stream=False):
    """Imperative handler registration. Same registry as @handler.

    Set stream=True to register a streaming handler — fn must return an
    iterable/generator. kLex consumers call it via bridgeStream() and
    receive one channel value per yielded item.
    """
    _HANDLERS[name] = {
        "fn":      fn,
        "args":    list(args) if args else [],
        "returns": returns,
        "stream":  stream,
    }


def stream_handler(*, name=None, args=None, yields="any"):
    """Decorator: register the wrapped function as a STREAMING bridge handler.

    The decorated function must be a generator (use `yield`) or otherwise
    return an iterable. kLex callers receive a channel and consume yielded
    items one at a time. Each item is type-checked against `yields` (same
    schema mini-language as `returns`).

    Use @handler for single-response functions, @stream_handler for
    iterators / progress streams / chunked downloads.
    """
    def wrap(fn):
        register(name or fn.__name__, fn, args=args, returns=yields, stream=True)
        return fn
    return wrap


def schema():
    """Return the schema for every registered user handler.

    The dispatch loop exposes this as the special __schema__ call so kLex
    can pull the bridge's signature during handshake. Internal names that
    start with __ are excluded so __schema__ itself doesn't show up.

    Each entry carries:
      args    — list of [param_name, type_string] pairs
      returns — type string for the return value (single-response handlers)
                or for each yielded item (streaming handlers)
      stream  — true for streaming handlers (callable via bridgeStream);
                false for single-response handlers (bridgeCall).
    """
    return {
        name: {
            "args":    h["args"],
            "returns": h["returns"],
            "stream":  h.get("stream", False),
        }
        for name, h in _HANDLERS.items()
        if not name.startswith("__")
    }


# ── Protocol negotiation ────────────────────────────────────────────────────

# Wire protocol version this helper speaks. Bumped only for incompatible
# breaking changes; new additive features ship as capability flags instead.
PROTOCOL_VERSION = 1

# Helper version — separate from PROTOCOL_VERSION. Tracks helper-only changes
# (new convenience APIs, bug fixes) that don't affect the wire format.
HELPER_VERSION = "0.7.0"

# Capabilities this helper supports. Negotiated against kLex's advertised set
# during the __hello__ handshake; the intersection is what's actually used.
HELPER_CAPABILITIES = ["schema", "binary"]


def _hello(client):
    """Reply to kLex's __hello__ handshake.

    Returns the bridge side's protocol version, capability set, and
    identity fields. kLex computes the negotiated capability set as the
    intersection of `client["capabilities"]` and the array returned here.

    Argument shape (set by kLex):
      { "protocol": N, "capabilities": [...], "client": "klex" }

    The `client` argument is accepted but currently unused — captured so
    future helpers can adapt behaviour based on kLex's reported version
    without a wire change.
    """
    return {
        "protocol":         PROTOCOL_VERSION,
        "capabilities":     list(HELPER_CAPABILITIES),
        "helper":           "klex_bridge.py/" + HELPER_VERSION,
        "language":         "python",
        "language_version": "%d.%d.%d" % sys.version_info[:3],
    }


def notify(payload):
    """Send an unsolicited notification to the kLex side.

    Use during long-running handlers to stream progress. The kLex bridge
    delivers these on the bridge's notification channel — bridges receive
    them via bridgeNotifications(bridge).

    Payload may be any JSON-serialisable value or include native bytes; the
    bytes get encoded into the wire sentinel transparently via _write.
    """
    _write({"notif": payload})


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
    if base == "bytes":
        # Until the wire format gains a native binary capability, bytes are
        # carried as base64-encoded strings on the wire. Accept either form
        # so handlers can declare "bytes" today and switch to native binary
        # transparently when capability negotiation enables it.
        return isinstance(value, (bytes, bytearray, str))

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


def _validate_return(fn_name, declared, value):
    """Raise TypeError if a handler's return value doesn't match its declared type.

    Caught and surfaced to kLex as an ordinary error response — bridge stays
    alive, the bridge author sees the violation immediately. "any" return
    type (the default) accepts everything except null; declare nullable
    explicitly with "any?" if your handler may return None.
    """
    if not _matches(value, declared):
        got = type(value).__name__ if value is not None else "None"
        raise TypeError(
            f"{fn_name}: return value: expected {declared}, got {got}"
        )


# ── Binary payload codec ─────────────────────────────────────────────────────
#
# Wire form for bytes is a single-entry JSON object: {"__bytes__": "<base64>"}.
# Only used when the `binary` capability has been negotiated via __hello__ —
# kLex's side enforces that, so by the time we get here we can safely walk and
# decode any such object. The walk is recursive so bytes embedded inside hashes
# and arrays round-trip transparently.

_BYTES_WIRE_KEY = "__bytes__"


def _encode_bytes_tree(value):
    """Replace native bytes/bytearray with the wire sentinel everywhere in
    value. Dicts and lists are walked recursively; tuples are converted to
    lists (JSON has no tuple form). Strings and other primitives pass through
    untouched."""
    if isinstance(value, (bytes, bytearray)):
        return {_BYTES_WIRE_KEY: base64.b64encode(bytes(value)).decode("ascii")}
    if isinstance(value, dict):
        return {k: _encode_bytes_tree(v) for k, v in value.items()}
    if isinstance(value, (list, tuple)):
        return [_encode_bytes_tree(v) for v in value]
    return value


def _decode_bytes_tree(value):
    """Inverse of _encode_bytes_tree. A dict with exactly one key
    "__bytes__" whose value is a string is decoded to native bytes; all other
    dicts and lists are walked recursively. Malformed base64 falls through as
    the original dict so the error surfaces at the handler level rather than
    being silently swallowed."""
    if isinstance(value, dict):
        if len(value) == 1 and _BYTES_WIRE_KEY in value and isinstance(value[_BYTES_WIRE_KEY], str):
            try:
                return base64.b64decode(value[_BYTES_WIRE_KEY], validate=True)
            except Exception:
                return value
        return {k: _decode_bytes_tree(v) for k, v in value.items()}
    if isinstance(value, list):
        return [_decode_bytes_tree(v) for v in value]
    return value


# ── Dispatch loop ────────────────────────────────────────────────────────────

def _write(msg):
    """Serialise msg and flush. Centralised so streaming and single-response
    paths use the same line-delimited JSON write. Walks the message tree to
    encode any native bytes into the wire sentinel form first — bridge
    authors return bytes naturally and the wire form happens for them."""
    sys.stdout.write(json.dumps(_encode_bytes_tree(msg)) + "\n")
    sys.stdout.flush()


# Pending cancellations addressed to in-flight streaming calls. kLex emits
# {"cancel": N} when the consumer side of a stream goes away (for-in break
# or the channel was otherwise abandoned). The reader thread parks ids
# here; the active stream loop checks between yields and breaks out.
_pending_cancels = set()

# Non-cancel lines (e.g. follow-up bridgeCall requests that arrived during
# a stream) live in this queue and are consumed by the main dispatch loop.
# Bounded only by available memory; in practice kLex serialises requests
# per bridge, so depth stays small.
_request_queue = queue.Queue()

# Per-stream backpressure state. _stream_window[id] is the max in-flight cap
# (set when the request arrives); _stream_in_flight[id] is the count of items
# yielded but not yet acked by kLex. A condition variable per stream lets the
# yield loop wait when full and wake when an ack arrives. window==0 means
# backpressure disabled for that stream — the loop never blocks.
_stream_window = {}
_stream_in_flight = {}
_stream_cv = {}
_stream_lock = threading.Lock()


def _reader_loop():
    """Background thread: pull lines off sys.stdin, classify, dispatch.

    Three message shapes are routed off the request_queue path so streaming
    handlers can react to them between yields:
      - {"cancel": N}            → parked in _pending_cancels
      - {"ack": K, "id": M}      → bumps _stream_in_flight[M] down and wakes
                                    the per-stream condition variable
      - everything else          → forwarded to the main dispatch queue

    Reading from a dedicated thread sidesteps the trap that bit an earlier
    select()-based attempt: Python's TextIOWrapper can buffer lines that
    select() on the underlying FD doesn't see, so a cancel could arrive
    yet remain invisible until the next blocking readline(). A blocking
    readline in a thread is the simplest reliable demultiplexer.
    """
    while True:
        line = sys.stdin.readline()
        if not line:
            # EOF — kLex closed the bridge. Wake the main thread and any
            # streaming loops blocked on a window so they can unwind.
            _request_queue.put(None)
            with _stream_lock:
                for cv in _stream_cv.values():
                    cv.notify_all()
            return
        stripped = line.strip()
        if not stripped:
            continue
        try:
            msg = json.loads(stripped)
        except Exception:
            # Malformed — pass through so the main loop's error path logs it.
            _request_queue.put(line)
            continue
        # Cancel — addressed to a streaming handler, parked for the loop.
        # Also wake any stream loop blocked on the backpressure window for
        # this id, otherwise a cancel that arrives while we're in cv.wait
        # would only be observed on the next ack — and if the bridge had
        # paused exactly because the window was full, no ack would ever
        # come without a wake. Race condition we hit in early testing.
        if "fn" not in msg and "cancel" in msg:
            cid = msg.get("cancel")
            if isinstance(cid, int):
                _pending_cancels.add(cid)
                with _stream_lock:
                    cv = _stream_cv.get(cid)
                    if cv is not None:
                        cv.notify_all()
            continue
        # Ack — kLex signalling it consumed K items from stream M. Bump the
        # in-flight counter down and wake whichever yield loop is waiting.
        if "fn" not in msg and "ack" in msg:
            sid = msg.get("id")
            k = msg.get("ack")
            if isinstance(sid, int) and isinstance(k, int) and k > 0:
                with _stream_lock:
                    if sid in _stream_in_flight:
                        _stream_in_flight[sid] -= k
                        if _stream_in_flight[sid] < 0:
                            _stream_in_flight[sid] = 0
                        cv = _stream_cv.get(sid)
                        if cv is not None:
                            cv.notify_all()
            continue
        _request_queue.put(line)


def _next_request_line():
    """Yield the next line of input for the dispatch loop. Blocks on the
    request queue; the reader thread feeds it. Returns when EOF arrives."""
    while True:
        line = _request_queue.get()
        if line is None:
            return
        yield line


def serve():
    """Run the dispatch loop. Blocks until stdin closes (kLex closed the bridge).

    Registers __schema__ automatically so kLex can introspect the bridge.
    Validates incoming args against each handler's declared schema before
    invoking the handler.

    Two response modes:
      - single-response (req without "stream": true) → emits {"id": N, "result": ...}
        once per request. Used by kLex's bridgeCall().
      - streaming (req with "stream": true on a @stream_handler) → emits one
        {"id": N, "stream": item} per yielded value, then {"id": N, "stream_end": true}.
        Used by kLex's bridgeStream().

    Errors at any point (parse, validation, exception inside the handler)
    surface as {"id": N, "error": "...", "error_type": "...", "traceback": "..."}.
    For streams, an error message terminates the stream — no stream_end
    follows.
    """
    register("__hello__", _hello, args=[("client", "hash")], returns="hash")
    register("__schema__", schema, args=[], returns="hash")

    # Start the background reader. It owns sys.stdin from now on; the main
    # thread reads classified lines from _request_queue.
    threading.Thread(target=_reader_loop, daemon=True).start()

    for line in _next_request_line():
        line = line.strip()
        if not line:
            continue

        req_id = -1
        try:
            req         = json.loads(line)
            req_id      = req.get("id", -1)
            fn_name     = req["fn"]
            actual      = req.get("args", [])
            stream_req  = bool(req.get("stream", False))

            h = _HANDLERS.get(fn_name)
            if h is None:
                _write({"id": req_id, "error": f"unknown function: {fn_name}"})
                continue

            # Decode any wire-form bytes BEFORE validation so handlers see
            # native Python bytes and the schema validator can be strict about
            # the type. Round-trip discipline: encode on _write, decode here.
            actual = _decode_bytes_tree(actual)

            _validate_args(fn_name, h["args"], actual)

            if h["stream"]:
                if not stream_req:
                    raise ValueError(
                        f"{fn_name}: streaming handler — call via bridgeStream, not bridgeCall"
                    )
                # Iterate the generator/iterable. Each yielded item is
                # validated against the declared per-item type and sent as
                # its own stream message. The reader thread fills
                # _pending_cancels asynchronously; we just check the set
                # between yields and break out if our id appears, closing
                # the generator so the handler's finally blocks still run.
                #
                # Backpressure: if the request carried a positive "window",
                # we register an in-flight counter for this stream id. Each
                # write bumps it; we block on a condition variable when it
                # hits the cap; the reader thread's ack-handler wakes us.
                _pending_cancels.discard(req_id)
                window = req.get("window")
                if not isinstance(window, int) or window < 0:
                    window = 0
                if window > 0:
                    with _stream_lock:
                        _stream_window[req_id] = window
                        _stream_in_flight[req_id] = 0
                        _stream_cv[req_id] = threading.Condition(_stream_lock)
                gen = h["fn"](*actual)
                cancelled = False
                try:
                    for item in gen:
                        if req_id in _pending_cancels:
                            _pending_cancels.discard(req_id)
                            cancelled = True
                            break
                        _validate_return(fn_name, h["returns"], item)
                        # Block until in-flight drops below the window. The
                        # cv is signalled by the reader thread on ack, on
                        # cancel for this id, or on EOF — so a wait can
                        # never live longer than the events that should
                        # legitimately wake it.
                        if window > 0:
                            wait_cancelled = False
                            with _stream_lock:
                                cv = _stream_cv[req_id]
                                while _stream_in_flight[req_id] >= window:
                                    if req_id in _pending_cancels:
                                        wait_cancelled = True
                                        break
                                    cv.wait()
                                if req_id in _pending_cancels:
                                    wait_cancelled = True
                                if not wait_cancelled:
                                    _stream_in_flight[req_id] += 1
                            if wait_cancelled:
                                _pending_cancels.discard(req_id)
                                cancelled = True
                                break
                        _write({"id": req_id, "stream": item})
                finally:
                    if hasattr(gen, "close"):
                        gen.close()
                    if window > 0:
                        with _stream_lock:
                            _stream_window.pop(req_id, None)
                            _stream_in_flight.pop(req_id, None)
                            _stream_cv.pop(req_id, None)
                _write({"id": req_id, "stream_end": True, "cancelled": cancelled})
            else:
                if stream_req:
                    raise ValueError(
                        f"{fn_name}: not a streaming handler — call via bridgeCall, not bridgeStream"
                    )
                result = h["fn"](*actual)
                _validate_return(fn_name, h["returns"], result)
                _write({"id": req_id, "result": result})

        except Exception as e:
            # Mirror traceback to stderr so bridgeStderr() still works for
            # legacy consumers, and ALSO include structured fields in the
            # response so kLex's Error object can expose error_type +
            # traceback programmatically — no need to scrape stderr.
            tb = traceback.format_exc()
            sys.stderr.write(tb)
            sys.stderr.flush()
            _write({
                "id":         req_id,
                "error":      str(e),
                "error_type": type(e).__name__,
                "traceback":  tb,
            })
