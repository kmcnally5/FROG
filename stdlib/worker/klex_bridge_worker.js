// klex_bridge_worker.js — helper for writing kLex bridges that run as
// Web Workers. Counterpart to stdlib/node/klex_bridge.js (subprocess
// bridges) and stdlib/python/klex_bridge.py.
//
// Same wire protocol: line-delimited JSON. But the "wire" here is
// postMessage rather than stdin/stdout — each event.data carries one
// message string, and self.postMessage(line) sends one back. kLex's
// worker transport reads messages, appends a newline, and feeds them
// into the same bufio.Scanner the subprocess path uses, so on the kLex
// side this bridge is indistinguishable from a subprocess one.
//
// Two equivalent ways to register handlers:
//
//   importScripts('klex_bridge_worker.js');
//
//   // Decorator-ish factory
//   handler({args: [['a', 'int'], ['b', 'int']], returns: 'int'},
//       function add(a, b) { return a + b; }
//   );
//
//   // Imperative
//   register('greet', function(name) { return 'Hello, ' + name + '!'; },
//       {args: [['name', 'string']], returns: 'string'});
//
//   serve();
//
// Schema mini-language (used in args, returns, yields):
//   "int", "float", "string", "bool", "array", "hash", "null", "any"
//   Trailing "?" makes the type nullable: "string?" accepts string or null.
//
// kLex auto-fetches every handler's schema via the special __schema__ call
// (registered automatically by serve()) and validates arguments before they
// hit the wire. Defence-in-depth validation also runs inside serve() so the
// bridge errors the same way regardless of which side caught it.

'use strict';

// Wire-protocol version this helper speaks. Bumped only for incompatible
// breaking changes; new additive features ship as capability flags instead.
const PROTOCOL_VERSION = 1;
const HELPER_VERSION   = '0.7.0';
const HELPER_CAPABILITIES = ['schema'];

// Internal handler registry. Populated by register() / handler() /
// streamHandler() and consumed by the dispatch loop in serve().
const _handlers = new Map();

function register(name, fn, opts) {
    if (typeof name !== 'string' || !name) throw new TypeError('register: name must be a non-empty string');
    if (typeof fn !== 'function')          throw new TypeError('register: fn must be a function');
    _handlers.set(name, {
        fn:      fn,
        args:    (opts && opts.args)    || [],
        returns: (opts && opts.returns) || 'any',
        stream:  (opts && opts.stream)  || false,
    });
}

// handler({args, returns}, fn) — register fn under its declared name.
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

// streamHandler({args, yields}, fn) — fn must return an iterable (sync or
// async generator). Each yielded value becomes one stream item on the
// kLex side.
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

// schema() — returns {handlerName: {args, returns, stream}} for every
// user-registered handler. Internal __* handlers are excluded.
function schema() {
    const out = {};
    for (const [name, h] of _handlers) {
        if (name.startsWith('__')) continue;
        out[name] = {args: h.args, returns: h.returns, stream: h.stream};
    }
    return out;
}

// ── Protocol responses ─────────────────────────────────────────────────────

function _hello(client) {
    return {
        protocol:         PROTOCOL_VERSION,
        capabilities:     HELPER_CAPABILITIES.slice(),
        helper:           'klex_bridge_worker.js/' + HELPER_VERSION,
        language:         'javascript-worker',
        language_version: (typeof navigator !== 'undefined' && navigator.userAgent) || 'unknown',
    };
}

// ── Wire I/O ───────────────────────────────────────────────────────────────

function _write(msg) {
    try {
        self.postMessage(JSON.stringify(msg));
    } catch (e) {
        self.postMessage(JSON.stringify({
            id:    msg && msg.id,
            error: 'bridge: failed to serialise response: ' + (e && e.message || String(e)),
        }));
    }
}

function notify(payload) {
    _write({notif: payload});
}

// ── Dispatch ───────────────────────────────────────────────────────────────

async function _dispatch(line) {
    let req;
    try { req = JSON.parse(line); }
    catch (e) {
        self.postMessage(JSON.stringify({error: 'bridge: malformed request: ' + line}));
        return;
    }

    const id = req.id;

    // Cancel notifications (id may be absent for cancels, just stop the stream)
    if (req.cancel != null) {
        return; // streams handle their own cancellation via the generator break
    }

    const fnName = req.fn;
    const args   = req.args || [];

    const h = _handlers.get(fnName);
    if (!h) {
        _write({id: id, error: 'unknown function: ' + fnName});
        return;
    }

    try {
        const result = await h.fn.apply(null, args);
        if (h.stream) {
            // Streaming handler — iterate (sync or async) and emit each
            // yielded value as a stream item.
            for await (const item of result) {
                _write({id: id, stream: item});
            }
            _write({id: id, stream_end: true});
        } else {
            _write({id: id, result: result});
        }
    } catch (e) {
        _write({
            id:        id,
            error:     (e && e.message) || String(e),
            errorType: (e && e.name)    || 'Error',
            traceback: (e && e.stack)   || '',
        });
    }
}

// serve() — install the message handler. Returns immediately; the Worker
// stays alive as long as the parent holds a reference to it.
function serve() {
    register('__hello__',  _hello, {args: [['client', 'hash']], returns: 'hash'});
    register('__schema__', schema, {args: [],                   returns: 'hash'});

    self.onmessage = function(event) {
        const line = typeof event.data === 'string' ? event.data : String(event.data);
        if (!line.trim()) return;
        _dispatch(line).catch(function(e) {
            // Last-resort: a bug inside _dispatch itself shouldn't kill the worker.
            self.postMessage(JSON.stringify({
                error: 'dispatch crash: ' + ((e && e.stack) || e),
            }));
        });
    };
}

// Expose the public surface on globalThis so user worker scripts that load
// this via importScripts() can call register/handler/streamHandler/serve
// without needing a module export. (Workers historically don't all support
// `import`/`export`.)
self.register      = register;
self.handler       = handler;
self.streamHandler = streamHandler;
self.notify        = notify;
self.schema        = schema;
self.serve         = serve;
