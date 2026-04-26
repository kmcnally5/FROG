// stdlib/functional.lex — higher-order function utilities
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
        result = x
        i = 0
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
// The returned function accepts the remaining arguments.
// Supports up to 3 total arguments combined.
fn partial(fnRef, fixedArgs...) {
    return fn(restArgs...) {
        all = []
        i = 0
        while i < len(fixedArgs) {
            all = push(all, fixedArgs[i])
            i = i + 1
        }
        i = 0
        while i < len(restArgs) {
            all = push(all, restArgs[i])
            i = i + 1
        }
        if len(all) == 0 {
            return fnRef()
        } else if len(all) == 1 {
            return fnRef(all[0])
        } else if len(all) == 2 {
            return fnRef(all[0], all[1])
        } else if len(all) == 3 {
            return fnRef(all[0], all[1], all[2])
        } else {
            return null, "partial: too many arguments (max 3)"
        }
    }
}

// flip(fnRef) returns a version of fnRef with its first two arguments swapped.
fn flip(fnRef) {
    return fn(a, b, rest...) {
        all = []
        all = push(all, b)
        all = push(all, a)
        i = 0
        while i < len(rest) {
            all = push(all, rest[i])
            i = i + 1
        }
        if len(all) == 0 {
            return fnRef()
        } else if len(all) == 1 {
            return fnRef(all[0])
        } else if len(all) == 2 {
            return fnRef(all[0], all[1])
        } else if len(all) == 3 {
            return fnRef(all[0], all[1], all[2])
        } else {
            return null, "flip: too many arguments (max 3)"
        }
    }
}
