package main

import (
	"container/list"
	"klex/ast"
	"os"
	"sync"
	"time"
)

// parsed_cache.go — mtime-keyed LRU of parsed kLex files.
//
// Cross-file hover (e.g. hovering `foo.bar` where `foo` is a kLex module)
// resolves the symbol by reading + parsing the imported .lex file. The naive
// implementation does this on every hover keystroke, which is the most
// expensive thing in the LSP for users who hover inside import dot-chains.
//
// This cache stores (content, AST, symbol table) keyed by file path, with an
// mtime check on lookup so external edits invalidate the entry automatically.
// The bound is small (parsedCacheCap = 64) because the working set is the
// stdlib plus user modules — usually well under 64 distinct paths.
//
// Eviction is strict LRU via container/list: each cache entry is a list
// element, accesses move it to the front, and inserts past the cap drop the
// back. All operations hold parsedCacheMu — the work behind the lock is a
// few map/list pointer ops, so contention is negligible in practice.

const parsedCacheCap = 64

type parsedEntry struct {
	path    string
	mtime   time.Time
	content string
	ast     *ast.Program
	symbols *SymbolTable
}

var (
	parsedCacheMu  sync.Mutex
	parsedCacheLRU = list.New()                       // *parsedEntry; front = MRU
	parsedCacheMap = make(map[string]*list.Element)   // path → list element
)

// getParsedFile returns (content, AST, symbols) for the file at filePath.
// On a cache hit with matching mtime, the cached values are returned and the
// entry is bumped to MRU. On a miss or stale mtime, the file is re-read and
// re-parsed; the result is inserted and the LRU tail is dropped if the cache
// is now over capacity.
//
// Returns ("", nil, nil) when the file can't be read — callers should treat
// that as "skip this hover" rather than a hard failure.
func getParsedFile(filePath string) (string, *ast.Program, *SymbolTable) {
	fi, err := os.Stat(filePath)
	if err != nil {
		return "", nil, nil
	}
	mtime := fi.ModTime()

	parsedCacheMu.Lock()
	if elem, ok := parsedCacheMap[filePath]; ok {
		entry := elem.Value.(*parsedEntry)
		if entry.mtime.Equal(mtime) {
			parsedCacheLRU.MoveToFront(elem)
			parsedCacheMu.Unlock()
			return entry.content, entry.ast, entry.symbols
		}
		// Stale — drop and refall through to a fresh parse.
		parsedCacheLRU.Remove(elem)
		delete(parsedCacheMap, filePath)
	}
	parsedCacheMu.Unlock()

	// Cache miss (or stale). Parse outside the lock so concurrent hovers on
	// different files don't serialise on the parse cost.
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", nil, nil
	}
	content := string(data)
	prog, syms := ParseDocumentAndBuildSymbols(filePath, content)

	parsedCacheMu.Lock()
	defer parsedCacheMu.Unlock()

	// Defensive: another goroutine may have inserted while we were parsing.
	// Overwrite that entry's payload with ours and bump to MRU — both versions
	// would have produced equivalent (content, AST, symbols), so we keep the
	// fresher one rather than create a duplicate list node.
	if existing, ok := parsedCacheMap[filePath]; ok {
		existing.Value = &parsedEntry{
			path: filePath, mtime: mtime, content: content, ast: prog, symbols: syms,
		}
		parsedCacheLRU.MoveToFront(existing)
		return content, prog, syms
	}

	entry := &parsedEntry{
		path: filePath, mtime: mtime, content: content, ast: prog, symbols: syms,
	}
	newElem := parsedCacheLRU.PushFront(entry)
	parsedCacheMap[filePath] = newElem

	// Evict the LRU tail until we're back under cap.
	for parsedCacheLRU.Len() > parsedCacheCap {
		oldest := parsedCacheLRU.Back()
		if oldest == nil {
			break
		}
		old := oldest.Value.(*parsedEntry)
		delete(parsedCacheMap, old.path)
		parsedCacheLRU.Remove(oldest)
	}

	return content, prog, syms
}
