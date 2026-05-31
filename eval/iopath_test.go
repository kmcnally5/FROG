package eval

import "testing"

func TestParseIOPath(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		scheme    IOScheme
		remainder string
	}{
		// Empty / bare paths
		{"empty", "", SchemeFile, ""},
		{"unix-abs", "/etc/hostname", SchemeFile, "/etc/hostname"},
		{"unix-rel", "./foo.txt", SchemeFile, "./foo.txt"},
		{"name-only", "foo.txt", SchemeFile, "foo.txt"},

		// file://
		{"file-tripleslash", "file:///etc/hostname", SchemeFile, "/etc/hostname"},
		{"file-noslashes", "file:relative.txt", SchemeFile, "relative.txt"},
		{"file-uppercase-scheme", "FILE://upper.txt", SchemeFile, "upper.txt"},
		{"file-doubleslash", "file://etc/hostname", SchemeFile, "etc/hostname"},

		// opfs://
		{"opfs-simple", "opfs://config.json", SchemeOPFS, "config.json"},
		{"opfs-nested", "opfs://dir/sub/file.json", SchemeOPFS, "dir/sub/file.json"},
		{"opfs-empty-remainder", "opfs://", SchemeOPFS, ""},
		{"opfs-uppercase", "OPFS://x", SchemeOPFS, "x"},

		// http / https — Remainder keeps the full URL for fetch
		{"https-simple", "https://example.com/x.json", SchemeHTTPS, "https://example.com/x.json"},
		{"http-simple", "http://example.com", SchemeHTTP, "http://example.com"},
		{"https-uppercase", "HTTPS://Example.COM/", SchemeHTTPS, "HTTPS://Example.COM/"},

		// data:
		{"data-simple", "data:text/plain;base64,SGVsbG8=", SchemeData, "data:text/plain;base64,SGVsbG8="},

		// Windows drive letters MUST NOT be misread as scheme "c"
		{"win-drive-backslash", `C:\Users\foo`, SchemeFile, `C:\Users\foo`},
		{"win-drive-slash", "C:/Users/foo", SchemeFile, "C:/Users/foo"},
		{"win-drive-lower", `c:\users`, SchemeFile, `c:\users`},
		{"win-drive-relative-noseparator", "C:foo", SchemeFile, "C:foo"},
		{"win-drive-bare", "C:", SchemeFile, "C:"},

		// Unknown schemes fall through to SchemeFile (preserves current
		// native behaviour — os.Open will error on the literal path)
		{"unknown-scheme", "myscheme://something", SchemeFile, "myscheme://something"},
		{"ftp-scheme", "ftp://server/x", SchemeFile, "ftp://server/x"},
		{"plus-in-scheme", "git+ssh://repo", SchemeFile, "git+ssh://repo"}, // '+' breaks isAllSchemeAlpha
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseIOPath(tc.in)
			if got.Scheme != tc.scheme {
				t.Errorf("scheme: got %s, want %s", got.Scheme, tc.scheme)
			}
			if got.Remainder != tc.remainder {
				t.Errorf("remainder: got %q, want %q", got.Remainder, tc.remainder)
			}
			if got.Original != tc.in {
				t.Errorf("original: got %q, want %q", got.Original, tc.in)
			}
		})
	}
}

func TestIOSchemeString(t *testing.T) {
	cases := map[IOScheme]string{
		SchemeFile:  "file",
		SchemeOPFS:  "opfs",
		SchemeHTTP:  "http",
		SchemeHTTPS: "https",
		SchemeData:  "data",
		IOScheme(99): "unknown",
	}
	for s, want := range cases {
		if got := s.String(); got != want {
			t.Errorf("%d.String(): got %q, want %q", int(s), got, want)
		}
	}
}
