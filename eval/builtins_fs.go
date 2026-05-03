package eval

import (
	"fmt"
	"io"
	"klex/ast"
	"os"
	"strconv"
)

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
}
