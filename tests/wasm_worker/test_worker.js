// test_worker.js — sample Web Worker bridge for kLex.
//
// Demonstrates the same handler shape as Python/Node bridges:
// importScripts the helper, register handlers, call serve().

importScripts('klex_bridge_worker.js');

handler({args: [['a', 'int'], ['b', 'int']], returns: 'int'},
    function add(a, b) { return a + b; }
);

handler({args: [['name', 'string']], returns: 'string'},
    function greet(name) { return 'Hello from the worker, ' + name + '!'; }
);

handler({args: [['s', 'string']], returns: 'int'},
    function strlen(s) { return s.length; }
);

handler({args: [['nums', 'array']], returns: 'hash'},
    function stats(nums) {
        const n = nums.length;
        if (n === 0) return {count: 0, sum: 0, mean: 0};
        const sum = nums.reduce(function(a, b) { return a + b; }, 0);
        return {count: n, sum: sum, mean: sum / n};
    }
);

serve();
