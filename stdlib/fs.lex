// fs.lex
// Filesystem operations for kLex.
//
// All mutating operations return (null, err) tuples.
// read returns (content, err). stat returns (FileInfo, err).
// exists is the only function that returns a plain bool — a missing file
// is not an error, it is information.
//
// modTime in FileInfo is a unix timestamp (integer seconds), compatible
// with datetime.lex fromUnix().
//
// Usage:
//   import "fs.lex" as fs
//   content, err = fs.read("/etc/hostname")
//   if err != null { println(err)  return null }
//   println(content)

struct FileInfo {
    name, size, isDir, modTime
}

// read reads an entire file and returns (content, err).
fn read(path) {
    return _fsRead(path)
}

// write creates or truncates a file and writes content. Returns (null, err).
fn write(path, content) {
    return _fsWrite(path, content)
}

// append appends content to a file, creating it if absent. Returns (null, err).
fn append(path, content) {
    return _fsAppend(path, content)
}

// exists returns true if the path exists (file or directory), false otherwise.
fn exists(path) {
    return _fsExists(path)
}

// remove deletes a file or empty directory. Returns (null, err).
fn remove(path) {
    return _fsRemove(path)
}

// removeAll deletes path and everything inside it. Returns (null, err).
fn removeAll(path) {
    return _fsRemoveAll(path)
}

// mkdir creates a single directory (parent must exist). Returns (null, err).
fn mkdir(path) {
    return _fsMkdir(path)
}

// mkdirAll creates a directory and all missing parents. Returns (null, err).
fn mkdirAll(path) {
    return _fsMkdirAll(path)
}

// listDir returns (array_of_names, err) for the given directory.
// Names are filenames only (not full paths), sorted alphabetically.
fn listDir(path) {
    return _fsListDir(path)
}

// rename moves or renames src to dst. Returns (null, err).
fn rename(src, dst) {
    return _fsRename(src, dst)
}

// stat returns (FileInfo, err) for the given path.
fn stat(path) {
    info, err = _fsStat(path)
    if err != null { return null, err }
    return FileInfo {
        name:    info["name"],
        size:    info["size"],
        isDir:   info["isDir"],
        modTime: info["modTime"]
    }, null
}

// copy copies src to dst byte-for-byte. Returns (null, err).
fn copy(src, dst) {
    return _fsCopy(src, dst)
}
