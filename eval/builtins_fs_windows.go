// +build windows

package eval

import (
	"fmt"
	"io"
	"io/ioutil"
	"klex/ast"
	"os"
)

// Windows version: no fadvise or mmap support
// These are Unix/Darwin-specific optimizations

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
			return runtimeError("_fsRead expects 1 argument (path)", ast.Pos{})
		}
		path, ok := args[0].(*String)
		if !ok {
			return typeError("_fsRead path must be string", ast.Pos{})
		}
		content, err := ioutil.ReadFile(path.Value)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{&String{Value: string(content)}, NULL}}
	}}

	// _fsReadChunk(path, offset, byteCount) → (content, isEOF, err)
	Builtins["_fsReadChunk"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 3 {
			return runtimeError("_fsReadChunk expects 3 arguments", ast.Pos{})
		}
		path, pathOk := args[0].(*String)
		offset, offsetOk := args[1].(*Integer)
		byteCount, countOk := args[2].(*Integer)

		if !pathOk || !offsetOk || !countOk {
			return typeError("_fsReadChunk args must be (string, int, int)", ast.Pos{})
		}

		f, err := os.Open(path.Value)
		if err != nil {
			return &Tuple{Elements: []Object{&String{Value: ""}, &Boolean{Value: false}, &String{Value: err.Error()}}}
		}
		defer f.Close()

		_, err = f.Seek(int64(offset.Value), 0)
		if err != nil {
			return &Tuple{Elements: []Object{&String{Value: ""}, &Boolean{Value: false}, &String{Value: err.Error()}}}
		}

		buf := make([]byte, byteCount.Value)
		n, err := f.Read(buf)
		if err != nil && err != io.EOF {
			return &Tuple{Elements: []Object{&String{Value: ""}, &Boolean{Value: false}, &String{Value: err.Error()}}}
		}

		isEOF := n < int(byteCount.Value)
		return &Tuple{Elements: []Object{&String{Value: string(buf[:n])}, &Boolean{Value: isEOF}, NULL}}
	}}

	// _fsMap(path) → (content, err) - Not supported on Windows, falls back to _fsRead
	Builtins["_fsMap"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_fsMap expects 1 argument (path)", ast.Pos{})
		}
		path, ok := args[0].(*String)
		if !ok {
			return typeError("_fsMap path must be string", ast.Pos{})
		}
		// Windows doesn't support mmap, fall back to regular read
		content, err := ioutil.ReadFile(path.Value)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{&String{Value: string(content)}, NULL}}
	}}

	// _fsStat(path) → (info, err)
	Builtins["_fsStat"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_fsStat expects 1 argument", ast.Pos{})
		}
		path, ok := args[0].(*String)
		if !ok {
			return typeError("_fsStat: path must be string", ast.Pos{})
		}

		fi, err := os.Stat(path.Value)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}

		return &Tuple{Elements: []Object{fsInfoHash(fi, false), NULL}}
	}}

	// _fsLstat(path) → (info, err)
	Builtins["_fsLstat"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_fsLstat expects 1 argument", ast.Pos{})
		}
		path, ok := args[0].(*String)
		if !ok {
			return typeError("_fsLstat: path must be string", ast.Pos{})
		}

		fi, err := os.Lstat(path.Value)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}

		isSymlink := fi.Mode()&os.ModeSymlink != 0
		return &Tuple{Elements: []Object{fsInfoHash(fi, isSymlink), NULL}}
	}}

	// _fsExists(path) → bool
	Builtins["_fsExists"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_fsExists expects 1 argument", ast.Pos{})
		}
		path, ok := args[0].(*String)
		if !ok {
			return &Boolean{Value: false}
		}
		_, err := os.Stat(path.Value)
		return &Boolean{Value: err == nil}
	}}

	// _fsListDir(path) → (names, err)
	Builtins["_fsListDir"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_fsListDir expects 1 argument", ast.Pos{})
		}
		path, ok := args[0].(*String)
		if !ok {
			return typeError("_fsListDir: path must be string", ast.Pos{})
		}

		entries, err := os.ReadDir(path.Value)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}

		names := make([]Object, 0)
		for _, e := range entries {
			names = append(names, &String{Value: e.Name()})
		}
		return &Tuple{Elements: []Object{&Array{Elements: names}, NULL}}
	}}

	// _fsReadDir(path) → (entries, err)
	Builtins["_fsReadDir"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_fsReadDir expects 1 argument", ast.Pos{})
		}
		path, ok := args[0].(*String)
		if !ok {
			return typeError("_fsReadDir: path must be string", ast.Pos{})
		}

		entries, err := os.ReadDir(path.Value)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}

		infos := make([]Object, 0)
		for _, e := range entries {
			fi, err := e.Info()
			if err != nil {
				continue
			}
			isSymlink := e.Type()&os.ModeSymlink != 0
			infos = append(infos, fsInfoHash(fi, isSymlink))
		}
		return &Tuple{Elements: []Object{&Array{Elements: infos}, NULL}}
	}}

	// _fsWrite(path, content) → (null, err)
	Builtins["_fsWrite"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_fsWrite expects 2 arguments", ast.Pos{})
		}
		path, pathOk := args[0].(*String)
		content, contentOk := args[1].(*String)

		if !pathOk || !contentOk {
			return typeError("_fsWrite expects (path, content) as strings", ast.Pos{})
		}

		err := ioutil.WriteFile(path.Value, []byte(content.Value), 0644)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{NULL, NULL}}
	}}

	// _fsAppend(path, content) → (null, err)
	Builtins["_fsAppend"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_fsAppend expects 2 arguments", ast.Pos{})
		}
		path, pathOk := args[0].(*String)
		content, contentOk := args[1].(*String)

		if !pathOk || !contentOk {
			return typeError("_fsAppend expects (path, content) as strings", ast.Pos{})
		}

		f, err := os.OpenFile(path.Value, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		defer f.Close()

		_, err = f.WriteString(content.Value)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{NULL, NULL}}
	}}

	// _fsRemove(path) → (null, err)
	Builtins["_fsRemove"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_fsRemove expects 1 argument", ast.Pos{})
		}
		path, ok := args[0].(*String)
		if !ok {
			return typeError("_fsRemove: path must be string", ast.Pos{})
		}

		err := os.Remove(path.Value)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{NULL, NULL}}
	}}

	// _fsRemoveAll(path) → (null, err)
	Builtins["_fsRemoveAll"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_fsRemoveAll expects 1 argument", ast.Pos{})
		}
		path, ok := args[0].(*String)
		if !ok {
			return typeError("_fsRemoveAll: path must be string", ast.Pos{})
		}

		err := os.RemoveAll(path.Value)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{NULL, NULL}}
	}}

	// _fsMkdir(path) → (null, err)
	Builtins["_fsMkdir"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_fsMkdir expects 1 argument", ast.Pos{})
		}
		path, ok := args[0].(*String)
		if !ok {
			return typeError("_fsMkdir: path must be string", ast.Pos{})
		}

		err := os.Mkdir(path.Value, 0755)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{NULL, NULL}}
	}}

	// _fsMkdirAll(path) → (null, err)
	Builtins["_fsMkdirAll"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_fsMkdirAll expects 1 argument", ast.Pos{})
		}
		path, ok := args[0].(*String)
		if !ok {
			return typeError("_fsMkdirAll: path must be string", ast.Pos{})
		}

		err := os.MkdirAll(path.Value, 0755)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{NULL, NULL}}
	}}

	// _fsRename(src, dst) → (null, err)
	Builtins["_fsRename"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_fsRename expects 2 arguments", ast.Pos{})
		}
		src, srcOk := args[0].(*String)
		dst, dstOk := args[1].(*String)

		if !srcOk || !dstOk {
			return typeError("_fsRename expects (src, dst) as strings", ast.Pos{})
		}

		err := os.Rename(src.Value, dst.Value)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{NULL, NULL}}
	}}

	// _fsCopy(src, dst) → (null, err)
	Builtins["_fsCopy"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_fsCopy expects 2 arguments", ast.Pos{})
		}
		src, srcOk := args[0].(*String)
		dst, dstOk := args[1].(*String)

		if !srcOk || !dstOk {
			return typeError("_fsCopy expects (src, dst) as strings", ast.Pos{})
		}

		srcFile, err := os.Open(src.Value)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		defer srcFile.Close()

		dstFile, err := os.Create(dst.Value)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		defer dstFile.Close()

		_, err = io.Copy(dstFile, srcFile)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{NULL, NULL}}
	}}

	// _fsChmod(path, mode) → (null, err)
	Builtins["_fsChmod"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_fsChmod expects 2 arguments", ast.Pos{})
		}
		path, pathOk := args[0].(*String)
		modeStr, modeOk := args[1].(*String)

		if !pathOk || !modeOk {
			return typeError("_fsChmod expects (path, mode) as strings", ast.Pos{})
		}

		var mode int64
		_, err := fmt.Sscanf(modeStr.Value, "%o", &mode)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: "invalid mode format"}}}
		}

		err = os.Chmod(path.Value, os.FileMode(mode))
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{NULL, NULL}}
	}}

	// _fsSymlink(target, link) → (null, err) - Not supported on Windows without special privileges
	Builtins["_fsSymlink"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_fsSymlink expects 2 arguments", ast.Pos{})
		}
		target, targetOk := args[0].(*String)
		link, linkOk := args[1].(*String)

		if !targetOk || !linkOk {
			return typeError("_fsSymlink expects (target, link) as strings", ast.Pos{})
		}

		err := os.Symlink(target.Value, link.Value)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{NULL, NULL}}
	}}

	// _fsReadlink(path) → (target, err)
	Builtins["_fsReadlink"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_fsReadlink expects 1 argument", ast.Pos{})
		}
		path, ok := args[0].(*String)
		if !ok {
			return typeError("_fsReadlink: path must be string", ast.Pos{})
		}

		target, err := os.Readlink(path.Value)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{&String{Value: target}, NULL}}
	}}

	// _fsTmpFile(dir, pattern) → (path, err)
	Builtins["_fsTmpFile"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_fsTmpFile expects 2 arguments", ast.Pos{})
		}
		dir, dirOk := args[0].(*String)
		pattern, patternOk := args[1].(*String)

		if !dirOk || !patternOk {
			return typeError("_fsTmpFile expects (dir, pattern) as strings", ast.Pos{})
		}

		f, err := ioutil.TempFile(dir.Value, pattern.Value)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		f.Close()
		return &Tuple{Elements: []Object{&String{Value: f.Name()}, NULL}}
	}}

	// _fsTmpDir(dir, pattern) → (path, err)
	Builtins["_fsTmpDir"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_fsTmpDir expects 2 arguments", ast.Pos{})
		}
		dir, dirOk := args[0].(*String)
		pattern, patternOk := args[1].(*String)

		if !dirOk || !patternOk {
			return typeError("_fsTmpDir expects (dir, pattern) as strings", ast.Pos{})
		}

		path, err := ioutil.TempDir(dir.Value, pattern.Value)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{&String{Value: path}, NULL}}
	}}
}
