// schema.lex — JSON Schema builder for kLex AI structured output.
// @module    schema
// @version   1.0.0
// @since     klex 0.3.35
// @author    karl
// @summary   JSON Schema builder for kLex AI structured output.
//
// Each function returns a hash matching the JSON Schema 2020-12 spec.
// Hashes compose recursively, so nested objects + arrays just work.
//
// Usage:
//   import "stdlib/ai/schema.lex" as schema
//
//   PersonSchema = schema.object({
//       "name":  schema.string("Full name"),
//       "age":   schema.integer("Age", 0, 150),
//       "email": schema.string("Email address"),
//   })
//
//   person, err = claude.complete(c, PersonSchema, "Generate a fake profile")
//   println(person["name"])
//
// Provider libraries (anthropic.lex, ollama.lex) accept these hashes
// directly via their complete() / completeWith() functions, ship them to
// each provider's structured-output mode, and parse the response back into
// a validated kLex hash.
//
// Design notes:
//   - All hashes are JSON Schema documents — they can be inspected,
//     persisted, serialised, or passed straight through to anything that
//     speaks JSON Schema. No provider-specific magic.
//   - object() defaults to all-properties-required (the safer behaviour
//     for AI prompts — you don't want the model silently omitting fields).
//     Pass an explicit `required` list to relax.
//   - Builders are pure functions — no side effects, no global state.


// ── Primitive scalars ─────────────────────────────────────────────────────

// string returns a string-type schema. Optional description annotates
// the field for the model (helpful for guiding output).
fn string(description = null) {
    let out = {"type": "string"}
    if description != null { out["description"] = description }
    return out
}


// integer returns an integer-type schema with optional bounds.
fn integer(description = null, minimum = null, maximum = null) {
    let out = {"type": "integer"}
    if description != null { out["description"] = description }
    if minimum != null     { out["minimum"]     = minimum }
    if maximum != null     { out["maximum"]     = maximum }
    return out
}


// number returns a number-type (float) schema with optional bounds.
fn number(description = null, minimum = null, maximum = null) {
    let out = {"type": "number"}
    if description != null { out["description"] = description }
    if minimum != null     { out["minimum"]     = minimum }
    if maximum != null     { out["maximum"]     = maximum }
    return out
}


// boolean returns a boolean-type schema.
fn boolean(description = null) {
    let out = {"type": "boolean"}
    if description != null { out["description"] = description }
    return out
}


// ── Compound types ────────────────────────────────────────────────────────

// array returns an array schema whose elements all match `items`.
// items is itself a schema (built with any of these functions).
fn array(items, description = null) {
    let out = {"type": "array", "items": items}
    if description != null { out["description"] = description }
    return out
}


// object returns an object schema with the given properties.
//
// `props` is a hash mapping property name → schema. By default every
// property is required (so the model can't silently drop fields). Pass
// a `required` array to restrict the required list.
//
//   // All three required (default)
//   schema.object({
//       "name":  schema.string(),
//       "age":   schema.integer(),
//       "email": schema.string(),
//   })
//
//   // Only name + age required; email optional
//   schema.object({
//       "name":  schema.string(),
//       "age":   schema.integer(),
//       "email": schema.string(),
//   }, ["name", "age"])
fn object(props, required = null, description = null) {
    if required == null {
        required = makeArray(0)
        for k, v in props {
            required = concat(required, [k])
        }
    }
    let out = {
        "type":                 "object",
        "properties":           props,
        "required":             required,
        "additionalProperties": false,
    }
    if description != null { out["description"] = description }
    return out
}


// enumOf returns a string schema constrained to one of `values`.
// (Avoiding the name `enum` because kLex reserves it for enum decls.)
//
//   schema.enumOf(["red", "green", "blue"], "Primary colour")
fn enumOf(values, description = null) {
    let out = {"type": "string", "enum": values}
    if description != null { out["description"] = description }
    return out
}


// ── Modifiers ─────────────────────────────────────────────────────────────

// nullable wraps a schema to allow null. Uses JSON Schema 2020-12's
// `type: [<original>, "null"]` form, which Anthropic, OpenAI, and Ollama
// all accept.
//
//   schema.nullable(schema.string("Email if known"))
fn nullable(inner) {
    if !hasKey(inner, "type") { return inner }
    let t = inner["type"]
    if type(t) == "STRING" {
        inner["type"] = [t, "null"]
    } else if type(t) == "ARRAY" {
        let hasNull = false
        for x in t { if x == "null" { hasNull = true } }
        if !hasNull { inner["type"] = concat(t, ["null"]) }
    }
    return inner
}
