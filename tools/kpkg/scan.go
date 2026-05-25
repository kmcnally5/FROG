package main

// scan.go — module discovery and metadata extraction.
//
// scanStdlib walks the configured stdlib directory and returns one
// moduleInfo per .lex file. Each moduleInfo carries:
//
//   - the canonical module name (header @module, else basename)
//   - the metadata header values that ARE present (any subset)
//   - the content hash (sha256 of the file bytes)
//   - the import list (every `import "X"` line, in source order)
//   - the export list (every top-level `fn name(...)` declaration whose
//     name does NOT start with `_`)
//
// The scanner is intentionally lenient. Missing fields don't error —
// they just leave the corresponding field empty. That makes incremental
// adoption painless: stamp a header into one file at a time, the rest
// keep working with sensible defaults.

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// moduleInfo is the per-file scan result. Used by every kpkg
// subcommand — list, info, lock, verify.
type moduleInfo struct {
	// Canonical name. Comes from `// @module foo` if present, otherwise
	// the filename without its `.lex` extension.
	Name string

	// Repo-relative path of the source file, e.g. "stdlib/rest.lex".
	// Always uses forward slashes for stable lockfile output across
	// macOS / Linux / Windows.
	Path string

	// SHA-256 of the file bytes, hex-encoded. Forms the integrity
	// check basis for kpkg lock / kpkg verify.
	Hash string

	// Size in bytes. Cheap to compute alongside the hash, useful in
	// the inventory listing.
	Size int64

	// Metadata-header values. Any field can be empty.
	Version string // @version
	Since   string // @since
	Author  string // @author
	Summary string // @summary

	// Import targets — the string literal inside each `import "..."`
	// line at the top level of the file. Order preserved.
	Imports []string

	// Top-level exported function names (no leading `_`). Order
	// preserved (source order).
	Exports []string
}

// HasMetadata reports whether any @tag was set in the header. Used
// by `kpkg list` to render a small marker next to versioned modules.
func (m *moduleInfo) HasMetadata() bool {
	return m.Version != "" || m.Since != "" || m.Author != "" || m.Summary != ""
}

// ── Scan entry point ────────────────────────────────────────────────────────

// scanStdlib walks stdlibDir and returns one moduleInfo per .lex file
// found (including nested subdirectories — e.g. stdlib/ai/anthropic.lex).
// The result is sorted by Name for stable output.
//
// Errors reading individual files do not abort the scan — the affected
// module is simply omitted. A scanner that bails on one bad file would
// be useless for the "inventory" use case.
func scanStdlib(stdlibDir string) []moduleInfo {
	var mods []moduleInfo
	_ = filepath.Walk(stdlibDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(info.Name()), ".lex") {
			return nil
		}
		m := scanOne(path, info)
		if m != nil {
			mods = append(mods, *m)
		}
		return nil
	})
	sort.Slice(mods, func(i, j int) bool { return mods[i].Name < mods[j].Name })
	return mods
}

// scanOne reads one .lex file and produces its moduleInfo. Returns nil
// if the file can't be read at all (so the caller can skip it).
func scanOne(path string, info os.FileInfo) *moduleInfo {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	root := findRoot()
	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = path
	}
	rel = filepath.ToSlash(rel)

	m := &moduleInfo{
		Path: rel,
		Size: info.Size(),
		Hash: hashBytes(data),
		Name: defaultName(info.Name()),
	}

	parseHeaderTags(data, m)
	m.Imports = extractImports(data)
	m.Exports = extractExports(data)
	return m
}

// defaultName turns "rest.lex" into "rest". When no @module tag is
// present, this is the module's canonical name.
func defaultName(filename string) string {
	return strings.TrimSuffix(strings.TrimSuffix(filename, ".lex"), ".LEX")
}

// hashBytes returns the lowercase hex SHA-256 of data.
func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// ── Metadata header parsing ─────────────────────────────────────────────────

// reHeaderTag matches `// @tag value` (any indent, any whitespace after
// the colon, case-insensitive tag). The leading `// ` may have any
// amount of whitespace before the `@`. We bind the tag name and the
// value separately.
var reHeaderTag = regexp.MustCompile(`^\s*//\s*@([A-Za-z_]+)\s+(.+?)\s*$`)

// parseHeaderTags reads the file's leading comment block and populates
// the matching moduleInfo fields. We stop at the first non-comment line
// (or first blank line that's followed by a non-comment line). Tags
// after that point are ignored — they're inside the code body and
// don't belong to the module's header.
func parseHeaderTags(data []byte, m *moduleInfo) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	// Stdlib files comfortably fit; bump the buffer in case of long
	// generated lines.
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	inHeader := true
	for scanner.Scan() {
		line := scanner.Text()
		trim := strings.TrimSpace(line)
		switch {
		case trim == "":
			// Blank line inside the header is fine; keep scanning.
			continue
		case strings.HasPrefix(trim, "//"):
			if !inHeader {
				return
			}
			if mt := reHeaderTag.FindStringSubmatch(line); mt != nil {
				applyTag(m, strings.ToLower(mt[1]), strings.TrimSpace(mt[2]))
			}
		default:
			// First code line — header is done.
			inHeader = false
			return
		}
	}
}

// applyTag assigns a recognised tag value to the corresponding
// moduleInfo field. Unknown tags are silently ignored — that way
// authors can add custom annotations (`// @owner team-x`) without kpkg
// complaining, and a future version of kpkg can pick those tags up
// without breaking the older binary.
func applyTag(m *moduleInfo, tag, value string) {
	switch tag {
	case "module":
		m.Name = value
	case "version":
		m.Version = value
	case "since":
		m.Since = value
	case "author":
		m.Author = value
	case "summary":
		m.Summary = value
	}
}

// ── Imports and exports ─────────────────────────────────────────────────────

// reImport matches an `import "X"` line at the top of a file. The
// optional ` as Y` alias is captured but currently unused.
var reImport = regexp.MustCompile(`^\s*import\s+"([^"]+)"(?:\s+as\s+[A-Za-z_][A-Za-z0-9_]*)?\s*$`)

// extractImports returns every import path declared in the file, in
// source order. Duplicates are preserved on purpose — they're a doc
// bug worth surfacing, not something kpkg should hide.
func extractImports(data []byte) []string {
	var out []string
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if mt := reImport.FindStringSubmatch(line); mt != nil {
			out = append(out, mt[1])
		}
	}
	return out
}

// reTopLevelFn matches column-zero `fn name(...)` declarations — the
// kLex convention for an exported function. Indented fns (methods,
// closures, nested definitions) are deliberately skipped.
var reTopLevelFn = regexp.MustCompile(`^fn\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)

// extractExports returns the names of every column-zero `fn` whose
// name does not start with `_` (kLex's convention for module-private
// functions). Order is source order.
func extractExports(data []byte) []string {
	var out []string
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if mt := reTopLevelFn.FindStringSubmatch(line); mt != nil {
			name := mt[1]
			if !strings.HasPrefix(name, "_") {
				out = append(out, name)
			}
		}
	}
	return out
}
