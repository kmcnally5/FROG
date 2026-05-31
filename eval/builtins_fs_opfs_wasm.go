//go:build js && wasm

package eval

import (
	"errors"
	"klex/ast"
	"syscall/js"
)

// Browser-side OPFS (Origin Private File System) implementation for the
// opfs:// scheme. Every fs.* call routed through the dispatcher to one
// of the opfsFs* helpers below ends up invoking a method on the JS-side
// shim `window.__klex_opfs`, which wraps the asynchronous OPFS API into
// a uniform `{ok, value, error}` Promise contract.
//
// The Go side uses the standard kLex cooperative-async pattern: each
// helper registers a then/catch pair, blocks the eval goroutine on a
// channel, and decodes the resolved value into a kLex Object. The
// shared `opfsCall` helper carries all the boilerplate.
//
// The shim is installed once at package init via js.Global().Call("eval").
// If the browser does not support OPFS (no navigator.storage, insecure
// context, etc.), every call will fail with a descriptive error from
// the shim's try/catch — no Go-side panic.

func init() {
	// Install the shim — it sets window.__klex_opfs as a side effect.
	// The eval return value is unused (and undefined in practice).
	js.Global().Call("eval", opfsShimJS)
}

// opfsCall invokes window.__klex_opfs[method](args...) and blocks until
// the returned Promise resolves. Returns the JS `value` field of the
// shim's response object, or an error if the shim reported failure.
func opfsCall(method string, args ...interface{}) (js.Value, error) {
	opfs := js.Global().Get("__klex_opfs")
	if opfs.IsUndefined() {
		return js.Undefined(), errors.New("OPFS shim not installed (window.__klex_opfs missing) — likely an unsupported browser or insecure context")
	}

	type result struct {
		val js.Value
		err string
	}
	ch := make(chan result, 1)

	var thenCb, catchCb js.Func
	cleanup := func() {
		thenCb.Release()
		catchCb.Release()
	}

	thenCb = js.FuncOf(func(this js.Value, jargs []js.Value) interface{} {
		if len(jargs) == 0 {
			ch <- result{err: "opfs: shim returned no value"}
			cleanup()
			return nil
		}
		resp := jargs[0]
		if !resp.Get("ok").Bool() {
			ch <- result{err: resp.Get("error").String()}
		} else {
			ch <- result{val: resp.Get("value")}
		}
		cleanup()
		return nil
	})
	catchCb = js.FuncOf(func(this js.Value, jargs []js.Value) interface{} {
		msg := "opfs: unknown error"
		if len(jargs) > 0 && !jargs[0].IsUndefined() {
			if m := jargs[0].Get("message"); !m.IsUndefined() {
				msg = m.String()
			} else {
				msg = jargs[0].String()
			}
		}
		ch <- result{err: msg}
		cleanup()
		return nil
	})

	promise := opfs.Call(method, args...)
	promise.Call("then", thenCb).Call("catch", catchCb)

	r := <-ch
	if r.err != "" {
		return js.Undefined(), errors.New(r.err)
	}
	return r.val, nil
}

func opfsErrTuple(err error) Object {
	return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
}

// opfsStatHash converts the shim's {name,size,isDir,isSymlink,modTime,mode}
// object into a kLex Hash matching the desktop nativeFs*Stat shape.
func opfsStatHash(v js.Value) *Hash {
	h := &Hash{Pairs: make(map[HashKey]HashPair)}
	set := func(k string, val Object) {
		h.Pairs[HashKey{Type: STRING_OBJ, Value: k}] = HashPair{Key: &String{Value: k}, Value: val}
	}
	set("name", &String{Value: v.Get("name").String()})
	set("size", &Integer{Value: v.Get("size").Int()})
	set("isDir", &Boolean{Value: v.Get("isDir").Bool()})
	set("isSymlink", &Boolean{Value: v.Get("isSymlink").Bool()})
	set("modTime", &Integer{Value: v.Get("modTime").Int()})
	set("mode", &String{Value: v.Get("mode").String()})
	return h
}

func opfsFsRead(path string) Object {
	v, err := opfsCall("read", path)
	if err != nil {
		return opfsErrTuple(err)
	}
	return &Tuple{Elements: []Object{&String{Value: v.String()}, NULL}}
}

func opfsFsReadBytes(path string) Object {
	v, err := opfsCall("readBytes", path)
	if err != nil {
		return opfsErrTuple(err)
	}
	buf := make([]byte, v.Length())
	js.CopyBytesToGo(buf, v)
	return &Tuple{Elements: []Object{&Bytes{Value: buf}, NULL}}
}

func opfsFsWrite(path string, content Object) Object {
	contentStr, ok := content.(*String)
	if !ok {
		return typeError("_fsWrite: content must be string, got "+string(content.Type()), ast.Pos{})
	}
	_, err := opfsCall("write", path, contentStr.Value)
	if err != nil {
		return opfsErrTuple(err)
	}
	return &Tuple{Elements: []Object{NULL, NULL}}
}

func opfsFsAppend(path string, content Object) Object {
	contentStr, ok := content.(*String)
	if !ok {
		return typeError("_fsAppend: content must be string, got "+string(content.Type()), ast.Pos{})
	}
	_, err := opfsCall("append", path, contentStr.Value)
	if err != nil {
		return opfsErrTuple(err)
	}
	return &Tuple{Elements: []Object{NULL, NULL}}
}

func opfsFsAppendBytesSync(path string, raw Object) Object {
	bs, ok := raw.(*Bytes)
	if !ok {
		return typeError("_fsAppendBytesSync: bytes must be bytes, got "+string(raw.Type()), ast.Pos{})
	}
	arr := js.Global().Get("Uint8Array").New(len(bs.Value))
	js.CopyBytesToJS(arr, bs.Value)
	_, err := opfsCall("appendBytes", path, arr)
	if err != nil {
		return opfsErrTuple(err)
	}
	return &Tuple{Elements: []Object{NULL, NULL}}
}

func opfsFsExists(path string) Object {
	v, err := opfsCall("exists", path)
	if err != nil {
		return &Boolean{Value: false}
	}
	return &Boolean{Value: v.Bool()}
}

func opfsFsStat(path string) Object {
	v, err := opfsCall("stat", path)
	if err != nil {
		return opfsErrTuple(err)
	}
	return &Tuple{Elements: []Object{opfsStatHash(v), NULL}}
}

func opfsFsLstat(path string) Object {
	// OPFS has no symlinks; lstat is identical to stat.
	return opfsFsStat(path)
}

func opfsFsListDir(path string) Object {
	v, err := opfsCall("listDir", path)
	if err != nil {
		return opfsErrTuple(err)
	}
	n := v.Length()
	names := make([]Object, n)
	for i := 0; i < n; i++ {
		names[i] = &String{Value: v.Index(i).String()}
	}
	return &Tuple{Elements: []Object{&Array{Elements: names}, NULL}}
}

func opfsFsReadDir(path string) Object {
	v, err := opfsCall("readDir", path)
	if err != nil {
		return opfsErrTuple(err)
	}
	n := v.Length()
	infos := make([]Object, n)
	for i := 0; i < n; i++ {
		infos[i] = opfsStatHash(v.Index(i))
	}
	return &Tuple{Elements: []Object{&Array{Elements: infos}, NULL}}
}

func opfsFsRemove(path string) Object {
	_, err := opfsCall("remove", path)
	if err != nil {
		return opfsErrTuple(err)
	}
	return &Tuple{Elements: []Object{NULL, NULL}}
}

func opfsFsRemoveAll(path string) Object {
	_, err := opfsCall("removeAll", path)
	if err != nil {
		return opfsErrTuple(err)
	}
	return &Tuple{Elements: []Object{NULL, NULL}}
}

func opfsFsMkdir(path string) Object {
	_, err := opfsCall("mkdir", path)
	if err != nil {
		return opfsErrTuple(err)
	}
	return &Tuple{Elements: []Object{NULL, NULL}}
}

func opfsFsMkdirAll(path string) Object {
	_, err := opfsCall("mkdirAll", path)
	if err != nil {
		return opfsErrTuple(err)
	}
	return &Tuple{Elements: []Object{NULL, NULL}}
}

func opfsFsTruncate(path string, raw Object) Object {
	sz, ok := raw.(*Integer)
	if !ok {
		return typeError("_fsTruncate: newSize must be integer, got "+string(raw.Type()), ast.Pos{})
	}
	if sz.Value < 0 {
		return runtimeError("_fsTruncate: newSize must be non-negative", ast.Pos{})
	}
	_, err := opfsCall("truncate", path, sz.Value)
	if err != nil {
		return &String{Value: err.Error()}
	}
	return NULL
}

func opfsFsCopy(src, dst string) Object {
	_, err := opfsCall("copy", src, dst)
	if err != nil {
		return opfsErrTuple(err)
	}
	return &Tuple{Elements: []Object{NULL, NULL}}
}

func opfsFsRename(src, dst string) Object {
	_, err := opfsCall("rename", src, dst)
	if err != nil {
		return opfsErrTuple(err)
	}
	return &Tuple{Elements: []Object{NULL, NULL}}
}

func opfsFsReadChunk(path string, offsetRaw, byteCountRaw Object) Object {
	offsetObj, ok := offsetRaw.(*Integer)
	if !ok {
		return typeError("_fsReadChunk: offset must be integer, got "+string(offsetRaw.Type()), ast.Pos{})
	}
	byteCountObj, ok := byteCountRaw.(*Integer)
	if !ok {
		return typeError("_fsReadChunk: byteCount must be integer, got "+string(byteCountRaw.Type()), ast.Pos{})
	}
	if offsetObj.Value < 0 {
		return runtimeError("_fsReadChunk: offset cannot be negative", ast.Pos{})
	}
	if byteCountObj.Value < 0 {
		return runtimeError("_fsReadChunk: byteCount cannot be negative", ast.Pos{})
	}
	v, err := opfsCall("readChunk", path, offsetObj.Value, byteCountObj.Value)
	if err != nil {
		return &Tuple{Elements: []Object{NULL, NULL, &String{Value: err.Error()}}}
	}
	return &Tuple{Elements: []Object{
		&String{Value: v.Get("content").String()},
		&Boolean{Value: v.Get("isEOF").Bool()},
		NULL,
	}}
}

// opfsFsMap on browser is identical to opfsFsRead (no mmap in the
// browser FS APIs; the contract is "memory-resident content string").
func opfsFsMap(path string) Object {
	return opfsFsRead(path)
}

func opfsFsCountLines(path string) Object {
	v, err := opfsCall("countLines", path)
	if err != nil {
		return &Tuple{Elements: []Object{&Integer{Value: 0}, &String{Value: err.Error()}}}
	}
	return &Tuple{Elements: []Object{&Integer{Value: v.Int()}, NULL}}
}

func opfsFsTmpFile(dir string, raw Object) Object {
	pattern, ok := raw.(*String)
	if !ok {
		return typeError("_fsTmpFile: pattern must be string, got "+string(raw.Type()), ast.Pos{})
	}
	v, err := opfsCall("tmpFile", dir, pattern.Value)
	if err != nil {
		return opfsErrTuple(err)
	}
	return &Tuple{Elements: []Object{&String{Value: v.String()}, NULL}}
}

func opfsFsTmpDir(dir string, raw Object) Object {
	pattern, ok := raw.(*String)
	if !ok {
		return typeError("_fsTmpDir: pattern must be string, got "+string(raw.Type()), ast.Pos{})
	}
	v, err := opfsCall("tmpDir", dir, pattern.Value)
	if err != nil {
		return opfsErrTuple(err)
	}
	return &Tuple{Elements: []Object{&String{Value: v.String()}, NULL}}
}

// opfsShimJS is the JS-side helper installed at package init. It hides
// the OPFS Promise dance behind a uniform `{ok, value, error}` contract
// so the Go side only deals with one shape per call.
const opfsShimJS = `
(function() {
  var root = (typeof globalThis !== 'undefined') ? globalThis : (typeof window !== 'undefined') ? window : (typeof self !== 'undefined') ? self : this;
  if (typeof navigator === 'undefined' || !navigator.storage || typeof navigator.storage.getDirectory !== 'function') {
    root.__klex_opfs = new Proxy({}, {
      get: function(_, prop) {
        return async function() {
          return {ok: false, error: 'OPFS unavailable: navigator.storage.getDirectory is not present (Node.js, insecure context, or unsupported environment)'};
        };
      }
    });
    return;
  }

  const ok = (value) => ({ok: true, value: value});
  const fail = (err) => ({ok: false, error: typeof err === 'string' ? err : (err && err.message ? err.message : String(err))});

  function splitPath(path) {
    return (path || '').split('/').filter(p => p.length > 0);
  }

  async function walkDir(path, createIntermediates) {
    const parts = splitPath(path);
    let dir = await navigator.storage.getDirectory();
    for (const part of parts) {
      dir = await dir.getDirectoryHandle(part, {create: !!createIntermediates});
    }
    return dir;
  }

  async function walkToParent(path, createIntermediates) {
    const parts = splitPath(path);
    if (parts.length === 0) {
      throw new Error('path is empty');
    }
    const name = parts.pop();
    let dir = await navigator.storage.getDirectory();
    for (const part of parts) {
      dir = await dir.getDirectoryHandle(part, {create: !!createIntermediates});
    }
    return {parent: dir, name: name};
  }

  async function statFromFile(handle, name) {
    const file = await handle.getFile();
    return {
      name: name,
      size: file.size,
      isDir: false,
      isSymlink: false,
      modTime: Math.floor(file.lastModified / 1000),
      mode: '0644'
    };
  }

  function statForDir(name) {
    return {name: name, size: 0, isDir: true, isSymlink: false, modTime: 0, mode: '0755'};
  }

  root.__klex_opfs = {
    async read(path) {
      try {
        const {parent, name} = await walkToParent(path);
        const handle = await parent.getFileHandle(name);
        const file = await handle.getFile();
        return ok(await file.text());
      } catch (e) { return fail(e); }
    },
    async readBytes(path) {
      try {
        const {parent, name} = await walkToParent(path);
        const handle = await parent.getFileHandle(name);
        const file = await handle.getFile();
        return ok(new Uint8Array(await file.arrayBuffer()));
      } catch (e) { return fail(e); }
    },
    async write(path, content) {
      try {
        const {parent, name} = await walkToParent(path);
        const handle = await parent.getFileHandle(name, {create: true});
        const w = await handle.createWritable();
        await w.write(content);
        await w.close();
        return ok(null);
      } catch (e) { return fail(e); }
    },
    async append(path, content) {
      try {
        const {parent, name} = await walkToParent(path);
        const handle = await parent.getFileHandle(name, {create: true});
        let existing = '';
        try {
          existing = await (await handle.getFile()).text();
        } catch (_) {}
        const w = await handle.createWritable();
        await w.write(existing + content);
        await w.close();
        return ok(null);
      } catch (e) { return fail(e); }
    },
    async appendBytes(path, bytes) {
      try {
        const {parent, name} = await walkToParent(path);
        const handle = await parent.getFileHandle(name, {create: true});
        let existing = new Uint8Array(0);
        try {
          existing = new Uint8Array(await (await handle.getFile()).arrayBuffer());
        } catch (_) {}
        const combined = new Uint8Array(existing.length + bytes.length);
        combined.set(existing);
        combined.set(bytes, existing.length);
        const w = await handle.createWritable();
        await w.write(combined);
        await w.close();
        return ok(null);
      } catch (e) { return fail(e); }
    },
    async exists(path) {
      try {
        const {parent, name} = await walkToParent(path);
        try { await parent.getFileHandle(name); return ok(true); } catch (_) {}
        try { await parent.getDirectoryHandle(name); return ok(true); } catch (_) {}
        return ok(false);
      } catch (_) { return ok(false); }
    },
    async stat(path) {
      try {
        const {parent, name} = await walkToParent(path);
        try {
          const fh = await parent.getFileHandle(name);
          return ok(await statFromFile(fh, name));
        } catch (_) {
          await parent.getDirectoryHandle(name);
          return ok(statForDir(name));
        }
      } catch (e) { return fail(e); }
    },
    async lstat(path) { return this.stat(path); },
    async listDir(path) {
      try {
        const dir = await walkDir(path, false);
        const names = [];
        for await (const k of dir.keys()) names.push(k);
        names.sort();
        return ok(names);
      } catch (e) { return fail(e); }
    },
    async readDir(path) {
      try {
        const dir = await walkDir(path, false);
        const entries = [];
        for await (const [name, handle] of dir.entries()) {
          if (handle.kind === 'file') entries.push(await statFromFile(handle, name));
          else entries.push(statForDir(name));
        }
        entries.sort((a, b) => a.name.localeCompare(b.name));
        return ok(entries);
      } catch (e) { return fail(e); }
    },
    async remove(path) {
      try {
        const {parent, name} = await walkToParent(path);
        await parent.removeEntry(name);
        return ok(null);
      } catch (e) { return fail(e); }
    },
    async removeAll(path) {
      try {
        const {parent, name} = await walkToParent(path);
        await parent.removeEntry(name, {recursive: true});
        return ok(null);
      } catch (e) { return fail(e); }
    },
    async mkdir(path) {
      try {
        const {parent, name} = await walkToParent(path);
        await parent.getDirectoryHandle(name, {create: true});
        return ok(null);
      } catch (e) { return fail(e); }
    },
    async mkdirAll(path) {
      try { await walkDir(path, true); return ok(null); }
      catch (e) { return fail(e); }
    },
    async truncate(path, newSize) {
      try {
        const {parent, name} = await walkToParent(path);
        const handle = await parent.getFileHandle(name, {create: true});
        const w = await handle.createWritable({keepExistingData: true});
        await w.truncate(newSize);
        await w.close();
        return ok(null);
      } catch (e) { return fail(e); }
    },
    async copy(src, dst) {
      try {
        const r = await this.readBytes(src);
        if (!r.ok) return r;
        const {parent, name} = await walkToParent(dst);
        const handle = await parent.getFileHandle(name, {create: true});
        const w = await handle.createWritable();
        await w.write(r.value);
        await w.close();
        return ok(null);
      } catch (e) { return fail(e); }
    },
    async rename(src, dst) {
      try {
        const {parent: srcParent, name: srcName} = await walkToParent(src);
        let srcHandle;
        try { srcHandle = await srcParent.getFileHandle(srcName); }
        catch (_) { srcHandle = await srcParent.getDirectoryHandle(srcName); }
        if (typeof srcHandle.move === 'function') {
          const {parent: dstParent, name: dstName} = await walkToParent(dst, true);
          await srcHandle.move(dstParent, dstName);
          return ok(null);
        }
        if (srcHandle.kind === 'directory') {
          return fail('rename: directory rename requires FileSystemHandle.move() which is not available in this browser');
        }
        const cr = await this.copy(src, dst);
        if (!cr.ok) return cr;
        return await this.remove(src);
      } catch (e) { return fail(e); }
    },
    async readChunk(path, offset, length) {
      try {
        const {parent, name} = await walkToParent(path);
        const handle = await parent.getFileHandle(name);
        const file = await handle.getFile();
        const slice = file.slice(offset, offset + length);
        const text = await slice.text();
        const isEOF = (offset + slice.size) >= file.size;
        return ok({content: text, isEOF: isEOF});
      } catch (e) { return fail(e); }
    },
    async countLines(path) {
      try {
        const {parent, name} = await walkToParent(path);
        const handle = await parent.getFileHandle(name);
        const file = await handle.getFile();
        let count = 0, hasContent = false, lastByte = 0;
        const chunkSize = 64 * 1024;
        for (let off = 0; off < file.size; off += chunkSize) {
          const slice = file.slice(off, Math.min(off + chunkSize, file.size));
          const buf = new Uint8Array(await slice.arrayBuffer());
          if (buf.length > 0) {
            hasContent = true;
            for (let i = 0; i < buf.length; i++) if (buf[i] === 10) count++;
            lastByte = buf[buf.length - 1];
          }
        }
        if (hasContent && lastByte !== 10) count++;
        return ok(count);
      } catch (e) { return fail(e); }
    },
    async tmpFile(dir, pattern) {
      try {
        const stem = (pattern || 'tmp').replace(/\*/g, '');
        const name = stem + '-' + Math.random().toString(36).slice(2, 10);
        const base = (dir || '').replace(/\/$/, '');
        const fullPath = base ? base + '/' + name : name;
        const r = await this.write(fullPath, '');
        if (!r.ok) return r;
        return ok(fullPath);
      } catch (e) { return fail(e); }
    },
    async tmpDir(dir, pattern) {
      try {
        const stem = (pattern || 'tmp').replace(/\*/g, '');
        const name = stem + '-' + Math.random().toString(36).slice(2, 10);
        const base = (dir || '').replace(/\/$/, '');
        const fullPath = base ? base + '/' + name : name;
        const r = await this.mkdirAll(fullPath);
        if (!r.ok) return r;
        return ok(fullPath);
      } catch (e) { return fail(e); }
    }
  };
})();
`
