// stdlib/functional.lex — higher-order function utilities
// @module    functional
// @version   1.0.0
// @since     klex 0.3.35
// @author    karl
// @summary   higher-order function utilities
//
// Pure function combinators: identity, compose, pipe, tap, always, partial, flip.
// None of these are stateful — no struct needed.
//
// Usage:
//   import "functional.lex" as f
//   double = fn(x) { x * 2 }
//   inc    = fn(x) { x + 1 }
//   f.pipe(inc, double)(3)   // 8

// identity returns its argument unchanged.
fn identity(x) {
    return x
}

// compose(f, g) returns a function h where h(x) = f(g(x)) (right-to-left).
fn compose(f, g) {
    return fn(x) {
        return f(g(x))
    }
}

// pipe(fns...) returns a function that applies fns left-to-right.
fn pipe(fns...) {
    return fn(x) {
        let result = x
        let i = 0
        while i < len(fns) {
            result = fns[i](result)
            i = i + 1
        }
        return result
    }
}

// tap(fnRef) returns a function that calls fnRef for its side effect,
// then returns the original value unchanged. Useful for debug steps in pipelines.
fn tap(fnRef) {
    return fn(x) {
        fnRef(x)
        return x
    }
}

// always(v) returns a function that ignores its argument and always returns v.
fn always(v) {
    return fn(x) {
        return v
    }
}

// partial(fnRef, fixedArgs...) binds fixedArgs to the left of fnRef.
// The returned function accepts the remaining arguments at call time and
// invokes fnRef with the concatenated list — no arity cap.
//
//   add = fn(a, b, c) { a + b + c }
//   addOne = partial(add, 1)
//   addOne(2, 3)        // 6
fn partial(fnRef, fixedArgs...) {
    return fn(restArgs...) {
        let n = len(fixedArgs) + len(restArgs)
        let all = makeArray(n, null)
        let i = 0
        while i < len(fixedArgs) {
            all[i] = fixedArgs[i]
            i = i + 1
        }
        let j = 0
        while j < len(restArgs) {
            all[i + j] = restArgs[j]
            j = j + 1
        }
        return apply(fnRef, all)
    }
}

// flip(fnRef) returns a version of fnRef with its first two arguments swapped.
// Any further arguments are forwarded unchanged.
//
//   sub = fn(a, b) { a - b }
//   rsub = flip(sub)
//   rsub(2, 10)         // 8
fn flip(fnRef) {
    return fn(a, b, rest...) {
        let n = 2 + len(rest)
        let all = makeArray(n, null)
        all[0] = b
        all[1] = a
        let i = 0
        while i < len(rest) {
            all[2 + i] = rest[i]
            i = i + 1
        }
        return apply(fnRef, all)
    }
}
