//go:build !js

package eval

import "github.com/go-gl/glfw/v3.3/glfw"

// uiInput returns the current input + window snapshot from the desktop
// `gfx` global. Naming differences with the WASM equivalent are
// laundered here: gfx.mouseJustClicked → snapshot.mouseClicked,
// gfx.winW/winH → winW/winH.
//
// Clipboard paste: detected when Cmd/Ctrl+V was just pressed; reads
// the clipboard synchronously via GLFW.
func uiInput() uiInputSnapshot {
	cmdOrCtrl := gfx.keys[glfw.KeyLeftSuper] || gfx.keys[glfw.KeyRightSuper] ||
		gfx.keys[glfw.KeyLeftControl] || gfx.keys[glfw.KeyRightControl]
	paste := ""
	if cmdOrCtrl && gfx.justPressed[glfw.KeyV] && gfx.win != nil {
		paste = gfx.win.GetClipboardString()
	}
	return uiInputSnapshot{
		mouseX:            float32(gfx.mouseX),
		mouseY:            float32(gfx.mouseY),
		mouseDown:         gfx.mouseDown,
		mouseClicked:      gfx.mouseJustClicked,
		mouseRightDown:    gfx.mouseRightDown,
		mouseRightClicked: gfx.mouseRightClicked,
		scrollY:           gfx.uiScrollDelta,
		scrollX:           gfx.uiScrollX,
		typedChars:        string(gfx.charBuf),
		shift:             gfx.keys[glfw.KeyLeftShift] || gfx.keys[glfw.KeyRightShift],
		ctrl:              gfx.keys[glfw.KeyLeftControl] || gfx.keys[glfw.KeyRightControl],
		cmd:               gfx.keys[glfw.KeyLeftSuper] || gfx.keys[glfw.KeyRightSuper],
		backspaceCount:    gfx.uiBackspaceCount,
		deleteCount:       gfx.uiDeleteCount,
		leftCount:         gfx.uiLeftCount,
		rightCount:        gfx.uiRightCount,
		upCount:           gfx.uiUpCount,
		downCount:         gfx.uiDownCount,
		enterPressed:      gfx.justPressed[glfw.KeyEnter],
		tabPressed:        gfx.justPressed[glfw.KeyTab],
		homePressed:       gfx.justPressed[glfw.KeyHome],
		endPressed:        gfx.justPressed[glfw.KeyEnd],
		clipPaste:         paste,
		keyA:              gfx.justPressed[glfw.KeyA],
		keyC:              gfx.justPressed[glfw.KeyC],
		keyV:              gfx.justPressed[glfw.KeyV],
		keyX:              gfx.justPressed[glfw.KeyX],
		keyY:              gfx.justPressed[glfw.KeyY],
		keyZ:              gfx.justPressed[glfw.KeyZ],
		winW:              gfx.winW,
		winH:              gfx.winH,
		frameCount:        gfx.frameCount,
	}
}

// uiClipboardWrite copies text to the GLFW window clipboard.
// No-op if the window is not yet open.
func uiClipboardWrite(text string) {
	if gfx.win == nil {
		return
	}
	gfx.win.SetClipboardString(text)
}

// uiPublishSelection is a no-op on desktop. The WASM build needs to mirror the
// live selection into a JS buffer ahead of the keypress (see the WASM
// uiPublishSelection); GLFW's SetClipboardString in uiClipboardWrite is
// synchronous, so the Cmd/Ctrl+C handler can write the clipboard directly with
// no pre-publish required.
func uiPublishSelection(string) {}
