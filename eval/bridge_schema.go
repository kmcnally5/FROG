// bridge_schema.go — parse and validate bridge handler schemas.
//
// Bridges declare argument and return types using a tiny mini-language.
// Schema strings match the lowercase form of kLex's type() builtin:
//
//     int, float, string, bool, array, hash, null, any
//
// A trailing "?" makes the type nullable (accepts null as well):
//
//     string?, array?, int?
//
// Schemas are exchanged on the wire as JSON strings inside the response
// to the bridge's __schema__ call. ParseSchema turns the wire form into
// a Go struct; ValidateValue checks a runtime kLex Object against it.
//
// This file is intentionally standalone — no wiring into bridgeCall yet.
// That comes in step 3 of the phase, alongside the handshake.

package eval

import (
	"fmt"
	"strings"
)

// SchemaKind is the base type tag of a schema.
type SchemaKind int

const (
	SchemaAny SchemaKind = iota
	SchemaInt
	SchemaFloat
	SchemaString
	SchemaBool
	SchemaArray
	SchemaHash
	SchemaNull
)

func (k SchemaKind) String() string {
	switch k {
	case SchemaAny:
		return "any"
	case SchemaInt:
		return "int"
	case SchemaFloat:
		return "float"
	case SchemaString:
		return "string"
	case SchemaBool:
		return "bool"
	case SchemaArray:
		return "array"
	case SchemaHash:
		return "hash"
	case SchemaNull:
		return "null"
	}
	return "?"
}

// Schema describes the expected type of a single argument or return value.
type Schema struct {
	Kind     SchemaKind
	Nullable bool
}

// String returns the canonical wire form: "int", "string?", etc.
// "null" is never rendered with a trailing "?" — it's already null-only.
func (s Schema) String() string {
	if s.Nullable && s.Kind != SchemaNull {
		return s.Kind.String() + "?"
	}
	return s.Kind.String()
}

// ParseSchema parses one schema string into a Schema.
// Empty input is treated as "any" so bridges with no declared schema
// default to "no checking".
func ParseSchema(s string) (Schema, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Schema{Kind: SchemaAny}, nil
	}
	nullable := false
	if strings.HasSuffix(s, "?") {
		nullable = true
		s = strings.TrimSpace(s[:len(s)-1])
	}
	var kind SchemaKind
	switch s {
	case "any":
		kind = SchemaAny
	case "int":
		kind = SchemaInt
	case "float":
		kind = SchemaFloat
	case "string":
		kind = SchemaString
	case "bool":
		kind = SchemaBool
	case "array":
		kind = SchemaArray
	case "hash":
		kind = SchemaHash
	case "null":
		kind = SchemaNull
	default:
		return Schema{}, fmt.Errorf("unknown schema type %q", s)
	}
	return Schema{Kind: kind, Nullable: nullable}, nil
}

// ValidateValue checks obj against schema. Returns nil on match, or an
// error describing the mismatch suitable for inclusion in a
// BRIDGE_SCHEMA_ARG error message.
//
// Type rules worth knowing:
//   - "any" matches anything except null (use "any?" if null is acceptable).
//   - null matches only when the schema is nullable or explicitly "null".
//   - "float" accepts kLex Integer values as well — same loose rule as
//     kLex's own arithmetic, and necessary because JSON has only "number".
//     The reverse (Float satisfying "int") is rejected.
func ValidateValue(obj Object, schema Schema) error {
	// Null handling first — applies regardless of declared kind.
	if obj == nil || obj == NULL {
		if schema.Nullable || schema.Kind == SchemaNull {
			return nil
		}
		return fmt.Errorf("expected %s, got null", schema)
	}

	switch schema.Kind {
	case SchemaAny:
		return nil
	case SchemaInt:
		if _, ok := obj.(*Integer); ok {
			return nil
		}
	case SchemaFloat:
		if _, ok := obj.(*Float); ok {
			return nil
		}
		if _, ok := obj.(*Integer); ok {
			return nil // ints satisfy float slots
		}
	case SchemaString:
		if _, ok := obj.(*String); ok {
			return nil
		}
	case SchemaBool:
		if _, ok := obj.(*Boolean); ok {
			return nil
		}
	case SchemaArray:
		if _, ok := obj.(*Array); ok {
			return nil
		}
	case SchemaHash:
		if _, ok := obj.(*Hash); ok {
			return nil
		}
	case SchemaNull:
		// Reaching here means obj is non-null (handled above).
		return fmt.Errorf("expected null, got %s", lowerType(obj))
	}

	return fmt.Errorf("expected %s, got %s", schema, lowerType(obj))
}

// lowerType returns the schema-mini-language spelling of obj.Type() so error
// messages stay consistent with declared schemas ("int" not "integer", "bool"
// not "boolean", "string" not "STRING").
func lowerType(obj Object) string {
	switch obj.Type() {
	case INTEGER_OBJ:
		return "int"
	case BOOLEAN_OBJ:
		return "bool"
	default:
		return strings.ToLower(string(obj.Type()))
	}
}

// ── Handler-level schemas ────────────────────────────────────────────────────

// ArgSchema is one positional argument's declared name and type.
type ArgSchema struct {
	Name   string
	Schema Schema
}

// FnSchema describes a single bridge handler: positional args + return type.
type FnSchema struct {
	Args    []ArgSchema
	Returns Schema
}

// validateArgs checks a slice of kLex Objects against a handler's declared
// argument schemas. Used by bridgeCall before marshalling.
//
// Returns nil on match, or an error suitable for inclusion in a
// BRIDGE_SCHEMA_ARG message ("fnName: arg N 'pname': expected X, got Y").
func validateArgs(fnName string, fn *FnSchema, args []Object) error {
	if fn == nil {
		return nil // no schema known — accept anything
	}
	if len(args) != len(fn.Args) {
		return fmt.Errorf("%s: expected %d arg(s), got %d",
			fnName, len(fn.Args), len(args))
	}
	for i, a := range fn.Args {
		if err := ValidateValue(args[i], a.Schema); err != nil {
			return fmt.Errorf("%s: arg %d %q: %s", fnName, i, a.Name, err.Error())
		}
	}
	return nil
}

// parseFnSchema decodes one entry from the __schema__ JSON response into an
// FnSchema. The wire form for a single handler is:
//
//	{ "args": [["pname", "type"], ...], "returns": "type" }
//
// Anything malformed is rejected with a descriptive error so the caller can
// log it and skip that one handler rather than poisoning the whole map.
func parseFnSchema(raw map[string]interface{}) (*FnSchema, error) {
	fn := &FnSchema{}

	if argsAny, ok := raw["args"]; ok && argsAny != nil {
		argsList, ok := argsAny.([]interface{})
		if !ok {
			return nil, fmt.Errorf("args must be an array, got %T", argsAny)
		}
		fn.Args = make([]ArgSchema, 0, len(argsList))
		for i, entryAny := range argsList {
			entry, ok := entryAny.([]interface{})
			if !ok || len(entry) != 2 {
				return nil, fmt.Errorf("args[%d] must be [name, type], got %v", i, entryAny)
			}
			name, ok := entry[0].(string)
			if !ok {
				return nil, fmt.Errorf("args[%d] name must be string, got %T", i, entry[0])
			}
			typeStr, ok := entry[1].(string)
			if !ok {
				return nil, fmt.Errorf("args[%d] type must be string, got %T", i, entry[1])
			}
			sch, err := ParseSchema(typeStr)
			if err != nil {
				return nil, fmt.Errorf("args[%d] %q: %v", i, name, err)
			}
			fn.Args = append(fn.Args, ArgSchema{Name: name, Schema: sch})
		}
	}

	// Returns is optional — default to "any" if omitted.
	if retAny, ok := raw["returns"]; ok && retAny != nil {
		retStr, ok := retAny.(string)
		if !ok {
			return nil, fmt.Errorf("returns must be string, got %T", retAny)
		}
		sch, err := ParseSchema(retStr)
		if err != nil {
			return nil, fmt.Errorf("returns: %v", err)
		}
		fn.Returns = sch
	} else {
		fn.Returns = Schema{Kind: SchemaAny}
	}

	return fn, nil
}

// parseSchemaResponse turns the parsed JSON result of __schema__ into the
// per-handler schema map kept on the Bridge. Malformed entries are skipped
// with a warning rather than failing the whole handshake — defensive against
// future bridges adding fields kLex doesn't yet understand.
func parseSchemaResponse(raw interface{}) (map[string]*FnSchema, error) {
	top, ok := raw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("__schema__: expected object, got %T", raw)
	}
	out := make(map[string]*FnSchema, len(top))
	for name, entryAny := range top {
		entry, ok := entryAny.(map[string]interface{})
		if !ok {
			continue // skip — defensive
		}
		fn, err := parseFnSchema(entry)
		if err != nil {
			continue // skip malformed entries; rest of the map remains usable
		}
		out[name] = fn
	}
	return out, nil
}

// fnSchemaToHash converts an FnSchema into the kLex Hash returned by the
// bridgeSchema(b, fn) builtin. Shape mirrors the wire format:
//
//	{ "args": [["name", "type"], ...], "returns": "type" }
func fnSchemaToHash(fn *FnSchema) *Hash {
	h := &Hash{Pairs: make(map[HashKey]HashPair, 2)}

	// args: array of [name, type] string pairs.
	pairs := make([]Object, len(fn.Args))
	for i, a := range fn.Args {
		pairs[i] = &Array{Elements: []Object{
			&String{Value: a.Name},
			&String{Value: a.Schema.String()},
		}}
	}
	h.Pairs[HashKey{Type: STRING_OBJ, Value: "args"}] = HashPair{
		Key:   &String{Value: "args"},
		Value: &Array{Elements: pairs},
	}

	// returns: single type string.
	h.Pairs[HashKey{Type: STRING_OBJ, Value: "returns"}] = HashPair{
		Key:   &String{Value: "returns"},
		Value: &String{Value: fn.Returns.String()},
	}
	return h
}

// fnSchemaMapToHash converts the full schemas map into the kLex Hash returned
// by bridgeSchema(b). Keys are handler names; values are per-handler schemas.
func fnSchemaMapToHash(m map[string]*FnSchema) *Hash {
	h := &Hash{Pairs: make(map[HashKey]HashPair, len(m))}
	for name, fn := range m {
		h.Pairs[HashKey{Type: STRING_OBJ, Value: name}] = HashPair{
			Key:   &String{Value: name},
			Value: fnSchemaToHash(fn),
		}
	}
	return h
}
