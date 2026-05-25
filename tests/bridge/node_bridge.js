// node_bridge.js — Node companion to python_bridge.py.
//
// Same set of handlers wherever it makes sense, so the same nodeTest.lex can
// drive add/multiply/streams/cancel/crash scenarios and we know the helper
// behaves identically across languages.

'use strict';

const {handler, streamHandler, serve} = require('klex_bridge');


handler({args: [['a', 'int'], ['b', 'int']], returns: 'int'},
    function add(a, b) { return a + b; }
);

handler({args: [['a', 'int'], ['b', 'int']], returns: 'int'},
    function multiply(a, b) { return a * b; }
);

handler({args: [['name', 'string']], returns: 'string'},
    function greet(name) { return `Hello from Node, ${name}!`; }
);

handler({args: [['numbers', 'array']], returns: 'hash'},
    function stats(numbers) {
        const n = numbers.length;
        if (n === 0) return {count: 0, sum: 0, mean: 0, min: 0, max: 0};
        const total = numbers.reduce((a, b) => a + b, 0);
        return {
            count: n,
            sum:   total,
            mean:  total / n,
            min:   Math.min(...numbers),
            max:   Math.max(...numbers),
        };
    }
);

handler({args: [['sentence', 'string']], returns: 'string'},
    function reverse_words(sentence) {
        return sentence.split(/\s+/).filter(Boolean).reverse().join(' ');
    }
);

handler({args: [['n', 'int']], returns: 'bool'},
    function is_prime(n) {
        if (n < 2) return false;
        for (let i = 2; i * i <= n; i++) {
            if (n % i === 0) return false;
        }
        return true;
    }
);


// Deliberately misbehaving — declared int, returns a string. Used by the
// return-type validation test (mirrors python_bridge.lies_about_return).
handler({args: [], returns: 'int'},
    function lies_about_return() { return "actually a string"; }
);


// Deliberately throwing — verifies structured errors (errorType + traceback)
// survive the trip across the wire intact.
const fs = require('fs');
handler({args: [['path', 'string']], returns: 'string'},
    function open_missing(path) { return fs.readFileSync(path, 'utf8'); }
);


// Sync generator — verifies for-of dispatch path.
streamHandler({args: [['start', 'int'], ['count', 'int']], yields: 'int'},
    function* count_from(start, count) {
        for (let i = 0; i < count; i++) yield start + i;
    }
);


// Async generator — verifies for-await dispatch path.
streamHandler({args: [], yields: 'int'},
    async function* broken_stream() {
        yield 1;
        yield 2;
        throw new Error('intentional mid-stream failure');
    }
);


// Slow stream + cancel counter for cancelTest equivalent on the JS side.
let _cancelCount = 0;
const _sleep = (ms) => new Promise((r) => setTimeout(r, ms));

streamHandler({args: [['count', 'int'], ['delay_ms', 'int']], yields: 'int'},
    async function* cancel_demo(count, delay_ms) {
        _cancelCount = 0;
        for (let i = 0; i < count; i++) {
            _cancelCount = i + 1;     // bump before yield so the count reflects
            yield i;                  // items the consumer actually received
            if (delay_ms > 0) await _sleep(delay_ms);
        }
    }
);

handler({args: [], returns: 'int'},
    function get_cancel_count() { return _cancelCount; }
);


// Aborts the subprocess so poolTest can verify pool health folds in
// runtime crashes (mirrors python_bridge.suicide).
handler({args: [], returns: 'string'},
    function suicide() { process.exit(7); }
);


// ── Binary payload round-trip handlers (binaryTest.lex) ────────────────────

// Echo back the bytes unchanged. Asserts the helper handed us a real Buffer
// so the wire round-trip is doing native decode, not falling back to base64.
handler({args: [['data', 'bytes']], returns: 'bytes'},
    function echo_bytes(data) {
        if (!Buffer.isBuffer(data)) throw new TypeError('expected Buffer, got ' + typeof data);
        return data;
    }
);

// Generate N bytes of deterministic content (i & 0xff per index).
handler({args: [['n', 'int']], returns: 'bytes'},
    function make_bytes(n) {
        const buf = Buffer.alloc(n);
        for (let i = 0; i < n; i++) buf[i] = i & 0xff;
        return buf;
    }
);

// Hash containing bytes — concatenated bytes round-trip nested.
handler({args: [['h', 'hash']], returns: 'hash'},
    function grow_bytes_in_hash(h) {
        if (!Buffer.isBuffer(h.data)) throw new TypeError('h.data must be Buffer');
        return {data: Buffer.concat([h.data, h.data]), name: h.name || ''};
    }
);

// Streaming bytes — yields N chunks of `size` bytes; first byte is chunk index.
streamHandler({args: [['count', 'int'], ['size', 'int']], yields: 'bytes'},
    function* stream_bytes(count, size) {
        for (let i = 0; i < count; i++) {
            const buf = Buffer.alloc(size);
            if (size > 0) buf[0] = i & 0xff;
            yield buf;
        }
    }
);


serve();
