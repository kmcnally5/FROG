//go:build js && wasm

package eval

// WASM browser-side stubs for the file:// scheme (i.e. the "native"
// filesystem). Browsers have no host FS, so every file:// fs.* call
// returns the same shape its desktop counterpart would on failure — a
// (null, errString) tuple, a bool, etc. — so scripts can detect the
// condition and fall back to opfs:// or http(s):// for browser use:
//
//   let content, err = fs.read("./config.json")    // bare path = file://
//   if err != null {
//       content, err = fs.read("opfs://config.json")
//   }

const browserNoFsMsg = "file:// scheme has no host filesystem in the browser build — use opfs:// for persistent storage or http(s):// for fetched content"

func browserNoFsTuple2() Object {
	return &Tuple{Elements: []Object{NULL, &String{Value: browserNoFsMsg}}}
}

func browserNoFsTuple3() Object {
	return &Tuple{Elements: []Object{NULL, NULL, &String{Value: browserNoFsMsg}}}
}

func nativeFsRead(string) Object                  { return browserNoFsTuple2() }
func nativeFsWrite(string, Object) Object         { return browserNoFsTuple2() }
func nativeFsReadBytes(string) Object             { return browserNoFsTuple2() }
func nativeFsReadChunk(string, Object, Object) Object {
	return browserNoFsTuple3()
}
func nativeFsMap(string) Object                   { return browserNoFsTuple2() }
func nativeFsCountLines(string) Object {
	return &Tuple{Elements: []Object{&Integer{Value: 0}, &String{Value: browserNoFsMsg}}}
}
func nativeFsExists(string) Object                { return &Boolean{Value: false} }
func nativeFsStat(string) Object                  { return browserNoFsTuple2() }
func nativeFsLstat(string) Object                 { return browserNoFsTuple2() }
func nativeFsListDir(string) Object               { return browserNoFsTuple2() }
func nativeFsReadDir(string) Object               { return browserNoFsTuple2() }
func nativeFsAppend(string, Object) Object        { return browserNoFsTuple2() }
func nativeFsAppendBytesSync(string, Object) Object {
	return browserNoFsTuple2()
}
func nativeFsTruncate(string, Object) Object      { return &String{Value: browserNoFsMsg} }
func nativeFsRemove(string) Object                { return browserNoFsTuple2() }
func nativeFsRemoveAll(string) Object             { return browserNoFsTuple2() }
func nativeFsMkdir(string) Object                 { return browserNoFsTuple2() }
func nativeFsMkdirAll(string) Object              { return browserNoFsTuple2() }
func nativeFsChmod(string, Object) Object         { return browserNoFsTuple2() }
func nativeFsTmpFile(string, Object) Object       { return browserNoFsTuple2() }
func nativeFsTmpDir(string, Object) Object        { return browserNoFsTuple2() }
func nativeFsCopy(string, string) Object          { return browserNoFsTuple2() }
func nativeFsRename(string, string) Object        { return browserNoFsTuple2() }
func nativeFsSymlink(string, string) Object       { return browserNoFsTuple2() }
func nativeFsReadlink(string) Object              { return browserNoFsTuple2() }
