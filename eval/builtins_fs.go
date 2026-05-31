//go:build !windows && !js

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
// macOS: F_RDAHEAD would enable aggressive caching (opposite of what we need);
// no POSIX_FADV_SEQUENTIAL equivalent — sequential reads are efficient enough natively.
// Errors are ignored since these are purely advisory hints.
func tryFadviseSequential(fd int, offset int64, length int64) {
	if runtime.GOOS == "linux" {
		syscall.Syscall6(uintptr(221), uintptr(fd), uintptr(offset), uintptr(length), uintptr(2), 0, 0)
	}
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

// Native fs.* helpers for Unix/macOS. The dispatcher in
// builtins_fs_dispatch_gen.go has already handled arity check and path-arg
// type extraction, so helpers receive a primitive `path string` for the
// parsed Remainder and raw Object for passthrough args.

func nativeFsRead(path string) Object {
	data, err := os.ReadFile(path)
	if err != nil {
		return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
	}
	return &Tuple{Elements: []Object{&String{Value: string(data)}, NULL}}
}

func nativeFsWrite(path string, content Object) Object {
	contentStr, ok := content.(*String)
	if !ok {
		return typeError("_fsWrite: content must be string, got "+string(content.Type()), ast.Pos{})
	}
	if err := os.WriteFile(path, []byte(contentStr.Value), 0644); err != nil {
		return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
	}
	return &Tuple{Elements: []Object{NULL, NULL}}
}

func nativeFsReadBytes(path string) Object {
	data, err := os.ReadFile(path)
	if err != nil {
		return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
	}
	return &Tuple{Elements: []Object{&Bytes{Value: data}, NULL}}
}

// nativeFsAppendBytesSync appends bytes and calls fsync(2) before close.
// Durable variant of nativeFsAppend: on a power loss after this returns,
// the bytes ARE on disk (assuming the underlying hardware honours fsync).
// Use for crash-safe append-only logs.
func nativeFsAppendBytesSync(path string, raw Object) Object {
	bs, ok := raw.(*Bytes)
	if !ok {
		return typeError("_fsAppendBytesSync: bytes must be bytes, got "+string(raw.Type()), ast.Pos{})
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
	}
	if _, err = f.Write(bs.Value); err != nil {
		f.Close()
		return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return &Tuple{Elements: []Object{NULL, &String{Value: "fsync: " + err.Error()}}}
	}
	if err = f.Close(); err != nil {
		return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
	}
	return &Tuple{Elements: []Object{NULL, NULL}}
}

func nativeFsAppend(path string, content Object) Object {
	contentStr, ok := content.(*String)
	if !ok {
		return typeError("_fsAppend: content must be string, got "+string(content.Type()), ast.Pos{})
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
	}
	defer f.Close()
	if _, err = f.WriteString(contentStr.Value); err != nil {
		return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
	}
	return &Tuple{Elements: []Object{NULL, NULL}}
}

func nativeFsExists(path string) Object {
	_, err := os.Stat(path)
	return &Boolean{Value: err == nil}
}

func nativeFsRemove(path string) Object {
	if err := os.Remove(path); err != nil {
		return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
	}
	return &Tuple{Elements: []Object{NULL, NULL}}
}

func nativeFsRemoveAll(path string) Object {
	if err := os.RemoveAll(path); err != nil {
		return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
	}
	return &Tuple{Elements: []Object{NULL, NULL}}
}

func nativeFsMkdir(path string) Object {
	if err := os.Mkdir(path, 0755); err != nil {
		return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
	}
	return &Tuple{Elements: []Object{NULL, NULL}}
}

func nativeFsMkdirAll(path string) Object {
	if err := os.MkdirAll(path, 0755); err != nil {
		return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
	}
	return &Tuple{Elements: []Object{NULL, NULL}}
}

func nativeFsListDir(path string) Object {
	entries, err := os.ReadDir(path)
	if err != nil {
		return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
	}
	names := make([]Object, len(entries))
	for i, e := range entries {
		names[i] = &String{Value: e.Name()}
	}
	return &Tuple{Elements: []Object{&Array{Elements: names}, NULL}}
}

func nativeFsRename(src, dst string) Object {
	if err := os.Rename(src, dst); err != nil {
		return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
	}
	return &Tuple{Elements: []Object{NULL, NULL}}
}

// nativeFsStat returns an info hash with keys
// "name","size","isDir","isSymlink","modTime","mode".
// Stat follows symlinks — isSymlink is always false on the resolved target.
func nativeFsStat(path string) Object {
	fi, err := os.Stat(path)
	if err != nil {
		return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
	}
	return &Tuple{Elements: []Object{fsInfoHash(fi, false), NULL}}
}

func nativeFsCopy(src, dst string) Object {
	in, err := os.Open(src)
	if err != nil {
		return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
	}
	defer out.Close()
	if _, err = io.Copy(out, in); err != nil {
		return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
	}
	return &Tuple{Elements: []Object{NULL, NULL}}
}

// nativeFsChmod accepts an octal string mode like "755" / "644".
func nativeFsChmod(path string, raw Object) Object {
	modeStr, ok := raw.(*String)
	if !ok {
		return typeError("_fsChmod: mode must be string, got "+string(raw.Type()), ast.Pos{})
	}
	modeVal, parseErr := strconv.ParseUint(modeStr.Value, 8, 32)
	if parseErr != nil {
		return runtimeError("_fsChmod: invalid mode "+modeStr.Value+" — expected octal string like \"755\"", ast.Pos{})
	}
	if err := os.Chmod(path, os.FileMode(modeVal)); err != nil {
		return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
	}
	return &Tuple{Elements: []Object{NULL, NULL}}
}

// nativeFsReadDir returns an array of info hashes (same keys as nativeFsStat).
// Entries whose Info() fails are skipped.
func nativeFsReadDir(path string) Object {
	entries, err := os.ReadDir(path)
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
}

// nativeFsLstat is like nativeFsStat but does not follow symlinks.
func nativeFsLstat(path string) Object {
	fi, err := os.Lstat(path)
	if err != nil {
		return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
	}
	return &Tuple{Elements: []Object{fsInfoHash(fi, fi.Mode()&os.ModeSymlink != 0), NULL}}
}

func nativeFsSymlink(target, link string) Object {
	if err := os.Symlink(target, link); err != nil {
		return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
	}
	return &Tuple{Elements: []Object{NULL, NULL}}
}

func nativeFsReadlink(path string) Object {
	target, err := os.Readlink(path)
	if err != nil {
		return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
	}
	return &Tuple{Elements: []Object{&String{Value: target}, NULL}}
}

// nativeFsTmpFile creates a new temp file in dir matching pattern, closes it,
// returns (path, err). Pass "" for dir to use the system temp directory.
func nativeFsTmpFile(dir string, raw Object) Object {
	pattern, ok := raw.(*String)
	if !ok {
		return typeError("_fsTmpFile: pattern must be string, got "+string(raw.Type()), ast.Pos{})
	}
	f, err := os.CreateTemp(dir, pattern.Value)
	if err != nil {
		return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
	}
	name := f.Name()
	f.Close()
	return &Tuple{Elements: []Object{&String{Value: name}, NULL}}
}

// nativeFsTmpDir creates a new temp directory in dir matching pattern.
// Pass "" for dir to use the system temp directory.
func nativeFsTmpDir(dir string, raw Object) Object {
	pattern, ok := raw.(*String)
	if !ok {
		return typeError("_fsTmpDir: pattern must be string, got "+string(raw.Type()), ast.Pos{})
	}
	path, err := os.MkdirTemp(dir, pattern.Value)
	if err != nil {
		return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
	}
	return &Tuple{Elements: []Object{&String{Value: path}, NULL}}
}

// nativeFsMap memory-maps a file and returns its content as a string backed
// by the mmap'd region — no copying. Use for parallel large-file analysis
// where each worker accesses different ranges.
func nativeFsMap(path string) Object {
	f, err := os.Open(path)
	if err != nil {
		return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
	}
	size := fi.Size()
	if size == 0 {
		return &Tuple{Elements: []Object{&String{Value: ""}, NULL}}
	}
	data, err := syscall.Mmap(int(f.Fd()), 0, int(size), syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
	}
	// Convert byte slice to string without copying (unsafe but necessary for mmap).
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
}

// nativeFsReadChunk reads up to byteCount bytes from path starting at offset.
// Returns (content_string, isEOF_bool, error_or_null).
func nativeFsReadChunk(path string, offsetRaw, byteCountRaw Object) Object {
	offsetObj, ok := offsetRaw.(*Integer)
	if !ok {
		return typeError("_fsReadChunk: offset must be integer, got "+string(offsetRaw.Type()), ast.Pos{})
	}
	byteCountObj, ok := byteCountRaw.(*Integer)
	if !ok {
		return typeError("_fsReadChunk: byteCount must be integer, got "+string(byteCountRaw.Type()), ast.Pos{})
	}
	offset := int64(offsetObj.Value)
	byteCount := int(byteCountObj.Value)
	if offset < 0 {
		return runtimeError("_fsReadChunk: offset cannot be negative", ast.Pos{})
	}
	if byteCount < 0 {
		return runtimeError("_fsReadChunk: byteCount cannot be negative", ast.Pos{})
	}

	f, err := os.Open(path)
	if err != nil {
		return &Tuple{Elements: []Object{NULL, NULL, &String{Value: err.Error()}}}
	}
	defer f.Close()
	if _, err := f.Seek(offset, 0); err != nil {
		return &Tuple{Elements: []Object{NULL, NULL, &String{Value: err.Error()}}}
	}
	if offset == 0 {
		// Hint sequential access on the first chunk only — single syscall per file.
		tryFadviseSequential(int(f.Fd()), offset, int64(byteCount))
	}
	buf := make([]byte, byteCount)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return &Tuple{Elements: []Object{NULL, NULL, &String{Value: err.Error()}}}
	}
	isEOF := n < byteCount
	return &Tuple{Elements: []Object{
		&String{Value: string(buf[:n])},
		&Boolean{Value: isEOF},
		NULL,
	}}
}
