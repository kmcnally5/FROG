'use strict';

// bridgeMonitorBridge.js — test bridge for bridgeMonitor.lex
//
// Two handlers generate varied traffic so the dashboard has something
// interesting to show:
//   echo    — near-instant round-trip; bulk of the traffic
//   compute — fibonacci(n); produces visible latency spread in p50/p95/p99
//
// Run via bridgeMonitor.lex — do not invoke directly.

const { register, serve } = require('klex_bridge');

register('echo', function echo(msg) {
    return { msg: msg, ts: Date.now() };
}, { args: [['msg', 'string']], returns: 'hash' });

register('compute', function compute(n) {
    const safe = Math.min(Math.max(n, 1), 38);
    function fib(k) {
        if (k <= 1) return k;
        return fib(k - 1) + fib(k - 2);
    }
    return { n: safe, result: fib(safe) };
}, { args: [['n', 'int']], returns: 'hash' });

serve();
