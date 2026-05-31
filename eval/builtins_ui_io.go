package eval

// builtins_ui_io.go — uniform input + window snapshot for shared
// widget bodies.
//
// Widget code calls uiInput() once at the top of its body and reads
// the returned struct rather than reaching into the platform-specific
// gfx / gfxState directly. This is the smallest API that lets a single
// widget body work on both backends, while leaving the actual storage
// untouched (gfx fields stay on gfx, gfxState fields stay on gfxState).
//
// The snapshot is a value (not a pointer) — widgets that need to *set*
// hover/active focus do so via uiCore directly, since that storage IS
// shared. Only the read-only inputs need cross-platform abstraction.

// uiInputSnapshot captures the per-frame input + window state every
// Phase 1a widget needs. Additional fields will be added as later
// phases pull in more inputs (scroll wheel, modifier keys, typed
// chars, etc.).
type uiInputSnapshot struct {
	mouseX            float32
	mouseY            float32
	mouseDown         bool
	mouseClicked      bool // one-shot per frame (desktop: mouseJustClicked)
	mouseRightDown    bool
	mouseRightClicked bool
	scrollY           float64
	scrollX           float64
	typedChars        string // printable characters typed this frame

	// Modifier key state.
	shift bool
	ctrl  bool
	cmd   bool // macOS Command / Windows Super

	// Per-frame key-repeat counts for the text widgets.
	backspaceCount int
	deleteCount    int
	leftCount      int
	rightCount     int
	upCount        int
	downCount      int

	// One-shot flags for non-text keys textInput may consume.
	enterPressed bool
	tabPressed   bool
	homePressed  bool
	endPressed   bool

	// Clipboard paste text for this frame ("" if none). Set by the
	// browser paste event on WASM; set by Ctrl/Cmd+V detection plus
	// GLFW GetClipboardString on desktop.
	clipPaste string

	// One-shot letter-key flags for editor shortcuts. Widget combines
	// these with cmd/ctrl/shift to detect Cmd+A, Cmd+C, etc. The flags
	// fire on the first frame the key transitions from up → down
	// (no auto-repeat).
	keyA bool
	keyC bool
	keyV bool
	keyX bool
	keyY bool
	keyZ bool

	winW       int
	winH       int
	frameCount int
}

// uiInput is implemented per-target in eval/builtins_ui_io_unix.go and
// eval/builtins_ui_io_wasm.go.
//
// uiClipboardWrite copies `text` to the OS clipboard. Implemented per
// target — desktop uses the GLFW sync call; WASM uses
// navigator.clipboard.writeText (fire-and-forget; the Promise is
// dropped on the floor since widget code doesn't await it).
