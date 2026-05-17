// bridge_schema_test.go — unit tests for the schema parser and validator.

package eval

import (
	"strings"
	"testing"
)

// ── ParseSchema ──────────────────────────────────────────────────────────────

func TestParseSchema_AllKinds(t *testing.T) {
	cases := []struct {
		in   string
		want SchemaKind
	}{
		{"any", SchemaAny},
		{"int", SchemaInt},
		{"float", SchemaFloat},
		{"string", SchemaString},
		{"bool", SchemaBool},
		{"array", SchemaArray},
		{"hash", SchemaHash},
		{"null", SchemaNull},
	}
	for _, tc := range cases {
		got, err := ParseSchema(tc.in)
		if err != nil {
			t.Errorf("ParseSchema(%q) returned error: %v", tc.in, err)
			continue
		}
		if got.Kind != tc.want {
			t.Errorf("ParseSchema(%q).Kind = %v, want %v", tc.in, got.Kind, tc.want)
		}
		if got.Nullable {
			t.Errorf("ParseSchema(%q).Nullable = true, want false", tc.in)
		}
	}
}

func TestParseSchema_Nullable(t *testing.T) {
	s, err := ParseSchema("string?")
	if err != nil {
		t.Fatal(err)
	}
	if s.Kind != SchemaString {
		t.Errorf("Kind = %v, want SchemaString", s.Kind)
	}
	if !s.Nullable {
		t.Error("Nullable = false, want true")
	}
}

func TestParseSchema_EmptyIsAny(t *testing.T) {
	s, err := ParseSchema("")
	if err != nil {
		t.Fatal(err)
	}
	if s.Kind != SchemaAny {
		t.Errorf("Kind = %v, want SchemaAny", s.Kind)
	}
}

func TestParseSchema_TrimsWhitespace(t *testing.T) {
	s, err := ParseSchema("  int  ")
	if err != nil {
		t.Fatal(err)
	}
	if s.Kind != SchemaInt {
		t.Errorf("Kind = %v, want SchemaInt", s.Kind)
	}
}

func TestParseSchema_TrimsAroundQuestionMark(t *testing.T) {
	s, err := ParseSchema(" string ?")
	if err != nil {
		t.Fatal(err)
	}
	if s.Kind != SchemaString || !s.Nullable {
		t.Errorf("got %+v, want SchemaString nullable", s)
	}
}

func TestParseSchema_Unknown(t *testing.T) {
	_, err := ParseSchema("notarealtype")
	if err == nil {
		t.Error("expected error for unknown type, got nil")
	}
}

func TestSchemaString_RoundTrip(t *testing.T) {
	cases := []string{"int", "string?", "any", "null", "array", "float?", "hash?", "bool"}
	for _, in := range cases {
		s, err := ParseSchema(in)
		if err != nil {
			t.Errorf("%q: parse failed: %v", in, err)
			continue
		}
		if got := s.String(); got != in {
			t.Errorf("%q: round-trip → %q", in, got)
		}
	}
}

// ── ValidateValue ────────────────────────────────────────────────────────────

func TestValidateValue_NullHandling(t *testing.T) {
	nullable, _ := ParseSchema("string?")
	if err := ValidateValue(NULL, nullable); err != nil {
		t.Errorf("null vs string?: %v", err)
	}

	plain, _ := ParseSchema("string")
	if err := ValidateValue(NULL, plain); err == nil {
		t.Error("null vs string: expected error, got nil")
	}

	explicit, _ := ParseSchema("null")
	if err := ValidateValue(NULL, explicit); err != nil {
		t.Errorf("null vs null: %v", err)
	}
}

func TestValidateValue_AnyAcceptsAnythingButNull(t *testing.T) {
	sch, _ := ParseSchema("any")
	values := []Object{
		&Integer{Value: 5},
		&String{Value: "hi"},
		TRUE,
		&Array{Elements: nil},
		&Hash{Pairs: nil},
	}
	for _, v := range values {
		if err := ValidateValue(v, sch); err != nil {
			t.Errorf("any vs %s: %v", v.Type(), err)
		}
	}
	if err := ValidateValue(NULL, sch); err == nil {
		t.Error("any vs null: expected error, got nil")
	}
}

func TestValidateValue_AnyNullableAcceptsNull(t *testing.T) {
	sch, _ := ParseSchema("any?")
	if err := ValidateValue(NULL, sch); err != nil {
		t.Errorf("any? vs null: %v", err)
	}
}

func TestValidateValue_Primitives(t *testing.T) {
	type pair struct {
		schema string
		obj    Object
		ok     bool
	}
	cases := []pair{
		// matches
		{"int", &Integer{Value: 5}, true},
		{"float", &Float{Value: 1.5}, true},
		{"string", &String{Value: "x"}, true},
		{"bool", TRUE, true},
		{"array", &Array{}, true},
		{"hash", &Hash{}, true},
		// float accepts int (JSON has no separate int/float)
		{"float", &Integer{Value: 5}, true},
		// mismatches
		{"int", &Float{Value: 1.5}, false},
		{"int", &String{Value: "5"}, false},
		{"string", &Integer{Value: 5}, false},
		{"bool", &Integer{Value: 1}, false},
		{"array", &Hash{}, false},
		{"hash", &Array{}, false},
	}
	for _, tc := range cases {
		sch, err := ParseSchema(tc.schema)
		if err != nil {
			t.Fatalf("parse %q: %v", tc.schema, err)
		}
		verr := ValidateValue(tc.obj, sch)
		if tc.ok && verr != nil {
			t.Errorf("%s vs %s: unexpected error %v", tc.schema, tc.obj.Type(), verr)
		}
		if !tc.ok && verr == nil {
			t.Errorf("%s vs %s: expected error, got nil", tc.schema, tc.obj.Type())
		}
	}
}

func TestValidateValue_ErrorMessageMentionsBothTypes(t *testing.T) {
	sch, _ := ParseSchema("int")
	err := ValidateValue(&String{Value: "x"}, sch)
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "int") {
		t.Errorf("error %q missing 'int'", msg)
	}
	if !strings.Contains(msg, "string") {
		t.Errorf("error %q missing 'string'", msg)
	}
}

func TestValidateValue_ExplicitNullRejectsNonNull(t *testing.T) {
	sch, _ := ParseSchema("null")
	err := ValidateValue(&Integer{Value: 0}, sch)
	if err == nil {
		t.Error("null vs Integer: expected error, got nil")
	}
}

func TestValidateValue_NullableRejectsWrongType(t *testing.T) {
	// "string?" matches string or null — not, say, an integer.
	sch, _ := ParseSchema("string?")
	err := ValidateValue(&Integer{Value: 0}, sch)
	if err == nil {
		t.Error("string? vs Integer: expected error, got nil")
	}
}
