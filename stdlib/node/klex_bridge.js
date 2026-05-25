'use strict';

// klex_bridge.js — helper for writing kLex native bridges in Node.
//
// A bridge is a subprocess that speaks the kLex bridge protocol: line-delimited
// JSON over stdin/stdout. This module provides the boilerplate so bridge authors
// write only the actual functions they expose.
//
// Two equivalent ways to register handlers:
//
//   // "Decorator" style (factory wrapping a fn)
//   const {handler, streamHandler, serve} = require('klex_bridge');
//
//   handler({args: [['path', 'string']], returns: 'hash'},
//       function load(path) { return {loaded: true, path}; }
//   );
//
//   streamHandler({args: [['n', 'int']], yields: 'int'},
//       function* count(n) { for (let i = 0; i < n; i++) yield i; }
//   );
//
//   serve();
//
//   // Imperative style
//   const {register, serve} = require('klex_bridge');
//
//   function load(path) { return {loaded: true, path}; }
//   register('load', load, {args: [['path', 'string']], returns: 'hash'});
//   serve();
//
// Both populate the same registry. Mix freely.
//
// Schema mini-language (used in args, returns, yields):
//
//   'int', 'float', 'string', 'bool', 'array', 'hash', 'null', 'any'
//   Trailing '?' makes the type nullable: 'string?' accepts string or null.
//
// kLex auto-fetches every handler's schema via the special __schema__ call
// (registered automatically by serve()) and validates arguments before they
// hit the wire. This module also validates inside dispatch as defence in depth,
// so the bridge gives the same error whether or not kLex did its check first.

const readline = require('readline');


// Internal handler registry. Populated by register() (and the decorator
// wrappers around it). Keys are handler names; values are
// {fn, args, returns, stream}.
const _handlers = new Map();

// Pending cancellations addressed to in-flight streaming calls. kLex emits
// {"cancel": N} when the consumer side of a stream goes away. The readline
// 'line' handler parks ids here; the active stream loop checks between
// yields and breaks out.
const _pendingCancels = new Set();

// Per-stream backpressure state. _streams[id] = {window, inFlight, waiters}.
// Each yield increments inFlight; if it would exceed window, we park a
// Promise resolver in waiters and await it. kLex's {"ack": K, "id": M}
// message drains inFlight by K and resolves enough waiters to unblock the
// generator. Absent / window == 0 means backpressure disabled for that stream.
const _streams = new Map();


function register(name, fn, opts) {
    opts = opts || {};
    _handlers.set(name, {
        fn,
        args:    Array.isArray(opts.args) ? opts.args : [],
        returns: opts.returns || 'any',
        stream:  !!opts.stream,
    });
}


// handler({args, returns}, fn) — register fn as a single-response handler.
// The two-argument form (opts first, fn second) plays nicely with anonymous
// function expressions; for named functions, fn.name is used as the handler
// name unless opts.name is provided.
function handler(opts, fn) {
    if (typeof opts === 'function') { fn = opts; opts = {}; }
    if (typeof fn !== 'function')   throw new TypeError('handler: fn must be a function');
    register(opts.name || fn.name, fn, {
        args:    opts.args,
        returns: opts.returns,
        stream:  false,
    });
    return fn;
}


// streamHandler({args, yields}, fn) — register fn as a streaming handler.
// fn must return an iterable (sync or async). kLex consumers call it via
// bridgeStream() and receive one channel item per yielded value.
function streamHandler(opts, fn) {
    if (typeof opts === 'function') { fn = opts; opts = {}; }
    if (typeof fn !== 'function')   throw new TypeError('streamHandler: fn must be a function');
    register(opts.name || fn.name, fn, {
        args:    opts.args,
        returns: opts.yields || opts.returns,
        stream:  true,
    });
    return fn;
}


function schema() {
    const out = {};
    for (const [name, h] of _handlers) {
        if (name.startsWith('__')) continue;
        out[name] = {args: h.args, returns: h.returns, stream: h.stream};
    }
    return out;
}


// ── Protocol negotiation ────────────────────────────────────────────────────

// Wire protocol version this helper speaks. Bumped only for incompatible
// breaking changes; new additive features ship as capability flags instead.
const PROTOCOL_VERSION = 1;

// Helper version — tracks helper-only changes (new convenience APIs, bug
// fixes) that don't affect the wire format. Separate from PROTOCOL_VERSION.
const HELPER_VERSION = '0.7.0';

// Capabilities this helper supports. Negotiated against kLex's advertised
// set during the __hello__ handshake; the intersection is what's used.
const HELPER_CAPABILITIES = ['schema', 'binary'];


// ── Binary payload codec ─────────────────────────────────────────────────────
//
// Wire form for bytes is a single-entry JSON object: {"__bytes__": "<base64>"}.
// Only used when the `binary` capability has been negotiated via __hello__ —
// kLex's side enforces that, so by the time we get here we can safely walk
// and decode any such object. The walk is recursive so bytes embedded inside
// hashes and arrays round-trip transparently.

const _BYTES_WIRE_KEY = '__bytes__';

function _encodeBytesTree(value) {
    if (Buffer.isBuffer(value)) {
        return {[_BYTES_WIRE_KEY]: value.toString('base64')};
    }
    if (value instanceof Uint8Array) {
        return {[_BYTES_WIRE_KEY]: Buffer.from(value).toString('base64')};
    }
    if (Array.isArray(value)) {
        return value.map(_encodeBytesTree);
    }
    if (value !== null && typeof value === 'object') {
        const out = {};
        for (const k of Object.keys(value)) out[k] = _encodeBytesTree(value[k]);
        return out;
    }
    return value;
}

function _decodeBytesTree(value) {
    if (value === null || typeof value !== 'object') return value;
    if (Array.isArray(value)) return value.map(_decodeBytesTree);
    const keys = Object.keys(value);
    if (keys.length === 1 && keys[0] === _BYTES_WIRE_KEY && typeof value[_BYTES_WIRE_KEY] === 'string') {
        try {
            return Buffer.from(value[_BYTES_WIRE_KEY], 'base64');
        } catch (_) {
            // Malformed base64 — leave as is so the error surfaces upstream.
            return value;
        }
    }
    const out = {};
    for (const k of keys) out[k] = _decodeBytesTree(value[k]);
    return out;
}


// _hello(client) — reply to kLex's __hello__ handshake. Returns the bridge
// side's protocol version, capability set, and identity fields. kLex
// computes the negotiated capability set as the intersection of
// client.capabilities and the array returned here.
//
// The `client` argument is accepted but currently unused — captured so
// future helpers can adapt behaviour based on kLex's reported version
// without a wire change.
function _hello(_client) {
    return {
        protocol:         PROTOCOL_VERSION,
        capabilities:     HELPER_CAPABILITIES.slice(),
        helper:           'klex_bridge.js/' + HELPER_VERSION,
        language:         'node',
        language_version: process.versions.node,
    };
}


// notify(payload) — send an unsolicited message to the kLex side. Use for
// progress events or telemetry that doesn't belong on a stream. Delivered
// on the bridge's notification channel (bridgeNotifications). Payload may
// contain native Buffer / Uint8Array — _write encodes them transparently.
function notify(payload) {
    _write({notif: payload});
}


// ── Schema validation ────────────────────────────────────────────────────────

function _matches(value, schemaType) {
    if (schemaType === 'any') return true;
    const nullable = schemaType.endsWith('?');
    const base = nullable ? schemaType.slice(0, -1) : schemaType;
    if (value === null || value === undefined) {
        return nullable || base === 'null';
    }
    switch (base) {
        case 'int':    return typeof value === 'number' && Number.isInteger(value) && !Number.isNaN(value);
        case 'float':  return typeof value === 'number' && !Number.isNaN(value);
        case 'string': return typeof value === 'string';
        case 'bool':   return typeof value === 'boolean';
        case 'array':  return Array.isArray(value);
        case 'hash':   return typeof value === 'object' && value !== null && !Array.isArray(value);
        case 'null':   return value === null;
        // Until the wire format gains a native binary capability, bytes are
        // carried as base64-encoded strings on the wire. Accept either form
        // so handlers can declare 'bytes' today and switch to native binary
        // transparently when capability negotiation enables it.
        case 'bytes':  return typeof value === 'string' || Buffer.isBuffer(value) || value instanceof Uint8Array;
        default:       return true;  // unknown schema string — be permissive
    }
}


function _validateArgs(fnName, declared, actual) {
    if (actual.length !== declared.length) {
        throw new Error(`${fnName}: expected ${declared.length} arg(s), got ${actual.length}`);
    }
    for (let i = 0; i < declared.length; i++) {
        const [pname, ptype] = declared[i];
        if (!_matches(actual[i], ptype)) {
            const got = actual[i] === null ? 'null' : typeof actual[i];
            throw new Error(`${fnName}: arg ${i} '${pname}': expected ${ptype}, got ${got}`);
        }
    }
}


function _validateReturn(fnName, declared, value) {
    if (!_matches(value, declared)) {
        const got = value === null ? 'null' : typeof value;
        throw new TypeError(`${fnName}: return value: expected ${declared}, got ${got}`);
    }
}


// ── Dispatch ─────────────────────────────────────────────────────────────────

function _write(msg) {
    // Encode any native bytes (Buffer / Uint8Array) into the wire sentinel
    // before serialising so handler authors return bytes naturally.
    process.stdout.write(JSON.stringify(_encodeBytesTree(msg)) + '\n');
}


// _dispatch runs one request to completion. Wired up as the readline 'line'
// callback; runs async so multiple in-flight calls interleave naturally over
// Node's event loop (cancels addressed to a streaming call arrive via the
// same 'line' callback, get parked in _pendingCancels synchronously, and the
// stream loop sees them on its next iteration).
async function _dispatch(line) {
    let reqId = -1;
    try {
        const req = JSON.parse(line);

        // Cancel-only message (no "fn") — synchronously park, then return.
        // Whitelisted shape so we don't accidentally treat a real call that
        // happens to have a "cancel" field as cancellation.
        // Also drain any waiters parked on the backpressure window for this
        // id, otherwise a cancel that arrives while the producer is paused
        // would never be observed — no ack would come to wake it.
        if (!('fn' in req) && 'cancel' in req) {
            if (Number.isInteger(req.cancel)) {
                _pendingCancels.add(req.cancel);
                const s = _streams.get(req.cancel);
                if (s) {
                    while (s.waiters.length > 0) {
                        const resolve = s.waiters.shift();
                        resolve();
                    }
                }
            }
            return;
        }

        // Ack-only message — drains in-flight for the named stream and
        // resolves whichever waiters were parked because the window was
        // saturated. Same whitelist guard as cancel.
        if (!('fn' in req) && 'ack' in req) {
            const sid = req.id;
            const k = req.ack;
            if (Number.isInteger(sid) && Number.isInteger(k) && k > 0) {
                const s = _streams.get(sid);
                if (s) {
                    s.inFlight = Math.max(0, s.inFlight - k);
                    // Resolve as many parked waiters as the headroom allows.
                    while (s.waiters.length > 0 && s.inFlight < s.window) {
                        const resolve = s.waiters.shift();
                        s.inFlight++;
                        resolve();
                    }
                }
            }
            return;
        }

        reqId            = (typeof req.id === 'number') ? req.id : -1;
        const fnName     = req.fn;
        // Decode any wire-form bytes BEFORE validation so handlers see
        // native Buffers and the schema validator can be strict about type.
        const actual     = Array.isArray(req.args) ? _decodeBytesTree(req.args) : [];
        const streamReq  = !!req.stream;

        const h = _handlers.get(fnName);
        if (!h) {
            _write({id: reqId, error: `unknown function: ${fnName}`});
            return;
        }

        _validateArgs(fnName, h.args, actual);

        if (h.stream) {
            if (!streamReq) {
                throw new Error(`${fnName}: streaming handler — call via bridgeStream, not bridgeCall`);
            }
            _pendingCancels.delete(reqId);
            // Backpressure: register an in-flight slot when the request
            // carried a positive "window". The yield loop awaits a waiter
            // Promise when the cap is hit; the readline handler resolves
            // waiters on ack arrival.
            const window = Number.isInteger(req.window) && req.window > 0 ? req.window : 0;
            if (window > 0) {
                _streams.set(reqId, {window, inFlight: 0, waiters: []});
            }
            let cancelled = false;
            const iter = h.fn(...actual);
            try {
                for await (const item of iter) {
                    if (_pendingCancels.has(reqId)) {
                        _pendingCancels.delete(reqId);
                        cancelled = true;
                        break;
                    }
                    _validateReturn(fnName, h.returns, item);
                    // Block here when the window is saturated. The waiter
                    // Promise is resolved by the ack handler, which also
                    // increments inFlight on our behalf — so by the time we
                    // resume we already own a slot. The non-blocked path
                    // (in-flight < window) increments inline.
                    //
                    // A cancel arriving mid-wait drains the waiter list (the
                    // 'line' handler does it). When that happens our resolve
                    // fires without an inFlight increment; we re-check the
                    // cancel set and break out so we don't write a doomed item.
                    if (window > 0) {
                        const s = _streams.get(reqId);
                        if (s.inFlight >= s.window) {
                            await new Promise((resolve) => s.waiters.push(resolve));
                            if (_pendingCancels.has(reqId)) {
                                _pendingCancels.delete(reqId);
                                cancelled = true;
                                break;
                            }
                            s.inFlight++;
                        } else {
                            s.inFlight++;
                        }
                    }
                    _write({id: reqId, stream: item});
                }
            } finally {
                // for-await calls iter.return() on early break, which triggers
                // try/finally inside the generator — same guarantee as Python.
                if (iter && typeof iter.return === 'function') {
                    try { await iter.return(); } catch (_) { /* swallow */ }
                }
                if (window > 0) _streams.delete(reqId);
            }
            _write({id: reqId, stream_end: true, cancelled});
        } else {
            if (streamReq) {
                throw new Error(`${fnName}: not a streaming handler — call via bridgeCall, not bridgeStream`);
            }
            let result = h.fn(...actual);
            // Support both sync and async handlers transparently.
            if (result && typeof result.then === 'function') {
                result = await result;
            }
            // undefined disappears in JSON; bridges that return nothing get
            // null on the kLex side, matching Python's "no return → None".
            if (result === undefined) result = null;
            _validateReturn(fnName, h.returns, result);
            _write({id: reqId, result});
        }
    } catch (e) {
        // Mirror to stderr for bridgeStderr() consumers AND include structured
        // fields in the response so err.errorType / err.traceback work on the
        // kLex side without scraping stderr.
        const message   = (e && e.message) ? e.message : String(e);
        const errorType = (e && e.name)    ? e.name    : 'Error';
        const traceback = (e && e.stack)   ? e.stack   : '';
        process.stderr.write(traceback ? traceback + '\n' : message + '\n');
        _write({id: reqId, error: message, error_type: errorType, traceback});
    }
}


// serve() — run the dispatch loop. Returns immediately (Node is event-driven);
// the readline interface keeps the process alive until stdin closes, at which
// point we exit cleanly so kLex's bridgeClose path sees a clean EOF.
function serve() {
    register('__hello__', _hello, {args: [['client', 'hash']], returns: 'hash'});
    register('__schema__', schema, {args: [], returns: 'hash'});

    const rl = readline.createInterface({input: process.stdin, terminal: false});

    rl.on('line', (line) => {
        const stripped = line.trim();
        if (!stripped) return;
        // Fire-and-forget — multiple in-flight handlers are fine. Per-call
        // errors surface inside _dispatch and don't crash serve().
        _dispatch(stripped).catch((e) => {
            process.stderr.write('dispatch crash: ' + (e && e.stack || e) + '\n');
        });
    });

    rl.on('close', () => process.exit(0));
}


module.exports = {handler, streamHandler, register, notify, schema, serve};
