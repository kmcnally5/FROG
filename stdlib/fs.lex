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
    name, size, isDir, isSymlink, modTime, mode
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

// stat returns (FileInfo, err) for the given path. Follows symbolic links.
fn stat(path) {
    info, err = _fsStat(path)
    if err != null { return null, err }
    return FileInfo {
        name:      info["name"],
        size:      info["size"],
        isDir:     info["isDir"],
        isSymlink: info["isSymlink"],
        modTime:   info["modTime"],
        mode:      info["mode"]
    }, null
}

// lstat returns (FileInfo, err) like stat, but does not follow symbolic links.
// If path names a symlink, FileInfo describes the link itself (isSymlink == true).
fn lstat(path) {
    info, err = _fsLstat(path)
    if err != null { return null, err }
    return FileInfo {
        name:      info["name"],
        size:      info["size"],
        isDir:     info["isDir"],
        isSymlink: info["isSymlink"],
        modTime:   info["modTime"],
        mode:      info["mode"]
    }, null
}

// chmod changes the permission bits of the named file.
// mode is an octal string such as "755" or "644". Returns (null, err).
fn chmod(path, mode) {
    return _fsChmod(path, mode)
}

// readDir returns (array_of_FileInfo, err) for the given directory.
// Unlike listDir, each entry is a full FileInfo including size, mode, and isSymlink.
fn readDir(path) {
    raw, err = _fsReadDir(path)
    if err != null { return null, err }
    out = []
    i = 0
    while i < len(raw) {
        info = raw[i]
        out = push(out, FileInfo {
            name:      info["name"],
            size:      info["size"],
            isDir:     info["isDir"],
            isSymlink: info["isSymlink"],
            modTime:   info["modTime"],
            mode:      info["mode"]
        })
        i = i + 1
    }
    return out, null
}

// symlink creates a symbolic link named link pointing to target. Returns (null, err).
fn symlink(target, link) {
    return _fsSymlink(target, link)
}

// readlink returns (target, err) — the destination of a symbolic link.
fn readlink(path) {
    return _fsReadlink(path)
}

// tmpFile creates a new temporary file in dir with a name matching pattern,
// closes it, and returns (path, err). Pass "" for dir to use the system temp directory.
fn tmpFile(dir, pattern) {
    return _fsTmpFile(dir, pattern)
}

// tmpDir creates a new temporary directory in dir with a name matching pattern.
// Returns (path, err). Pass "" for dir to use the system temp directory.
fn tmpDir(dir, pattern) {
    return _fsTmpDir(dir, pattern)
}

// copy copies src to dst byte-for-byte. Returns (null, err).
fn copy(src, dst) {
    return _fsCopy(src, dst)
}
