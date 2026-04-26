// uuid.lex
// UUID generation for kLex.
//
// Usage:
//   import "uuid.lex" as uuid
//   id = uuid.v4()   // e.g. "f47ac10b-58cc-4372-a567-0e02b2c3d479"

// v4 generates and returns a random UUID v4 as a string.
fn v4() {
    return _uuid()
}
