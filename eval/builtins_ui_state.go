package eval

import "time"

// builtins_ui_state.go — target-agnostic UI state.
//
// The kLex UI library state machine (theme, hover/active focus IDs, the
// element registry, layout cursors, listings of selected/scrolled rows,
// per-widget text editor state, etc.) is identical on desktop and in the
// browser. Only the rendering surface differs.
//
// Up to the WASM port, this state lived inline on the desktop `gfx`
// struct (eval/builtins_graphics.go). It is moved here so a single
// uiCore global serves both backends — see [[feedback-wasm-url-scheme-dispatcher-pattern]]
// for the analogous fs.* refactor.
//
// Fields are migrated in tiers matching the UI-port phases:
//
//   Phase 1a (this commit) — theme, focus IDs, element registry,
//                            layout cursors, list scroll/select state.
//   Phase 2+               — text editor maps, popup deferral state,
//                            tooltip timer state, undo/redo stacks.
//
// Unmigrated fields stay on `gfx` (desktop) and are accessed only by
// widgets that haven't been ported yet — those widgets compile under
// !js anyway, so they never need to read uiCore-equivalent storage
// from the WASM build.

// uiPalette is the 14-slot colour theme used by every interactive
// widget. Written by uiTheme(); read by drawing code on both targets.
type uiPalette struct {
	widgetBg       [4]float32 // 0  button/widget normal background
	widgetBgHover  [4]float32 // 1  hovered state
	widgetBgActive [4]float32 // 2  pressed/checked state
	widgetText     [4]float32 // 3  text on interactive widgets
	labelText      [4]float32 // 4  label text above/beside widgets
	dimText        [4]float32 // 5  secondary / unselected text
	accent         [4]float32 // 6  accent line / stripe color
	accentBg       [4]float32 // 7  selected item / active tab background
	track          [4]float32 // 8  slider / progress track
	trackFill      [4]float32 // 9  slider / progress fill
	handle         [4]float32 // 10 slider handle, scrollbar thumb, checkmark
	inputBg        [4]float32 // 11 text input / list container background
	inputFocusBg   [4]float32 // 12 focused text input background
	shadow         [4]float32 // 13 drop shadow for floating elements (contextMenu, tooltips)
}

// themePreset returns (palette, style, ok) for one of the named preset
// themes. Used by setTheme(name).
func themePreset(name string) (uiPalette, uiStyle, bool) {
	switch name {
	case "nebula":
		// Original deep-space violet + ice cyan look.
		return uiPalette{
			widgetBg:       [4]float32{0.08, 0.05, 0.16, 1},
			widgetBgHover:  [4]float32{0.18, 0.10, 0.36, 1},
			widgetBgActive: [4]float32{0.05, 0.03, 0.10, 1},
			widgetText:     [4]float32{0.92, 0.88, 1.00, 1},
			labelText:      [4]float32{0.78, 0.76, 0.92, 1},
			dimText:        [4]float32{0.48, 0.40, 0.65, 1},
			accent:         [4]float32{0.45, 0.85, 1.00, 1},
			accentBg:       [4]float32{0.20, 0.42, 0.62, 1},
			track:          [4]float32{0.05, 0.03, 0.10, 1},
			trackFill:      [4]float32{0.30, 0.20, 0.55, 1},
			handle:         [4]float32{0.85, 0.85, 0.95, 1},
			inputBg:        [4]float32{0.05, 0.03, 0.10, 1},
			inputFocusBg:   [4]float32{0.10, 0.05, 0.22, 1},
			shadow:         [4]float32{0.00, 0.00, 0.00, 0.85},
		}, defaultUIStyle(), true

	case "light":
		// Clean light theme — near-white surfaces, slate text, blue accent.
		s := defaultUIStyle()
		return uiPalette{
			widgetBg:       [4]float32{0.96, 0.97, 0.98, 1},
			widgetBgHover:  [4]float32{0.90, 0.92, 0.95, 1},
			widgetBgActive: [4]float32{0.84, 0.87, 0.92, 1},
			widgetText:     [4]float32{0.12, 0.15, 0.20, 1},
			labelText:      [4]float32{0.30, 0.34, 0.40, 1},
			dimText:        [4]float32{0.55, 0.60, 0.66, 1},
			accent:         [4]float32{0.10, 0.45, 0.95, 1},
			accentBg:       [4]float32{0.85, 0.92, 1.00, 1},
			track:          [4]float32{0.88, 0.90, 0.93, 1},
			trackFill:      [4]float32{0.10, 0.45, 0.95, 1},
			handle:         [4]float32{1.00, 1.00, 1.00, 1},
			inputBg:        [4]float32{1.00, 1.00, 1.00, 1},
			inputFocusBg:   [4]float32{1.00, 1.00, 1.00, 1},
			shadow:         [4]float32{0.10, 0.15, 0.25, 0.35},
		}, s, true

	case "dark":
		// Modern dark — slate base, blue accent, gentle contrast.
		s := defaultUIStyle()
		return uiPalette{
			widgetBg:       [4]float32{0.20, 0.22, 0.27, 1},
			widgetBgHover:  [4]float32{0.26, 0.28, 0.34, 1},
			widgetBgActive: [4]float32{0.16, 0.18, 0.22, 1},
			widgetText:     [4]float32{0.92, 0.94, 0.97, 1},
			labelText:      [4]float32{0.72, 0.76, 0.83, 1},
			dimText:        [4]float32{0.50, 0.54, 0.60, 1},
			accent:         [4]float32{0.30, 0.65, 1.00, 1},
			accentBg:       [4]float32{0.16, 0.32, 0.55, 1},
			track:          [4]float32{0.14, 0.16, 0.20, 1},
			trackFill:      [4]float32{0.30, 0.65, 1.00, 1},
			handle:         [4]float32{0.96, 0.97, 0.99, 1},
			inputBg:        [4]float32{0.12, 0.14, 0.18, 1},
			inputFocusBg:   [4]float32{0.16, 0.20, 0.28, 1},
			shadow:         [4]float32{0.00, 0.00, 0.00, 0.85},
		}, s, true

	case "highContrast":
		// Accessibility-first — pure black/white with strong yellow accent.
		s := defaultUIStyle()
		s.radiusSmall = 0
		s.radiusMedium = 2
		s.radiusLarge = 4
		s.focusRingPx = 3
		return uiPalette{
			widgetBg:       [4]float32{0.00, 0.00, 0.00, 1},
			widgetBgHover:  [4]float32{0.20, 0.20, 0.20, 1},
			widgetBgActive: [4]float32{1.00, 1.00, 0.00, 1}, // yellow press
			widgetText:     [4]float32{1.00, 1.00, 1.00, 1},
			labelText:      [4]float32{1.00, 1.00, 1.00, 1},
			dimText:        [4]float32{0.75, 0.75, 0.75, 1},
			accent:         [4]float32{1.00, 0.95, 0.00, 1}, // yellow
			accentBg:       [4]float32{0.50, 0.48, 0.00, 1},
			track:          [4]float32{0.25, 0.25, 0.25, 1},
			trackFill:      [4]float32{1.00, 0.95, 0.00, 1},
			handle:         [4]float32{1.00, 1.00, 1.00, 1},
			inputBg:        [4]float32{0.00, 0.00, 0.00, 1},
			inputFocusBg:   [4]float32{0.20, 0.18, 0.00, 1},
			shadow:         [4]float32{1.00, 1.00, 1.00, 0.50}, // light glow
		}, s, true
	}
	return uiPalette{}, uiStyle{}, false
}

// defaultUIPalette returns the baseline neutral theme. Applied at
// startup; user code overrides via makeTheme()+uiTheme().
func defaultUIPalette() uiPalette {
	return uiPalette{
		widgetBg:       [4]float32{0.30, 0.30, 0.30, 1},
		widgetBgHover:  [4]float32{0.50, 0.50, 0.50, 1},
		widgetBgActive: [4]float32{0.20, 0.20, 0.20, 1},
		widgetText:     [4]float32{0.92, 0.90, 1.00, 1},
		labelText:      [4]float32{0.78, 0.76, 0.92, 1},
		dimText:        [4]float32{0.62, 0.62, 0.62, 1},
		accent:         [4]float32{0.68, 0.68, 0.68, 1},
		accentBg:       [4]float32{0.40, 0.40, 0.40, 1},
		track:          [4]float32{0.18, 0.18, 0.18, 1},
		trackFill:      [4]float32{0.55, 0.55, 0.55, 1},
		handle:         [4]float32{0.85, 0.85, 0.85, 1},
		inputBg:        [4]float32{0.20, 0.20, 0.20, 1},
		inputFocusBg:   [4]float32{0.10, 0.30, 0.50, 1},
		shadow:         [4]float32{0.00, 0.00, 0.00, 0.50},
	}
}

// toastEntry is one active toast notification, rendered by uiEnd().
type toastEntry struct {
	message   string
	style     string  // "info", "success", "warn", "error"
	expiresAt float64 // seconds since gfx.startTime
}

// pendingTooltip holds tooltip text to be rendered by uiEnd() on top
// of everything.
type pendingTooltip struct {
	active bool
	text   string
	mx, my float32
}

// pendingDropdown holds the data for an open dropdown popup that must
// be rendered last (on top of all other widgets) by uiEnd().
type pendingDropdown struct {
	active      bool
	id          string
	fx, fy, fw  float32
	items       []string
	selectedIdx int
	charH       float32
	textScale   float32
}

// uiStyle holds the non-colour design tokens — corner radii, spacing
// scale, and typography. Colours live in uiCoreState.theme; together
// the two define the full visual identity. Set via setTheme(name) or
// individually via the future setStyle() / setRadii() builtins.
type uiStyle struct {
	// Corner radii. Buttons / fields use radiusMedium; cards / modals
	// use radiusLarge; thin/inner elements (checkmarks, scroll thumbs,
	// caret) use radiusSmall.
	radiusSmall  float32 // 3
	radiusMedium float32 // 8
	radiusLarge  float32 // 12

	// 4-px-derived spacing scale.
	spacingXS float32 // 4
	spacingS  float32 // 8
	spacingM  float32 // 12
	spacingL  float32 // 16
	spacingXL float32 // 24

	// Default body font size in px (used by the Canvas2D renderer when
	// no uiCore.activeFont is set). 20 was the original baseline; 16
	// reads as "modern UI". Widgets pass `scale` which multiplies this.
	fontBasePx float32 // 16

	// Focus-ring width in px (drawn outside the widget bounds).
	focusRingPx float32 // 2
}

// defaultUIStyle returns the baseline style — current Phase 1a–4 look
// shifted toward modern radii + a more readable base font size.
// Widgets pass `scale` (typically 0.5) which multiplies fontBasePx;
// fontBasePx=28 gives a comfortable ~14 px effective default.
func defaultUIStyle() uiStyle {
	return uiStyle{
		radiusSmall:  3,
		radiusMedium: 8,
		radiusLarge:  12,
		spacingXS:    4,
		spacingS:     8,
		spacingM:     12,
		spacingL:     16,
		spacingXL:    24,
		fontBasePx:   28,
		focusRingPx:  2,
	}
}

// uiCoreState holds the UI-library state shared by both rendering
// backends. There is one process-global instance, `uiCore`, populated
// lazily as builtins fire.
type uiCoreState struct {
	// Theme palette — written by uiTheme(), read by every widget.
	theme uiPalette

	// Style tokens (radii / spacing / typography) — written by
	// setTheme(), read alongside `theme` by every widget.
	style uiStyle

	// Active proportional font for widget text. nil = use the embedded
	// monospace gomono (desktop) or default Canvas2D font (browser).
	activeFont *Font

	// Focus / hover IDs. Set during widget callbacks and at end-of-frame
	// in uiEnd() via uiCheckHover. Read by every interactive widget to
	// decide its hover/active visual state.
	hoveredID string
	activeID  string

	// Monotonic per-frame counter handed out by every widget for IDing
	// its element registration. Reset to 0 in uiBegin().
	nextID int

	// Element registry — id -> [x, y, w, h]. Populated by uiRegisterElement
	// during the draw pass and consumed by uiCheckHover at end-of-frame to
	// decide which element the cursor is currently over.
	// elementOrder tracks insertion order so uiCheckHover can iterate
	// last-registered-first, giving popup/overlay elements priority over
	// the widgets they visually cover (fixes dropdown-over-tab flicker).
	elements     map[string][4]float32
	elementOrder []string

	// ID of the most recently registered widget — read by tooltip()
	// to attach hover text to the preceding draw.
	lastElementID string

	// List widget state — per-list selected row + scroll offset.
	// Lazy-initialised to non-nil in uiBegin so list widgets can index
	// without a nil-check guard at every call site.
	listSelected map[string]int
	listScroll   map[string]int

	// Layout cursors — set by uiBeginRow / uiBeginCol, advanced by
	// uiRowAdvance / uiColAdvance, read by uiRowX/Y/H + uiColX/Y/W.
	rowCurX float32
	rowY    float32
	rowH    float32
	rowGap  float32
	colX    float32
	colCurY float32
	colW    float32
	colGap  float32

	// Phase 3 — popup / toast state.
	pendingDropdown pendingDropdown
	pendingTooltip  pendingTooltip
	toasts          []toastEntry

	// dropdownOpen is the ID of the currently-open dropdown
	// (one open at a time). "" = no open dropdown.
	dropdownOpen string

	// menuOpenFrame is the frameCount when a contextMenu first became
	// visible. contextMenu uses it to skip the outside-dismiss check
	// on the opening frame (otherwise the click that opened it would
	// also dismiss it).
	menuOpenFrame int

	// Phase 4 — per-widget text editor state. Each map is keyed by
	// the widget's element id. Lazy-initialised when the first
	// textInput/textArea fires.
	textCursor map[string]int     // cursor position (rune index)
	textAnchor map[string]int     // selection anchor (== cursor => no selection)
	textScroll map[string]float32 // horizontal pixel scroll (textInput only)
	textBlink  map[string]float64 // wall-seconds of last cursor movement (for blink)
	undoStacks map[string][]string
	redoStacks map[string][]string

	// Phase 5 — animation registry. Maps "id::key" → current colour.
	// Widget bodies pass the target colour each frame; the registry
	// interpolates toward it using an exponential decay sized to
	// the per-frame delta (uiCore.frameDt) so the visual speed feels
	// the same at 30 / 60 / 120 fps.
	animColors map[string][4]float32

	// Phase 5 — disabled stack. pushDisabled(true) pushes a frame
	// onto the stack; popDisabled pops. Widgets check uiDisabled()
	// — when true they render at half alpha and ignore hover/click.
	disabledStack []bool

	// Phase 5 — per-frame timing. Set at the top of uiBegin via
	// uiTickFrame(); widgets read frameDt for time-based animation.
	prevFrameTime time.Time
	frameDt       float64 // seconds since previous uiBegin

	// Phase 6 — sortable table state. Keyed by table id.
	tableSortCol map[string]int // -1 = no sort, else column index
	tableSortDir map[string]int // -1 desc, +1 asc

	// Tooltip hover timer — the hover-and-pause-for-500ms pattern.
	tooltipHoveredID         string  // which widget is hovered right now
	tooltipHoverStart        float64 // seconds-since-startTime when hover began
	tooltipMatchedThisFrame  bool    // set by tooltip() on a match; checked in uiEnd

	// startTime is set once per process at package init. All
	// time-based UI state (toast expiry, tooltip hover delay) is
	// measured as seconds since this reference. A single source of
	// truth shared between desktop and WASM avoids the gfx.startTime
	// vs gfxState.<noStartTime> divergence.
	startTime time.Time
}

// uiCore is the single global instance. Touched by:
//   - eval/builtins_ui.go            (desktop widgets)
//   - eval/builtins_ui_widgets.go    (shared Phase 1a+ widget bodies)
//   - eval/builtins_graphics.go      (desktop init + render-loop hover check)
//   - eval/builtins_graphics_wasm.go (browser init + frame reset)
//
// No mutex — UI builtins always fire from the eval goroutine inside the
// frame loop on both platforms, so concurrent access can't happen.
//
// The theme is initialised to the default palette HERE (not via an
// init() in a build-tag-gated file) so both desktop and WASM start
// with a visible palette. Without this, WASM widgets would draw with
// the Go zero value [4]float32{0,0,0,0} — fully transparent black —
// and nothing would appear on the canvas.
var uiCore = uiCoreState{
	theme:     defaultUIPalette(),
	style:     defaultUIStyle(),
	startTime: time.Now(),
}
