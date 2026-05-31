//go:build windows

package eval

import (
	"fmt"
	"io"
	"io/ioutil"
	"klex/ast"
	"os"
)

// Windows fs helpers — no fadvise or mmap support (those are
// Unix/macOS-specific). The dispatcher in builtins_fs_dispatch_gen.go
// handles arity + path-arg type-check; helpers receive a primitive
// `path string` and raw Object for passthrough args.

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

func nativeFsRead(path string) Object {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
	}
	return &Tuple{Elements: []Object{&String{Value: string(content)}, NULL}}
}

// nativeFsReadChunk on Windows uses plain Seek+Read; no fadvise hint
// (no equivalent on Windows).
func nativeFsReadChunk(path string, offsetRaw, byteCountRaw Object) Object {
	offset, offsetOk := offsetRaw.(*Integer)
	byteCount, countOk := byteCountRaw.(*Integer)
	if !offsetOk || !countOk {
		return typeError("_fsReadChunk: (offset, byteCount) must be integers", ast.Pos{})
	}
	f, err := os.Open(path)
	if err != nil {
		return &Tuple{Elements: []Object{&String{Value: ""}, &Boolean{Value: false}, &String{Value: err.Error()}}}
	}
	defer f.Close()
	if _, err = f.Seek(int64(offset.Value), 0); err != nil {
		return &Tuple{Elements: []Object{&String{Value: ""}, &Boolean{Value: false}, &String{Value: err.Error()}}}
	}
	buf := make([]byte, byteCount.Value)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return &Tuple{Elements: []Object{&String{Value: ""}, &Boolean{Value: false}, &String{Value: err.Error()}}}
	}
	isEOF := n < int(byteCount.Value)
	return &Tuple{Elements: []Object{&String{Value: string(buf[:n])}, &Boolean{Value: isEOF}, NULL}}
}

// nativeFsMap on Windows falls back to a regular read (no mmap).
func nativeFsMap(path string) Object {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
	}
	return &Tuple{Elements: []Object{&String{Value: string(content)}, NULL}}
}

func nativeFsStat(path string) Object {
	fi, err := os.Stat(path)
	if err != nil {
		return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
	}
	return &Tuple{Elements: []Object{fsInfoHash(fi, false), NULL}}
}

func nativeFsLstat(path string) Object {
	fi, err := os.Lstat(path)
	if err != nil {
		return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
	}
	isSymlink := fi.Mode()&os.ModeSymlink != 0
	return &Tuple{Elements: []Object{fsInfoHash(fi, isSymlink), NULL}}
}

func nativeFsExists(path string) Object {
	_, err := os.Stat(path)
	return &Boolean{Value: err == nil}
}

func nativeFsListDir(path string) Object {
	entries, err := os.ReadDir(path)
	if err != nil {
		return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
	}
	names := make([]Object, 0, len(entries))
	for _, e := range entries {
		names = append(names, &String{Value: e.Name()})
	}
	return &Tuple{Elements: []Object{&Array{Elements: names}, NULL}}
}

func nativeFsReadDir(path string) Object {
	entries, err := os.ReadDir(path)
	if err != nil {
		return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
	}
	infos := make([]Object, 0, len(entries))
	for _, e := range entries {
		fi, err := e.Info()
		if err != nil {
			continue
		}
		isSymlink := e.Type()&os.ModeSymlink != 0
		infos = append(infos, fsInfoHash(fi, isSymlink))
	}
	return &Tuple{Elements: []Object{&Array{Elements: infos}, NULL}}
}

func nativeFsWrite(path string, content Object) Object {
	contentStr, ok := content.(*String)
	if !ok {
		return typeError("_fsWrite: content must be string, got "+string(content.Type()), ast.Pos{})
	}
	if err := ioutil.WriteFile(path, []byte(contentStr.Value), 0644); err != nil {
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

// nativeFsAppendBytesSync appends bytes and calls Sync before close.
// Durable append-only write — bytes are on disk when this returns.
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

func nativeFsRename(src, dst string) Object {
	if err := os.Rename(src, dst); err != nil {
		return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
	}
	return &Tuple{Elements: []Object{NULL, NULL}}
}

func nativeFsCopy(src, dst string) Object {
	srcFile, err := os.Open(src)
	if err != nil {
		return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
	}
	defer srcFile.Close()
	dstFile, err := os.Create(dst)
	if err != nil {
		return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
	}
	defer dstFile.Close()
	if _, err = io.Copy(dstFile, srcFile); err != nil {
		return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
	}
	return &Tuple{Elements: []Object{NULL, NULL}}
}

func nativeFsChmod(path string, raw Object) Object {
	modeStr, ok := raw.(*String)
	if !ok {
		return typeError("_fsChmod: mode must be string, got "+string(raw.Type()), ast.Pos{})
	}
	var mode int64
	if _, err := fmt.Sscanf(modeStr.Value, "%o", &mode); err != nil {
		return &Tuple{Elements: []Object{NULL, &String{Value: "invalid mode format"}}}
	}
	if err := os.Chmod(path, os.FileMode(mode)); err != nil {
		return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
	}
	return &Tuple{Elements: []Object{NULL, NULL}}
}

// nativeFsSymlink on Windows requires special privileges; the call may fail
// at runtime if those privileges aren't present.
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

func nativeFsTmpFile(dir string, raw Object) Object {
	pattern, ok := raw.(*String)
	if !ok {
		return typeError("_fsTmpFile: pattern must be string, got "+string(raw.Type()), ast.Pos{})
	}
	f, err := ioutil.TempFile(dir, pattern.Value)
	if err != nil {
		return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
	}
	name := f.Name()
	f.Close()
	return &Tuple{Elements: []Object{&String{Value: name}, NULL}}
}

func nativeFsTmpDir(dir string, raw Object) Object {
	pattern, ok := raw.(*String)
	if !ok {
		return typeError("_fsTmpDir: pattern must be string, got "+string(raw.Type()), ast.Pos{})
	}
	path, err := ioutil.TempDir(dir, pattern.Value)
	if err != nil {
		return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
	}
	return &Tuple{Elements: []Object{&String{Value: path}, NULL}}
}
