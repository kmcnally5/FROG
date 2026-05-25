package main

import "strings"

// Category drives the new hover renderer's section header + cross-
// references. Every builtin is classified into exactly one category
// so the user sees "Builtin · AI / Embeddings" rather than a bare
// signature. The classifier is name-pattern based — cheap, no parsing
// of the Go source needed.
//
// Categories are also the basis for "See also" cross-references: the
// hover for any builtin in category X surfaces the top three sibling
// builtins in the same category (excluding itself).
type BuiltinCategory struct {
	Display string // human-readable group, e.g. "AI / Embeddings"
	Source  string // hint to the eval/ Go file that defines members
}

// categoryRule binds a list of prefixes / exact names to a category.
// The classifier walks rules in order; first match wins. Order matters:
// more specific rules go first.
type categoryRule struct {
	exacts   []string
	prefixes []string
	contains []string
	cat      BuiltinCategory
}

var categoryRules = []categoryRule{
	// ── Core / Output ──────────────────────────────────────────────
	{exacts: []string{"println", "print", "input"},
		cat: BuiltinCategory{Display: "Core · I/O", Source: "eval/builtins_strings.go"}},

	// ── Type / Conversion ─────────────────────────────────────────
	{exacts: []string{"type", "str", "int", "float", "bool",
		"parseInt", "parseFloat", "isError", "error"},
		cat: BuiltinCategory{Display: "Core · Types & Errors", Source: "eval/builtins_types.go"}},

	// ── Math ──────────────────────────────────────────────────────
	{exacts: []string{"abs", "sqrt", "pow", "min", "max", "floor",
		"ceil", "round", "mod", "fmod", "sin", "cos", "tan", "asin",
		"acos", "atan", "atan2", "log", "exp", "pi", "e",
		"remap", "constrain", "lerp", "hsl", "rgb",
		"random", "randomSeed", "randomRange"},
		cat: BuiltinCategory{Display: "Math · Numbers", Source: "eval/builtins_math.go"}},

	// ── Strings ───────────────────────────────────────────────────
	{exacts: []string{"chr", "ord", "substr", "split", "join",
		"strReplace", "strTrim", "strLower", "strUpper", "strContains",
		"strStartsWith", "strEndsWith", "strIndexOf", "strLastIndexOf",
		"strRepeat", "format", "strPad", "strPadLeft", "strPadRight"},
		cat: BuiltinCategory{Display: "Strings", Source: "eval/builtins_strings.go"}},

	// ── Collections ───────────────────────────────────────────────
	{exacts: []string{"len", "makeArray", "concat", "slice", "reverse",
		"sort", "sortBy", "unique", "keys", "values", "hasKey",
		"delete", "map", "filter", "reduce", "find", "indexOf",
		"sum", "range", "zip"},
		cat: BuiltinCategory{Display: "Collections", Source: "eval/builtins_collections.go"}},

	// ── Bytes ─────────────────────────────────────────────────────
	{exacts: []string{"bytes", "strToBytes", "bytesToStr",
		"bytesToHex", "hexToBytes", "bytesToBase64", "base64ToBytes",
		"bytesToFloats", "floatsToBytes", "bytesConcat"},
		cat: BuiltinCategory{Display: "Bytes & Encoding", Source: "eval/builtins_bytes.go"}},

	// ── Vectors / AI math ─────────────────────────────────────────
	{exacts: []string{"cosineSim", "dotProduct", "vectorNorm", "vectorAdd",
		"vectorSub", "vectorScale", "vectorNormalize"},
		cat: BuiltinCategory{Display: "AI · Vectors", Source: "eval/builtins_vector.go"}},

	// ── Concurrency ───────────────────────────────────────────────
	{exacts: []string{"async", "await", "channel", "send", "recv",
		"recvNonBlock", "close", "sleep", "_timeNanos", "_timeMillis",
		"atomicIntArray", "atomicFloatArray", "atomicAdd", "atomicLoad",
		"atomicStore", "atomicCompareAndSwap", "pmap", "tryRecv"},
		cat: BuiltinCategory{Display: "Concurrency", Source: "eval/builtins_concurrency.go"}},

	// ── Filesystem ────────────────────────────────────────────────
	{prefixes: []string{"_fs"},
		cat: BuiltinCategory{Display: "Filesystem", Source: "eval/builtins_fs.go"}},

	// ── Process ───────────────────────────────────────────────────
	{prefixes: []string{"_process"},
		cat: BuiltinCategory{Display: "Process", Source: "eval/builtins_process.go"}},

	// ── OS ────────────────────────────────────────────────────────
	{prefixes: []string{"_os"},
		cat: BuiltinCategory{Display: "Operating System", Source: "eval/builtins_os.go"}},

	// ── Bridge / FFI ──────────────────────────────────────────────
	{prefixes: []string{"bridge", "nativeBridge"},
		cat: BuiltinCategory{Display: "Bridge · FFI", Source: "eval/builtins_bridge.go"}},

	// ── MCP ───────────────────────────────────────────────────────
	{prefixes: []string{"_mcp"},
		cat: BuiltinCategory{Display: "Bridge · MCP", Source: "eval/builtins_mcp.go"}},

	// ── HTTP / REST ───────────────────────────────────────────────
	{prefixes: []string{"_http", "rest"},
		cat: BuiltinCategory{Display: "Networking · HTTP", Source: "eval/builtins_http.go"}},

	// ── SQL ───────────────────────────────────────────────────────
	{prefixes: []string{"_db", "db"},
		cat: BuiltinCategory{Display: "Databases · SQL", Source: "eval/builtins_sql.go"}},

	// ── Hash / Crypto ─────────────────────────────────────────────
	{exacts: []string{"_md5", "_sha1", "_sha256", "_sha512",
		"_hmac", "_randomBytes", "_constantTimeEquals",
		"_aesEncrypt", "_aesDecrypt",
		"_bcryptHash", "_bcryptVerify"},
		cat: BuiltinCategory{Display: "Crypto", Source: "eval/builtins_crypto.go"}},

	// ── JSON ──────────────────────────────────────────────────────
	{prefixes: []string{"_json", "json"},
		cat: BuiltinCategory{Display: "Encoding · JSON", Source: "eval/builtins_json.go"}},

	// ── CSV ───────────────────────────────────────────────────────
	{prefixes: []string{"_csv", "csv"},
		cat: BuiltinCategory{Display: "Encoding · CSV", Source: "eval/builtins_csv.go"}},

	// ── PDF / chunking ────────────────────────────────────────────
	{prefixes: []string{"_pdf", "pdf"},
		cat: BuiltinCategory{Display: "Encoding · PDF", Source: "eval/builtins_pdf.go"}},

	// ── Metal / GPU ──────────────────────────────────────────────
	{prefixes: []string{"mtl", "metal", "matmulMPS"},
		cat: BuiltinCategory{Display: "GPU · Metal", Source: "eval/builtins_metal.go"}},

	// ── Graphics primitives ──────────────────────────────────────
	{exacts: []string{"window", "background", "drawFrame",
		"fill", "stroke", "noStroke", "noFill",
		"rect", "ellipse", "circle", "triangle", "line",
		"point", "roundedRect", "arc", "polygon",
		"gradient", "blendMode", "pushMatrix", "popMatrix",
		"translate", "rotate", "scale",
		"pushClip", "popClip",
		"frameCount", "elapsedTime", "winWidth", "winHeight",
		"beginPath", "moveTo", "lineTo", "curveTo", "closePath",
		"fillPath", "strokePath",
		"drawParticles", "drawText", "textWidth", "textHeight",
		"loadFont", "textFont", "textSize",
		"loadImage", "drawImage", "imageWidth", "imageHeight"},
		cat: BuiltinCategory{Display: "Graphics · Drawing", Source: "eval/builtins_graphics.go"}},

	// ── Input ─────────────────────────────────────────────────────
	{exacts: []string{"mouseX", "mouseY", "mouseDown", "mouseClicked",
		"mouseRightDown", "mouseRightClicked",
		"mouseScrollX", "mouseScrollY",
		"keyPressed", "keyDown", "getTypedChars"},
		cat: BuiltinCategory{Display: "Graphics · Input", Source: "eval/builtins_graphics.go"}},

	// ── UI widgets ────────────────────────────────────────────────
	{exacts: []string{"uiBegin", "uiEnd", "uiSetFont", "uiResetFont",
		"uiTheme", "makeTheme",
		"button", "toggle", "checkbox", "slider", "textInput",
		"textArea", "dropdown", "tabs", "table", "accordion",
		"colorPicker", "tooltip", "menu", "menuItem", "splitter",
		"progressBar", "spinner", "scrollArea", "treeView", "listBox"},
		cat: BuiltinCategory{Display: "Graphics · UI Widgets", Source: "eval/builtins_ui.go"}},

	// ── Path ──────────────────────────────────────────────────────
	{prefixes: []string{"path"},
		cat: BuiltinCategory{Display: "Path · Vector Graphics", Source: "eval/builtins_path.go"}},

	// ── Safe / errors ─────────────────────────────────────────────
	{exacts: []string{"safe", "assert"},
		cat: BuiltinCategory{Display: "Core · Safety", Source: "eval/builtins_safe.go"}},

	// ── Module / introspection ────────────────────────────────────
	{exacts: []string{"_scriptDir", "_caller"},
		cat: BuiltinCategory{Display: "Module · Introspection", Source: "eval/builtins_os.go"}},
}

// categoryFor returns the matched category, or a sensible default for
// unclassified builtins.
func categoryFor(name string) BuiltinCategory {
	for _, rule := range categoryRules {
		for _, e := range rule.exacts {
			if e == name {
				return rule.cat
			}
		}
		for _, p := range rule.prefixes {
			if strings.HasPrefix(name, p) {
				return rule.cat
			}
		}
		for _, c := range rule.contains {
			if strings.Contains(name, c) {
				return rule.cat
			}
		}
	}
	return BuiltinCategory{Display: "Builtin", Source: ""}
}

// siblingsInCategory returns up to `max` other builtin names that share
// the same category as `name`. Used by the hover renderer to populate
// the "See also" cross-reference row. Order is stable (alphabetical)
// so the user sees the same "See also" set every time.
func siblingsInCategory(name string, max int) []string {
	target := categoryFor(name)
	if target.Display == "" || target.Display == "Builtin" {
		return nil
	}
	var siblings []string
	for other := range builtinSignatures {
		if other == name {
			continue
		}
		if categoryFor(other).Display == target.Display {
			siblings = append(siblings, other)
		}
	}
	// Stable order — sort alphabetically.
	for i := 0; i < len(siblings); i++ {
		for j := i + 1; j < len(siblings); j++ {
			if siblings[j] < siblings[i] {
				siblings[i], siblings[j] = siblings[j], siblings[i]
			}
		}
	}
	if len(siblings) > max {
		siblings = siblings[:max]
	}
	return siblings
}
