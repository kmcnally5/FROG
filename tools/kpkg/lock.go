package main

// lock.go — `kpkg lock` and the shared lockfile schema.
//
// klex.lock is a tiny JSON document at the repo root pinning every
// stdlib module by content hash and declared version. It exists for
// reproducibility: a kLex project that ships a klex.lock can be
// verified — bit-for-bit — against the stdlib it was tested against.
//
// Schema (intentionally minimal; add fields only when there's a
// concrete need, never speculatively):
//
//	{
//	  "schema":  1,
//	  "kpkg":    "0.1.0",
//	  "klex":    "0.3.35",
//	  "modules": [
//	    {
//	      "name":    "rest",
//	      "path":    "stdlib/rest.lex",
//	      "version": "1.0.0",     // omitted when unversioned
//	      "hash":    "sha256:...",
//	      "size":    1842
//	    },
//	    ...
//	  ]
//	}
//
// Modules are sorted by name for stable diffs across regenerations.
// `klex` is the running kLex version (read from the binary if
// available; literal "unknown" if not — we don't pretend to know).
//
// Deliberately NO `generated` timestamp. Running `kpkg lock` on an
// unchanged stdlib must produce a byte-identical lockfile so git
// only shows a diff when stdlib content actually changed. The commit
// time IS the lockfile's timestamp — `git log klex.lock` tells you
// when it was last regenerated.

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// lockFile is the on-disk shape of klex.lock. Field tags use lower-
// case JSON names so the file is conventional for any external tool
// that might read it.
type lockFile struct {
	Schema  int          `json:"schema"`
	Kpkg    string       `json:"kpkg"` // semver of the kpkg binary that wrote this file
	Klex    string       `json:"klex"`
	Modules []lockModule `json:"modules"`
}

// lockModule is one module's entry in the lockfile. We DON'T inline
// the full moduleInfo — most of those fields (author, summary,
// imports, exports) don't belong in a reproducibility manifest. Path
// + hash + size + version is the integrity primitive.
type lockModule struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Version string `json:"version,omitempty"` // omitted when unversioned
	Hash    string `json:"hash"`              // "sha256:<hex>"
	Size    int64  `json:"size"`
}

const lockSchemaVersion = 1

func runLock(args []string) {
	fs := flag.NewFlagSet("lock", flag.ExitOnError)
	stdlibRel := fs.String("stdlib", "stdlib", "directory to scan (relative to repo root)")
	lockRel := fs.String("lockfile", "klex.lock", "lockfile path (relative to repo root)")
	_ = fs.Parse(args)

	root := findRoot()
	mods := scanStdlib(filepath.Join(root, *stdlibRel))
	if len(mods) == 0 {
		fatal("no .lex files found under %s/", *stdlibRel)
	}

	lock := lockFile{
		Schema:  lockSchemaVersion,
		Kpkg:    kpkgVersion,
		Klex:    detectKlexVersion(root),
		Modules: make([]lockModule, 0, len(mods)),
	}
	for _, m := range mods {
		lock.Modules = append(lock.Modules, lockModule{
			Name:    m.Name,
			Path:    m.Path,
			Version: m.Version,
			Hash:    "sha256:" + m.Hash,
			Size:    m.Size,
		})
	}

	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		fatal("encode lockfile: %v", err)
	}
	// Trailing newline — POSIX convention, friendlier to text tooling.
	data = append(data, '\n')

	out := resolveAtRoot(root, *lockRel)
	if err := os.WriteFile(out, data, 0644); err != nil {
		fatal("write %s: %v", out, err)
	}
	fmt.Printf("wrote %s — %d modules pinned (%s)\n",
		*lockRel, len(lock.Modules), lock.Klex)
}

// detectKlexVersion asks the kLex binary for its version string. If
// klex isn't on PATH or doesn't respond we record "unknown" — never
// guess. The lockfile is honest about what it doesn't know.
func detectKlexVersion(root string) string {
	// Try the project-local build first (./klex), then a system one.
	candidates := []string{filepath.Join(root, "klex"), "klex"}
	for _, bin := range candidates {
		out, err := exec.Command(bin, "--version").Output()
		if err != nil {
			continue
		}
		v := strings.TrimSpace(string(out))
		if v != "" {
			return v
		}
	}
	return "unknown"
}

// readLockFile loads a lockfile from disk and unmarshals it. Used by
// `kpkg verify`. Returns a useful error message rather than just the
// raw JSON parse failure.
func readLockFile(path string) (*lockFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var lf lockFile
	if err := json.Unmarshal(data, &lf); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if lf.Schema != lockSchemaVersion {
		return nil, fmt.Errorf("%s: unsupported schema version %d (this kpkg expects %d)",
			path, lf.Schema, lockSchemaVersion)
	}
	return &lf, nil
}
