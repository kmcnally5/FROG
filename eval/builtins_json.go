package eval

// builtins_json.go — Go-side JSON parser/stringifier replacing the
// interpreted-kLex stdlib/json.lex implementation.
//
// The kLex parser walked every character through the tree-walking
// interpreter, which was catastrophic at scale: a 1 MB JSONL file
// could take 30+ seconds to replay (see OFI #14). Using Go's
// encoding/json with json.Decoder + UseNumber gives us native speed
// while still distinguishing integers from floats (encoding/json's
// default decodes all numbers as float64, losing int precision past
// 2^53).
//
// API contract (must match stdlib/json.lex's public signature so
// existing callers don't change):
//
//   _jsonParse(s: string)     → (value, errStr)
//   _jsonStringify(v: any)    → (string, errStr)
//
// Errors are returned as plain strings (NOT typed Error objects) —
// matches the historical kLex-side API where callers do
// `val, err = json.parse(s); if err != null { ... }`.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"klex/ast"
	"math"
	"sort"
	"strconv"
	"strings"
)

// jsonValueToKLex converts a value returned by json.Decoder (with UseNumber)
// into the corresponding kLex Object. Numbers are inspected as strings:
// if the literal has no decimal point or exponent AND fits in int64,
// it becomes an *Integer; otherwise *Float. This preserves the natural
// "JSON integers stay integers" expectation for round-tripping records
// like {"mtime": 1779000000} without turning mtime into 1.779e9.
func jsonValueToKLex(v interface{}) (Object, error) {
	switch x := v.(type) {
	case nil:
		return NULL, nil
	case bool:
		if x {
			return TRUE, nil
		}
		return FALSE, nil
	case string:
		return &String{Value: x}, nil
	case json.Number:
		s := string(x)
		// Integer literal (no '.' / 'e' / 'E') that fits int64 → Integer.
		if !strings.ContainsAny(s, ".eE") {
			if n, err := strconv.ParseInt(s, 10, 64); err == nil {
				return &Integer{Value: int(n)}, nil
			}
			// Falls through to float for ints that overflow int64.
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid number %q: %v", s, err)
		}
		return &Float{Value: f}, nil
	case []interface{}:
		out := make([]Object, len(x))
		for i, el := range x {
			obj, err := jsonValueToKLex(el)
			if err != nil {
				return nil, fmt.Errorf("array[%d]: %v", i, err)
			}
			out[i] = obj
		}
		return &Array{Elements: out}, nil
	case map[string]interface{}:
		h := &Hash{Pairs: make(map[HashKey]HashPair, len(x))}
		for k, vv := range x {
			obj, err := jsonValueToKLex(vv)
			if err != nil {
				return nil, fmt.Errorf("object[%q]: %v", k, err)
			}
			h.Pairs[HashKey{Type: STRING_OBJ, Value: k}] = HashPair{
				Key:   &String{Value: k},
				Value: obj,
			}
		}
		return h, nil
	default:
		return nil, fmt.Errorf("unsupported json type %T", v)
	}
}

// kLexToJSON writes the kLex value into buf as JSON. Sorted keys for
// hashes so output is deterministic (matches the historical kLex
// stringifier and makes diffing / hashing JSON outputs predictable).
func kLexToJSON(buf *bytes.Buffer, v Object) error {
	if v == nil {
		buf.WriteString("null")
		return nil
	}
	switch x := v.(type) {
	case *Null:
		buf.WriteString("null")
	case *Boolean:
		if x.Value {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case *Integer:
		buf.WriteString(strconv.FormatInt(int64(x.Value), 10))
	case *Float:
		f := x.Value
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return fmt.Errorf("cannot stringify non-finite float (NaN/Inf)")
		}
		// 'g' is the shortest round-tripping representation, matching
		// encoding/json's default behaviour for float64.
		buf.WriteString(strconv.FormatFloat(f, 'g', -1, 64))
	case *String:
		writeJSONString(buf, x.Value)
	case *Bytes:
		// kLex *Bytes has no canonical JSON form; the historical
		// stdlib/json.lex didn't handle it either. Reject explicitly
		// so the caller can choose base64/hex encoding instead.
		return fmt.Errorf("cannot stringify Bytes; use bytesToBase64() or bytesToHex() first")
	case *Array:
		buf.WriteByte('[')
		for i, el := range x.Elements {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := kLexToJSON(buf, el); err != nil {
				return fmt.Errorf("array[%d]: %v", i, err)
			}
		}
		buf.WriteByte(']')
	case *Hash:
		buf.WriteByte('{')
		// Sorted keys for stable output.
		keys := make([]HashKey, 0, len(x.Pairs))
		for k := range x.Pairs {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			return keys[i].Value < keys[j].Value
		})
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			pair := x.Pairs[k]
			// JSON object keys MUST be strings. kLex hashes allow
			// integer/bool keys too — coerce them to their natural
			// string form rather than erroring, to match the
			// historical stdlib behaviour.
			switch kv := pair.Key.(type) {
			case *String:
				writeJSONString(buf, kv.Value)
			case *Integer:
				writeJSONString(buf, strconv.FormatInt(int64(kv.Value), 10))
			case *Boolean:
				if kv.Value {
					writeJSONString(buf, "true")
				} else {
					writeJSONString(buf, "false")
				}
			default:
				return fmt.Errorf("hash key of type %s cannot be stringified", pair.Key.Type())
			}
			buf.WriteByte(':')
			if err := kLexToJSON(buf, pair.Value); err != nil {
				return fmt.Errorf("hash[%q]: %v", k.Value, err)
			}
		}
		buf.WriteByte('}')
	case *StructInstance:
		// Treat struct fields like a hash on the way out — matches what
		// programmers typically expect when round-tripping a struct
		// through JSON for persistence.
		buf.WriteByte('{')
		keys := make([]string, 0, len(x.Fields))
		for k := range x.Fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			writeJSONString(buf, k)
			buf.WriteByte(':')
			if err := kLexToJSON(buf, x.Fields[k]); err != nil {
				return fmt.Errorf("struct.%s: %v", k, err)
			}
		}
		buf.WriteByte('}')
	default:
		return fmt.Errorf("cannot stringify %s", v.Type())
	}
	return nil
}

// writeJSONString writes s into buf as a properly quoted+escaped JSON
// string. Delegates to encoding/json so we inherit its escaping rules
// (control chars, quotes, backslashes, ≥U+2028 line separators on the
// safe side, etc.).
func writeJSONString(buf *bytes.Buffer, s string) {
	b, err := json.Marshal(s)
	if err != nil {
		// json.Marshal of a string can fail on invalid UTF-8; fall
		// back to a manual quoted form so we never panic mid-encode.
		buf.WriteByte('"')
		buf.WriteString(strings.ReplaceAll(s, `"`, `\"`))
		buf.WriteByte('"')
		return
	}
	buf.Write(b)
}

func init() {
	// ── _jsonParse(s: string) → (value, errStr) ────────────────────────────
	//
	// Decode a JSON document into kLex objects. Returns (value, null)
	// on success, (null, "<error message>") on parse failure. Mirrors
	// the historical stdlib/json.lex `parse` signature byte-for-byte
	// so existing callers can switch over by changing the delegation
	// site only — no upstream churn.
	Builtins["_jsonParse"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_jsonParse expects 1 argument (string)", ast.Pos{})
		}
		s, ok := args[0].(*String)
		if !ok {
			return typeError("_jsonParse: argument must be string, got "+string(args[0].Type()), ast.Pos{})
		}
		dec := json.NewDecoder(strings.NewReader(s.Value))
		dec.UseNumber()
		var raw interface{}
		if err := dec.Decode(&raw); err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		// Trailing-content check matches the historical kLex parser:
		// "abc 123" would otherwise silently return "abc".
		if dec.More() {
			return &Tuple{Elements: []Object{NULL, &String{Value: "unexpected trailing characters"}}}
		}
		obj, err := jsonValueToKLex(raw)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{obj, NULL}}
	}}

	// ── _jsonStringify(v: any) → (string, errStr) ──────────────────────────
	//
	// Encode a kLex value as JSON. Returns (json, null) on success or
	// (null, "<error message>") if some value in the tree isn't
	// JSON-representable (Bytes, cycles, NaN/Inf floats). Hash keys are
	// emitted sorted so output is deterministic.
	//
	// Note: the historical stdlib/json.lex returned just a string (no
	// error tuple), so the wrapper in stdlib/json.lex unpacks `(s, _)`
	// and returns s only. New code may call _jsonStringify directly to
	// surface the error.
	Builtins["_jsonStringify"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_jsonStringify expects 1 argument", ast.Pos{})
		}
		var buf bytes.Buffer
		if err := kLexToJSON(&buf, args[0]); err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{&String{Value: buf.String()}, NULL}}
	}}
}
