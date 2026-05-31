//go:build !js

package eval

// Desktop stubs for the opfs:// scheme. OPFS (Origin Private File System)
// is a browser-only API; on desktop builds every opfs* call returns the
// same shape its native counterpart would on failure (a (null, errString)
// tuple, a bool, etc.) so scripts can detect the condition and degrade
// gracefully:
//
//   let content, err = fs.read("opfs://config.json")
//   if err != null { content, err = fs.read("./local-config.json") }
//
// The wasm build supplies real implementations in
// builtins_fs_opfs_wasm.go (added separately as part of Phase 5).

const opfsErrMsg = "opfs:// scheme requires the browser/WASM build"

func opfsErrTuple2() Object {
	return &Tuple{Elements: []Object{NULL, &String{Value: opfsErrMsg}}}
}

func opfsErrTuple3() Object {
	return &Tuple{Elements: []Object{NULL, NULL, &String{Value: opfsErrMsg}}}
}

func opfsFsRead(string) Object                  { return opfsErrTuple2() }
func opfsFsWrite(string, Object) Object         { return opfsErrTuple2() }
func opfsFsReadBytes(string) Object             { return opfsErrTuple2() }
func opfsFsReadChunk(string, Object, Object) Object {
	return opfsErrTuple3()
}
func opfsFsMap(string) Object                   { return opfsErrTuple2() }
func opfsFsCountLines(string) Object {
	return &Tuple{Elements: []Object{&Integer{Value: 0}, &String{Value: opfsErrMsg}}}
}
func opfsFsExists(string) Object                { return &Boolean{Value: false} }
func opfsFsStat(string) Object                  { return opfsErrTuple2() }
func opfsFsLstat(string) Object                 { return opfsErrTuple2() }
func opfsFsListDir(string) Object               { return opfsErrTuple2() }
func opfsFsReadDir(string) Object               { return opfsErrTuple2() }
func opfsFsAppend(string, Object) Object        { return opfsErrTuple2() }
func opfsFsAppendBytesSync(string, Object) Object {
	return opfsErrTuple2()
}
func opfsFsTruncate(string, Object) Object      { return &String{Value: opfsErrMsg} }
func opfsFsRemove(string) Object                { return opfsErrTuple2() }
func opfsFsRemoveAll(string) Object             { return opfsErrTuple2() }
func opfsFsMkdir(string) Object                 { return opfsErrTuple2() }
func opfsFsMkdirAll(string) Object              { return opfsErrTuple2() }
func opfsFsTmpFile(string, Object) Object       { return opfsErrTuple2() }
func opfsFsTmpDir(string, Object) Object        { return opfsErrTuple2() }
func opfsFsCopy(string, string) Object          { return opfsErrTuple2() }
func opfsFsRename(string, string) Object        { return opfsErrTuple2() }
