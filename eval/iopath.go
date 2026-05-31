package eval

import "strings"

// IOScheme identifies the transport/backend that an fs.* or http.* call
// should route to. Used by the Phase 5 URL-scheme dispatcher so that
// scripts can write `fs.read("opfs://config.json")` or
// `fs.read("https://example.com/x")` and have the same call work across
// desktop and browser builds.
type IOScheme int

const (
	SchemeFile  IOScheme = iota // file:// or bare path — native filesystem
	SchemeOPFS                  // opfs://             — Origin Private File System (browser)
	SchemeHTTP                  // http://             — fetch()
	SchemeHTTPS                 // https://            — fetch()
	SchemeData                  // data:               — RFC 2397 data URI (read-only)
)

func (s IOScheme) String() string {
	switch s {
	case SchemeFile:
		return "file"
	case SchemeOPFS:
		return "opfs"
	case SchemeHTTP:
		return "http"
	case SchemeHTTPS:
		return "https"
	case SchemeData:
		return "data"
	}
	return "unknown"
}

// ParsedPath is the structured form of an I/O path argument after scheme
// detection. Original is always the verbatim input; Remainder is the
// scheme-specific payload that backends consume:
//
//   file://, opfs://   Remainder = path without the scheme prefix
//   http(s)://, data:  Remainder = unchanged (backends need the full URI)
//   bare path          Remainder = the path itself
type ParsedPath struct {
	Scheme    IOScheme
	Original  string
	Remainder string
}

// ParseIOPath classifies an I/O path argument by scheme prefix. Bare
// paths (no scheme) and unrecognised schemes both fall through to
// SchemeFile so existing native behaviour is preserved — a file system
// open of a literal "weirdscheme://foo" will fail with the usual OS
// error rather than silently becoming a different kind of operation.
//
// Windows drive letters (e.g. C:\Users\foo) are detected before scheme
// parsing so they aren't misread as scheme "c".
func ParseIOPath(path string) ParsedPath {
	out := ParsedPath{Original: path, Scheme: SchemeFile, Remainder: path}

	if path == "" {
		return out
	}

	// Windows drive letter — single alpha char + ':' + separator. Must
	// short-circuit before scheme parsing or "C:\foo" would be read as
	// scheme "c" with remainder "\foo".
	if len(path) >= 3 && path[1] == ':' && isSchemeAlpha(path[0]) {
		if path[2] == '\\' || path[2] == '/' {
			return out
		}
	}

	colonIdx := strings.IndexByte(path, ':')
	if colonIdx < 2 {
		// No colon, or single-letter prefix — bare path (defense-in-depth
		// against the drive-letter case).
		return out
	}
	schemeStr := path[:colonIdx]
	if !isAllSchemeAlpha(schemeStr) {
		return out
	}

	rest := path[colonIdx+1:]
	switch strings.ToLower(schemeStr) {
	case "file":
		out.Scheme = SchemeFile
		out.Remainder = strings.TrimPrefix(rest, "//")
	case "opfs":
		out.Scheme = SchemeOPFS
		out.Remainder = strings.TrimPrefix(rest, "//")
	case "http":
		out.Scheme = SchemeHTTP
		out.Remainder = path
	case "https":
		out.Scheme = SchemeHTTPS
		out.Remainder = path
	case "data":
		out.Scheme = SchemeData
		out.Remainder = path
	}
	return out
}

func isSchemeAlpha(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isAllSchemeAlpha(s string) bool {
	for i := 0; i < len(s); i++ {
		if !isSchemeAlpha(s[i]) {
			return false
		}
	}
	return true
}
