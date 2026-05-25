package main

// verify.go — `kpkg verify` command. Drift detection vs klex.lock.
//
// Re-scans the stdlib, computes hashes, and compares them against the
// hashes recorded in klex.lock. Exits zero when everything matches and
// non-zero when ANY of the following are true:
//
//   - the lockfile is missing
//   - the lockfile is unreadable / corrupt / wrong schema
//   - a module recorded in the lockfile is missing from disk
//   - a module on disk is missing from the lockfile (new file)
//   - a module's hash has changed since the lockfile was written
//
// The exit code is deliberately a single bit — CI scripts and pre-
// commit hooks want a yes/no answer. The human-readable diff is on
// stdout for the person debugging the failure.

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func runVerify(args []string) {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	stdlibRel := fs.String("stdlib", "stdlib", "directory to scan (relative to repo root)")
	lockRel := fs.String("lockfile", "klex.lock", "lockfile path (relative to repo root)")
	_ = fs.Parse(args)

	root := findRoot()
	lockPath := resolveAtRoot(root, *lockRel)

	lf, err := readLockFile(lockPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kpkg verify: %v\n", err)
		os.Exit(1)
	}

	mods := scanStdlib(filepath.Join(root, *stdlibRel))
	drift := diffAgainstLock(mods, lf)
	if drift.clean() {
		fmt.Printf("✓ %s up-to-date — %d modules verified\n", *lockRel, len(mods))
		return
	}
	drift.report(*lockRel)
	os.Exit(1)
}

// driftReport bundles every kind of mismatch a verify pass can find.
// We collect them rather than fail-fast so the user sees the full
// picture in one run instead of fixing one and re-running.
type driftReport struct {
	missingFromDisk []lockModule        // in lockfile, not on disk
	newOnDisk       []moduleInfo        // on disk, not in lockfile
	hashChanged     []hashChange        // present in both, different hash
	versionChanged  []versionChange     // present in both, different declared version
}

type hashChange struct {
	name      string
	oldHash   string // from lockfile (with "sha256:" prefix)
	newHash   string // freshly computed (with "sha256:" prefix)
}

type versionChange struct {
	name       string
	oldVersion string
	newVersion string
}

func (d *driftReport) clean() bool {
	return len(d.missingFromDisk) == 0 &&
		len(d.newOnDisk) == 0 &&
		len(d.hashChanged) == 0 &&
		len(d.versionChanged) == 0
}

// diffAgainstLock produces a driftReport by walking both sides.
func diffAgainstLock(mods []moduleInfo, lf *lockFile) *driftReport {
	d := &driftReport{}

	// Index both sides by module name for O(1) lookup.
	onDisk := make(map[string]moduleInfo, len(mods))
	for _, m := range mods {
		onDisk[m.Name] = m
	}
	inLock := make(map[string]lockModule, len(lf.Modules))
	for _, lm := range lf.Modules {
		inLock[lm.Name] = lm
	}

	// Lockfile-side walk: find missing-from-disk and changed-hash.
	for _, lm := range lf.Modules {
		m, ok := onDisk[lm.Name]
		if !ok {
			d.missingFromDisk = append(d.missingFromDisk, lm)
			continue
		}
		freshHash := "sha256:" + m.Hash
		if freshHash != lm.Hash {
			d.hashChanged = append(d.hashChanged, hashChange{
				name:    lm.Name,
				oldHash: lm.Hash,
				newHash: freshHash,
			})
		}
		// Compare declared version too — surfaces metadata-header
		// edits even when the rest of the file hasn't been re-pinned.
		if lm.Version != m.Version {
			d.versionChanged = append(d.versionChanged, versionChange{
				name:       lm.Name,
				oldVersion: lm.Version,
				newVersion: m.Version,
			})
		}
	}

	// Disk-side walk: find new-on-disk (not in lockfile).
	for _, m := range mods {
		if _, ok := inLock[m.Name]; !ok {
			d.newOnDisk = append(d.newOnDisk, m)
		}
	}
	return d
}

// report writes the human-readable diff to stdout. Sections are only
// printed when they have entries — a clean section list reads as well
// as a brief summary.
func (d *driftReport) report(lockRel string) {
	fmt.Printf("✗ %s is out of date — run `kpkg lock` to refresh after verifying changes\n\n", lockRel)

	if len(d.missingFromDisk) > 0 {
		fmt.Printf("Modules in lockfile but missing from stdlib (%d):\n", len(d.missingFromDisk))
		for _, lm := range d.missingFromDisk {
			fmt.Printf("  - %s  (was %s)\n", lm.Name, lm.Path)
		}
		fmt.Println()
	}
	if len(d.newOnDisk) > 0 {
		fmt.Printf("New modules on disk but not in lockfile (%d):\n", len(d.newOnDisk))
		for _, m := range d.newOnDisk {
			fmt.Printf("  + %s  (%s)\n", m.Name, m.Path)
		}
		fmt.Println()
	}
	if len(d.hashChanged) > 0 {
		fmt.Printf("Modules with changed content (%d):\n", len(d.hashChanged))
		for _, hc := range d.hashChanged {
			fmt.Printf("  ~ %s\n", hc.name)
			fmt.Printf("      lockfile: %s\n", short(hc.oldHash))
			fmt.Printf("      on disk:  %s\n", short(hc.newHash))
		}
		fmt.Println()
	}
	if len(d.versionChanged) > 0 {
		fmt.Printf("Modules with changed declared version (%d):\n", len(d.versionChanged))
		for _, vc := range d.versionChanged {
			old := vc.oldVersion
			if old == "" {
				old = "(none)"
			}
			nu := vc.newVersion
			if nu == "" {
				nu = "(none)"
			}
			fmt.Printf("  ~ %s  %s → %s\n", vc.name, old, nu)
		}
		fmt.Println()
	}
}

// short trims a "sha256:<hex>" hash to a readable prefix for terminal
// reports. We keep enough characters to be unambiguous within the
// stdlib (12 hex chars after the prefix is plenty for ~50 modules).
func short(h string) string {
	if len(h) > 7+12 { // "sha256:" + 12 chars
		return h[:7+12] + "…"
	}
	return h
}
