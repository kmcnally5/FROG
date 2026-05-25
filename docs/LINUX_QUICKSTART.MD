# kLex on Linux — Quickstart

A practical guide to getting kLex built, installed, and running graphical apps (Tadpole, SecretHunter, etc.) on Ubuntu/Debian or Fedora. Distilled from real install sessions — every step in here was hit at least once.

Tested on Fedora and Ubuntu/Debian. Other distros (Arch, openSUSE, etc.) follow the same shape but with different package names.

---

## 1. Install Go (latest)

Ubuntu's `apt` and Fedora's `dnf` typically ship an old Go release — usually a major version behind. Use the official tarball:

```sh
# 1. Grab the latest from https://go.dev/dl/  (filename pattern: go1.XX.Y.linux-amd64.tar.gz)
cd /tmp
wget https://go.dev/dl/go1.24.0.linux-amd64.tar.gz   # ← replace with current version

# 2. Wipe any old install (skip if /usr/local/go doesn't exist), then install
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.24.0.linux-amd64.tar.gz

# 3. Add to PATH — append to ~/.bashrc or ~/.zshrc so it persists across logins
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

# 4. Verify
go version
```

**Note:** Go ships no installer script. The tarball *is* the install — you move `/usr/local/go` to the canonical location and add `bin/` to PATH. That's the whole procedure.

---

## 2. Install build dependencies

kLex uses cgo for the OpenGL / GLFW bindings, so the build needs C compiler tools, `pkg-config`, and the OpenGL / X11 **dev** headers (not just the runtime libs Ubuntu/Fedora ship by default).

### Ubuntu / Debian

```sh
sudo apt update    # always do this first on a fresh box
sudo apt install -y \
  build-essential \
  pkg-config \
  libgl1-mesa-dev \
  libglu1-mesa-dev \
  xorg-dev \
  libxi-dev \
  libxcursor-dev \
  libxinerama-dev \
  libxrandr-dev \
  libxxf86vm-dev
```

### Fedora

```sh
sudo dnf install -y \
  gcc gcc-c++ make \
  pkgconf-pkg-config \
  mesa-libGL-devel \
  mesa-libGLU-devel \
  libX11-devel \
  libXi-devel \
  libXcursor-devel \
  libXinerama-devel \
  libXrandr-devel \
  libXxf86vm-devel
```

### Why each one matters

| Package (Ubuntu / Fedora) | Purpose |
|---|---|
| `build-essential` / `gcc gcc-c++ make` | C toolchain — cgo needs it |
| `pkg-config` / `pkgconf-pkg-config` | How cgo locates `.pc` files |
| `libgl1-mesa-dev` / `mesa-libGL-devel` | Provides `gl.pc` — fixes `Package gl was not found in the pkg-config search path` |
| `libglu1-mesa-dev` / `mesa-libGLU-devel` | GLU utility lib |
| `xorg-dev` / `libX11-devel` | X11 base |
| `libxi-dev` / `libXi-devel` | X input (mouse, keyboard) |
| `libxcursor-dev` / `libXcursor-devel` | Cursors |
| `libxinerama-dev` / `libXinerama-devel` | Multi-monitor |
| `libxrandr-dev` / `libXrandr-devel` | Display resolution |
| `libxxf86vm-dev` / `libXxf86vm-devel` | XF86 video mode |

Verify after install:

```sh
pkg-config --modversion gl    # should print a version number
```

---

## 3. Build kLex

From the kLex repo root:

```sh
go build -o klex .
```

First build is slow (~30–60 s) because cgo compiles the GL/GLFW bindings. Subsequent builds use the cache and are fast.

If you see an error here, it's almost always a missing dev header from §2. Read the error — it'll tell you which library it couldn't find.

---

## 4. Install runtime dependencies

Two categories of stuff that Linux doesn't ship by default but kLex's graphical apps need at *runtime* (separate from the build-time dev headers in §2).

### 4a. Fonts

Without fonts installed, apps fail with `no usable font found` and exit. The kLex `tryFont` pattern looks for **DejaVu** and **Liberation** on Linux.

```sh
# Ubuntu / Debian
sudo apt install -y fonts-dejavu fonts-liberation

# Fedora
sudo dnf install -y dejavu-sans-fonts liberation-fonts
```

### 4b. Clipboard tools

Without these, copy buttons in Tadpole and SecretHunter silently no-op or — worse, with old clipboard chains — hang the UI.

```sh
# Ubuntu / Debian
sudo apt install -y wl-clipboard xclip xsel

# Fedora
sudo dnf install -y wl-clipboard xclip xsel
```

The clipboard chain in the apps falls back: `wl-copy` (Wayland) → `xsel` (X11, clean exit) → `xclip` (X11, last resort, can hang on Wayland). Installing `wl-clipboard` is the most important — modern desktop sessions (GNOME, KDE) default to Wayland, where `xclip` doesn't work properly.

Check your session type:

```sh
echo $XDG_SESSION_TYPE       # "wayland" or "x11"
```

---

## 5. Install kLex (pick a layout)

Two clean install patterns. kLex's import resolver handles both — no `KLEX_PATH` env var needed.

### Drop-in (simplest)

Binary + stdlib in one folder:

```
~/klex/
├── klex            # the binary
└── stdlib/         # copy of the repo's stdlib/
```

```sh
mkdir -p ~/klex
cp klex ~/klex/
cp -a stdlib ~/klex/
echo 'export PATH="$HOME/klex:$PATH"' >> ~/.bashrc
source ~/.bashrc
```

### FHS-style (more standard)

```sh
install -m755 klex ~/.local/bin/klex
mkdir -p ~/.local/share/klex
cp -a stdlib ~/.local/share/klex/
# ~/.local/bin is already on PATH on most distros
```

The resolver walks from `<bin>/klex` to `<bin-parent>/share/klex/stdlib/` automatically.

For system-wide install, swap `~/.local/` for `/usr/local/`.

---

## 6. Running scripts

**Run from the kLex repo root** if you're invoking example scripts that ship in the repo. Many examples use relative paths like `tests/examples/SecretHunter/secretHunterLib.lex` for imports and asset loading — these resolve against the current working directory, not the script's location.

```sh
cd /path/to/kLex
./klex tests/examples/SecretHunter/secretHunterUI.lex
./klex tests/examples/tadPole/aiImage.lex
./klex tests/unit/masterTest.lex
```

For your own scripts, run from wherever — the resolver finds `stdlib/` via the install layout you set up in §5.

---

## 7. Troubleshooting

### `Package gl was not found in the pkg-config search path`
Missing OpenGL dev headers — do §2. Specifically `libgl1-mesa-dev` (Ubuntu) or `mesa-libGL-devel` (Fedora).

### `apt-get` says no candidate for those packages
`sudo apt update` first — the package index is stale or never fetched. If still missing, enable the **universe** repo: `sudo add-apt-repository universe`.

### `ERROR: no usable font found`
Install fonts (§4a). Apps print a hint pointing at the package name.

### Copy button on a SecretHunter / Tadpole row appears to crash / hang the UI
Install the Wayland clipboard tool (§4b) — `wl-clipboard`. The apps now run clipboard subprocesses via `async()` so they shouldn't hang the UI even without it, but if you're on an older build, update.

### `RuntimeError: return outside function`
A script has a `return` at module scope (not inside a `fn`). This typically fires on Linux when a macOS-only path (e.g. hardcoded `/System/Library/Fonts/`) fails to resolve and an error guard with `return` hits. Fix: replace `return` with `_osExit(1)` for hard fail, or wrap the happy path in `else { ... }`. Tadpole and SecretHunter already use `_osExit(1)`.

### `cannot list tests/: open tests/unit: no such file or directory`
You're running an example script from somewhere other than the kLex repo root. `cd` to the repo root and try again.

### Apps crash with "loadFont: failed to build atlas"
You're trying to load a `.ttc` (TrueType Collection) file — kLex's font parser only handles `.ttf` / `.otf`. Pick a `.ttf` from the same family. The `tryFont` pattern in the example apps handles this by cascading through known-good `.ttf` paths.

---

## Worth knowing

- **Renderer quality:** Linux uses Mesa (still actively developed, OpenGL up to 4.6). macOS is frozen at OpenGL 4.1 through a Metal compatibility shim. SDF text rendering and edge anti-aliasing often look noticeably crisper on Linux as a result.

- **`glxinfo`** shows which renderer and GL version you're on:
  ```sh
  glxinfo | grep -E "OpenGL (renderer|version|vendor)"
  ```

- **Headless / server Linux** (no display): the build still works, but graphical apps fail at `window()` because there's no display. Non-GUI scripts run fine.

- **WSL2:** technically possible but graphics flaky — depends heavily on Windows version and GPU passthrough setup. Native Linux is the well-tested path.

---

## Quick verification

After completing §1–4, run the kLex test suite and a graphical app to confirm everything works:

```sh
cd /path/to/kLex
./klex tests/unit/masterTest.lex                       # ~46/47 tests should pass
./klex tests/examples/SecretHunter/secretHunterUI.lex  # graphical app
```

The DB test is a known cross-platform fail unless you have a SQL Server / Postgres available for the bridge.
