//go:build js && wasm

package eval

import "syscall/js"

// uiInput returns the current input + window snapshot from the WASM
// `gfxState` global. Holds gfxState.mu for the read so the snapshot is
// internally consistent against the JS event-handler goroutines that
// mutate the fields concurrently.
func uiInput() uiInputSnapshot {
	gfxState.mu.Lock()
	defer gfxState.mu.Unlock()
	snap := uiInputSnapshot{
		mouseX:            float32(gfxState.mouseX),
		mouseY:            float32(gfxState.mouseY),
		mouseDown:         gfxState.mouseDown,
		mouseClicked:      gfxState.mouseClicked,
		mouseRightDown:    gfxState.mouseRightDown,
		mouseRightClicked: gfxState.mouseRightClicked,
		scrollY:           gfxState.scrollDeltaY,
		scrollX:           gfxState.scrollDeltaX,
		typedChars:        string(gfxState.charBuf),
		shift:             gfxState.keys["shift"],
		ctrl:              gfxState.keys["control"],
		cmd:               gfxState.keys["meta"], // macOS Cmd / Windows Win
		backspaceCount:    gfxState.backspaceCount,
		deleteCount:       gfxState.deleteCount,
		leftCount:         gfxState.leftCount,
		rightCount:        gfxState.rightCount,
		upCount:           gfxState.upCount,
		downCount:         gfxState.downCount,
		enterPressed:      gfxState.keysPressed["enter"],
		tabPressed:        gfxState.keysPressed["tab"],
		homePressed:       gfxState.keysPressed["home"],
		endPressed:        gfxState.keysPressed["end"],
		clipPaste:         gfxState.pasteBuf,
		keyA:              gfxState.keysPressed["a"],
		keyC:              gfxState.keysPressed["c"],
		keyV:              gfxState.keysPressed["v"],
		keyX:              gfxState.keysPressed["x"],
		keyY:              gfxState.keysPressed["y"],
		keyZ:              gfxState.keysPressed["z"],
		winW:              gfxState.width,
		winH:              gfxState.height,
		frameCount:        gfxState.frameCount,
	}
	// NOTE: do NOT clear gfxState.pasteBuf here. uiInput() is called once
	// per widget per frame, so clearing on the first call would starve every
	// later widget — including the focused textArea, which is drawn after the
	// toolbar buttons. The render loop clears pasteBuf at frame end (alongside
	// keysPressed), so the paste text survives for every widget in the frame
	// and only the focused text widget acts on it. This mirrors keysPressed
	// (keyC/keyV) and the desktop path, which recompute paste each call.
	return snap
}

// uiClipboardWrite writes text to the system clipboard.
// Primary path on WASM: store in __klex_copy_buf so the hidden-textarea
// 'copy'/'cut' event listener (installInputHandlers) can hand it to the
// browser synchronously inside the user gesture — the only path Safari
// allows, because navigator.clipboard.writeText() requires transient
// activation and the widget body runs in a requestAnimationFrame callback,
// outside that gesture.
//
// navigator.clipboard.writeText() is still attempted for browsers that do
// honour it from rAF (Chrome on a focused tab). Its promise is rejected on
// Safari with NotAllowedError; the catch swallows that so it doesn't surface
// as an uncaught-promise console error.
func uiClipboardWrite(text string) {
	js.Global().Set("__klex_copy_buf", text)
	clip := js.Global().Get("navigator").Get("clipboard")
	if !clip.IsUndefined() && !clip.IsNull() {
		clip.Call("writeText", text).Call("then", clipNoop, clipNoop)
	}
}

// clipNoop is a long-lived no-op callback used to swallow the writeText
// promise's resolution/rejection. Kept package-level so uiClipboardWrite
// doesn't allocate (and leak) a fresh js.Func on every copy.
var clipNoop = js.FuncOf(func(this js.Value, _ []js.Value) interface{} { return nil })

// uiPublishSelection mirrors the focused text widget's current selection into
// __klex_copy_buf every frame, ahead of any Cmd/Ctrl+C. Because the browser's
// native 'copy'/'cut' event fires synchronously on the keypress — before the
// widget body runs on the next frame — the selection must already be sitting
// in the buffer when that event reads it. Pass "" when there is no selection.
func uiPublishSelection(sel string) {
	js.Global().Set("__klex_copy_buf", sel)
}
