// kpkg — kLex package manager.
//
// kpkg is the inventory + integrity tool for kLex's standard library.
// It does NOT (yet) fetch third-party packages, resolve dependency
// graphs, or manage a registry — kLex's ecosystem is one author and
// one bundled stdlib, and any tooling that pretends otherwise is
// over-engineering.
//
// What kpkg does today:
//
//   - `kpkg list`             one-line-per-module inventory of stdlib/.
//                             Shows module name, version (from header),
//                             content hash, and exported-function count.
//
//   - `kpkg info <module>`    deep view of one module: full metadata
//                             header, file path, content hash, imports
//                             (i.e. its in-tree dependencies), and the
//                             list of exported top-level `fn` names.
//
//   - `kpkg lock`             writes klex.lock at the repo root: the
//                             current kLex version + per-module hash
//                             + per-module declared version. Reproducible
//                             pinning.
//
//   - `kpkg verify`           recomputes hashes for every module and
//                             compares to klex.lock. Exits non-zero on
//                             any mismatch — CI-friendly drift detection.
//
// Metadata header convention:
//
// kpkg reads optional `// @tag value` lines from each .lex file's
// leading comment block. All tags are optional; missing tags fall back
// to sensible defaults (module name = filename, version = "unversioned").
// Recognised tags:
//
//   // @module     rest          canonical module name (else: filename)
//   // @version    1.0.0         semver string (else: "unversioned")
//   // @since      klex 0.3.35   first kLex release that shipped this module
//   // @author     karl          maintainer (free text)
//   // @summary    one-liner     short description (else: first prose line)
//
// The headers are plain `//` comments — the kLex interpreter ignores
// them, so adoption is incremental: stamp a header into one module at
// a time as you touch it, the others continue to work without any.
//
// USAGE
//
//	go run ./tools/kpkg list
//	go run ./tools/kpkg info rest
//	go run ./tools/kpkg lock
//	go run ./tools/kpkg verify
//
// No external dependencies. Pure Go.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// kpkgVersion is the semver of this kpkg binary.
//
// Bumping rules:
//
//	patch (0.1.X)   — bug fix, no behaviour or schema change
//	minor (0.X.0)   — new subcommand, new flag, additive lockfile field
//	major (X.0.0)   — lockfile schema bump that's not backward-readable,
//	                  removed/renamed subcommand, breaking flag rename
//
// The version is stamped into every klex.lock under the `kpkg` field
// so a future kpkg can detect — and migrate or reject — lockfiles
// written by an older release.
const kpkgVersion = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "list":
		runList(os.Args[2:])
	case "info":
		runInfo(os.Args[2:])
	case "lock":
		runLock(os.Args[2:])
	case "verify":
		runVerify(os.Args[2:])
	case "version", "--version":
		fmt.Println("kpkg " + kpkgVersion)
	case "help", "--help", "-h":
		printHelp()
	default:
		fmt.Fprintf(os.Stderr, "kpkg: unknown command %q\n\n", os.Args[1])
		printHelp()
		os.Exit(2)
	}
}

func printHelp() {
	fmt.Fprintln(os.Stderr, `kpkg — kLex package manager

Commands:
  list                          one-line-per-module inventory of stdlib/
  info <module>                 full metadata, imports, and exports for one module
  lock                          write klex.lock with current versions + hashes
  verify                        recompute hashes; non-zero exit on drift vs lockfile
  version                       print kpkg version and exit (also: --version)

Flags (per-command):
  list / lock / verify
    --stdlib   directory to scan (default "stdlib")
  lock / verify
    --lockfile path to klex.lock (default "klex.lock" at repo root)
  list
    --format   table | json (default "table")

Metadata headers (optional, in any .lex file's leading comment block):
  // @module    <name>        canonical module name (else: filename)
  // @version   <semver>      e.g. 1.0.0 (else: "unversioned")
  // @since     <klex ver>    e.g. "klex 0.3.35"
  // @author    <name>        free text
  // @summary   <one-liner>   short description

Examples:
  go run ./tools/kpkg list
  go run ./tools/kpkg info rest
  go run ./tools/kpkg lock
  go run ./tools/kpkg verify`)
}

// ── Common helpers ──────────────────────────────────────────────────────────

// findRoot walks up from the current working directory until it finds
// a go.mod whose `module` line names `klex`. Mirrors frogdocs's
// findRoot — kept independent so the tool has no cross-tool imports.
func findRoot() string {
	dir, _ := os.Getwd()
	for {
		data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil && strings.Contains(string(data), "module klex") {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "."
		}
		dir = parent
	}
}

// fatal prints a message to stderr prefixed with the tool name and exits.
func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "kpkg: "+format+"\n", args...)
	os.Exit(1)
}

// resolveAtRoot returns path as-is when it's already absolute, else
// joins it under root. filepath.Join's quirk of treating an absolute
// second argument as relative would otherwise silently misplace a
// caller-supplied --lockfile=/tmp/foo.lock.
func resolveAtRoot(root, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, path)
}
