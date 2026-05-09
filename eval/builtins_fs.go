package eval

import (
	"fmt"
	"io"
	"klex/ast"
	"os"
	"runtime"
	"strconv"
	"syscall"
	"unsafe"
)

// tryFadviseSequential hints sequential access to the kernel to reduce cache eviction.
// Linux: uses fadvise64 syscall with POSIX_FADV_SEQUENTIAL (value 2)
// macOS: uses fcntl F_RDAHEAD to enable readahead
// Errors are ignored since these are purely advisory hints.
func tryFadviseSequential(fd int, offset int64, length int64) {
	switch runtime.GOOS {
	case "linux":
		// fadvise64 syscall 221 on Linux x86_64 tells kernel:
		// "I'm reading sequentially—don't keep old pages in cache"
		syscall.Syscall6(uintptr(221), uintptr(fd), uintptr(offset), uintptr(length), uintptr(2), 0, 0)
	case "darwin":
		// macOS: F_RDAHEAD would enable aggressive caching (opposite of what we need).
		// No equivalent to POSIX_FADV_SEQUENTIAL on macOS; skip optimization.
		// The sequential read pattern itself is efficient enough on modern macOS.
	}
	// Other systems: no optimization available
}

func fsInfoHash(fi os.FileInfo, isSymlink bool) *Hash {
	h := &Hash{Pairs: make(map[HashKey]HashPair)}
	set := func(k string, v Object) {
		key := &String{Value: k}
		h.Pairs[HashKey{Type: STRING_OBJ, Value: k}] = HashPair{Key: key, Value: v}
	}
	set("name", &String{Value: fi.Name()})
	set("size", &Integer{Value: int(fi.Size())})
	set("isDir", &Boolean{Value: fi.IsDir()})
	set("isSymlink", &Boolean{Value: isSymlink})
	set("modTime", &Integer{Value: int(fi.ModTime().Unix())})
	set("mode", &String{Value: fmt.Sprintf("0%o", fi.Mode().Perm())})
	return h
}

func init() {
	// _fsRead(path) → (content, err)
	Builtins["_fsRead"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_fsRead expects 1 argument", ast.Pos{})
		}
		p, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_fsRead: argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		data, err := os.ReadFile(p.Value)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{&String{Value: string(data)}, NULL}}
	}}

	// _fsWrite(path, content) → (null, err)  — creates or truncates
	Builtins["_fsWrite"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_fsWrite expects 2 arguments", ast.Pos{})
		}
		p, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_fsWrite: first argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		content, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_fsWrite: second argument must be string, got %s", args[1].Type()), ast.Pos{})
		}
		if err := os.WriteFile(p.Value, []byte(content.Value), 0644); err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{NULL, NULL}}
	}}

	// _fsAppend(path, content) → (null, err)  — creates or appends
	Builtins["_fsAppend"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_fsAppend expects 2 arguments", ast.Pos{})
		}
		p, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_fsAppend: first argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		content, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_fsAppend: second argument must be string, got %s", args[1].Type()), ast.Pos{})
		}
		f, err := os.OpenFile(p.Value, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		defer f.Close()
		if _, err = f.WriteString(content.Value); err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{NULL, NULL}}
	}}

	// _fsExists(path) → bool  — no error; false means absent or inaccessible
	Builtins["_fsExists"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_fsExists expects 1 argument", ast.Pos{})
		}
		p, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_fsExists: argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		_, err := os.Stat(p.Value)
		return &Boolean{Value: err == nil}
	}}

	// _fsRemove(path) → (null, err)  — removes a file or empty directory
	Builtins["_fsRemove"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_fsRemove expects 1 argument", ast.Pos{})
		}
		p, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_fsRemove: argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		if err := os.Remove(p.Value); err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{NULL, NULL}}
	}}

	// _fsRemoveAll(path) → (null, err)  — removes path and all its contents
	Builtins["_fsRemoveAll"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_fsRemoveAll expects 1 argument", ast.Pos{})
		}
		p, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_fsRemoveAll: argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		if err := os.RemoveAll(p.Value); err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{NULL, NULL}}
	}}

	// _fsMkdir(path) → (null, err)  — creates a single directory (parent must exist)
	Builtins["_fsMkdir"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_fsMkdir expects 1 argument", ast.Pos{})
		}
		p, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_fsMkdir: argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		if err := os.Mkdir(p.Value, 0755); err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{NULL, NULL}}
	}}

	// _fsMkdirAll(path) → (null, err)  — creates directory and all missing parents
	Builtins["_fsMkdirAll"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_fsMkdirAll expects 1 argument", ast.Pos{})
		}
		p, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_fsMkdirAll: argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		if err := os.MkdirAll(p.Value, 0755); err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{NULL, NULL}}
	}}

	// _fsListDir(path) → (array_of_names, err)  — lists directory entries (names only, sorted)
	Builtins["_fsListDir"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_fsListDir expects 1 argument", ast.Pos{})
		}
		p, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_fsListDir: argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		entries, err := os.ReadDir(p.Value)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		names := make([]Object, len(entries))
		for i, e := range entries {
			names[i] = &String{Value: e.Name()}
		}
		return &Tuple{Elements: []Object{&Array{Elements: names}, NULL}}
	}}

	// _fsRename(src, dst) → (null, err)  — renames or moves a file
	Builtins["_fsRename"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_fsRename expects 2 arguments", ast.Pos{})
		}
		src, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_fsRename: first argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		dst, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_fsRename: second argument must be string, got %s", args[1].Type()), ast.Pos{})
		}
		if err := os.Rename(src.Value, dst.Value); err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{NULL, NULL}}
	}}

	// _fsStat(path) → (info_hash, err)
	// info_hash keys: "name","size","isDir","isSymlink","modTime","mode"
	// Stat follows symlinks — isSymlink is always false on the resolved target.
	Builtins["_fsStat"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_fsStat expects 1 argument", ast.Pos{})
		}
		p, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_fsStat: argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		fi, err := os.Stat(p.Value)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{fsInfoHash(fi, false), NULL}}
	}}

	// _fsCopy(src, dst) → (null, err)  — copies a file byte-for-byte
	Builtins["_fsCopy"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_fsCopy expects 2 arguments", ast.Pos{})
		}
		src, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_fsCopy: first argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		dst, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_fsCopy: second argument must be string, got %s", args[1].Type()), ast.Pos{})
		}
		in, err := os.Open(src.Value)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		defer in.Close()
		out, err := os.Create(dst.Value)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		defer out.Close()
		if _, err = io.Copy(out, in); err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{NULL, NULL}}
	}}

	// _fsChmod(path, mode) → (null, err)  — mode is an octal string e.g. "755", "644"
	Builtins["_fsChmod"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_fsChmod expects 2 arguments", ast.Pos{})
		}
		p, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_fsChmod: first argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		modeStr, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_fsChmod: second argument must be string, got %s", args[1].Type()), ast.Pos{})
		}
		modeVal, parseErr := strconv.ParseUint(modeStr.Value, 8, 32)
		if parseErr != nil {
			return runtimeError(fmt.Sprintf("_fsChmod: invalid mode %q — expected octal string like \"755\"", modeStr.Value), ast.Pos{})
		}
		if err := os.Chmod(p.Value, os.FileMode(modeVal)); err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{NULL, NULL}}
	}}

	// _fsReadDir(path) → (array_of_info_hashes, err)
	// Each hash has the same keys as _fsStat. Entries whose Info() fails are skipped.
	Builtins["_fsReadDir"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_fsReadDir expects 1 argument", ast.Pos{})
		}
		p, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_fsReadDir: argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		entries, err := os.ReadDir(p.Value)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		infos := make([]Object, 0, len(entries))
		for _, e := range entries {
			fi, ferr := e.Info()
			if ferr != nil {
				continue
			}
			infos = append(infos, fsInfoHash(fi, e.Type()&os.ModeSymlink != 0))
		}
		return &Tuple{Elements: []Object{&Array{Elements: infos}, NULL}}
	}}

	// _fsLstat(path) → (info_hash, err)  — like _fsStat but does not follow symlinks
	Builtins["_fsLstat"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_fsLstat expects 1 argument", ast.Pos{})
		}
		p, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_fsLstat: argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		fi, err := os.Lstat(p.Value)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{fsInfoHash(fi, fi.Mode()&os.ModeSymlink != 0), NULL}}
	}}

	// _fsSymlink(target, link) → (null, err)  — creates a symbolic link
	Builtins["_fsSymlink"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_fsSymlink expects 2 arguments", ast.Pos{})
		}
		target, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_fsSymlink: first argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		link, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_fsSymlink: second argument must be string, got %s", args[1].Type()), ast.Pos{})
		}
		if err := os.Symlink(target.Value, link.Value); err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{NULL, NULL}}
	}}

	// _fsReadlink(path) → (target_string, err)  — reads the target of a symlink
	Builtins["_fsReadlink"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_fsReadlink expects 1 argument", ast.Pos{})
		}
		p, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_fsReadlink: argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		target, err := os.Readlink(p.Value)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{&String{Value: target}, NULL}}
	}}

	// _fsTmpFile(dir, pattern) → (path, err)
	// Creates a new temp file in dir matching pattern, closes it, returns its path.
	// Pass "" for dir to use the system temp directory.
	Builtins["_fsTmpFile"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_fsTmpFile expects 2 arguments", ast.Pos{})
		}
		dir, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_fsTmpFile: first argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		pattern, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_fsTmpFile: second argument must be string, got %s", args[1].Type()), ast.Pos{})
		}
		f, err := os.CreateTemp(dir.Value, pattern.Value)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		name := f.Name()
		f.Close()
		return &Tuple{Elements: []Object{&String{Value: name}, NULL}}
	}}

	// _fsTmpDir(dir, pattern) → (path, err)
	// Creates a new temp directory in dir matching pattern, returns its path.
	// Pass "" for dir to use the system temp directory.
	Builtins["_fsTmpDir"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_fsTmpDir expects 2 arguments", ast.Pos{})
		}
		dir, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_fsTmpDir: first argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		pattern, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_fsTmpDir: second argument must be string, got %s", args[1].Type()), ast.Pos{})
		}
		path, err := os.MkdirTemp(dir.Value, pattern.Value)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{&String{Value: path}, NULL}}
	}}

	// _fsMap(path) → (content, err)
	// Memory-maps a file and returns its content as a string.
	// The string is backed by the mmap'd region—no copying.
	// Perfect for analyzing large files in parallel (each worker accesses different ranges).
	Builtins["_fsMap"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_fsMap expects 1 argument", ast.Pos{})
		}
		p, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_fsMap: argument must be string, got %s", args[0].Type()), ast.Pos{})
		}

		// Open file for reading
		f, err := os.Open(p.Value)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		defer f.Close()

		// Get file size
		fi, err := f.Stat()
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		size := fi.Size()

		if size == 0 {
			return &Tuple{Elements: []Object{&String{Value: ""}, NULL}}
		}

		// Memory-map the file
		data, err := syscall.Mmap(int(f.Fd()), 0, int(size), syscall.PROT_READ, syscall.MAP_SHARED)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}

		// Convert byte slice to string without copying (unsafe but necessary for mmap)
		str := (*String)(unsafe.Pointer(&struct {
			data uintptr
			len  int
			cap  int
		}{
			data: uintptr(unsafe.Pointer(&data[0])),
			len:  len(data),
			cap:  len(data),
		}))

		return &Tuple{Elements: []Object{str, NULL}}
	}}

	// _fsReadChunk(path, offset, byteCount) → (content, isEOF, err)
	// Reads up to byteCount bytes from file starting at offset.
	// Returns tuple: (content_string, isEOF_bool, error_or_null)
	// isEOF is true if the returned chunk reaches the end of the file.
	// Useful for streaming large files without loading entirely into memory.
	Builtins["_fsReadChunk"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 3 {
			return runtimeError("_fsReadChunk expects 3 arguments (path, offset, byteCount)", ast.Pos{})
		}
		p, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_fsReadChunk: first argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		offsetObj, ok := args[1].(*Integer)
		if !ok {
			return typeError(fmt.Sprintf("_fsReadChunk: second argument must be integer, got %s", args[1].Type()), ast.Pos{})
		}
		byteCountObj, ok := args[2].(*Integer)
		if !ok {
			return typeError(fmt.Sprintf("_fsReadChunk: third argument must be integer, got %s", args[2].Type()), ast.Pos{})
		}

		offset := int64(offsetObj.Value)
		byteCount := int(byteCountObj.Value)

		if offset < 0 {
			return runtimeError("_fsReadChunk: offset cannot be negative", ast.Pos{})
		}
		if byteCount < 0 {
			return runtimeError("_fsReadChunk: byteCount cannot be negative", ast.Pos{})
		}

		f, err := os.Open(p.Value)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, NULL, &String{Value: err.Error()}}}
		}
		defer f.Close()

		// Seek to offset
		if _, err := f.Seek(offset, 0); err != nil {
			return &Tuple{Elements: []Object{NULL, NULL, &String{Value: err.Error()}}}
		}

		// Hint to kernel: sequential access pattern (only on first chunk to avoid syscall overhead).
		// Reduces cache eviction overhead by telling kernel not to keep pages we won't revisit.
		// Only called once per file (when offset == 0) to minimize syscall overhead.
		fd := int(f.Fd())
		if offset == 0 {
			tryFadviseSequential(fd, offset, int64(byteCount))
		}

		// Read up to byteCount bytes
		buf := make([]byte, byteCount)
		n, err := f.Read(buf)
		if err != nil && err != io.EOF {
			return &Tuple{Elements: []Object{NULL, NULL, &String{Value: err.Error()}}}
		}

		// Check if we've reached EOF
		// If n < byteCount, we've hit EOF
		isEOF := n < byteCount

		return &Tuple{Elements: []Object{
			&String{Value: string(buf[:n])},
			&Boolean{Value: isEOF},
			NULL,
		}}
	}}
}
