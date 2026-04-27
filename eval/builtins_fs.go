package eval

import (
	"fmt"
	"io"
	"klex/ast"
	"os"
)

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
	// info_hash has keys: "name" (string), "size" (integer bytes),
	// "isDir" (bool), "modTime" (integer unix seconds)
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
		info := &Hash{Pairs: make(map[HashKey]HashPair)}
		set := func(k string, v Object) {
			key := &String{Value: k}
			info.Pairs[HashKey{Type: STRING_OBJ, Value: k}] = HashPair{Key: key, Value: v}
		}
		set("name", &String{Value: fi.Name()})
		set("size", &Integer{Value: int(fi.Size())})
		set("isDir", &Boolean{Value: fi.IsDir()})
		set("modTime", &Integer{Value: int(fi.ModTime().Unix())})
		return &Tuple{Elements: []Object{info, NULL}}
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
}
