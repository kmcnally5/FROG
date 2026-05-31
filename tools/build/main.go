// Command build compiles the kLex interpreter and all public tooling into bin/.
//
// Usage (from the repo root):
//
//	go run ./tools/build           # klex + every public tool into bin/
//	go run ./tools/build --wasm    # also build the WASM playground bundle (bin/klex.wasm)
//	go run ./tools/build --list    # show what would be built, then exit (no compile)
//
// Why a Go program and not a shell script or Makefile: kLex is OS-agnostic
// (Windows, Linux, macOS must all work). Go is already the only build
// dependency, so a Go builder runs identically everywhere — no bash, no make,
// no .ps1/.sh pair to keep in sync.
//
// Why auto-discovery: the build targets are found by scanning tools/*,
// vm/cmd/*, and snowball/froglsp for `package main` rather than hardcoding a
// list. Adding or renaming a tool needs no change here, so this builder can
// never drift out of sync with the tree.
//
// bin/ is gitignored — it is build output, not shipped. This command creates
// it on demand.
package main

import (
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type target struct {
	name string // output binary name (with .exe on Windows)
	pkg  string // package path relative to repo root, e.g. "./vm/cmd/vmbuiltins"
}

func main() {
	wasm := flag.Bool("wasm", false, "also build the WASM playground bundle (bin/klex.wasm)")
	list := flag.Bool("list", false, "list what would be built, then exit without compiling")
	flag.Parse()

	root, err := repoRoot()
	if err != nil {
		fatal(err)
	}
	binDir := filepath.Join(root, "bin")

	targets := discover(root)

	if *list {
		fmt.Println("Would build into bin/ :")
		for _, t := range targets {
			fmt.Printf("  %-18s <- %s\n", t.name, t.pkg)
		}
		if *wasm {
			fmt.Printf("  %-18s <- ./cmd/wasm   (GOOS=js GOARCH=wasm, after stdlibgen + vmbuiltins)\n", "klex.wasm")
		}
		return
	}

	if err := os.MkdirAll(binDir, 0o755); err != nil {
		fatal(err)
	}

	fmt.Printf("Building %d binaries into %s\n", len(targets), binDir)
	failed := 0
	for _, t := range targets {
		fmt.Printf("  → %s\n", t.name)
		if err := goBuild(root, filepath.Join(binDir, t.name), t.pkg, nil); err != nil {
			fmt.Fprintf(os.Stderr, "  ✗ %s failed: %v\n", t.name, err)
			failed++
		}
	}

	if *wasm {
		if err := buildWasm(root, binDir); err != nil {
			fmt.Fprintf(os.Stderr, "  ✗ %v\n", err)
			failed++
		}
	}

	if failed > 0 {
		fmt.Fprintf(os.Stderr, "\n%d target(s) failed.\n", failed)
		os.Exit(1)
	}
	fmt.Printf("\n✓ Done. Binaries are in %s (gitignored — build output, not shipped).\n", binDir)
}

// discover returns the interpreter plus every auto-discovered main package
// under tools/ and vm/cmd/, plus the froglsp language server.
func discover(root string) []target {
	targets := []target{{name: binName("klex"), pkg: "."}}

	for _, parent := range []string{"tools", filepath.Join("vm", "cmd")} {
		entries, err := os.ReadDir(filepath.Join(root, parent))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() || e.Name() == "build" { // never build ourselves
				continue
			}
			pkgDir := filepath.Join(parent, e.Name())
			if isMainPackage(filepath.Join(root, pkgDir)) {
				targets = append(targets, target{
					name: binName(e.Name()),
					pkg:  "./" + filepath.ToSlash(pkgDir),
				})
			}
		}
	}

	// The LSP server is a public binary that lives outside tools/.
	if isMainPackage(filepath.Join(root, "snowball", "froglsp")) {
		targets = append(targets, target{name: binName("froglsp"), pkg: "./snowball/froglsp"})
	}

	sort.Slice(targets, func(i, j int) bool { return targets[i].name < targets[j].name })
	return targets
}

// buildWasm mirrors examples/playground/serve.sh: the embedded stdlib map and
// the WASM builtin table are generated files that a fresh clone does not have,
// so they must be regenerated before the WASM binary will compile.
func buildWasm(root, binDir string) error {
	fmt.Printf("  → klex.wasm  (regenerating embedded stdlib + WASM builtin table first)\n")
	if err := goRun(root, "./tools/stdlibgen", nil); err != nil {
		return fmt.Errorf("stdlibgen: %w", err)
	}
	if err := goRun(root, "./vm/cmd/vmbuiltins", nil); err != nil {
		return fmt.Errorf("vmbuiltins: %w", err)
	}
	env := []string{"GOOS=js", "GOARCH=wasm", "CGO_ENABLED=0"}
	if err := goBuild(root, filepath.Join(binDir, "klex.wasm"), "./cmd/wasm", env); err != nil {
		return fmt.Errorf("klex.wasm: %w", err)
	}
	return nil
}

func goBuild(root, out, pkg string, extraEnv []string) error {
	cmd := exec.Command("go", "build", "-o", out, pkg)
	cmd.Dir = root
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if extraEnv != nil {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	return cmd.Run()
}

func goRun(root, pkg string, extraEnv []string) error {
	cmd := exec.Command("go", "run", pkg)
	cmd.Dir = root
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if extraEnv != nil {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	return cmd.Run()
}

// binName appends .exe on Windows for normal binaries (never for *.wasm).
func binName(n string) string {
	if runtime.GOOS == "windows" && !strings.HasSuffix(n, ".wasm") {
		return n + ".exe"
	}
	return n
}

// isMainPackage reports whether dir contains at least one non-test .go file
// whose package clause is `main`. Uses PackageClauseOnly so build tags don't
// hide a main package whose files are all platform-specific.
func isMainPackage(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, parser.PackageClauseOnly)
		if err == nil && f.Name.Name == "main" {
			return true
		}
	}
	return false
}

func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil && strings.Contains(string(data), "module klex") {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find repo root (go.mod with 'module klex') from %s", dir)
		}
		dir = parent
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "build:", err)
	os.Exit(1)
}
