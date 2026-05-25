// doclinks — detect and repair stale inter-document links across kLex docs.
//
// PROBLEM
// Every long-lived project accumulates references between files: READMEs
// pointing at examples, LANGUAGE.TXT citing eval/builtins_*.go, stdlib
// modules name-dropping each other in doc comments. When you rename or
// move a file, those references rot silently — nobody notices until a
// click fails or a search misses.
//
// SOLUTION
// Three resolvers run in order of confidence, and we surface only what
// they can't trivially repair:
//
//   1. Git rename history (`git log --diff-filter=R --name-status`).
//      The most powerful layer — for any file Git knows was renamed,
//      we can map old path to current path deterministically. Zero
//      ambiguity in the common case.
//
//   2. Basename match. Walk the repo, build a basename→path index. A
//      broken link to `auth.md` resolves to any `auth.md` that exists,
//      scored by path-similarity to the link's original location so
//      `docs/auth.md` doesn't collide with `vendor/foo/auth.md`.
//
//   3. Content fingerprint. Each tracked .md/.txt file may carry an
//      anchor like `<!-- doclink: a3f7c9 -->` near the top. The hash
//      is computed from the first non-blank meaningful lines, so it
//      stays stable across renames AND mild edits but changes when
//      the file's role changes. Broken links can carry the same
//      anchor hint inline; we resolve by hash lookup when the path
//      itself is gone and git/basename couldn't help.
//
// USAGE
//   go run ./tools/doclinks                       # audit, no changes
//   go run ./tools/doclinks --apply               # rewrite trivially-fixable links
//   go run ./tools/doclinks --add-anchors <path>  # add doclink anchor(s) to file(s)
//   go run ./tools/doclinks --update-anchors      # refresh hashes in all anchored files
//   go run ./tools/doclinks --file <path>          # audit one file only
//
// Self-contained, no API calls, no cost.
package main

import (
	"crypto/sha1"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ── Constants ────────────────────────────────────────────────────────────────

// Files we scan and treat as link sources / resolution candidates.
var scanExts = map[string]bool{
	".md":  true,
	".MD":  true,
	".txt": true,
	".TXT": true,
}

// Directories we never descend into. Walking them is slow and produces
// noise (vendored copies often shadow first-party files).
var skipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	"build":        true,
	"dist":         true,
}

// External URL schemes — we never try to resolve these.
var externalSchemes = []string{
	"http://", "https://", "mailto:", "ftp://", "ftps://", "ssh://",
	"git://", "data:", "tel:",
}

// fingerprintHashLen is the truncated SHA1 length used in anchors.
// Eight hex chars = 32 bits — collision probability is ~negligible
// across a single repo's file count, and the anchor stays short enough
// to embed comfortably in any doc.
const fingerprintHashLen = 8

// ── Regexes ──────────────────────────────────────────────────────────────────

var (
	// Markdown inline link: `[label](target "optional title")`
	// Captures: 1=label, 2=target, 3=full match (for replacement).
	// We deliberately reject targets containing spaces — those almost always
	// signal a malformed link or non-link bracket sequence.
	reMdLink = regexp.MustCompile(`\[([^\]]*)\]\(([^\s)]+)(?:\s+"[^"]*")?\)`)

	// Inline doclink anchor next to a link: `[…](…) <!-- doclink: a3f7c9 -->`.
	// Used as a resilience hint by the fingerprint resolver when path+git+basename
	// all fail. Optional — most links won't have it.
	reInlineAnchor = regexp.MustCompile(`<!--\s*doclink:\s*([a-fA-F0-9]+)\s*-->`)

	// Anchor declared at the top of a target file:
	//   <!-- doclink: a3f7c9 -->
	// Tracked files opt into fingerprint resolution by carrying this.
	reFileAnchor = regexp.MustCompile(`(?m)^\s*<!--\s*doclink:\s*([a-fA-F0-9]+)\s*-->\s*$`)
)

// ── Types ────────────────────────────────────────────────────────────────────

// link is one detected markdown link in a source file.
type link struct {
	sourceFile  string // absolute path of file containing the link
	sourceRel   string // repo-relative source path (for human display)
	line        int    // 1-based line in source
	col         int    // 1-based column where the `[` of the link starts
	label       string // link text
	rawTarget   string // raw target as written (may include URL fragment)
	cleanTarget string // target with `#fragment` stripped
	anchorHint  string // optional doclink hash hint following the link
	rawMatch    string // entire matched substring including any inline anchor
	resolved    string // empty until we resolve the absolute target
	exists      bool   // true if cleanTarget points at an existing file
	external    bool   // true for http/https/mailto/etc; never validated
}

// resolution describes one suggested repair for a broken link.
type resolution struct {
	method     string // "git-rename" | "basename" | "fingerprint"
	confidence string // "high" | "medium" | "low"
	newTarget  string // proposed repo-relative path
	reason     string // human-readable justification (shown in report)
}

// ── Main ─────────────────────────────────────────────────────────────────────

func main() {
	var (
		apply         bool
		addAnchorsArg string
		updateAnchors bool
		fileFilter    string
	)
	flag.BoolVar(&apply, "apply", false, "rewrite source files in place when a broken link has a single high-confidence resolution")
	flag.StringVar(&addAnchorsArg, "add-anchors", "", "comma-separated list of files to add doclink anchors to (or 'all' for every tracked .md/.txt)")
	flag.BoolVar(&updateAnchors, "update-anchors", false, "recompute and update doclink anchors in every file that already has one")
	flag.StringVar(&fileFilter, "file", "", "audit only this file (repo-relative path)")
	flag.Parse()

	root := findRoot()

	// Anchor-management modes short-circuit the audit.
	if addAnchorsArg != "" {
		addAnchors(root, addAnchorsArg)
		return
	}
	if updateAnchors {
		updateAllAnchors(root)
		return
	}

	// 1. Scan tracked files for links.
	allLinks := scanLinks(root, fileFilter)

	// 2. Validate each link.
	for i := range allLinks {
		validateLink(root, &allLinks[i])
	}

	// 3. Build resolver indices once (expensive bits cached for all queries).
	renames := loadRenameHistory(root)
	basenames := buildBasenameIndex(root)
	anchors := buildAnchorIndex(root)

	// 4. Resolve broken links.
	type fix struct {
		link        link
		resolutions []resolution
	}
	var fixes []fix
	cleanCount := 0
	externalCount := 0
	for _, l := range allLinks {
		if l.external {
			externalCount++
			continue
		}
		if l.exists {
			cleanCount++
			continue
		}
		res := resolveLink(root, l, renames, basenames, anchors)
		fixes = append(fixes, fix{link: l, resolutions: res})
	}

	// 5. Report.
	printHeader(len(allLinks), cleanCount, externalCount, len(fixes))
	for _, f := range fixes {
		printFinding(f.link, f.resolutions)
	}

	// 6. Apply mode — rewrite when there's exactly one high-confidence fix.
	if apply {
		applied := 0
		for _, f := range fixes {
			top := pickAutoApply(f.resolutions)
			if top == nil {
				continue
			}
			if err := rewriteLink(f.link, top.newTarget); err != nil {
				fmt.Fprintf(os.Stderr, "  apply error on %s:%d: %v\n", f.link.sourceRel, f.link.line, err)
				continue
			}
			applied++
		}
		fmt.Printf("\nApplied %d unambiguous rewrites.\n", applied)
	}
}

// pickAutoApply returns the resolution suitable for --apply (single
// high-confidence candidate), or nil if the link should stay manual.
// Multiple high-confidence resolutions are deliberately NOT auto-applied —
// the user picks.
func pickAutoApply(res []resolution) *resolution {
	highs := 0
	var pick *resolution
	for i := range res {
		if res[i].confidence == "high" {
			highs++
			pick = &res[i]
		}
	}
	if highs == 1 {
		return pick
	}
	return nil
}

// ── Scanner ─────────────────────────────────────────────────────────────────

// scanLinks walks the repo from `root` and returns every markdown link
// found in .md and .txt files. If `fileFilter` is non-empty, only that
// file is scanned (path is resolved relative to root).
func scanLinks(root, fileFilter string) []link {
	var out []link

	walk := func(path string, isDir bool) {
		if isDir {
			return
		}
		ext := filepath.Ext(path)
		if !scanExts[ext] {
			return
		}
		links := extractLinksFromFile(root, path)
		out = append(out, links...)
	}

	if fileFilter != "" {
		walk(filepath.Join(root, fileFilter), false)
		return out
	}

	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		walk(path, false)
		return nil
	})
	return out
}

// extractLinksFromFile parses one file and returns every markdown link in
// it, including any inline doclink anchor following the link.
func extractLinksFromFile(root, absPath string) []link {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil
	}
	rel, _ := filepath.Rel(root, absPath)
	lines := strings.Split(string(data), "\n")

	var out []link
	for i, line := range lines {
		matches := reMdLink.FindAllStringSubmatchIndex(line, -1)
		for _, m := range matches {
			label := line[m[2]:m[3]]
			rawTarget := line[m[4]:m[5]]
			fullMatch := line[m[0]:m[1]]

			// Look for an inline anchor immediately after the link.
			anchor := ""
			rawMatch := fullMatch
			tail := line[m[1]:]
			if a := reInlineAnchor.FindStringSubmatchIndex(tail); a != nil {
				// Only count it as belonging to THIS link if it appears
				// before any next non-whitespace content.
				prefix := tail[:a[0]]
				if strings.TrimSpace(prefix) == "" {
					anchor = tail[a[2]:a[3]]
					rawMatch = fullMatch + tail[:a[1]]
				}
			}

			// Skip matches that don't actually look like a link target —
			// most commonly `[label](something)` inside code examples
			// (LANGUAGE.TXT has JSON-RPC samples like [fn](*req["args"])).
			if !isExternal(rawTarget) && !looksLikeAPath(rawTarget) {
				continue
			}

			cleanTarget := rawTarget
			if idx := strings.Index(cleanTarget, "#"); idx >= 0 {
				cleanTarget = cleanTarget[:idx]
			}

			ext := isExternal(rawTarget)
			out = append(out, link{
				sourceFile:  absPath,
				sourceRel:   rel,
				line:        i + 1,
				col:         m[0] + 1,
				label:       label,
				rawTarget:   rawTarget,
				cleanTarget: cleanTarget,
				anchorHint:  anchor,
				rawMatch:    rawMatch,
				external:    ext,
			})
		}
	}
	return out
}

func isExternal(target string) bool {
	for _, scheme := range externalSchemes {
		if strings.HasPrefix(strings.ToLower(target), scheme) {
			return true
		}
	}
	return false
}

// ── Validation ──────────────────────────────────────────────────────────────

// validateLink resolves the link's target against the source file's
// directory and the repo root, marks `exists` true if either resolves,
// and stores the resolved absolute path in `resolved`.
//
// kLex docs use BOTH conventions: README-style relative links (relative
// to the file containing them) and absolute-from-repo-root links
// (e.g. `docs/ASYNC_BEST_PRACTICES.md` written in any file). We accept
// either, in that order.
//
// Intra-document anchors (rawTarget starts with `#`) are NOT broken —
// they point at a heading inside the same file. We mark them as valid
// without doing any filesystem check.
func validateLink(root string, l *link) {
	if l.external {
		return // skip
	}
	if strings.HasPrefix(l.rawTarget, "#") {
		// Intra-doc anchor — not a broken cross-file link.
		l.exists = true
		l.resolved = l.sourceFile
		return
	}
	if l.cleanTarget == "" {
		return
	}
	candidates := []string{
		filepath.Join(filepath.Dir(l.sourceFile), l.cleanTarget),
		filepath.Join(root, l.cleanTarget),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			l.resolved = c
			l.exists = true
			return
		}
	}
}

// looksLikeAPath returns true when `target` plausibly identifies a file
// path or URL rather than an accidental `[foo](bar)` sequence inside a
// code example. We require ONE of:
//
//   - intra-doc anchor (`#section`)
//   - explicit relative marker (`./` or `../`)
//   - path separator (`/`)
//   - recognisable file extension at the end
//
// This stops the regex from flagging things like `[fn](*req["args"])`
// that appear inside JSON-RPC examples in LANGUAGE.TXT.
func looksLikeAPath(target string) bool {
	if target == "" {
		return false
	}
	if strings.HasPrefix(target, "#") {
		return true
	}
	if strings.HasPrefix(target, "./") || strings.HasPrefix(target, "../") {
		return true
	}
	if strings.Contains(target, "/") {
		return true
	}
	if ext := filepath.Ext(target); ext != "" && isPlausibleFileExt(ext) {
		return true
	}
	return false
}

// isPlausibleFileExt vets a file extension against the set of types
// kLex docs actually link to. Conservative on purpose — `.0`, `.1`,
// `.99` and similar numeric suffixes are common in JSON payloads and
// should NOT count as file extensions.
func isPlausibleFileExt(ext string) bool {
	known := map[string]bool{
		".md": true, ".txt": true, ".lex": true, ".go": true,
		".py": true, ".js": true, ".ts": true, ".json": true,
		".yaml": true, ".yml": true, ".toml": true, ".sh": true,
		".rs": true, ".c": true, ".h": true, ".cpp": true,
		".html": true, ".css": true, ".png": true, ".jpg": true,
		".jpeg": true, ".gif": true, ".svg": true, ".pdf": true,
	}
	return known[strings.ToLower(ext)]
}

// ── Resolver ────────────────────────────────────────────────────────────────

// resolveLink tries each resolver in order and returns the candidate
// resolutions. The caller decides which to apply (and how to display
// ambiguity).
//
// We do NOT return early on the first match: a link broken by a rename
// may also have a basename collision elsewhere, and the user benefits
// from seeing both with explicit confidence labels.
func resolveLink(root string, l link, renames map[string]string, basenames map[string][]string, anchors map[string]string) []resolution {
	var out []resolution

	// 1. Git rename history — deterministic mapping for files moved via git mv.
	// The link's `cleanTarget` is what was written; if git knows that path
	// was renamed, that's authoritative.
	oldRel := l.cleanTarget
	// Normalise to a repo-relative slashpath.
	oldRel = strings.TrimPrefix(oldRel, "./")
	if newPath, ok := renames[oldRel]; ok {
		out = append(out, resolution{
			method:     "git-rename",
			confidence: "high",
			newTarget:  newPath,
			reason:     fmt.Sprintf("git history records `%s` was renamed to `%s`", oldRel, newPath),
		})
	}

	// 2. Anchor hint match — if the broken link carries an explicit
	// `<!-- doclink: HASH -->` and we find a file with that anchor, that's
	// an authoritative pointer. Score "high" since the user put the
	// fingerprint there on purpose.
	if l.anchorHint != "" {
		if newPath, ok := anchors[l.anchorHint]; ok {
			out = append(out, resolution{
				method:     "fingerprint",
				confidence: "high",
				newTarget:  newPath,
				reason:     fmt.Sprintf("file `%s` carries the matching doclink anchor `%s`", newPath, l.anchorHint),
			})
		}
	}

	// 3. Basename match — fuzzy fallback. Score by directory similarity
	// to the link's original location. If only one match in the repo,
	// "medium" confidence; if several, "low" (ambiguous).
	//
	// Directory-shape vs file-shape lookup:
	//   - If `rawTarget` ends with `/`, the author meant a directory.
	//     We look up the basename with the trailing slash so we hit
	//     directory entries in the index (and not same-named files).
	//   - Otherwise we look up the file form. If THAT misses but a
	//     same-basenamed DIRECTORY exists, we still surface it as a
	//     candidate — common when an author drops the trailing slash.
	base := filepath.Base(l.cleanTarget)
	dirShape := strings.HasSuffix(l.rawTarget, "/")
	lookupBase := base
	if dirShape {
		lookupBase = base + "/"
	}
	candidates, ok := basenames[lookupBase]
	if !ok && !dirShape {
		// Fall back to directory entries even when the target had no
		// trailing slash — covers `[Tadpole](../tadPole)` style links
		// where the author omitted the slash.
		candidates, ok = basenames[base+"/"]
	}
	if ok && len(candidates) > 0 {
		sortByPathSimilarity(candidates, l.cleanTarget)
		conf := "low"
		if len(candidates) == 1 {
			conf = "high"
		}
		for i, c := range candidates {
			// Avoid duplicating earlier high-confidence findings.
			if alreadyProposed(out, c) {
				continue
			}
			kind := "file"
			if strings.HasSuffix(c, "/") {
				kind = "directory"
			}
			reason := fmt.Sprintf("a %s with basename `%s` exists at `%s`", kind, base, c)
			if len(candidates) > 1 {
				reason += fmt.Sprintf(" (%d candidates total, ranked by path similarity)", len(candidates))
			}
			out = append(out, resolution{
				method:     "basename",
				confidence: conf,
				newTarget:  c,
				reason:     reason,
			})
			if i >= 2 { // cap basename suggestions to top 3
				break
			}
		}
	}

	return out
}

func alreadyProposed(rs []resolution, target string) bool {
	for _, r := range rs {
		if r.newTarget == target {
			return true
		}
	}
	return false
}

// sortByPathSimilarity orders `paths` so that the path most similar to
// `original` comes first. Similarity = number of matching path components
// from the leaf back (so `docs/foo/bar.md` and `docs/foo/baz.md` rank
// higher against each other than `docs/foo/bar.md` and `vendor/bar.md`).
func sortByPathSimilarity(paths []string, original string) {
	origParts := strings.Split(original, "/")
	score := func(p string) int {
		parts := strings.Split(p, "/")
		// Compare from the END (leaf) backwards.
		s := 0
		for i := 1; i <= len(parts) && i <= len(origParts); i++ {
			if parts[len(parts)-i] == origParts[len(origParts)-i] {
				s++
			} else {
				break
			}
		}
		return s
	}
	sort.Slice(paths, func(i, j int) bool {
		si, sj := score(paths[i]), score(paths[j])
		if si != sj {
			return si > sj
		}
		// Tiebreak: shorter path wins (likely first-party over vendored).
		return len(paths[i]) < len(paths[j])
	})
}

// ── Git rename history ──────────────────────────────────────────────────────

// loadRenameHistory parses `git log --diff-filter=R --name-status` and
// returns a map of old-path → current-path. We chain renames: if A→B
// then B→C, the map ends up with A→C and B→C, so a link to either is
// resolved.
//
// Files that no longer exist as a leaf in any chain are dropped — we
// only return targets that currently exist on disk.
func loadRenameHistory(root string) map[string]string {
	out := make(map[string]string)
	cmd := exec.Command("git", "-C", root, "log", "--diff-filter=R",
		"--name-status", "--format=", "--reverse")
	data, err := cmd.Output()
	if err != nil {
		return out // not a git repo, or git unavailable; resolver still runs without this layer
	}
	// Lines look like:  R087\told/path\tnew/path
	// We chain by always rebinding the chain to the latest new path.
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "R") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			continue
		}
		oldPath, newPath := parts[1], parts[2]
		// If oldPath was previously the new-side of an earlier rename,
		// chain-update so the older origin still resolves.
		for k, v := range out {
			if v == oldPath {
				out[k] = newPath
			}
		}
		out[oldPath] = newPath
	}
	// Drop entries whose final target doesn't exist anymore.
	for k, v := range out {
		if _, err := os.Stat(filepath.Join(root, v)); err != nil {
			delete(out, k)
		}
	}
	return out
}

// ── Basename index ──────────────────────────────────────────────────────────

// buildBasenameIndex walks the repo and returns a map of basename →
// list of repo-relative paths. Used by the basename-match resolver.
//
// Both files and directories are indexed:
//
//   - Files: any extension in `isPlausibleFileExt` — broader than the
//     scanner so a `.md` file's broken link to a `.lex` or `.go` can
//     still be resolved.
//   - Directories: any non-skipped directory (we honour `skipDirs` to
//     avoid suggesting vendored / build-output paths). Directory entries
//     are stored with a trailing `/` to disambiguate from same-named
//     files and so the resolver can offer a proper directory-link
//     replacement (with the trailing slash the author originally wrote).
//
// Why directories: kLex docs link to module DIRECTORIES too — e.g.
// `[frogMcp](../frogMcp/)`. If frogMcp moves, the basename resolver
// needs to find the new directory location the same way it finds a
// renamed file.
func buildBasenameIndex(root string) map[string][]string {
	out := make(map[string][]string)
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			// Skip the repo root itself — basename "" is meaningless.
			if path == root {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)
			base := filepath.Base(rel) + "/" // trailing slash marks directory entries
			out[base] = append(out[base], rel+"/")
			return nil
		}
		ext := filepath.Ext(path)
		if !isPlausibleFileExt(ext) {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		base := filepath.Base(rel)
		out[base] = append(out[base], rel)
		return nil
	})
	return out
}

// ── Anchor index ────────────────────────────────────────────────────────────

// buildAnchorIndex walks the repo and returns a map of doclink anchor
// hash → repo-relative path. Used by the fingerprint resolver.
//
// Only files that explicitly contain `<!-- doclink: HASH -->` participate;
// the anchor is the opt-in mechanism.
func buildAnchorIndex(root string) map[string]string {
	out := make(map[string]string)
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !scanExts[filepath.Ext(path)] {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if m := reFileAnchor.FindSubmatch(data); m != nil {
			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)
			out[strings.ToLower(string(m[1]))] = rel
		}
		return nil
	})
	return out
}

// ── Anchor management ───────────────────────────────────────────────────────

// addAnchors inserts a doclink anchor into one or more files. If an
// anchor already exists, the file is left alone (use --update-anchors
// to refresh).
//
// `arg` is a comma-separated list of repo-relative paths, OR the literal
// string "all" to add anchors to every tracked .md/.txt file that
// doesn't already have one.
func addAnchors(root, arg string) {
	var paths []string
	if arg == "all" {
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				if info != nil && info.IsDir() && skipDirs[info.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			if scanExts[filepath.Ext(path)] {
				rel, _ := filepath.Rel(root, path)
				paths = append(paths, rel)
			}
			return nil
		})
	} else {
		paths = strings.Split(arg, ",")
	}

	added, skipped := 0, 0
	for _, rel := range paths {
		rel = strings.TrimSpace(rel)
		if rel == "" {
			continue
		}
		abs := filepath.Join(root, rel)
		data, err := os.ReadFile(abs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  skip %s (read error: %v)\n", rel, err)
			continue
		}
		if reFileAnchor.Find(data) != nil {
			skipped++
			continue
		}
		hash := fingerprint(data)
		anchor := fmt.Sprintf("<!-- doclink: %s -->\n", hash)
		newContent := insertAnchor(string(data), anchor, filepath.Ext(rel))
		if err := os.WriteFile(abs, []byte(newContent), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "  write error on %s: %v\n", rel, err)
			continue
		}
		fmt.Printf("  added %s → %s\n", rel, hash)
		added++
	}
	fmt.Printf("\nAnchors: %d added, %d skipped (already anchored).\n", added, skipped)
}

// updateAllAnchors refreshes the hash in every file that already has an
// anchor. Used when content drift means an existing anchor's hash no
// longer matches the recomputed fingerprint.
//
// Note: changing an anchor breaks any inline anchor hints in OTHER files
// that reference the old hash. Run --apply afterwards (the fingerprint
// resolver can't help once the index changes; basename + git rename
// usually still work).
func updateAllAnchors(root string) {
	updated, unchanged := 0, 0
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !scanExts[filepath.Ext(path)] {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		m := reFileAnchor.FindSubmatchIndex(data)
		if m == nil {
			return nil // no anchor to update
		}
		oldHash := strings.ToLower(string(data[m[2]:m[3]]))
		// Fingerprint computed on the file WITHOUT the anchor line — so the
		// anchor itself doesn't influence its own hash.
		without := string(data[:m[0]]) + string(data[m[1]:])
		newHash := fingerprint([]byte(without))
		if newHash == oldHash {
			unchanged++
			return nil
		}
		newAnchor := fmt.Sprintf("<!-- doclink: %s -->", newHash)
		newContent := string(data[:m[0]]) + newAnchor + string(data[m[1]:])
		if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "  write error on %s: %v\n", path, err)
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		fmt.Printf("  %s: %s → %s\n", rel, oldHash, newHash)
		updated++
		return nil
	})
	fmt.Printf("\nAnchors: %d updated, %d unchanged.\n", updated, unchanged)
}

// fingerprint computes the doclink hash for a piece of content.
//
// We hash the first ~20 lines of "meaningful" content (non-blank, not a
// doclink anchor itself) so the hash:
//   - survives a file rename (the bytes don't change)
//   - survives mild edits below line 20
//   - changes if the file's role / opening content changes substantively
//
// 20 is a balance: short enough that mid-file edits don't shift it,
// long enough that small docs still get a meaningful fingerprint.
func fingerprint(content []byte) string {
	lines := strings.Split(string(content), "\n")
	var sample []string
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if trim == "" {
			continue
		}
		if reFileAnchor.MatchString(line) {
			continue // don't include the anchor in its own input
		}
		sample = append(sample, trim)
		if len(sample) >= 20 {
			break
		}
	}
	h := sha1.Sum([]byte(strings.Join(sample, "\n")))
	return hex.EncodeToString(h[:])[:fingerprintHashLen]
}

// insertAnchor places the anchor near the top of the file content.
// For markdown, we insert after any YAML front matter (`---\n...\n---`)
// and before the first content line. For plain text, we place at the
// very top.
func insertAnchor(content, anchor, ext string) string {
	if ext == ".md" || ext == ".MD" {
		// Skip YAML front matter if present.
		if strings.HasPrefix(content, "---\n") {
			rest := content[4:]
			if idx := strings.Index(rest, "\n---\n"); idx >= 0 {
				cut := 4 + idx + 5
				return content[:cut] + anchor + content[cut:]
			}
		}
		return anchor + content
	}
	return anchor + content
}

// ── Apply ────────────────────────────────────────────────────────────────────

// rewriteLink updates the link's source file in place: replaces the
// original target inside the matched link with `newTarget`. Idempotent
// in the sense that running it twice on an already-fixed link is a
// no-op (the rawMatch won't be present any more).
//
// The label, title, and any inline anchor are preserved.
func rewriteLink(l link, newTarget string) error {
	data, err := os.ReadFile(l.sourceFile)
	if err != nil {
		return err
	}
	// Construct the replacement: same label, new target, drop the
	// fragment unless the original had one we want to keep.
	fragment := ""
	if idx := strings.Index(l.rawTarget, "#"); idx >= 0 {
		fragment = l.rawTarget[idx:]
	}
	newLink := fmt.Sprintf("[%s](%s%s)", l.label, newTarget, fragment)
	if l.anchorHint != "" {
		newLink += fmt.Sprintf(" <!-- doclink: %s -->", l.anchorHint)
	}

	// Replace the FIRST occurrence of the rawMatch — there could be
	// multiple identical links in the file, but our `link` records the
	// one at a specific line/col. We bound the replacement to that
	// neighbourhood by walking line-by-line.
	lines := strings.Split(string(data), "\n")
	if l.line-1 >= len(lines) {
		return fmt.Errorf("source line %d out of range", l.line)
	}
	lines[l.line-1] = strings.Replace(lines[l.line-1], l.rawMatch, newLink, 1)
	return os.WriteFile(l.sourceFile, []byte(strings.Join(lines, "\n")), 0644)
}

// ── Report ──────────────────────────────────────────────────────────────────

func printHeader(total, clean, external, broken int) {
	fmt.Println("╔══════════════════════════════════════════════════════╗")
	fmt.Println("║  kLex doclinks — inter-document link audit           ║")
	fmt.Println("╚══════════════════════════════════════════════════════╝")
	fmt.Printf("  links found:     %d\n", total)
	fmt.Printf("  ✓ valid:         %d\n", clean)
	fmt.Printf("  ↗ external URLs: %d  (not validated)\n", external)
	fmt.Printf("  ✗ broken:        %d\n", broken)
	fmt.Println()
}

func printFinding(l link, res []resolution) {
	fmt.Printf("✗ %s:%d  [%s](%s)\n", l.sourceRel, l.line, l.label, l.rawTarget)
	if l.anchorHint != "" {
		fmt.Printf("    anchor hint: %s\n", l.anchorHint)
	}
	if len(res) == 0 {
		fmt.Println("    no resolution found — repair by hand or remove the link.")
		fmt.Println()
		return
	}
	for _, r := range res {
		fmt.Printf("    → [%s, %s] %s\n", r.method, r.confidence, r.newTarget)
		fmt.Printf("      %s\n", r.reason)
	}
	fmt.Println()
}

// ── Helpers ─────────────────────────────────────────────────────────────────

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
