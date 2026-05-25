// tadPole.lex — Tadpole: multi-provider AI image generator in kLex.
//
// Providers wired:
//   AI Horde       — free; anonymous works; keyed = faster queue
//   Hugging Face   — free tier (requires token); FLUX / SDXL
//   OpenAI DALL-E  — paid; best quality
//   Local SD       — AUTOMATIC1111 / Forge / reForge over /sdapi/v1 — local,
//                    no API key, no bridge process; pure stdlib HTTP via
//                    stdlib/ai/sd.lex
//
// Settings (gear button) → tabbed config modal with per-provider fields, a
// Test button (validates key without burning a generation), and Save which
// writes ~/.tadpole/config.json so keys survive restarts.
//
// Run from the kLex project root:
//   ./klex examples/tadPole/tadPole.lex

import "stdlib/ui_themes.lex"     as themes
import "stdlib/json.lex"          as json
import "stdlib/ai/anthropic.lex"  as claude
import "stdlib/ai/ollama.lex"     as ollama
import "stdlib/ai/sd.lex"         as sd
import "stdlib/ai/ai_common.lex"  as ai
import "stdlib/mcp.lex"           as mcp
import "stdlib/retry.lex"         as retry
import "stdlib/mtl_fx.lex"        as fx


// Retry policy applied to every Claude/Ollama call. The default classifier
// in retry.lex treats *_RATE_LIMIT, *_SERVER, *_TIMEOUT, *_OVERLOADED,
// *_NETWORK, *_CONNECTION as transient; *_AUTH / *_FORBIDDEN / *_NOT_FOUND
// stay fatal — so a bad API key still fails fast.
//
// Tight deadline for interactive UX: 12s total budget, capped per-attempt
// at 4s, so a wedged Claude doesn't freeze the UI thread for 30+ seconds.
let RETRY_OPTS = {
    "maxAttempts": 3,
    "baseDelay":   500,
    "maxDelay":    4000,
    "deadline":    12000,
    "jitter":      true,
}


// ── Platform helpers ──────────────────────────────────────────────────────
// Tiny cross-platform shims used throughout the app. Keep them simple — they
// only need to disambiguate Windows from POSIX (macOS/Linux behave the same).

fn isWindows() {
    return env("OS") == "Windows_NT"
}

// _fsExists is a cheap directory probe — far more reliable than parsing
// `uname` output (which would need a subprocess) and works whether the
// shell exists or not. /System/Library is unique to macOS; /proc to Linux.
fn isMacOS() {
    if isWindows() { return false }
    return _fsExists("/System/Library/CoreServices")
}

fn isLinux() {
    if isWindows() { return false }
    if isMacOS()   { return false }
    return true
}

// osName returns the human-readable platform string used in the system
// prompt (so Claude doesn't tell Linux users to use 'open -a Safari').
fn osName() {
    if isWindows() { return "Windows" }
    if isMacOS()   { return "macOS" }
    return "Linux"
}

fn userHomeDir() {
    let h = env("HOME")
    if len(h) > 0 { return h }
    return env("USERPROFILE")
}

fn tmpDir() {
    let t = env("TMPDIR")
    if len(t) > 0 { return t }
    t = env("TEMP")
    if len(t) > 0 { return t }
    return "/tmp"
}


// ── Fonts ─────────────────────────────────────────────────────────────────

fn tryFont(paths, size) {
    let i = 0
    let n = len(paths)
    while i < n {
        let f, err = safe(loadFont, paths[i], size)
        if err == null { return f }
        i = i + 1
    }
    return null
}

let uiFont = tryFont([
    // macOS
    "/System/Library/Fonts/Supplemental/Tahoma.ttf",
    "/System/Library/Fonts/Supplemental/Verdana.ttf",
    "/System/Library/Fonts/Supplemental/Arial.ttf",
    "/System/Library/Fonts/Supplemental/Arial Bold.ttf",
    "/System/Library/Fonts/SFNS.ttf",
    // Linux — Fedora first, then Debian/Ubuntu
    "/usr/share/fonts/dejavu-sans-fonts/DejaVuSans.ttf",
    "/usr/share/fonts/dejavu-sans-fonts/DejaVuSans-Bold.ttf",
    "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
    "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf",
    "/usr/share/fonts/liberation-sans/LiberationSans-Regular.ttf",
    "/usr/share/fonts/truetype/liberation/LiberationSans-Regular.ttf",
    // Windows
    "C:/Windows/Fonts/segoeui.ttf",
    "C:/Windows/Fonts/arial.ttf",
    "C:/Windows/Fonts/tahoma.ttf",
    "C:/Windows/Fonts/verdana.ttf",
], 24)

// Sibling files (Outfit display font, tadpole logo, Python bridge script)
// are resolved via _scriptDir() so the app launches regardless of CWD —
// either `klex snowball/tadPole/tadPole.lex` from the repo root, or
// `klex tadPole.lex` from the tadPole folder itself.
let displayFont, _ = safe(loadFont, _scriptDir() + "/fonts/Outfit.ttf", 32)
if displayFont == null { displayFont = uiFont }

// Monospace font for fenced code blocks in chat. The embedded bitmap
// `text()` builtin is functional but visibly rougher than the rest of
// the chrome. We try a curated list of programmer-friendly monos
// across platforms and fall back to uiFont (proportional but still TTF
// — looks better than the bitmap) if nothing is found. Skip .ttc files
// — loadFont() doesn't parse them.
let codeFont = tryFont([
    // Bundled with Tadpole — always wins, ships portable across platforms
    _scriptDir() + "/fonts/JetBrainsMono-Regular.ttf",
    // macOS
    "/Library/Fonts/SF-Mono-Regular.otf",
    "/System/Applications/Utilities/Terminal.app/Contents/Resources/Fonts/SFMono-Regular.otf",
    "/System/Library/Fonts/Supplemental/Andale Mono.ttf",
    "/System/Library/Fonts/Supplemental/Courier New.ttf",
    // Linux — Fedora paths first, then Debian/Ubuntu
    "/usr/share/fonts/jetbrains-mono/JetBrainsMono-Regular.ttf",
    "/usr/share/fonts/truetype/jetbrains-mono/JetBrainsMono-Regular.ttf",
    "/usr/share/fonts/cascadia-code/CascadiaCode-Regular.ttf",
    "/usr/share/fonts/dejavu-sans-mono-fonts/DejaVuSansMono.ttf",
    "/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf",
    "/usr/share/fonts/liberation-mono/LiberationMono-Regular.ttf",
    "/usr/share/fonts/truetype/liberation/LiberationMono-Regular.ttf",
    // Windows
    "C:/Windows/Fonts/CascadiaMono.ttf",
    "C:/Windows/Fonts/CascadiaCode.ttf",
    "C:/Windows/Fonts/consola.ttf",
    "C:/Windows/Fonts/cour.ttf",
    "C:/Windows/Fonts/lucon.ttf",
], 24)
if codeFont == null { codeFont = uiFont }

let tadpoleLogo, _ = safe(loadImage, _scriptDir() + "/tadpole_logo.png")

if uiFont == null {
    println("ERROR: no usable font found. Install dejavu-sans-fonts (Fedora) or fonts-dejavu (Debian/Ubuntu) on Linux; macOS and Windows have defaults built in.")
    _osExit(1)
}


// ── Bridge ─────────────────────────────────────────────────────────────────
// Try the canonical Python invocation names in order: python3 (macOS/Linux
// default), python (Linux fallback / older systems), py (Windows launcher
// shipped with the official Python installer). First one that successfully
// spawns the bridge process wins.

fn tryBridge(commands, scriptArgs, opts) {
    let lastErr = null
    let i = 0
    let n = len(commands)
    while i < n {
        let b, err = nativeBridge(commands[i], scriptArgs, opts)
        if err == null { return b, null }
        lastErr = err
        i = i + 1
    }
    return null, lastErr
}

let bridge, berr = tryBridge(
    ["python3", "python", "py"],
    [_scriptDir() + "/ai_image_bridge.py"],
    {
    "timeout_seconds": 360,
        "max_response_mb": 16,
    }
)
if berr != null {
    println("Failed to start bridge — no working Python interpreter found. Last error: " + berr.message)
    _osExit(1)
}

let bInfo = bridgeInfo(bridge)
let hasBinary = false
for cap in bInfo["capabilities"] {
    if cap == "binary" { hasBinary = true }
}
if !hasBinary {
    println("ERROR: bridge did not negotiate the 'binary' capability.")
    bridgeClose(bridge)
    _osExit(1)
}
let backendLabel, _ = bridgeCall(bridge, "backend", [])
let providerIds,   _ = bridgeCall(bridge, "providers", [])
// Local Stable Diffusion is a pure-kLex provider — no bridge involvement.
// We append it to the bridge-provided list so the Settings tabs, Image-tab
// dropdowns and startGeneration() picker all surface it like any other.
providerIds = concat(providerIds, ["a1111"])
let notifCh = bridgeNotifications(bridge)


// ── Theme ──────────────────────────────────────────────────────────────────

let theme        = themes.crimson()
let themeApplied = false


// ── Config persistence ────────────────────────────────────────────────────

let CONFIG_PATH = userHomeDir() + "/.tadpole/config.json"

fn defaultConfig() {
    return {
        "active_provider":     "aihorde",
        "active_chat_backend": "claude",
        "aihorde":             { "api_key": "" },
        "huggingface":         { "api_key": "", "model": "black-forest-labs/FLUX.1-schnell" },
        "openai":              { "api_key": "", "model": "dall-e-3", "quality": "standard" },
        "claude":              { "api_key": "", "model": "claude-haiku-4-5-20251001" },
        "ollama":              { "base_url": "http://localhost:11434", "model": "llama3.2", "think": false, "tools": true },
        "a1111":               { "base_url": "http://127.0.0.1:7860", "model": "", "steps": 25, "cfg_scale": 7.0, "sampler": "Euler" },
    }
}

fn loadConfig() {
    let raw, err = safe(readFile, CONFIG_PATH)
    if err != null { return defaultConfig() }
    let parsed, perr = safe(json.parse, raw)
    if perr != null { return defaultConfig() }
    // Merge over defaults so missing keys fill in.
    let cfg = defaultConfig()
    if hasKey(parsed, "active_provider")     { cfg["active_provider"]     = parsed["active_provider"] }
    if hasKey(parsed, "active_chat_backend") { cfg["active_chat_backend"] = parsed["active_chat_backend"] }
    if hasKey(parsed, "aihorde")     { cfg["aihorde"]     = parsed["aihorde"] }
    if hasKey(parsed, "huggingface") { cfg["huggingface"] = parsed["huggingface"] }
    if hasKey(parsed, "openai")      { cfg["openai"]      = parsed["openai"] }
    if hasKey(parsed, "claude")      { cfg["claude"]      = parsed["claude"] }
    if hasKey(parsed, "ollama")      { cfg["ollama"]      = parsed["ollama"] }
    if hasKey(parsed, "a1111")       { cfg["a1111"]       = parsed["a1111"] }
    return cfg
}

fn saveConfigToDisk(cfg) {
    let dir = userHomeDir() + "/.tadpole"
    _, _ = safe(_fsMkdirAll, dir)
    let body, _ = safe(json.stringify, cfg)
    let _, werr = safe(writeFile, CONFIG_PATH, body)
    return werr
}


// ── App state ──────────────────────────────────────────────────────────────

let config             = loadConfig()
let activeProvider     = config["active_provider"]
let activeChatBackend  = config["active_chat_backend"]

let prompt        = "a tadpole wearing sunglasses, vector art, neon background"
// Canonical SD 1.5 negative prompt. Cleared once-per-user via the UI;
// the default is a hard-won baseline that fixes most "bad hands, extra
// limbs, watermark" failure modes without being too restrictive.
let negPrompt     = "ugly, blurry, low quality, deformed, extra fingers, watermark, text"
// Style suffix appended to the user's prompt at generation time only —
// never mutates the typed prompt, so toggling styles doesn't churn text.
// "(none)" means no suffix; anything else is appended after ", ".
let stylePreset   = "(none)"
let imgW          = 768
let imgH          = 768
// Aspect-ratio lock: when on, dragging either dimension slider auto-updates
// the other to preserve lockRatio (captured at the moment the lock was set).
let lockAspect    = false
let lockRatio     = 1.0
let generating    = false
let resultCh      = null
let currentImage  = null
let lastBytes     = 0
let lastElapsed   = 0
let status        = "ready"
let errorMessage  = ""

// Vision state — we keep the raw generated bytes and detected MIME so
// the user can attach the last image to a Claude turn without re-fetching.
let lastImageBytes = null
let lastImageMime  = "image/png"
let lastImageW     = 0      // pixel dimensions of lastImageBytes — populated
let lastImageH     = 0      // alongside the bytes; used by the Adjust panel.
let attachImage    = false

// Adjust panel state (right-panel "Adjust" tab) — slider values mapped to
// stdlib/mtl_fx.lex calls. Defaults produce a no-op chain. The panel
// caches a filtered preview Image; adjPreviewFingerprint is the encoded
// slider state from when we last rebuilt the preview, so we re-run the
// filter chain only when sliders actually move (not every frame).
let adjExposure    = 0.0    // -3..+3   stops
let adjBrightness  = 0.0    // -1..+1
let adjContrast    = 0.0    // -1..+1
let adjSaturation  = 0.0    // -1..+1
let adjHue         = 0.0    // -180..+180 degrees
let adjVignette    = 0.0    // 0..1 strength
let adjSepia       = 0.0    // 0..1
let adjGamma       = 1.0    // 0.4..2.5 (1.0 = no change)
let adjInvert      = false
let adjDesaturate  = false
let adjShowMore    = false  // expander toggle for the "More…" filters
let adjPreviewImage       = null
let adjPreviewFingerprint = ""
// adjSourceRgba is the raw RGBA byte buffer that the filter chain
// consumes. It's decoded once at generation time from the SD-returned
// PNG bytes (which live in lastImageBytes, unmodified, so Attach/Save
// keep working). After Apply, the adjusted raw bytes become the new
// adjSourceRgba so further sliders stack on top.
let adjSourceRgba = null

// img2img state — when useImg2Img is on AND initImageBytes is non-null
// AND the active provider is Local SD, startGeneration routes to
// sd.img2img() with the bytes as the init image. Otherwise the standard
// txt2img path runs and these are ignored.
//   initImageBytes   — raw .png/.jpg bytes loaded from the dropped file
//                      (or copied from lastImageBytes via "Use last gen")
//   initImagePreview — loadImage handle for rendering the source thumbnail
//                      inside the drop zone
//   initImageLabel   — short human label ("dropped: cat.jpg" / "last gen")
//   useImg2Img       — checkbox toggle that shows/hides the drop zone
//   denoiseStrength  — slider value passed as denoising_strength
//                      (0.2 subtle · 0.5 mid · 0.8 mostly ignore source)
let initImageBytes   = null
let initImagePreview = null
let initImageLabel   = ""
let useImg2Img       = false
let denoiseStrength  = 0.5

// Copy-feedback state — when a Copy button is clicked, remember which
// bubble and when so we can flip the label to "✓ Copied" for ~2s.
let copyFlashIdx = -1
let copyFlashAt  = -1.0

// Save controls
let saveBasename = "tadpole"
let lastSavedTo  = ""
let saveError    = ""

// Load-from-disk controls. The Load row lives under Generate on the
// main page so users can bring in any RGBA image to adjust without
// generating one first. Drag-drop into the window also routes here
// when img2img is off (see pollDroppedFiles).
let loadPathInput = ""
let loadError     = ""

// Settings modal
let settingsOpen = false
let settingsTab  = activeProvider           // which provider's fields are visible
let testStatus   = ""                        // last test_key result text
let testError    = false

// Prompt enhancement (Claude)
let enhancing  = false
let enhanceCh  = null
let enhanceErr = ""

// Right-panel tabs (0 = Image, 1 = Chat)
let rightTab     = 0
let chatMessages = makeArray(0)             // display list — {role, content} or {role:"tool", ...}
let chatDraft    = ""
let chatSending  = false
let chatCh       = null
let chatScroll   = 0

// After Send (Enter or button click) we want focus to land back on the
// chat input next frame so the user can keep typing without clicking.
// The widget's positional ID can shift between frames (the Cancel button
// appears once chatSending=true), so we set this flag here and consume it
// inside drawChatTab against the live frame's chatInputId.
let chatRefocusPending = false

// Last Ollama model whose capabilities we probed. When the user switches
// models we re-check on the next turn and surface a one-shot warning if
// the new model doesn't advertise tool support — preventing the silent
// "agent isn't firing tools" footgun that older / smaller Ollama models
// cause when they ignore tool definitions and answer in prose.
let _ollamaCapsCheckedFor = ""

// chatGen invalidates in-flight async results. Each cancel/clear bumps it
// so any pending bridgeCall that completes after the user moved on can
// be silently dropped by pollChat.
let chatGen = 0

// Running token + cost totals for this session. Each Claude response brings
// usage info that we accumulate; cost is computed from per-model rates
// (per 1M tokens, approximate Anthropic public pricing for the 4.x family).
let sessionInTokens  = 0
let sessionOutTokens = 0

fn _modelRates(modelId) {
    // Returns (in_per_million, out_per_million) in USD.
    if indexOf(modelId, "opus") >= 0   { return [15.0, 75.0] }
    if indexOf(modelId, "sonnet") >= 0 { return [3.0, 15.0] }
    // Default: haiku-class.
    return [1.0, 5.0]
}
fn _estimateCost(modelId, inTok, outTok) {
    let rates = _modelRates(modelId)
    return float(inTok) * rates[0] / 1000000.0 + float(outTok) * rates[1] / 1000000.0
}

// Agent (tool-use) state
//
// agentMessages is the API-side messages array — distinct from chatMessages
// (display) because tool_use / tool_result content blocks live in it. Each
// turn appends 1+ entries. chatMessages mirrors what the user sees.
let agentMessages       = makeArray(0)
let agentStep           = 0
let AGENT_MAX_STEPS     = 12
let agentPendingTool    = null      // {id, name, input, assistant_content} awaiting approval (head of queue)
// Claude can emit multiple tool_use blocks in one assistant turn (parallel
// tool calls). All of them must be resolved before the next API request,
// and for Claude every tool_result block must share a single follow-up
// user message — otherwise Anthropic rejects with HTTP 400 ("tool_use ids
// were found without tool_result blocks immediately after"). The queue
// holds the remaining tool_uses; the accumulator holds the tool_result
// blocks that get flushed as one user message when the queue drains.
let agentPendingQueue   = makeArray(0)
let agentResultsAccum   = makeArray(0)
let agentToolLog        = userHomeDir() + "/.tadpole/tool-log.jsonl"

// Chat render cache. The chat panel used to re-parse markdown, re-tokenise
// every paragraph and re-measure every word on EVERY frame for EVERY
// message — at 60 FPS with 20 messages that's tens of thousands of
// textWidth() calls per frame, the dominant jitter source. We now cache
// the compiled segments (with their tokens + line counts + height) per
// message index, keyed by content length + render width. Streaming
// messages grow → cache miss → recompile (just that one entry); stable
// messages hit and skip all the work. Wiped on clearChat / backend swap
// / width change.
let chatRenderCache     = makeArray(0)   // per-message: {len, segs, height} or null
let chatRenderCacheWidth = -1            // innerW the cache was built for; -1 = empty

// Streaming state — tracks the in-progress assistant turn as Claude
// emits tokens. Reset at the start of every _startNextAgentStep().
let streamCurrentMsgIdx  = -1               // chatMessages index of the live bubble (-1 = none)
let streamCurrentContent = makeArray(0)     // content blocks being assembled for agentMessages
let streamCurrentBlock   = null             // currently-open content block (text or tool_use)
let streamLastStopReason = ""
let streamLastUsage      = null

// Tool definitions — what we advertise to Claude. Each tool maps to a kLex
// builtin in executeTool() below. Read-only tools auto-execute; mutating
// tools (write_file, shell) require Approve.
let agentTools = [
    {
        "name":        "read_file",
        "description": "Read the contents of a file from disk and return it as a string. Use for inspecting files the user has referenced or that you need to understand before doing something else.",
        "input_schema": {
            "type": "object",
            "properties": {
                "path": {"type": "string", "description": "Path to the file. Absolute paths or paths relative to the user's home directory."},
            },
            "required": ["path"],
        },
    },
    {
        "name":        "list_dir",
        "description": "List the entries in a directory. Returns one entry name per line. Use to discover what's in a folder before reading files.",
        "input_schema": {
            "type": "object",
            "properties": {
                "path": {"type": "string"},
            },
            "required": ["path"],
        },
    },
    {
        "name":        "http_get",
        "description": "Fetch a URL via HTTP GET and return the response body as text. For JSON APIs, return the raw text and parse it yourself.",
        "input_schema": {
            "type": "object",
            "properties": {
                "url": {"type": "string"},
            },
            "required": ["url"],
        },
    },
    {
        "name":        "write_file",
        "description": "Write content to a file, creating or overwriting it. The user must approve each call. Use sparingly and only when explicitly asked to create or modify a file.",
        "input_schema": {
            "type": "object",
            "properties": {
                "path":    {"type": "string"},
                "content": {"type": "string"},
            },
            "required": ["path", "content"],
        },
    },
    {
        "name":        "shell",
        "description": "Run a shell command and return its stdout. The user must approve each call. Use for tasks that need standard CLI tools (ls, grep, git, etc.). Do NOT use for things you can do with the other tools — read_file is safer than `cat`, and prefer the launch tool over `open -a`.",
        "input_schema": {
            "type": "object",
            "properties": {
                "command": {"type": "string"},
            },
            "required": ["command"],
        },
    },
    {
        "name":        "launch",
        "description": "Open a file/folder in its default OS handler, or open a URL in the default browser. Requires the user's approval. Provide at least one of: path, url. The `app` argument is honoured on macOS only (combine with path to open a file in a specific application, e.g. app='Visual Studio Code', path='~/notes.md') — ignored on Linux/Windows where the OS handler is fixed by file association. Uses `open` on macOS, `xdg-open` on Linux, `cmd /c start` on Windows. No shell, so spaces in names are fine.",
        "input_schema": {
            "type": "object",
            "properties": {
                "app":  {"type": "string", "description": "Application name, e.g. 'Safari', 'Visual Studio Code', 'Finder'."},
                "path": {"type": "string", "description": "File or folder path. ~ is expanded. Combine with app to open with a specific application."},
                "url":  {"type": "string", "description": "URL like 'https://example.com'. Opens in the default browser."},
            },
        },
    },
]

// Tools that don't require explicit user approval (read-only side effects).
fn isReadOnlyTool(name) {
    if name == "read_file" || name == "list_dir" || name == "http_get" { return true }
    // Every MCP-sourced tool is treated as read-only for approval purposes.
    // frogMcp's surface is search/describe/lookup — none of it mutates the
    // user's machine. If we ever wire an MCP server with mutating tools
    // (filesystem-mcp, git-mcp), revisit this gate.
    if hasKey(mcpToolNames, name) { return true }
    return false
}


// ── img2img helpers ──────────────────────────────────────────────────────

// _isImageExt is a cheap case-insensitive extension check. We only accept
// PNG/JPEG into the drop zone — formats SD's VAE encodes without issue.
fn _isImageExt(path) {
    let n = len(path)
    if n < 4 { return false }
    // lowercase the tail in-place by comparing both cases. ord() costs a
    // builtin call per char; the alternative is a substr + toLower call
    // chain that's no clearer.
    let last4 = substr(path, n - 4, n)
    if last4 == ".png" || last4 == ".PNG" || last4 == ".jpg" || last4 == ".JPG" { return true }
    if n >= 5 {
        let last5 = substr(path, n - 5, n)
        if last5 == ".jpeg" || last5 == ".JPEG" { return true }
    }
    return false
}

// _basename pulls the last segment of a path so the drop-zone label
// reads "dropped: photo.jpg" not "dropped: /Users/karl/Documents/photo.jpg".
// Handles both POSIX (/) and Windows (\) separators.
fn _basename(path) {
    let n = len(path)
    let sep = -1
    let i = n - 1
    while i >= 0 {
        let c = path[i]
        if c == "/" || c == "\\" { sep = i  i = -1   continue }
        i = i - 1
    }
    if sep < 0 { return path }
    return substr(path, sep + 1, n)
}

// loadInitImageFromPath reads the file at path and populates the three
// init-image globals: bytes (for the API), preview (for the thumbnail
// render), and label (for the UI). Returns true on success, false on
// any read/decode failure so the caller can show a toast/error.
fn loadInitImageFromPath(path) {
    if !_isImageExt(path) { return false }
    let content, rerr = _fsRead(path)
    if rerr != null { return false }
    let raw = strToBytes(content)
    // loadImage accepts bytes since the saveImage round-trip works that
    // way too — caught by safe() in case the file is corrupt.
    let img, derr = safe(loadImage, raw)
    if derr != null { return false }
    initImageBytes   = raw
    initImagePreview = img
    initImageLabel   = "dropped: " + _basename(path)
    return true
}

// loadMainImageFromPath reads the file at `path` and adopts it as the
// current image — same state mutation a successful SD generation
// performs (currentImage, lastImageBytes, dims, adjSourceRgba) plus
// reset of the adjust panel. Returns true on success, false on any
// read/decode failure; on failure, loadError is set so the UI can
// surface the reason.
fn loadMainImageFromPath(path) {
    if !_isImageExt(path) {
        loadError = "Unsupported file type — use .png / .jpg / .jpeg / .webp"
        return false
    }
    let content, rerr = _fsRead(path)
    if rerr != null {
        loadError = "Could not read " + _basename(path)
        return false
    }
    let raw = strToBytes(content)
    let img, derr = safe(loadImage, raw)
    if derr != null {
        loadError = "Could not decode " + _basename(path) + " — file may be corrupt"
        return false
    }
    currentImage   = img
    lastImageBytes = raw
    lastImageMime  = _detectImageMime(raw)
    lastBytes      = len(raw)
    let iw, ih         = imageSize(img)
    lastImageW     = iw
    lastImageH     = ih
    adjSourceRgba  = imageToRgba(img)
    resetAdjust()
    errorMessage   = ""
    loadError      = ""
    loadPathInput  = ""
    status         = "loaded: " + _basename(path)
    return true
}

// pollDroppedFiles drains droppedFiles() every frame and routes the
// first image either to the img2img init slot (when useImg2Img is on)
// or to the main image (otherwise). Multiple drops in one frame are
// common (Mac drag-multiple) — only the first image wins.
fn pollDroppedFiles() {
    let files = droppedFiles()
    if len(files) == 0 { return }
    let handler = loadMainImageFromPath
    if useImg2Img { handler = loadInitImageFromPath }
    let i = 0
    while i < len(files) {
        if handler(files[i]) { return }
        i = i + 1
    }
}

// useLastGenAsInit copies the current generated image into the init
// slot so the user can iterate on Tadpole's own output. Cheap — same
// bytes, just a second reference. The preview reuses lastImage's
// pixels via a fresh loadImage so the thumbnail draws independently
// of the main image rendering.
fn useLastGenAsInit() {
    if lastImageBytes == null { return false }
    let img, derr = safe(loadImage, lastImageBytes)
    if derr != null { return false }
    initImageBytes   = lastImageBytes
    initImagePreview = img
    initImageLabel   = "last gen"
    return true
}

fn clearInitImage() {
    initImageBytes   = null
    initImagePreview = null
    initImageLabel   = ""
}


// ── MCP integration ──────────────────────────────────────────────────────
// Tadpole spawns frogMcp at startup (best-effort — silently degrades if
// Python or the server's deps are missing) and registers its tools with
// Claude alongside the built-in agentTools. Lets the assistant answer
// kLex-language questions with authoritative data from the source index
// instead of hallucinating builtin signatures.
//
// mcpClient    — MCP_CLIENT object once the handshake completes, else null
// mcpToolNames — hash {tool_name: true} so executeTool can route calls
// mcpStatus    — "starting" | "ready" | "disabled" | "failed" — surfaced
//                in the status bar so the user sees the integration state
// mcpInitCh    — async-init result channel; drained by pollMcpInit each
//                frame until the async spawn completes
let mcpClient    = null
let mcpToolNames = {}
let mcpStatus    = "starting"
let mcpInitCh    = null


// ── Text helpers ──────────────────────────────────────────────────────────

fn fillC(rgba) { fill(rgba[0], rgba[1], rgba[2], rgba[3]) }
fn strokeC(rgba) { stroke(rgba[0], rgba[1], rgba[2], rgba[3]) }
fn say(s, x, y, scale)        { textFont(uiFont, s, x, y, scale) }
fn sayDisplay(s, x, y, scale) { textFont(displayFont, s, x, y, scale) }
fn sayCode(s, x, y, scale)    { textFont(codeFont, s, x, y, scale) }

// _shadow paints a soft drop shadow underneath the next rect we draw.
// Tiny offset, low-alpha black. Cheap and gives the dark UI a sense of
// depth without being showy.
fn _shadow(x, y, w, h, r) {
    fill(0.0, 0.0, 0.0, 0.40)
    noStroke()
    roundedRect(x + 2, y + 4, w, h, r)
}

// _topHighlight draws a single ~1px lighter line just inside the top edge
// of a panel — the classic "glass" trick that hints at a light source from
// above without screaming about it.
fn _topHighlight(x, y, w) {
    fill(1.0, 1.0, 1.0, 0.06)
    noStroke()
    rect(x + 3, y + 1, w - 6, 1)
}

fn drawPanel(x, y, w, h) {
    _shadow(x, y, w, h, 8)
    fillC(theme["panelBg"])
    strokeC(theme["panelBorder"])
    strokeWeight(1)
    roundedRect(x, y, w, h, 8)
    _topHighlight(x, y, w)
}
// drawSpinner is a Tadpole-local copy of stdlib/ui.lex's spinner() that
// uses theme["accentBar"] so it matches whichever theme the user has
// active. We can't import stdlib/ui.lex directly — its widget exports
// (button, slider, etc.) would collide with Tadpole's own draw calls.
//
// 12 dots travel around a circle of radius r centred on (x, y); each
// fades from 10% to 100% alpha to create the chase. elapsedTime drives
// rotation independent of frame rate.
fn drawSpinner(x, y, r) {
    let segments = 12
    let t = elapsedTime() * 5.0
    let accent = theme["accentBar"]
    for i in range(0, segments) {
        let angle = float(i) / float(segments) * 6.2832 - t
        let alpha = float(i) / float(segments) * 0.90 + 0.10
        fill(accent[0], accent[1], accent[2], alpha)
        noStroke()
        let sx = x + cos(angle) * r
        let sy = y + sin(angle) * r
        circle(sx, sy, r * 0.20)
    }
}

fn drawSep(x, y, w) {
    strokeC(theme["sectionLine"])
    strokeWeight(1)
    line(x, y, x + w, y)
    noStroke()
}

fn drawWrapped(s, x, y, cols, scale) {
    let words = split(s, " ")
    let wi = 0
    let wn = len(words)
    let cy = y
    while wi < wn {
        let line = words[wi]
        wi = wi + 1
        while wi < wn && len(line) + 1 + len(words[wi]) <= cols {
            line = line + " " + words[wi]
            wi = wi + 1
        }
        say(line, x, cy, scale)
        cy = cy + 15
    }
    return cy
}

// copyToClipboard writes text to a temp file then pipes it through the
// platform's clipboard utility. Avoids shell-quoting risk and works for
// multi-line markdown content. Runs the subprocess via async() so xclip's
// blocking clipboard handshake (notorious on Wayland) never freezes the UI
// thread. Chain order: wl-copy (Wayland, clean exit) → pbcopy (macOS) →
// xsel (X11, clean exit) → xclip (X11, last resort) → clip.exe (Windows).
fn copyToClipboard(text) {
    let tmp = tmpDir() + "/.tadpole_clip"
    let _, werr = safe(writeFile, tmp, text)
    if werr != null { return false }
    if isWindows() {
        async(fn() {
            _, _, _, _ = _processExec("cmd.exe", ["/c", "type " + tmp + " | clip"])
        })
    } else {
        async(fn() {
            _, _, _, _ = _processExec("sh", ["-c",
                "wl-copy < " + tmp + " 2>/dev/null || " +
                "pbcopy < " + tmp + " 2>/dev/null || " +
                "xsel --clipboard --input < " + tmp + " 2>/dev/null || " +
                "xclip -selection clipboard -in < " + tmp + " 2>/dev/null"])
        })
    }
    return true
}

fn providerLabel(id) {
    if id == "aihorde"     { return "AI Horde" }
    if id == "huggingface" { return "Hugging Face" }
    if id == "openai"      { return "OpenAI" }
    if id == "claude"      { return "Claude" }
    if id == "ollama"      { return "Ollama" }
    if id == "a1111"       { return "Local SD" }
    return id
}

// Tabs in the settings modal — image providers plus Claude (assistant). The
// active IMAGE provider tracks the user's last image-provider tab selection;
// the Claude tab is purely a config panel and doesn't participate in image
// provider selection.
fn settingsTabIds() {
    return concat(providerIds, ["claude", "ollama"])
}

// chatBackendIds — the subset of settings tabs that represent chat backends
// (as opposed to image-provider configuration). The "active chat backend"
// can only be one of these; image-provider tabs are config-only for chat.
fn chatBackendIds() {
    return ["claude", "ollama"]
}

// isChatBackend — true if the given tab id is one of the chat backends.
fn isChatBackend(id) {
    for cb in chatBackendIds() {
        if cb == id { return true }
    }
    return false
}
fn isImageProvider(id) {
    let i = 0
    let n = len(providerIds)
    while i < n {
        if providerIds[i] == id { return true }
        i = i + 1
    }
    return false
}


// ── Generation flow ────────────────────────────────────────────────────────

fn startGeneration() {
    generating   = true
    status       = "submitting…"
    errorMessage = ""
    resultCh     = channel(1)
    // Clear the previously displayed image so the empty-state branch
    // takes over and shows the chase spinner. Without this, the panel
    // keeps rendering the last image and the spinner is hidden — the
    // user can't see that a new generation is in progress. We keep
    // lastImageBytes (used by "Attach image" + "Use last gen as init")
    // so that workflow continues to work — only the on-screen render
    // is cleared.
    currentImage = null
    // Build the effective prompt by appending the selected style preset,
    // without mutating what the user typed. Empty/none preset → no change.
    let effPrompt = prompt
    if stylePreset != "(none)" && len(stylePreset) > 0 {
        effPrompt = prompt + ", " + stylePreset
    }
    let promptCopy   = effPrompt
    let negCopy      = negPrompt
    let wCopy        = imgW
    let hCopy        = imgH
    let ch           = resultCh
    let b            = bridge
    let prov         = activeProvider
    let opts         = config[prov]
    // img2img branch flags — snapshot here so the async closure captures
    // a stable view. Only relevant when activeProvider == "a1111".
    let img2imgOn    = useImg2Img
    let initBytes    = initImageBytes
    let denoise      = denoiseStrength
    async(fn() {
        if prov == "a1111" {
            // Pure-kLex path: talks straight to AUTOMATIC1111's /sdapi/v1 via
            // stdlib/ai/sd.lex. No Python bridge involved. The daemon's
            // currently-loaded checkpoint is used unless the user pinned a
            // specific one in Settings → Local SD → Model.
            let sdc = sd.newClient(opts["base_url"])
            if hasKey(opts, "model") && type(opts["model"]) == "STRING" && len(opts["model"]) > 0 {
                _, _ = sd.setModel(sdc, opts["model"])
            }
            let stepsV = 25
            let cfgV   = 7.0
            let sampV  = "Euler"
            if hasKey(opts, "steps")     { stepsV = opts["steps"] }
            if hasKey(opts, "cfg_scale") { cfgV   = opts["cfg_scale"] }
            if hasKey(opts, "sampler")   { sampV  = opts["sampler"] }
            let sdOpts = {
                "prompt":          promptCopy,
                "negative_prompt": negCopy,
                "width":           wCopy,
                "height":          hCopy,
                "steps":           stepsV,
                "cfg_scale":       cfgV,
                "sampler_name":    sampV,
                "seed":            -1,
                "timeout_sec":     600,
            }
            // img2img branch — adds init_image + denoising_strength and
            // routes to sd.img2img(). Same opts otherwise; the daemon
            // resizes the init to match width/height.
            if img2imgOn && initBytes != null {
                sdOpts["init_image"]         = initBytes
                sdOpts["denoising_strength"] = denoise
                let bytes, callErr = sd.img2img(sdc, sdOpts)
            } else {
                let bytes, callErr = sd.generate(sdc, sdOpts)
            }
            send(ch, {"bytes": bytes, "err": callErr})
        } else {
            // Pass the negative prompt through opts so other providers (Horde,
            // HF, OpenAI) can use it where supported. The Python bridge reads
            // opts["negative_prompt"] when present.
            let optsCopy = opts
            optsCopy["negative_prompt"] = negCopy
            let bytes, callErr = bridgeCall(b, "generate", [prov, promptCopy, wCopy, hCopy, optsCopy])
            send(ch, {"bytes": bytes, "err": callErr})
        }
    })
}

// _detectImageMime sniffs the magic bytes of an image to return the right
// MIME type for Anthropic's vision API. Falls back to image/png if the
// bytes don't match a known format — Claude is tolerant of mislabeling.
fn _detectImageMime(b) {
    if b == null || type(b) != "BYTES" || len(b) < 4 { return "image/png" }
    // PNG: 89 50 4E 47
    if b[0] == 137 && b[1] == 80 && b[2] == 78 && b[3] == 71 { return "image/png" }
    // JPEG: FF D8 FF
    if b[0] == 255 && b[1] == 216 && b[2] == 255 { return "image/jpeg" }
    // GIF: "GIF8"
    if b[0] == 71 && b[1] == 73 && b[2] == 70 && b[3] == 56 { return "image/gif" }
    // WebP: "RIFF....WEBP"
    if len(b) >= 12 &&
       b[0] == 82 && b[1] == 73 && b[2] == 70 && b[3] == 70 &&
       b[8] == 87 && b[9] == 69 && b[10] == 66 && b[11] == 80 { return "image/webp" }
    return "image/png"
}

fn pollGeneration() {
    if !generating { return }
    let msg = recvNonBlock(resultCh)
    if msg == null { return }
    generating = false
    if msg["err"] != null {
        status = "error"
        errorMessage = msg["err"].message
        return
    }
    let raw = msg["bytes"]
    let img, decodeErr = safe(loadImage, raw)
    if decodeErr != null {
        status = "decode failed"
        errorMessage = decodeErr.message
        return
    }
    currentImage   = img
    lastImageBytes = raw
    lastImageMime  = _detectImageMime(raw)
    lastBytes      = len(raw)
    let iw, ih         = imageSize(img)
    lastImageW     = iw
    lastImageH     = ih
    // Extract the decoded raw RGBA pixels NOW, while img.pixels is
    // still populated by loadImage. After the first drawImage uploads
    // to a GPU texture, imageToRgba would have to glGetTexImage the
    // pixels back — which still works but is more expensive. Grabbing
    // them here keeps the cost to one defensive copy.
    adjSourceRgba  = imageToRgba(img)
    resetAdjust()
    status         = "done"
}

fn pollNotifications() {
    let msg = recvNonBlock(notifCh)
    if msg == null { return }
    if !hasKey(msg, "phase") { return }
    let phase = msg["phase"]
    if phase == "submit"     { status = "submitting…" }
    if phase == "requesting" { status = "requesting…" }
    if phase == "queued"     { status = "queued" }
    if phase == "polling" {
        let pos     = msg["pos"]
        let wait    = msg["wait"]
        let elapsed = msg["elapsed"]
        if pos > 0 {
            status = "queue #" + str(pos) + " · " + str(wait) + "s wait (" + str(elapsed) + "s)"
        } else {
            status = "rendering… (" + str(elapsed) + "s)"
        }
    }
    if phase == "done" { lastElapsed = msg["elapsed_ms"] }
}

fn doSave(ext) {
    if currentImage == null { return }
    let name = trim(saveBasename)
    if len(name) == 0 { name = "tadpole" }
    // No auto-increment: save to the exact name the user typed and
    // overwrite if it exists. Karl wants WYSIWYG file naming — keeping a
    // counter got in the way of replacing a single iteration target.
    let fullPath = "./" + name + "." + ext
    let _, serr = safe(saveImage, currentImage, fullPath)
    if serr != null {
        saveError    = serr.message
        lastSavedTo  = ""
    } else {
        lastSavedTo = fullPath
        saveError   = ""
    }
}

// ── Claude API wiring (via stdlib/ai/anthropic.lex) ──────────────────────
// Tadpole's Claude integration runs over the native anthropic.lex library:
// the key test, prompt enhancement, and per-step agent calls are all pure
// kLex HTTP — no Python bridge round-trip. The bridge handles only image
// generation now.

// _claudeClient builds a Claude client from the current config["claude"]
// settings. Returns null if no API key is configured — callers must check.
fn _claudeClient() {
    let cfg = config["claude"]
    if !hasKey(cfg, "api_key") { return null }
    let apiKey = cfg["api_key"]
    if apiKey == null || len(apiKey) == 0 { return null }
    let chosenModel = null
    if hasKey(cfg, "model") { chosenModel = cfg["model"] }
    return claude.newClient(apiKey, chosenModel)
}

// _reshapeClaudeResponse turns an anthropic.lex messages() response into
// the kind-based shape Tadpole's pollChat already understands — mirrors
// what the old Python bridge claude_step returned so the polling code
// doesn't have to change.
fn _reshapeClaudeResponse(resp) {
    let usage = {"input_tokens": 0, "output_tokens": 0}
    if hasKey(resp, "usage") {
        let u = resp["usage"]
        if hasKey(u, "input_tokens")  { usage["input_tokens"]  = u["input_tokens"] }
        if hasKey(u, "output_tokens") { usage["output_tokens"] = u["output_tokens"] }
    }
    if claude.stopReasonOf(resp) == "tool_use" {
        let blocks = resp["content"]
        for blk in blocks {
            if type(blk) == "HASH" && hasKey(blk, "type") && blk["type"] == "tool_use" {
                return {
                    "kind":              "tool_use",
                    "id":                blk["id"],
                    "name":              blk["name"],
                    "input":             blk["input"],
                    "assistant_content": blocks,
                    "usage":             usage,
                }
            }
        }
    }
    return {
        "kind":    "text",
        "content": claude.textOf(resp),
        "usage":   usage,
    }
}

fn testActiveProviderKey() {
    let opts = config[settingsTab]
    if settingsTab == "claude" {
        // Validate the key with a minimal native call — a 1-token request
        // surfaces 401s as ANTHROPIC_AUTH without burning real generation.
        let c = _claudeClient()
        if c == null {
            testStatus = "No API key configured"
            testError  = true
            return
        }
        let _, err = retry.doWith(fn() {
            return claude.messages(c, {
                "max_tokens": 1,
                "messages":   [ai.userMsg("hi")],
            })
        }, RETRY_OPTS)
        if err != null {
            testStatus = err.message
            testError  = true
        } else {
            testStatus = "API key valid"
            testError  = false
        }
        return
    }
    if settingsTab == "ollama" {
        // Probe reachability + model availability. listModels is cheap and
        // tells us both: daemon up AND lists what's pulled.
        let cfg = config["ollama"]
        let oc = ollama.newClient(cfg["base_url"], cfg["model"])
        let models, lerr = ollama.listModels(oc)
        if lerr != null {
            testStatus = lerr.message
            testError  = true
            return
        }
        // Verify the configured model is installed.
        let found = false
        for m in models {
            if m["name"] == cfg["model"] || m["model"] == cfg["model"] { found = true }
        }
        if !found {
            testStatus = "Daemon reachable, but model `" + cfg["model"] + "` is not installed. Run `ollama pull " + cfg["model"] + "`."
            testError  = true
            return
        }
        testStatus = "Daemon reachable; model `" + cfg["model"] + "` ready"
        testError  = false
        return
    }
    if settingsTab == "a1111" {
        // Probe the A1111 daemon over /sdapi/v1/sd-models — a cheap call that
        // both verifies reachability and tells us what checkpoints the user
        // has installed. If they pinned a specific model in settings, we
        // confirm it's one of the installed titles.
        let sdcfg = config["a1111"]
        let sdc = sd.newClient(sdcfg["base_url"])
        let models, lerr = sd.models(sdc)
        if lerr != null {
            testStatus = lerr.message
            testError  = true
            return
        }
        let n = len(models)
        if n == 0 {
            testStatus = "Daemon reachable, but no models are installed. Drop a .safetensors file into stable-diffusion-webui/models/Stable-diffusion/ and restart."
            testError  = true
            return
        }
        if hasKey(sdcfg, "model") && type(sdcfg["model"]) == "STRING" && len(sdcfg["model"]) > 0 {
            let found = false
            for m in models {
                if hasKey(m, "title") && m["title"] == sdcfg["model"] { found = true }
            }
            if !found {
                testStatus = "Daemon reachable (" + str(n) + " models installed), but `" + sdcfg["model"] + "` is not one of them. Model title uses the daemon's exact format — copy from the WebUI's checkpoint dropdown."
                testError  = true
                return
            }
        }
        testStatus = "Daemon reachable; " + str(n) + " model(s) installed"
        testError  = false
        return
    }
    let res, terr = bridgeCall(bridge, "test_key", [settingsTab, opts])
    if terr != null { testStatus = terr.message  testError = true }
    else            { testStatus = res            testError = false }
}

// ── Prompt enhancement (Claude) ──────────────────────────────────────────

fn startEnhance() {
    if enhancing { return }
    if len(trim(prompt)) == 0 { return }
    let cClient = _claudeClient()
    if cClient == null {
        enhanceErr = "No Claude API key configured (Settings → Claude)"
        return
    }
    enhancing  = true
    enhanceErr = ""
    enhanceCh  = channel(1)
    let ch         = enhanceCh
    let rawPrompt  = prompt
    async(fn() {
        // Diagnostic wrap (2026-05-23): the async builtin's goroutine
        // wrapper stores any runtime error from the closure body in
        // task.result, but Karl's code discards the Task — so a
        // mid-body error vanishes and the channel never gets a send,
        // hanging pollEnhance forever. safe() catches the error and
        // routes it through the channel instead. Also doubles as a
        // diagnostic — under --vm we'll now see the actual failure
        // message in the UI instead of a frozen "enhancing…".
        let result, callErr = safe(fn() {
            let resp, err = retry.doWith(fn() {
                return claude.messages(cClient, {
                    "system":     "You expand short image-generation prompts into richer, more detailed prompts that capture style, lighting, composition, and atmosphere. Return ONLY the enhanced prompt, no preamble.",
                    "max_tokens": 512,
                    "messages":   [ai.userMsg(rawPrompt)],
                })
            }, RETRY_OPTS)
            if err != null {
                return {"text": null, "err": err}
            }
            return {"text": claude.textOf(resp), "err": null}
        })
        if callErr != null {
            send(ch, {"text": null, "err": callErr})
        } else {
            send(ch, result)
        }
    })
}

fn pollEnhance() {
    if !enhancing { return }
    let msg = recvNonBlock(enhanceCh)
    if msg == null { return }
    enhancing = false
    if msg["err"] != null {
        enhanceErr = msg["err"].message
        return
    }
    prompt = msg["text"]
}

// ── Chat (Claude) ────────────────────────────────────────────────────────

// ── Tool execution ───────────────────────────────────────────────────────

fn _truncForLog(s, max) {
    if type(s) != "STRING" { s = str(s) }
    if len(s) > max { return substr(s, 0, max) + "..." }
    return s
}

fn logToolCall(name, input, result, isError) {
    let entry = {
        "ts":       elapsedTime(),
        "name":     name,
        "input":    input,
        "is_error": isError,
        "result":   _truncForLog(result, 2000),
    }
    let body, _ = safe(json.stringify, entry)
    _, _ = safe(_fsMkdirAll, userHomeDir() + "/.tadpole")
    _, _ = safe(appendFile, agentToolLog, body + "\n")
}

// _expandPath turns a path starting with "~" or "~/" into the absolute
// equivalent using $HOME. kLex's file builtins don't auto-expand the way
// shells do, so we have to do it here before handing off.
fn _expandPath(p) {
    if p == "~" { return userHomeDir() }
    if len(p) >= 2 && substr(p, 0, 2) == "~/" {
        return userHomeDir() + substr(p, 1)
    }
    return p
}

fn executeTool(name, input) {
    if name == "read_file" {
        let path = _expandPath(input["path"])
        // readFile raises on failure; safe() catches that into an Error obj.
        let out, err = safe(readFile, path)
        if err != null { return {"is_error": true, "content": "read_file: " + err.message} }
        return {"is_error": false, "content": out}
    }
    if name == "list_dir" {
        let path = _expandPath(input["path"])
        // _fsListDir RETURNS a (entries, err) tuple — never raises. Direct
        // call, no safe() wrap (which would nest the tuple inside another).
        //
        // Per-platform shape gotcha: on macOS the entries are plain strings
        // (filenames), on Windows they're hashes. We handle both so this
        // tool works regardless of the underlying kLex platform build.
        let entries, lerr = _fsListDir(path)
        if lerr != null { return {"is_error": true, "content": "list_dir: " + str(lerr)} }
        let names = makeArray(0)
        let i = 0
        while i < len(entries) {
            let e = entries[i]
            let nm = e
            if type(e) == "HASH" { nm = e["name"] }
            names = concat(names, [str(nm)])
            i = i + 1
        }
        if len(names) == 0 { return {"is_error": false, "content": "(empty directory)"} }
        return {"is_error": false, "content": join(names, "\n")}
    }
    if name == "http_get" {
        let url = input["url"]
        // _processExec returns (stdout, stderr, exit_code, err) — 4-tuple.
        let out, errOut, code, perr = _processExec("curl", ["-sS", "--max-time", "30", url])
        if perr != null { return {"is_error": true, "content": "http_get: " + str(perr)} }
        if code != 0 {
            return {"is_error": true,
                    "content": "http_get exit " + str(code) + ": " + str(errOut)}
        }
        return {"is_error": false, "content": str(out)}
    }
    if name == "write_file" {
        let path    = _expandPath(input["path"])
        let content = input["content"]
        let _, werr = safe(writeFile, path, content)
        if werr != null { return {"is_error": true, "content": "write_file: " + werr.message} }
        return {"is_error": false, "content": "Wrote " + str(len(content)) + " bytes to " + path}
    }
    if name == "shell" {
        let cmd = input["command"]
        // Platform dispatch — cmd.exe on Windows, sh -c elsewhere. The
        // user's `command` text is passed verbatim; semantics differ
        // between shells (pipes, redirects, glob expansion) so Claude
        // should be cued by the OS in SYSTEM_PROMPT.
        if isWindows() {
            let out, errOut, code, perr = _processExec("cmd", ["/c", cmd])
        } else {
            let out, errOut, code, perr = _processExec("sh", ["-c", cmd])
        }
        if perr != null { return {"is_error": true, "content": "shell: " + str(perr)} }
        // Non-zero exit isn't an internal error — surface it so Claude can
        // react. Include both stdout and stderr; agents often want both.
        let content = str(out)
        if len(str(errOut)) > 0 {
            if len(content) > 0 { content = content + "\n" }
            content = content + "[stderr] " + str(errOut)
        }
        if code != 0 {
            content = "[exit " + str(code) + "]\n" + content
        }
        return {"is_error": code != 0, "content": content}
    }
    if name == "launch" {
        // Defensive extraction: any missing key returns null in kLex, and
        // we want to safely handle each combination of (app, path, url).
        let app  = ""
        let path = ""
        let url  = ""
        if hasKey(input, "app")  { let v = input["app"]   if type(v) == "STRING" { app  = v } }
        if hasKey(input, "path") { let v = input["path"]  if type(v) == "STRING" { path = v } }
        if hasKey(input, "url")  { let v = input["url"]   if type(v) == "STRING" { url  = v } }

        // Resolve the target (path or url) — at least one must be set, OR
        // the user supplied just `app` on macOS (which we honour).
        let target = ""
        if len(path) > 0 { target = _expandPath(path) }
        if len(url)  > 0 { target = url }
        if len(target) == 0 && len(app) == 0 {
            return {"is_error": true,
                    "content": "launch: provide at least one of app, path, url"}
        }

        // Platform dispatch. Only macOS supports the "open with this app"
        // idiom directly. Linux/Windows: drop the app arg with a note so
        // Claude sees what was ignored rather than silently mis-running.
        let summary  = ""
        let execCmd  = ""
        let execArgs = makeArray(0)
        // targetLabel is "path=" or "url=" depending on which the user
        // supplied; both can't be set non-empty here because the second
        // assignment to target above overwrites the first.
        let targetLabel = "path="
        if len(path) == 0 { targetLabel = "url=" }

        if isMacOS() {
            execCmd = "open"
            if len(app) > 0 {
                execArgs = concat(execArgs, ["-a", app])
                summary  = "app=" + app
            }
            if len(target) > 0 {
                execArgs = concat(execArgs, [target])
                if len(summary) > 0 { summary = summary + ", " }
                summary  = summary + targetLabel + target
            }
        } else if isLinux() {
            // xdg-open ignores the concept of "open with"; the file
            // associations are user-configured. We honour path/url and
            // surface a note if app was set.
            if len(target) == 0 {
                return {"is_error": true,
                        "content": "launch on Linux: `app` alone has no effect; provide path or url too"}
            }
            execCmd  = "xdg-open"
            execArgs = [target]
            summary  = targetLabel + target
            if len(app) > 0 { summary = summary + " (note: 'app' ignored on Linux)" }
        } else if isWindows() {
            // `start` is a cmd builtin, not a standalone executable. The
            // empty "" is start's first arg — the window title — required
            // when the path is quoted, otherwise the path is interpreted
            // as the title.
            if len(target) == 0 {
                return {"is_error": true,
                        "content": "launch on Windows: `app` alone is not supported; provide path or url too"}
            }
            execCmd  = "cmd"
            execArgs = ["/c", "start", "", target]
            summary  = targetLabel + target
            if len(app) > 0 { summary = summary + " (note: 'app' ignored on Windows)" }
        } else {
            return {"is_error": true,
                    "content": "launch: unsupported platform"}
        }

        let out, errOut, code, perr = _processExec(execCmd, execArgs)
        if perr != null { return {"is_error": true, "content": "launch: " + str(perr)} }
        if code != 0 {
            return {"is_error": true,
                    "content": "launch exit " + str(code) + ": " + str(errOut)}
        }
        return {"is_error": false, "content": "opened: " + summary}
    }
    // MCP fallback — any tool name not handled above and present in the
    // registered MCP catalogue is routed to the server via mcp.callToolText.
    // Errors surface as is_error:true with the typed err.message so Claude
    // sees the failure and can adjust on the next turn.
    if mcpClient != null && hasKey(mcpToolNames, name) {
        let text, terr = mcp.callToolText(mcpClient, name, input, 30)
        if terr != null {
            return {"is_error": true,
                    "content": name + " (mcp): " + terr.message}
        }
        return {"is_error": false, "content": text}
    }
    return {"is_error": true, "content": "unknown tool: " + name}
}

// Tool results larger than this get a head+tail trim before going into the
// API message history, with an "[N bytes omitted]" marker in the middle.
// Keeps the input-tokens-per-minute budget in check — a single big web
// fetch can otherwise dominate every subsequent turn and trip Anthropic's
// rate limit. The full content stays on the display side and in the log.
const TOOL_RESULT_API_CAP = 8000

// MCP tools (klex_list_builtins, klex_list_files, klex_search, …) are
// the right answer for "how many X?" / "list all X" questions, and their
// output is typically structured JSON that Claude needs intact to count
// or enumerate. The lower cap was forcing fallback to shell+grep. Bump
// just for MCP-sourced results — built-in tools (http_get of a webpage,
// shell of a verbose command) keep the tighter cap so a single chatty
// call can't blow the rate-limit budget for the rest of the turn.
const MCP_TOOL_RESULT_API_CAP = 24000

fn _truncateForAPI(s, fromMcp) {
    if type(s) != "STRING" { s = str(s) }
    let n = len(s)
    let cap = TOOL_RESULT_API_CAP
    if fromMcp { cap = MCP_TOOL_RESULT_API_CAP }
    if n <= cap { return s }
    // Proportional head/tail (80/20 split of the cap) so the longer cap
    // keeps the structural envelope intact for JSON-shaped responses.
    let headLen = (cap * 4) / 5
    let tailLen = cap - headLen
    let head = substr(s, 0, headLen)
    let tail = substr(s, n - tailLen, n)
    let omitted = n - headLen - tailLen
    return head + "\n\n[... " + str(omitted) +
        " bytes omitted to save tokens; ask for a smaller range if you need more ...]\n\n" +
        tail
}

// runAndRecordTool dispatches a tool call, logs the result, and pushes the
// (tool_use+tool_result) pair onto the API messages array AND a display
// entry onto chatMessages. Wraps executeTool in safe() so any unexpected
// runtime error (bad arg type, missing key, internal exception) becomes a
// clean error result instead of wedging the agent loop.
// runAndRecordTool dispatches a tool call and appends the result to the
// API-side conversation in the active backend's wire format.
//
// NOTE: assistantContent is intentionally unused. The streaming pipeline
// (_streamFinalize) already appends the assistant turn to agentMessages
// before we get here, so duplicating it would corrupt the conversation
// shape. We keep the parameter for caller compatibility — pass null if
// you don't have it.
fn runAndRecordTool(toolId, toolName, toolInput, assistantContent) {
    _ = assistantContent
    let result, exc = safe(executeTool, toolName, toolInput)
    if exc != null {
        result = {"is_error": true,
                  "content": "internal error in " + toolName + ": " + exc.message}
    }
    logToolCall(toolName, toolInput, result["content"], result["is_error"])
    // MCP-sourced tools get the higher cap so list/enumerate calls survive
    // intact. hasKey() against the global registry is constant-time.
    let fromMcp = hasKey(mcpToolNames, toolName)
    let apiContent = _truncateForAPI(result["content"], fromMcp)

    if activeChatBackend == "ollama" {
        // Ollama's tool-result format: a plain {role: "tool", content: <text>}
        // message. No id linkage — Ollama uses message order to associate
        // results with calls.
        agentMessages = concat(agentMessages, [{
                "role":    "tool",
            "content": apiContent,
        }])
    } else {
        // Claude requires ALL tool_result blocks from one assistant turn
        // to live in a SINGLE user message that immediately follows it.
        // We accumulate the block here; _flushPendingToolResults() emits
        // the combined user message once every tool in the batch has
        // resolved (see _processNextPendingTool).
        agentResultsAccum = concat(agentResultsAccum, [{
                "type":        "tool_result",
            "tool_use_id": toolId,
            "content":     apiContent,
            "is_error":    result["is_error"],
        }])
    }

    chatMessages = concat(chatMessages, [{
            "role":     "tool",
        "name":     toolName,
        "input":    toolInput,
        "result":   result["content"],
        "isError":  result["is_error"],
    }])
    chatScroll = 0
}

// _processNextPendingTool walks agentPendingQueue one entry at a time:
//   read-only  → run immediately (pushes its tool_result into the accum)
//                and continue the loop
//   mutating   → park it on agentPendingTool for user Approve/Deny; the
//                handlers resume the queue when the user answers
// When the queue empties, _flushPendingToolResults emits the single
// follow-up user message Anthropic requires, then we kick off the next
// agent step.
fn _processNextPendingTool() {
    while len(agentPendingQueue) > 0 {
        let next = agentPendingQueue[0]
        let rest = makeArray(0)
        let i = 1
        while i < len(agentPendingQueue) {
            rest = concat(rest, [agentPendingQueue[i]])
            i = i + 1
        }
        agentPendingQueue = rest

        let tid    = next["id"]
        let tname  = next["name"]
        let tinput = next["input"]
        if isReadOnlyTool(tname) {
            runAndRecordTool(tid, tname, tinput, null)
        } else {
            agentPendingTool = {
                "id":                tid,
                "name":              tname,
                "input":             tinput,
                "assistant_content": null,
            }
            chatScroll = 0
            return
        }
    }
    _flushPendingToolResults()
    _startNextAgentStep()
}

// _flushPendingToolResults emits one user message carrying every
// tool_result block accumulated for the current assistant turn (Claude
// path only — Ollama uses ordered role:"tool" messages and is appended
// per-tool inside runAndRecordTool). No-op if the accumulator is empty.
fn _flushPendingToolResults() {
    if activeChatBackend == "ollama" { return }
    if len(agentResultsAccum) == 0 { return }
    agentMessages = concat(agentMessages, [{
            "role":    "user",
        "content": agentResultsAccum,
    }])
    agentResultsAccum = makeArray(0)
}


// ── MCP startup + polling ────────────────────────────────────────────────

// _mcpToolToClaude converts an MCP tool definition (per the MCP spec —
// {name, description, inputSchema}) into the Anthropic tool shape
// ({name, description, input_schema}). The only material difference is
// the schema field name; everything else passes through verbatim.
fn _mcpToolToClaude(t) {
    if t == null || type(t) != "HASH" || !hasKey(t, "name") { return null }
    let out = {"name": t["name"]}
    if hasKey(t, "description") { out["description"] = t["description"] }
    if hasKey(t, "inputSchema") { out["input_schema"] = t["inputSchema"] }
    return out
}


// _findFrogMcpServer searches likely install locations for the frogMcp
// server.py and returns the first existing path, or "" if none found.
// Order is "closest to the script" first, then user-level, then env
// overrides. Adding a new install layout means adding one entry here.
fn _findFrogMcpServer() {
    // Tadpole lives at examples/tadPole/tadPole.lex inside the kLex
    // repo. From there, ../../snowball/frogMcp/server/server.py is the
    // canonical dev-checkout path. The userHomeDir entry covers a
    // packaged drop-in (e.g. ~/.tadpole/frogmcp/server.py). Env
    // overrides win so an integrator can point at any location.
    let candidates = makeArray(0)
    let overrideHome = env("KLEX_HOME")
    let overridePath = env("KLEX_PATH")
    // env() returns null when the variable isn't set, not "" — guard before
    // calling len().
    if overrideHome != null && len(overrideHome) > 0 {
        candidates = concat(candidates,
            [overrideHome + "/snowball/frogMcp/server/server.py"])
    }
    if overridePath != null && len(overridePath) > 0 {
        candidates = concat(candidates,
            [overridePath + "/snowball/frogMcp/server/server.py"])
    }
    candidates = concat(candidates, [
        _scriptDir() + "/../../snowball/frogMcp/server/server.py",
        userHomeDir() + "/.tadpole/frogmcp/server.py",
    ])
    let i = 0
    while i < len(candidates) {
        if _fsExists(candidates[i]) { return candidates[i] }
        i = i + 1
    }
    return ""
}

// _tryMcpClient walks the python-command candidate list (mirrors
// tryBridge) and returns the first successful spawn. Returns (client, null)
// or (null, err) with the last error encountered.
fn _tryMcpClient(cmds, scriptPath, opts) {
    let lastErr = null
    let i = 0
    let n = len(cmds)
    while i < n {
        let c, err = mcp.newClient(cmds[i], [scriptPath], opts)
        if err == null { return c, null }
        lastErr = err
        i = i + 1
    }
    return null, lastErr
}

// _initMcpAsync spawns frogMcp in the background and reports the result
// (or failure) via mcpInitCh. Called once at startup. Treated as OPTIONAL —
// failure (no Python, frogMcp not installed, deps missing) just means
// Tadpole runs with the built-in tool set; mcpStatus reflects what
// happened for diagnostics.
//
// Cross-platform:
//   - python command: tries python3 (macOS/Linux default), python, py
//     (Windows launcher) in order — same logic as tryBridge.
//   - server path:    walks the search list in _findFrogMcpServer so
//     Tadpole works in dev checkouts, packaged installs, and env-
//     overridden locations.
fn _initMcpAsync() {
    mcpInitCh = channel(1)
    let ch        = mcpInitCh
    let serverPath = _findFrogMcpServer()
    if len(serverPath) == 0 {
        // No frogMcp install detected — not an error, just a "skipped"
        // outcome. The poll picks this up and sets mcpStatus = "disabled".
        send(ch, {"err": null, "client": null, "tools": null, "names": null,
                  "skipped": "frogMcp server.py not found"})
        return null
    }
    let pyCmds = ["python3", "python", "py"]
    async(fn() {
        let client, err = _tryMcpClient(pyCmds, serverPath, {"timeout_sec": 30})
        if err != null {
            send(ch, {"err": err, "client": null, "tools": null, "names": null})
            return null
        }
        // Pull the tool catalogue and convert to Anthropic shape. Build the
        // names set in the same pass so executeTool's routing has a constant-
        // time lookup.
        let raw, lerr = mcp.listTools(client)
        if lerr != null {
            mcp.close(client)
            send(ch, {"err": lerr, "client": null, "tools": null, "names": null})
            return null
        }
        let converted = makeArray(0)
        let names     = {}
        let i = 0
        while i < len(raw) {
            let t = _mcpToolToClaude(raw[i])
            if t != null {
                converted = concat(converted, [t])
                names[t["name"]] = true
            }
            i = i + 1
        }
        send(ch, {"err": null, "client": client, "tools": converted, "names": names})
        return null
    })
}


// pollMcpInit drains mcpInitCh once the async spawn completes and applies
// the result to the global state. After a successful init, agentTools
// grows by however many tools frogMcp exposed (currently 13) and
// mcpToolNames lets executeTool route by name. After a failure, mcpStatus
// captures the error message for the status bar.
//
// Idempotent: once the channel has been drained, mcpInitCh is nulled so
// subsequent frames are a no-op.
fn pollMcpInit() {
    if mcpInitCh == null { return }
    let res = recvNonBlock(mcpInitCh)
    if res == null { return }
    mcpInitCh = null  // one-shot — drop the channel
    // "skipped" path — no frogMcp install detected. Distinguish from
    // "failed" so the status bar can be honest about what happened.
    if hasKey(res, "skipped") && res["skipped"] != null {
        mcpStatus = "disabled"
        return
    }
    if res["err"] != null {
        mcpStatus = "failed"
        // Don't surface as a chat error — it's a startup notice, not a
        // user-visible failure. The status bar shows "MCP failed" and any
        // attempt to use MCP tools simply won't happen.
        return
    }
    mcpClient    = res["client"]
    mcpToolNames = res["names"]
    agentTools   = concat(agentTools, res["tools"])
    mcpStatus    = "ready"
}


// ── Chat / agent flow ────────────────────────────────────────────────────

// System prompt: gives Claude a baseline understanding of the environment
// and tool conventions. Single source of truth across every conversation.
//
// OS + CWD are captured ONCE at startup and embedded as literals so Claude
// can form correct shell commands and absolute paths. Without this, the
// model has no reliable way to know where Tadpole was launched from or
// which shell flavour `shell(...)` runs (sh vs cmd.exe) — observed
// symptom: it guesses `/Users/Shared/...` and loops on shell failures.
let _tadCwd, _tadCwdErr = _osCwd()
if _tadCwdErr != null { _tadCwd = "(unknown)" }
let _tadOS    = osName()
let _tadShell = "sh -c"
if isWindows() { _tadShell = "cmd /c" }

let SYSTEM_PROMPT = "You are an assistant running inside Tadpole, a kLex desktop app on " + _tadOS + ". " +
    "Your `shell`, `read_file`, and `list_dir` tools run with the current working directory set to: " + _tadCwd + ". " +
    "Use absolute paths (or paths relative to that directory) — do NOT invent paths under /Users/Shared or similar.\n" +
    "The `shell` tool invokes `" + _tadShell + "`, so commands must use the syntax of that shell (POSIX sh on macOS/Linux, cmd.exe on Windows — pipes, redirects, and glob expansion differ).\n" +
    "You can call these tools:\n" +
    "  • read_file(path)        — auto-runs (read-only)\n" +
    "  • list_dir(path)         — auto-runs (read-only)\n" +
    "  • http_get(url)          — auto-runs (read-only)\n" +
    "  • write_file(path, content) — requires the user's approval\n" +
    "  • shell(command)         — requires the user's approval\n" +
    "  • launch(app?, path?, url?) — requires the user's approval; opens a file or URL in its default OS handler. The `app` argument is honoured on macOS only.\n" +
    "Additional kLex-language tools may be available via frogMcp (klex_list_builtins, klex_describe_symbol, klex_search, klex_find_references, klex_describe_file, …). These read from a pre-built index and are vastly faster + more reliable than shell+grep for any question about the kLex codebase.\n" +
    "For \"how many X?\" or \"list all X\" questions, prefer the appropriate frogMcp tool with a high `limit` and count from the structured result — do NOT fall back to `shell` + grep as a workaround when an MCP tool exists. " +
    "Paths may use ~/ to mean the user's home directory; this is expanded for you. " +
    "Prefer tools over speculation: if you can read a file or list a directory to answer, do that. " +
    "Be concise. Use code blocks (```) for any code or commands. " +
    "When a tool would mutate the user's machine (write_file / shell / launch), explain WHY before calling it so the approval prompt is easy to evaluate. " +
    "If a tool fails, try a different approach or ask the user; don't repeat the same call. " +
    "kLex / FROG is the scripting language hosting this app — if the user asks for code, prefer kLex unless they specify otherwise."

// ── Session persistence ──────────────────────────────────────────────────

let SESSION_PATH         = userHomeDir() + "/.tadpole/last_session.json"
let SESSION_MAX_BYTES    = 200000  // legacy bloated sessions are skipped on load

// Image content blocks carry base64 payloads that can hit megabytes — useless
// to persist (Claude doesn't need them after the turn) and slow to JSON-parse
// in kLex on next launch. Replace image blocks with a tiny text marker.
fn _stripLargeContent(msgs) {
    let out = makeArray(0)
    for m in msgs {
        let c = m["content"]
        if type(c) == "ARRAY" {
            let nc = makeArray(0)
            for block in c {
                if type(block) == "HASH" && hasKey(block, "type") && block["type"] == "image" {
                    nc = concat(nc, [{"type": "text", "text": "[image attached previously; not persisted]"}])
                } else {
                    nc = concat(nc, [block])
                }
            }
            out = concat(out, [{"role": m["role"], "content": nc}])
        } else {
            out = concat(out, [m])
        }
    }
    return out
}

fn saveSession() {
    let body, jerr = safe(json.stringify, {
        "chat_messages":      chatMessages,
        "agent_messages":     _stripLargeContent(agentMessages),
        "session_in_tokens":  sessionInTokens,
        "session_out_tokens": sessionOutTokens,
    })
    if jerr != null { return }
    _, _ = safe(_fsMkdirAll, userHomeDir() + "/.tadpole")
    _, _ = safe(writeFile, SESSION_PATH, body)
}

fn loadSession() {
    let raw, rerr = safe(readFile, SESSION_PATH)
    if rerr != null { return }
    // Defensive: an older build may have persisted image bytes — refuse to
    // parse anything that big in the kLex JSON parser (slow → looks like a hang).
    if len(raw) > SESSION_MAX_BYTES { return }
    let parsed, perr = safe(json.parse, raw)
    if perr != null { return }
    if hasKey(parsed, "chat_messages")      { chatMessages      = parsed["chat_messages"] }
    if hasKey(parsed, "agent_messages")     { agentMessages     = parsed["agent_messages"] }
    if hasKey(parsed, "session_in_tokens")  { sessionInTokens   = parsed["session_in_tokens"] }
    if hasKey(parsed, "session_out_tokens") { sessionOutTokens  = parsed["session_out_tokens"] }
}

fn clearChat() {
    chatMessages = makeArray(0)
    agentMessages = makeArray(0)
    sessionInTokens  = 0
    sessionOutTokens = 0
    chatScroll = 0
    chatSending = false
    agentPendingTool  = null
    agentPendingQueue = makeArray(0)
    agentResultsAccum = makeArray(0)
    chatRenderCache      = makeArray(0)
    chatRenderCacheWidth = -1
    chatGen = chatGen + 1   // drop any in-flight result
    saveSession()
}

fn cancelChat() {
    if !chatSending && agentPendingTool == null { return }
    chatGen = chatGen + 1
    chatSending = false
    agentPendingTool  = null
    agentPendingQueue = makeArray(0)
    agentResultsAccum = makeArray(0)
    chatMessages = concat(chatMessages, [{
            "role":    "assistant",
        "content": "(interrupted by user)",
        "isError": true,
    }])
    chatScroll = 0
    saveSession()
}

// ── Streaming helpers ────────────────────────────────────────────────────
// These maintain streamCurrentContent (the assistant turn's content-block
// array) as Claude emits SSE events. pollChat() drives them; runtime
// callers shouldn't invoke them directly.

fn _streamReset() {
    streamCurrentMsgIdx   = -1
    streamCurrentContent  = makeArray(0)
    streamCurrentBlock    = null
    streamLastStopReason  = ""
    streamLastUsage       = null
}

// _streamCloseBlock finalises the currently-open content block and
// appends it to streamCurrentContent. For tool_use blocks the
// accumulated partial_json string is parsed into the block's "input".
fn _streamCloseBlock() {
    if streamCurrentBlock == null { return }
    let blk = streamCurrentBlock
    if blk["type"] == "tool_use" {
        let pj = ""
        if hasKey(blk, "_partial_json") { pj = blk["_partial_json"] }
        if len(pj) > 0 {
            let parsed, perr = json.parse(pj)
            if perr == null { blk["input"] = parsed }
            // else leave input as {} — the model wins back nothing useful
            // and the agent loop will surface the malformed call as a
            // tool failure.
        }
        delete(blk, "_partial_json")
    }
    streamCurrentContent = concat(streamCurrentContent, [blk])
    streamCurrentBlock = null
}

// _streamHandleText appends a text delta. Opens a new text block + new
// chatMessages bubble if one isn't already in progress.
fn _streamHandleText(delta) {
    if streamCurrentBlock != null && streamCurrentBlock["type"] != "text" {
        _streamCloseBlock()
    }
    if streamCurrentBlock == null {
        streamCurrentBlock = {"type": "text", "text": ""}
        chatMessages = concat(chatMessages, [{
                "role":        "assistant",
            "content":     "",
            "isStreaming": true,
        }])
        streamCurrentMsgIdx = len(chatMessages) - 1
    }
    streamCurrentBlock["text"] = streamCurrentBlock["text"] + delta
    if streamCurrentMsgIdx >= 0 && streamCurrentMsgIdx < len(chatMessages) {
        chatMessages[streamCurrentMsgIdx]["content"] =
            chatMessages[streamCurrentMsgIdx]["content"] + delta
    }
}

// _streamHandleToolUse closes any open text block and opens a new
// tool_use block. The tool's input is built up by subsequent
// _streamHandleToolInput calls and finalised in _streamCloseBlock.
fn _streamHandleToolUse(toolId, toolName) {
    if streamCurrentBlock != null {
        _streamCloseBlock()
    }
    // Mark any preceding text bubble as complete — tool_use itself isn't
    // shown as a regular chat bubble (the approval card handles that).
    if streamCurrentMsgIdx >= 0 && streamCurrentMsgIdx < len(chatMessages) {
        delete(chatMessages[streamCurrentMsgIdx], "isStreaming")
    }
    streamCurrentMsgIdx = -1
    streamCurrentBlock = {
        "type":          "tool_use",
        "id":            toolId,
        "name":          toolName,
        "input":         {},
        "_partial_json": "",
    }
}

fn _streamHandleToolInput(partialJson) {
    if streamCurrentBlock == null { return }
    if streamCurrentBlock["type"] != "tool_use" { return }
    streamCurrentBlock["_partial_json"] =
        streamCurrentBlock["_partial_json"] + partialJson
}

// _streamHandleToolUseComplete is the Ollama-shape variant: tool input
// arrives whole in one event (no partial_json deltas), so we build the
// full block directly and append it to streamCurrentContent. No open
// block is left dangling — there's nothing else to accumulate.
fn _streamHandleToolUseComplete(toolId, toolName, toolInput) {
    if streamCurrentBlock != null { _streamCloseBlock() }
    if streamCurrentMsgIdx >= 0 && streamCurrentMsgIdx < len(chatMessages) {
        delete(chatMessages[streamCurrentMsgIdx], "isStreaming")
    }
    streamCurrentMsgIdx = -1
    streamCurrentContent = concat(streamCurrentContent, [{
            "type":  "tool_use",
        "id":    toolId,
        "name":  toolName,
        "input": toolInput,
    }])
}

// _streamFinalize is called when the "done" event arrives. Closes the
// final block, appends the assistant turn to agentMessages in the active
// backend's wire format, and drives the agent loop if tool calls were
// emitted.
fn _streamFinalize() {
    _streamCloseBlock()
    if streamCurrentMsgIdx >= 0 && streamCurrentMsgIdx < len(chatMessages) {
        delete(chatMessages[streamCurrentMsgIdx], "isStreaming")
    }
    let finalContent = streamCurrentContent

    // Translate streamCurrentContent (an array of content blocks built up
    // by _streamHandleText / _streamHandleToolUseComplete) into the active
    // backend's assistant-message shape.
    if activeChatBackend == "ollama" {
        // Ollama: {role: "assistant", content: <joined text>, tool_calls?: [...]}
        let assistantText = ""
        let toolCalls = makeArray(0)
        for blk in finalContent {
            if blk["type"] == "text" {
                assistantText = assistantText + blk["text"]
            } else if blk["type"] == "tool_use" {
                toolCalls = concat(toolCalls, [{
                        "function": {"name": blk["name"], "arguments": blk["input"]},
                }])
            }
        }
        let msg = {"role": "assistant", "content": assistantText}
        if len(toolCalls) > 0 { msg["tool_calls"] = toolCalls }
        agentMessages = concat(agentMessages, [msg])
    } else {
        // Claude: {role: "assistant", content: [content_block, …]}
        agentMessages = concat(agentMessages, [{
                "role":    "assistant",
            "content": finalContent,
        }])
    }

    // Tool-dispatch decision is backend-agnostic — check the content blocks
    // directly rather than the (Claude-specific) "tool_use" stop_reason.
    // Claude can emit MULTIPLE tool_use blocks in one assistant turn (parallel
    // tool calls). Collect them all into the queue and let _processNextPendingTool
    // drive the batch; partial completion is what triggered the HTTP 400
    // "tool_use ids without tool_result blocks" failure mode.
    let toolUses = makeArray(0)
    for blk in finalContent {
        if blk["type"] == "tool_use" {
            toolUses = concat(toolUses, [blk])
        }
    }
    if len(toolUses) > 0 {
        agentPendingQueue = toolUses
        agentResultsAccum = makeArray(0)
        _processNextPendingTool()
        return
    }

    // Natural end of turn.
    chatSending = false
    chatScroll  = 0
    saveSession()
}


fn _startNextAgentStep() {
    if agentStep >= AGENT_MAX_STEPS {
        chatMessages = concat(chatMessages, [{
                "role":    "assistant",
            "content": "(stopped — hit max step limit of " + str(AGENT_MAX_STEPS) + ")",
            "isError": true,
        }])
        chatSending = false
        return
    }
    if activeChatBackend == "ollama" {
        _startOllamaStep()
    } else {
        _startClaudeStep()
    }
}


// _withCacheControl turns a plain system-prompt string into the
// content-block form Anthropic needs to attach prompt-cache markers:
//   [{type: "text", text: <prompt>, cache_control: {type: "ephemeral"}}]
// The block tells the server "everything up to and including this block
// is cacheable". A subsequent identical request within 5 minutes reads
// from cache for 10% of the input-token cost (~90% savings).
fn _withCacheControl(text) {
    return [{
            "type":          "text",
        "text":          text,
        "cache_control": {"type": "ephemeral"},
    }]
}

// _toolsWithCacheControl returns a new tools array with cache_control on
// the LAST tool. Anthropic semantics: a cache_control marker covers the
// whole prefix up to and including that block — so marking the final tool
// caches every tool definition above it. Tadpole's tool set is stable
// across turns, so this is a steady cache hit after the first call.
//
// Skip when tools is empty (no point) or null (defensive).
fn _toolsWithCacheControl(tools) {
    if tools == null || type(tools) != "ARRAY" || len(tools) == 0 {
        return tools
    }
    let n   = len(tools)
    let out = makeArray(n, null)
    let i   = 0
    while i < n - 1 {
        out[i] = tools[i]
        i = i + 1
    }
    // Shallow-clone the last tool and attach cache_control. We don't
    // mutate the original (kLex hashes are reference types and the global
    // agentTools is the source of truth).
    let last = tools[n - 1]
    let cloned = {}
    for k, v in last {
        cloned[k] = v
    }
    cloned["cache_control"] = {"type": "ephemeral"}
    out[n - 1] = cloned
    return out
}


fn _startClaudeStep() {
    let cClient = _claudeClient()
    if cClient == null {
        chatMessages = concat(chatMessages, [{
                "role":    "assistant",
            "content": "(stopped — no Claude API key configured. Open Settings → Claude.)",
            "isError": true,
        }])
        chatSending = false
        return
    }
    agentStep   = agentStep + 1
    chatCh      = channel(64)
    let ch          = chatCh
    let msgs        = agentMessages
    // Wrap system + tools with prompt-cache markers per Anthropic spec.
    // The system prompt and the entire tools array are stable across turns
    // within a chat, so they're prime cache candidates:
    //   - First send: cache write (25% input-token surcharge)
    //   - Every subsequent send within 5 min: cache read (90% discount)
    // Tadpole's tool definitions are ~2KB of JSON; SYSTEM_PROMPT is ~1KB.
    // Even a 3-turn chat amortises the write within one round-trip.
    let sysPrompt   = _withCacheControl(SYSTEM_PROMPT)
    let tools       = _toolsWithCacheControl(agentTools)
    let genAtStart  = chatGen
    _streamReset()
    async(fn() {
        // retry.doWith covers the *initial* HTTP connect — rate limits and
        // 5xx blips return cleanly to retry instead of failing the stream
        // setup. Mid-stream events still flow normally; errors that arrive
        // through the channel are handled by the consumer below.
        let streamCh, sErr = retry.doWith(fn() {
            return claude.stream(cClient, {
                "system":     sysPrompt,
                "max_tokens": 4096,
                "messages":   msgs,
                "tools":      tools,
            })
        }, RETRY_OPTS)
        if sErr != null {
            send(ch, {"streamEvent": null, "err": sErr, "gen": genAtStart, "endOfStream": true})
        } else {
            for evt in streamCh {
                if send(ch, {"streamEvent": evt, "err": null, "gen": genAtStart, "endOfStream": false}) == false {
                    break
                }
            }
            send(ch, {"streamEvent": null, "err": null, "gen": genAtStart, "endOfStream": true})
        }
    })
}


// _maybeWarnOllamaCapabilities probes the active Ollama model once per
// model-switch via /api/show and surfaces a chat-thread warning if the
// model doesn't advertise tool support. Cheap (single HTTP call), runs
// only on the first turn after a model change, silent-fail on errors so
// a flaky daemon never blocks chat.
fn _maybeWarnOllamaCapabilities(oc, modelName) {
    if modelName == _ollamaCapsCheckedFor { return }
    _ollamaCapsCheckedFor = modelName
    // If the user has explicitly disabled tool-sending in settings, they
    // are knowingly running a non-tool model. Skip the warning — it's
    // noise, not news.
    let toolsOn = true
    if hasKey(config["ollama"], "tools") { toolsOn = config["ollama"]["tools"] }
    if !toolsOn { return }
    let info, err = ollama.show(oc, null)
    if err != null { return }
    if ollama.hasCapability(info, "tools") { return }
    chatMessages = concat(chatMessages, [{
            "role":    "assistant",
        "content": "⚠️ Model '" + modelName + "' does not advertise tool support. " +
                   "MCP tools and agent actions will be silently ignored by this model — " +
                   "either uncheck 'Send tools to model' in Settings → Ollama, or switch " +
                   "to a tools-capable model (Gemma 3+, Llama 3.1+, Qwen 2.5+, Mistral-Nemo).",
    }])
}

fn _startOllamaStep() {
    let cfg = config["ollama"]
    if cfg == null || cfg["base_url"] == null || cfg["base_url"] == "" {
        chatMessages = concat(chatMessages, [{
                "role":    "assistant",
            "content": "(stopped — Ollama not configured. Open Settings → Ollama.)",
            "isError": true,
        }])
        chatSending = false
        return
    }
    let oc = ollama.newClient(cfg["base_url"], cfg["model"])
    _maybeWarnOllamaCapabilities(oc, cfg["model"])
    let thinkOpt = false
    if hasKey(cfg, "think") { thinkOpt = cfg["think"] }
    let toolsOpt = true
    if hasKey(cfg, "tools") { toolsOpt = cfg["tools"] }

    agentStep   = agentStep + 1
    chatCh      = channel(64)
    let ch          = chatCh
    let msgs        = agentMessages
    let tools       = agentTools
    let sysPrompt   = SYSTEM_PROMPT
    let genAtStart  = chatGen
    _streamReset()
    async(fn() {
        // retry.doWith covers the *initial* HTTP connect — daemon-restart
        // connection blips and any future Ollama rate limits return cleanly
        // to retry. Mid-stream events flow normally afterwards.
        //
        // Inline literals (no shared hash + mutation). Ollama 400s on
        // models that don't support a given capability if the matching
        // field is present at all, even as false. So we only send:
        //   - "think" when the user has enabled chain-of-thought
        //   - "tools" when the user has enabled tool use
        // num_ctx is the total prompt+response window. Ollama's default
        // is ~2k which truncates long chats and code blocks; 32k is
        // generous enough for an entire pasted doc plus multi-turn tool
        // use, without paying the full 64k+ RAM tax. num_predict caps
        // the response length only.
        let streamCh, sErr = retry.doWith(fn() {
            if thinkOpt && toolsOpt {
                return ollama.stream(oc, {
                    "system":   sysPrompt,
                    "messages": msgs,
                    "tools":    tools,
                    "think":    true,
                    "options":  {"num_predict": 4000, "num_ctx": 32768},
                })
            }
            if thinkOpt {
                return ollama.stream(oc, {
                    "system":   sysPrompt,
                    "messages": msgs,
                    "think":    true,
                    "options":  {"num_predict": 4000, "num_ctx": 32768},
                })
            }
            if toolsOpt {
                return ollama.stream(oc, {
                    "system":   sysPrompt,
                    "messages": msgs,
                    "tools":    tools,
                    "options":  {"num_predict": 4000, "num_ctx": 32768},
                })
            }
            return ollama.stream(oc, {
                "system":   sysPrompt,
                "messages": msgs,
                "options":  {"num_predict": 4000, "num_ctx": 32768},
            })
        }, RETRY_OPTS)
        if sErr != null {
            send(ch, {"streamEvent": null, "err": sErr, "gen": genAtStart, "endOfStream": true})
        } else {
            for evt in streamCh {
                if send(ch, {"streamEvent": evt, "err": null, "gen": genAtStart, "endOfStream": false}) == false {
                    break
                }
            }
            send(ch, {"streamEvent": null, "err": null, "gen": genAtStart, "endOfStream": true})
        }
    })
}

fn sendChat() {
    if chatSending { return }
    let text = trim(chatDraft)
    if len(text) == 0 { return }
    // If "Attach image" is on AND we have generated bytes, build a content
    // array containing both an image block and a text block. Anthropic's
    // vision endpoint then sees the image alongside the prompt. One-shot:
    // the toggle resets after sending so the next message is text-only.
    if attachImage && lastImageBytes != null {
        let b64 = bytesToBase64(lastImageBytes)
        let apiContent = [
            {"type": "image", "source": {
                    "type":       "base64",
                "media_type": lastImageMime,
                "data":       b64,
            }},
            {"type": "text", "text": text},
        ]
        agentMessages = concat(agentMessages, [{"role": "user", "content": apiContent}])
        chatMessages  = concat(chatMessages, [{
                "role":    "user",
            "content": text + "\n\n📷 _image attached_",
        }])
        attachImage = false
    } else {
        chatMessages  = concat(chatMessages, [{"role": "user", "content": text}])
        agentMessages = concat(agentMessages, [{"role": "user", "content": text}])
    }
    chatDraft         = ""
    chatSending       = true
    chatRefocusPending = true
    chatScroll        = 0
    agentStep         = 0
    agentPendingTool  = null
    agentPendingQueue = makeArray(0)
    agentResultsAccum = makeArray(0)
    _startNextAgentStep()
}

fn pollChat() {
    if !chatSending { return }
    if agentPendingTool != null { return }   // waiting for user approval

    // Drain as many stream events as are available this frame. Text deltas
    // arrive in quick bursts; processing one per frame would visibly stutter.
    // The cap prevents a flood from stalling the render thread.
    let drained = 0
    while drained < 64 {
        let msg = recvNonBlock(chatCh)
        if msg == null { return }
        drained = drained + 1

        // Drop events from a generation the user has since cancelled.
        if hasKey(msg, "gen") && msg["gen"] != chatGen { continue }

        // Stream-level error (couldn't open the request).
        if msg["err"] != null {
            let em = msg["err"]
            let ems = ""
            if type(em) == "STRING" { ems = em } else { ems = em.message }
            let friendly = ems
            if indexOf(ems, "429") >= 0 || indexOf(ems, "rate_limit") >= 0 || indexOf(ems, "rate limit") >= 0 {
                friendly = "⚠ Anthropic API rate limit hit (input-tokens-per-minute quota). " +
                           "Wait ~60 seconds before sending again. " +
                           "If this keeps happening, ask Claude shorter questions, " +
                           "use the read-only tools on smaller files, or upgrade your API tier " +
                           "at console.anthropic.com/settings/limits."
            }
            if streamCurrentMsgIdx >= 0 && streamCurrentMsgIdx < len(chatMessages) {
                delete(chatMessages[streamCurrentMsgIdx], "isStreaming")
            }
            chatMessages = concat(chatMessages, [{
                    "role":    "assistant",
                "content": friendly,
                "isError": true,
            }])
            chatSending = false
            chatScroll  = 0
            saveSession()
            return
        }

        if msg["endOfStream"] {
            // Producer finished sending. _streamFinalize() was already
            // called when the "done" event arrived; this is just the
            // close-of-channel sentinel. Defensive cleanup if we somehow
            // got here without a done.
            if chatSending {
                _streamFinalize()
            }
            return
        }

        let evt = msg["streamEvent"]
        if evt == null { continue }

        let et = evt["type"]
        if et == "text" {
            _streamHandleText(evt["text"])
        } else if et == "thinking" {
            // Reasoning models (qwen3, deepseek-r1) stream chain-of-thought
            // here when `think: true`. v1 doesn't surface them to the user
            // — they're noise for a chat UI. Drop silently.
        } else if et == "tool_use" {
            // Two shapes: Anthropic emits tool_use without `input` (the
            // input streams later as tool_input deltas), Ollama emits it
            // with the full `input` hash already populated. Distinguish on
            // the presence of the `input` key.
            if hasKey(evt, "input") {
                let tid = ""
                if hasKey(evt, "id") { tid = evt["id"] }
                if tid == "" { tid = "ollama_call_" + str(agentStep) }
                _streamHandleToolUseComplete(tid, evt["name"], evt["input"])
            } else {
                _streamHandleToolUse(evt["id"], evt["name"])
            }
        } else if et == "tool_input" {
            _streamHandleToolInput(evt["partial_json"])
        } else if et == "done" {
            if hasKey(evt, "stop_reason") { streamLastStopReason = evt["stop_reason"] }
            if hasKey(evt, "usage") {
                let u = evt["usage"]
                if hasKey(u, "output_tokens") {
                    sessionOutTokens = sessionOutTokens + u["output_tokens"]
                }
                // input_tokens come from message_start (not exposed in v1
                // streaming). Use countTokens() separately if precise input
                // budgeting is needed.
            }
            _streamFinalize()
            return
        } else if et == "error" {
            // Mid-stream error event from anthropic.lex.
            if streamCurrentMsgIdx >= 0 && streamCurrentMsgIdx < len(chatMessages) {
                delete(chatMessages[streamCurrentMsgIdx], "isStreaming")
            }
            chatMessages = concat(chatMessages, [{
                    "role":    "assistant",
                "content": "⚠ stream error: " + evt["message"],
                "isError": true,
            }])
            chatSending = false
            chatScroll  = 0
            saveSession()
            return
        }
    }
}

fn approvePendingTool() {
    if agentPendingTool == null { return }
    let pt = agentPendingTool
    agentPendingTool = null
    runAndRecordTool(pt["id"], pt["name"], pt["input"], pt["assistant_content"])
    _processNextPendingTool()
}

fn denyPendingTool() {
    if agentPendingTool == null { return }
    let pt = agentPendingTool
    agentPendingTool = null
    let denialText = "User denied this tool call. Continue without it or ask the user what they'd prefer."

    // The assistant turn was already appended by _streamFinalize before
    // agentPendingTool was set, so we ONLY add the synthetic tool result.
    // Claude path pushes into the accumulator so it joins any sibling
    // tool_results from the same assistant turn in one flush.
    if activeChatBackend == "ollama" {
        agentMessages = concat(agentMessages, [{
                "role":    "tool",
            "content": denialText,
        }])
    } else {
        agentResultsAccum = concat(agentResultsAccum, [{
                "type":        "tool_result",
            "tool_use_id": pt["id"],
            "content":     denialText,
            "is_error":    false,
        }])
    }
    chatMessages = concat(chatMessages, [{
            "role":     "tool",
        "name":     pt["name"],
        "input":    pt["input"],
        "result":   "(user denied)",
        "isError":  true,
    }])
    chatScroll = 0
    _processNextPendingTool()
}


// ── Frame draw ─────────────────────────────────────────────────────────────

fn drawFrame(frame) {
    background(theme["bg"][0], theme["bg"][1], theme["bg"][2])

    pollNotifications()
    pollGeneration()
    pollEnhance()
    pollChat()
    pollMcpInit()
    pollDroppedFiles()

    uiBegin()
    if !themeApplied {
        themes.applyTheme(theme)
        themeApplied = true
    }

    let W = winWidth()
    let H = winHeight()

    if settingsOpen {
        drawSettingsModal(W, H)
    } else {
        drawMainUI(W, H)
    }

    uiEnd()
}


fn drawMainUI(W, H) {
    let leftW   = 380
    let margin  = 18
    let panelX  = margin
    let panelY  = margin
    let panelW  = leftW
    let panelH  = H - margin * 2

    drawPanel(panelX, panelY, panelW, panelH)

    // ── Header ────────────────────────────────────────────────────────────
    // Logo centered horizontally in the left pane.
    let logoW = 234
    let logoH = 117
    let logoX = panelX + (panelW - logoW) / 2
    if tadpoleLogo != null {
        image(tadpoleLogo, logoX, panelY + 8, logoW, logoH, "fit")
    } else {
        let tw = textWidth(displayFont, "Tadpole", 0.7)
        fillC(theme["titleText"])
        sayDisplay("Tadpole", panelX + (panelW - tw) / 2, panelY + 16, 0.7)
    }

    // Settings (gear) button — right side of header
    if button("⚙", panelX + panelW - 50, panelY + 14, 32, 28, 0.55) {
        settingsOpen = true
        settingsTab  = activeProvider
        testStatus   = ""
    }

    drawSep(panelX + 20, panelY + 142, panelW - 40)
    let cy = panelY + 154

    // ── Prompt ────────────────────────────────────────────────────────────
    // textArea draws its label in the *current* fill colour — set the label
    // tone here explicitly so it stays readable regardless of whatever
    // widget ran last upstream.
    fillC(theme["mainLabel"])
    prompt = textArea("Prompt · (Cmd+V to paste)",
                      prompt, panelX + 20, cy + 22, panelW - 40, 130, 0.5)
    cy = cy + 162

    // ── Negative prompt (avoid:) ──────────────────────────────────────────
    // Smaller text area; default is the canonical SD 1.5 baseline. User can
    // clear or tweak. Plumbed through to a1111 (negative_prompt key) and to
    // the Python bridge for other providers.
    fillC(theme["mainLabel"])
    negPrompt = textArea("Negative prompt · (avoid:)",
                         negPrompt, panelX + 20, cy + 22, panelW - 40, 56, 0.45)
    cy = cy + 92

    // ── Style preset (suffix appended at gen time, prompt stays clean) ────
    fillC(theme["mainLabel"])
    say("Style", panelX + 20, cy + 8, 0.45)
    let styleOptions = ["(none)", "cinematic lighting", "watercolor", "isometric",
                    "oil painting", "cyberpunk", "studio ghibli style", "flat vector art"]
    stylePreset = dropdown("", styleOptions,
                           panelX + 80, cy + 4, panelW - 100, 0.45)
    cy = cy + 38

    // ── Enhance with Claude ───────────────────────────────────────────────
    let enhanceLabel = "✨  Enhance with Claude"
    if enhancing { enhanceLabel = "✨  enhancing…" }
    if !enhancing {
        if button(enhanceLabel, panelX + 20, cy, panelW - 40, 30, 0.42) {
            startEnhance()
        }
    } else {
        fillC(theme["cardBg"])
        roundedRect(panelX + 20, cy, panelW - 40, 30, 6)
        fillC(theme["subtitleText"])
        let tw = textWidth(uiFont, enhanceLabel, 0.42)
        say(enhanceLabel, panelX + 20 + (panelW - 40 - tw) / 2, cy + 8, 0.42)
    }
    cy = cy + 38
    if len(enhanceErr) > 0 {
        fillC(theme["crit"])
        cy = drawWrapped("✨ " + enhanceErr, panelX + 20, cy, 56, 0.4)
        cy = cy + 4
    }

    // ── Dimensions ────────────────────────────────────────────────────────
    let newLock = checkbox("Lock aspect ratio", panelX + 20, cy + 8, lockAspect, 0.45)
    if newLock && !lockAspect {
        // Capture the ratio at the moment the lock is engaged so subsequent
        // slider drags preserve it (rather than the last captured ratio).
        lockRatio = float(imgW) / float(imgH)
    }
    lockAspect = newLock
    cy = cy + 38

    let prevW = imgW
    let prevH = imgH
    imgW = int(slider("Width: " + str(imgW) + " px",
                      panelX + 20, cy + 16, panelW - 40, imgW, 256, 1280, 0.45))
    cy = cy + 42
    imgH = int(slider("Height: " + str(imgH) + " px",
                      panelX + 20, cy + 16, panelW - 40, imgH, 256, 1280, 0.45))
    cy = cy + 52

    // Propagate aspect lock: whichever slider moved this frame drives the
    // other. Both clamps mirror the sliders' own 256-1280 bounds so we never
    // produce a dimension the slider widget can't display next frame.
    if lockAspect {
        if imgW != prevW {
            let newH = int(float(imgW) / lockRatio)
            if newH < 256  { newH = 256 }
            if newH > 1280 { newH = 1280 }
            imgH = newH
        } else if imgH != prevH {
            let newW = int(float(imgH) * lockRatio)
            if newW < 256  { newW = 256 }
            if newW > 1280 { newW = 1280 }
            imgW = newW
        }
    }

    // ── img2img ───────────────────────────────────────────────────────────
    // Only Local SD supports this path today. The toggle is visible for
    // every provider, but startGeneration only fires the img2img branch
    // for activeProvider == "a1111" — other providers fall back to
    // standard txt2img and silently ignore the init image.
    let newImg2Img = checkbox("Img2img — start from a source image",
                          panelX + 20, cy + 8, useImg2Img, 0.45)
    if newImg2Img != useImg2Img {
        useImg2Img = newImg2Img
        if !useImg2Img { clearInitImage() }   // turning off clears state
    }
    cy = cy + 28

    if useImg2Img {
        // Drop zone: dashed-style rectangle, 120px tall. Two visual states:
        //   empty   → helper text + (optional) "Use last generated" button
        //   loaded  → thumbnail + filename label + Use last gen + Clear
        let dropX = panelX + 20
        let dropY = cy
        let dropW = panelW - 40
        let dropH = 120
        if initImageBytes == null {
            fillC(theme["cardBg"])
            roundedRect(dropX, dropY, dropW, dropH, 8)
            fillC(theme["subtitleText"])
            let msg = "Drag a .png / .jpg here"
            let tw = textWidth(uiFont, msg, 0.46)
            say(msg, dropX + (dropW - tw) / 2, dropY + dropH / 2 - 22, 0.46)
            if lastImageBytes != null {
                let btnW = 180
                let btnX = dropX + (dropW - btnW) / 2
                let btnY = dropY + dropH / 2 + 6
                if button("Use last generated", btnX, btnY, btnW, 28, 0.42) {
                    useLastGenAsInit()
                }
            }
        } else {
            fillC(theme["cardBg"])
            roundedRect(dropX, dropY, dropW, dropH, 8)
            let thumbX = dropX + 12
            let thumbY = dropY + 12
            let thumbSize = dropH - 24
            if initImagePreview != null {
                image(initImagePreview, thumbX, thumbY, thumbSize, thumbSize, "fit")
            }
            let infoX = thumbX + thumbSize + 14
            fillC(theme["mainLabel"])
            say(initImageLabel, infoX, dropY + 14, 0.46)
            if button("Use last gen", infoX, dropY + 42, 110, 26, 0.4) {
                useLastGenAsInit()
            }
            if button("Clear", infoX + 118, dropY + 42, 70, 26, 0.4) {
                clearInitImage()
            }
        }
        cy = cy + dropH + 12

        // Denoising strength slider — 0.0 means ignore the prompt entirely
        // (just return the source); 1.0 means ignore the source entirely
        // (pure txt2img). 0.5 is the sweet spot for "keep the composition,
        // change the style/details" iteration.
        denoiseStrength = slider("Denoising: " + str(denoiseStrength) +
                                 "  (0.2 subtle · 0.5 mid · 0.8 mostly ignore source)",
                                 panelX + 20, cy + 16, panelW - 40,
                                 denoiseStrength, 0.0, 1.0, 0.42)
        cy = cy + 52
    }

    // ── Generate ──────────────────────────────────────────────────────────
    if !generating {
        if button("Generate with " + providerLabel(activeProvider),
                  panelX + 20, cy, panelW - 40, 38, 0.5) {
            startGeneration()
        }
    } else {
        fillC(theme["cardBg"])
        roundedRect(panelX + 20, cy, panelW - 40, 38, 6)
        fillC(theme["subtitleText"])
        let lbl = "generating…"
        let tw = textWidth(uiFont, lbl, 0.5)
        say(lbl, panelX + 20 + (panelW - 40 - tw) / 2, cy + 12, 0.5)
    }
    cy = cy + 54

    // ── Load image (post-generation alternative) ─────────────────────────
    // Bring in any existing RGBA image from disk for adjusting. Drag-drop
    // onto the window also routes here when img2img is off.
    fillC(theme["mainLabel"])
    say("Load image (or drag onto window):", panelX + 20, cy, 0.42)
    cy = cy + 18
    let loadBtnW   = 76
    let loadInputW = panelW - 40 - loadBtnW - 8
    loadPathInput = textInput("", loadPathInput,
        panelX + 20, cy, loadInputW, 30, 0.42)
    if button("Load", panelX + 20 + loadInputW + 8, cy, loadBtnW, 30, 0.42) {
        if len(trim(loadPathInput)) > 0 {
            loadMainImageFromPath(loadPathInput)
        }
    }
    cy = cy + 36
    if len(loadError) > 0 {
        fillC(theme["crit"])
        cy = drawWrapped("✗ " + loadError, panelX + 20, cy, 70, 0.40)
        cy = cy + 4
    }

    // Anchor Status to the bottom of the left panel with a small floor pad.
    // The History block that used to live here has been removed; Status is
    // the only thing in this region now, so we float it down instead of
    // letting it stick to whatever cy happened to land at.
    let statusFloorPad = 18
    let statusBlockH   = 14 + 24              // separator gap + status row (always present)
    if lastBytes > 0          { statusBlockH = statusBlockH + 20 }
    if len(errorMessage) > 0  { statusBlockH = statusBlockH + 22 + 56 }   // header + ~3 wrapped lines
    cy = panelY + panelH - statusBlockH - statusFloorPad

    drawSep(panelX + 20, cy, panelW - 40)
    cy = cy + 14

    // ── Status ────────────────────────────────────────────────────────────
    // Leading dot encodes state at a glance: green=done, amber=working,
    // red=error, faint=idle. Amber pulses gently using frameCount() so
    // long generations don't feel frozen.
    let dotX = panelX + 24
    let dotY = cy + 8
    if generating {
        let a = 0.55 + 0.35 * sin(float(frameCount()) / 12.0)
        fill(0.95, 0.65, 0.10, a)
    } else if len(errorMessage) > 0 {
        fill(0.90, 0.22, 0.20, 1.0)
    } else if lastBytes > 0 {
        fill(0.30, 0.85, 0.42, 1.0)
    } else {
        fill(0.45, 0.45, 0.52, 0.6)
    }
    noStroke()
    circle(dotX, dotY, 4)
    fillC(theme["mainLabel"])
    say("Status", panelX + 36, cy, 0.53)
    fillC(theme["subtitleText"])
    let tw = textWidth(uiFont, status, 0.41)
    say(status, panelX + panelW - 20 - tw, cy + 4, 0.41)
    cy = cy + 24
    if lastBytes > 0 {
        fillC(theme["subtitleText"])
        let sizeKB = lastBytes / 1024
        let meta = "last image · " + str(sizeKB) + " KB · " + str(lastElapsed) + " ms"
        say(meta, panelX + 20, cy, 0.41)
        cy = cy + 20
    }
    if len(errorMessage) > 0 {
        fillC(theme["crit"])
        say("Error", panelX + 20, cy, 0.53)
        cy = cy + 22
        fillC(theme["subtitleText"])
        cy = drawWrapped(errorMessage, panelX + 20, cy, 58, 0.41)
        cy = cy + 4
    }

    // ── Right side: tabbed area (Image / Chat) ────────────────────────────
    let rightX = panelX + panelW + margin
    let rightY = margin
    let rightW = W - rightX - margin
    let rightH = H - margin * 2

    rightTab = tabs(rightX, rightY, rightW,
        ["Image", "Adjust", "Chat with " + providerLabel(activeChatBackend)],
        rightTab, 0.5)

    let contentY = rightY + 44
    let contentH = rightH - 44

    if rightTab == 0 {
        drawImageTab(rightX, contentY, rightW, contentH)
    } else if rightTab == 1 {
        drawAdjustTab(rightX, contentY, rightW, contentH)
    } else {
        drawChatTab(rightX, contentY, rightW, contentH)
    }
}


// ── Adjust panel ────────────────────────────────────────────────────
//
// Wraps stdlib/mtl_fx.lex behind a slider UI. The panel renders a live
// preview of the current filter chain on top of the original generated
// image (lastImageBytes is the immutable source, lastImageW/H its
// dimensions). On macOS the filter chain runs on the GPU at sub-ms per
// filter so every slider move re-runs the chain at interactive rates;
// on Linux/Windows the Go-backed CPU path takes 5-20 ms per filter.
//
// `Apply` bakes the chain into a new currentImage + lastImageBytes pair
// (non-destructive in the sense that the user has already saved the
// original elsewhere, or can `Reset` before saving — there is no undo
// stack yet). `Reset` zeros every slider without touching the source.

// resetAdjust clears every slider back to its no-op default and drops
// the preview cache. Called by drawAdjustTab's Reset button and on
// every new successful generation.
fn resetAdjust() {
    adjExposure   = 0.0
    adjBrightness = 0.0
    adjContrast   = 0.0
    adjGamma      = 1.0
    adjSaturation = 0.0
    adjHue        = 0.0
    adjVignette   = 0.0
    adjSepia      = 0.0
    adjInvert     = false
    adjDesaturate = false
    adjShowMore   = false
    adjPreviewImage       = null
    adjPreviewFingerprint = ""
}

// _adjFingerprint returns a string snapshot of every slider value.
// When this changes between frames, the preview is stale and we
// re-run the chain. Using a string keeps comparison cheap (string ==).
fn _adjFingerprint() {
    return str(adjExposure) + "|" + str(adjBrightness) + "|" +
           str(adjContrast) + "|" + str(adjGamma) + "|" +
           str(adjSaturation) + "|" + str(adjHue) + "|" +
           str(adjVignette) + "|" + str(adjSepia) + "|" +
           str(adjInvert) + "|" + str(adjDesaturate)
}

// _adjIsNoOp returns true when every slider sits at its default value.
// No-op chains skip the filter pass entirely and render the source
// directly so we don't pay a needless GPU dispatch each frame.
fn _adjIsNoOp() {
    return adjExposure   == 0.0 &&
           adjBrightness == 0.0 &&
           adjContrast   == 0.0 &&
           adjGamma      == 1.0 &&
           adjSaturation == 0.0 &&
           adjHue        == 0.0 &&
           adjVignette   == 0.0 &&
           adjSepia      == 0.0 &&
           !adjInvert    && !adjDesaturate
}

// _runAdjustChain applies every active filter in a fixed canonical
// order:  exposure → brightness → contrast → gamma → saturation →
// hueShift → sepia → desaturate → invert → vignette.
//
// Order matters: tonal adjustments (exposure/brightness/contrast/gamma)
// must come before colour transforms (saturation/hue), and spatial
// effects (vignette) come last so they apply over everything else.
// Filters whose slider is at its default value are skipped entirely.
fn _runAdjustChain(src, w, h) {
    let out = src
    let err = null
    if adjExposure != 0.0 {
        out, err = fx.exposure(out, w, h, adjExposure)
        if err != null { return null, err }
    }
    if adjBrightness != 0.0 {
        out, err = fx.brightness(out, w, h, adjBrightness)
        if err != null { return null, err }
    }
    if adjContrast != 0.0 {
        out, err = fx.contrast(out, w, h, adjContrast)
        if err != null { return null, err }
    }
    if adjGamma != 1.0 {
        out, err = fx.gamma(out, w, h, adjGamma)
        if err != null { return null, err }
    }
    if adjSaturation != 0.0 {
        out, err = fx.saturation(out, w, h, adjSaturation)
        if err != null { return null, err }
    }
    if adjHue != 0.0 {
        out, err = fx.hueShift(out, w, h, adjHue)
        if err != null { return null, err }
    }
    if adjSepia != 0.0 {
        out, err = fx.sepia(out, w, h, adjSepia)
        if err != null { return null, err }
    }
    if adjDesaturate {
        out, err = fx.desaturate(out, w, h)
        if err != null { return null, err }
    }
    if adjInvert {
        out, err = fx.invert(out, w, h)
        if err != null { return null, err }
    }
    if adjVignette != 0.0 {
        out, err = fx.vignette(out, w, h, adjVignette, 1.0)
        if err != null { return null, err }
    }
    return out, null
}

// _refreshAdjustPreview rebuilds adjPreviewImage whenever the slider
// fingerprint has changed since the last build. No-op chains clear
// the cache so drawAdjustTab falls back to currentImage directly.
fn _refreshAdjustPreview() {
    if adjSourceRgba == null || lastImageW == 0 || lastImageH == 0 {
        adjPreviewImage = null
        adjPreviewFingerprint = ""
        return
    }
    let fp = _adjFingerprint()
    if fp == adjPreviewFingerprint && adjPreviewImage != null { return }
    if _adjIsNoOp() {
        adjPreviewImage = null
        adjPreviewFingerprint = fp
        return
    }
    let out, err = _runAdjustChain(adjSourceRgba, lastImageW, lastImageH)
    if err != null { return }
    // The filter chain returns raw RGBA pixels off the Metal surface
    // (or the Go CPU fallback) — go straight to imageFromRgba, NOT
    // loadImage, which expects PNG/JPEG-encoded bytes.
    adjPreviewImage = imageFromRgba(out, lastImageW, lastImageH)
    adjPreviewFingerprint = fp
}

// applyAdjust bakes the current filter chain into lastImageBytes /
// currentImage. After this call the chain is reset; the just-applied
// filters become part of the new "source" image and the user can
// stack further adjustments on top.
fn applyAdjust() {
    if adjSourceRgba == null || lastImageW == 0 { return }
    if _adjIsNoOp() { return }
    let out, err = _runAdjustChain(adjSourceRgba, lastImageW, lastImageH)
    if err != null { return }
    // The adjusted RGBA becomes the new currentImage AND the new
    // adjSourceRgba so further sliders stack on top. lastImageBytes
    // stays as the original PNG so Attach/Save still work — Save
    // reads pixels from currentImage anyway, and Attach gets the
    // pristine original (a stacking-on-top product is one Save away
    // from being attachable).
    currentImage   = imageFromRgba(out, lastImageW, lastImageH)
    adjSourceRgba  = out
    resetAdjust()
}


// drawAdjustTab renders the Adjust panel. Layout:
//   ┌─────────────────────────────────────┐
//   │  [ live preview ]                   │   ~50% h
//   ├─────────────────────────────────────┤
//   │  Exposure  ────●────  +0.5          │
//   │  Contrast  ────●────  +0.3          │
//   │  ...                                 │
//   │  [ More… ] toggles advanced sliders │
//   │  [ Reset ]      [ Apply ]            │
//   └─────────────────────────────────────┘
fn drawAdjustTab(x, y, w, h) {
    drawPanel(x, y, w, h)

    if currentImage == null {
        fillC(theme["subtitleText"])
        let msg = "Generate an image first — sliders activate once there's something to adjust."
        let tw = textWidth(uiFont, msg, 0.45)
        say(msg, x + (w - tw) / 2, y + h / 2 - 10, 0.45)
        return
    }

    _refreshAdjustPreview()
    let showImg = adjPreviewImage
    if showImg == null { showImg = currentImage }

    // Preview area sits in the top half. We keep a small inset so the
    // panel border breathes around the image.
    let inset = 14
    let previewH = (h * 50) / 100
    image(showImg, x + inset, y + inset, w - inset * 2, previewH - inset * 2, "fit")

    // Sliders region starts below the preview.
    let sx = x + 18
    let sw = w - 36
    let sy = y + previewH + 6

    // Slider row helper inlined — fillC(label colour) + slider() per row.
    fillC(theme["mainLabel"])

    say("Exposure  " + str(adjExposure) + " stops", sx, sy, 0.42)
    adjExposure = slider("", sx, sy + 16, sw, adjExposure, -3.0, 3.0, 0.42)
    sy = sy + 38

    say("Contrast  " + str(adjContrast), sx, sy, 0.42)
    adjContrast = slider("", sx, sy + 16, sw, adjContrast, -1.0, 1.0, 0.42)
    sy = sy + 38

    say("Saturation  " + str(adjSaturation), sx, sy, 0.42)
    adjSaturation = slider("", sx, sy + 16, sw, adjSaturation, -1.0, 1.0, 0.42)
    sy = sy + 38

    say("Hue shift  " + str(adjHue) + "°", sx, sy, 0.42)
    adjHue = slider("", sx, sy + 16, sw, adjHue, -180.0, 180.0, 0.42)
    sy = sy + 38

    say("Vignette  " + str(adjVignette), sx, sy, 0.42)
    adjVignette = slider("", sx, sy + 16, sw, adjVignette, 0.0, 1.0, 0.42)
    sy = sy + 38

    say("Sepia  " + str(adjSepia), sx, sy, 0.42)
    adjSepia = slider("", sx, sy + 16, sw, adjSepia, 0.0, 1.0, 0.42)
    sy = sy + 38

    // More… expander gates four less-common knobs. The 'More…' label
    // flips to 'Hide' once expanded so the affordance is unambiguous.
    let moreLabel = "More…"
    if adjShowMore { moreLabel = "Hide advanced" }
    if button(moreLabel, sx, sy, 140, 26, 0.4) {
        adjShowMore = !adjShowMore
    }
    sy = sy + 34

    if adjShowMore {
        say("Brightness  " + str(adjBrightness), sx, sy, 0.42)
        adjBrightness = slider("", sx, sy + 16, sw, adjBrightness, -1.0, 1.0, 0.42)
        sy = sy + 38

        say("Gamma  " + str(adjGamma), sx, sy, 0.42)
        adjGamma = slider("", sx, sy + 16, sw, adjGamma, 0.4, 2.5, 0.42)
        sy = sy + 38

        // Two toggles on a single row — fit two checkboxes side by side.
        adjInvert     = checkbox("Invert",     sx,            sy, adjInvert,     0.42)
        adjDesaturate = checkbox("Desaturate", sx + sw / 2,   sy, adjDesaturate, 0.42)
        sy = sy + 28
    }

    // Action row floats to the bottom of the panel so it doesn't shift
    // when More… toggles on/off.
    let btnY = y + h - 44
    if button("Reset",        sx,                       btnY, sw / 2 - 8, 30, 0.46) {
        resetAdjust()
    }
    if button("Apply changes", sx + sw / 2 + 8,         btnY, sw / 2 - 8, 30, 0.46) {
        applyAdjust()
    }
}


// ── Right-panel tabs ───────────────────────────────────────────────────────

fn drawImageTab(x, y, w, h) {
    let saveStripH = 0
    if currentImage != null { saveStripH = 96 }

    drawPanel(x, y, w, h - saveStripH)

    if currentImage != null {
        let inset = 18
        image(currentImage, x + inset, y + inset, w - inset * 2, h - saveStripH - inset * 2, "fit")
    } else {
        let msg = "Type a prompt on the left and press Generate."
        if generating {
            // Provider-specific waiting copy. Local SD is fastest (no
            // queue, just MPS sampling time). AI Horde anonymous queues
            // are still the slow tail. Others are roughly a few seconds.
            if      activeProvider == "a1111"       { msg = "generating locally on " + providerLabel(activeProvider) + " — usually 30-90s on MPS." }
            else if activeProvider == "aihorde"     { msg = "generating image — AI Horde queue can take 30s to 3 minutes." }
            else if activeProvider == "huggingface" { msg = "generating image via Hugging Face — usually a few seconds." }
            else if activeProvider == "openai"      { msg = "generating image via OpenAI — usually a few seconds." }
            else                                    { msg = "generating image…" }
        }
        // While generating, the rest of the panel can't show meaningful
        // progress (we don't poll /sdapi/v1/progress yet) — a chase
        // spinner above the status message gives a visible "still
        // working" cue so the panel doesn't read as frozen.
        if generating {
            let spinR = 28
            let cx    = x + w / 2
            let cy    = y + h / 2 - 30
            drawSpinner(cx, cy, spinR)
            fillC(theme["subtitleText"])
            let tw = textWidth(uiFont, msg, 0.5)
            say(msg, x + (w - tw) / 2, cy + spinR + 22, 0.5)
        } else {
            fillC(theme["subtitleText"])
            let tw = textWidth(uiFont, msg, 0.5)
            say(msg, x + (w - tw) / 2, y + h / 2 - 10, 0.5)
        }
    }

    if currentImage != null {
        let stripY = y + h - saveStripH + 8
        drawPanel(x, stripY, w, saveStripH - 8)
        fillC(theme["mainLabel"])
        say("Save as", x + 18, stripY + 12, 0.45)
        saveBasename = textInput("", saveBasename, x + 18, stripY + 34, 240, 30, 0.45)
        fillC(theme["subtitleText"])
        // The basename above is appended with .png or .jpg by the Save
        // buttons. No auto-increment — clicking Save twice overwrites the
        // same file. Rename in the textbox to keep multiple iterations.
        let suffix = ".png  /  .jpg"
        say(suffix, x + 268, stripY + 42, 0.41)
        let btnY = stripY + 34
        if button("Save PNG", x + 18 + 240 + 150, btnY, 115, 30, 0.45) { doSave("png") }
        if button("Save JPG", x + 18 + 240 + 275, btnY, 115, 30, 0.45) { doSave("jpg") }
        if len(lastSavedTo) > 0 {
            fillC(theme["low"])
            say("✓ saved to " + lastSavedTo, x + 18, stripY + 72, 0.41)
        }
        if len(saveError) > 0 {
            fillC(theme["crit"])
            drawWrapped("✗ " + saveError, x + 18, stripY + 72, 90, 0.41)
        }
    }
}

// _toolInputSummary returns a compact one-line representation of a tool's
// input hash for display in the chat. Shows the first key/value pair.
fn _toolInputSummary(input) {
    let ks = keys(input)
    if len(ks) == 0 { return "\{\}" }
    // keys() iteration order is randomised every call (kLex hashes are
    // backed by Go maps). Sorting first gives a stable, deterministic
    // summary line — without this the input line flickers every frame
    // as the displayed first-key rotates. Also: show ALL key/value
    // pairs joined; the 64-char truncation upstream handles overflow.
    ks = sort(ks)
    let parts = makeArray(len(ks), "")
    let i = 0
    while i < len(ks) {
        parts[i] = ks[i] + ": " + str(input[ks[i]])
        i = i + 1
    }
    return join(parts, ", ")
}

const _TOOL_ENTRY_H = 64

fn drawToolEntry(entry, x, y, w) {
    let h = _TOOL_ENTRY_H
    _shadow(x, y, w, h, 6)
    fillC(theme["cardBg"])
    noStroke()
    roundedRect(x, y, w, h, 6)
    _topHighlight(x, y, w)
    if entry["isError"] {
        fillC(theme["crit"])
    } else {
        fillC(theme["accentBar"])
    }
    roundedRect(x, y, 3, h, 0)

    fillC(theme["accentBar"])
    let icon = "⚙ "
    if entry["isError"] { icon = "⚠ " }
    say(icon + entry["name"], x + 12, y + 8, 0.46)

    fillC(theme["subtitleText"])
    let inp = _toolInputSummary(entry["input"])
    if len(inp) > 64 { inp = substr(inp, 0, 61) + "..." }
    say(inp, x + 12, y + 28, 0.4)

    if entry["isError"] { fillC(theme["crit"]) } else { fillC(theme["low"]) }
    let res = entry["result"]
    if type(res) != "STRING" { res = str(res) }
    res = replace(res, "\n", " ⏎ ")
    if len(res) > 78 { res = substr(res, 0, 75) + "..." }
    say("→ " + res, x + 12, y + 46, 0.4)
}

fn drawApprovalCard(x, y, w, h, pt) {
    fill(0.18, 0.10, 0.04, 1.0)
    strokeC(theme["high"])
    strokeWeight(1)
    roundedRect(x + 6, y + 4, w - 12, h - 8, 8)

    fillC(theme["high"])
    say("⚠ " + providerLabel(activeChatBackend) + " wants to run: " + pt["name"], x + 18, y + 14, 0.5)

    fillC(theme["subtitleText"])
    let inputStr = _toolInputSummary(pt["input"])
    if len(inputStr) > 90 { inputStr = substr(inputStr, 0, 87) + "..." }
    say(inputStr, x + 18, y + 38, 0.42)

    let btnY = y + h - 44
    if button("Approve", x + 18,  btnY, 110, 34, 0.5) { approvePendingTool() }
    if button("Deny",    x + 138, btnY, 110, 34, 0.5) { denyPendingTool() }
}

// Markdown render constants — must be declared above drawChatTab and
// every other function that references them. Under --vm a function's
// free-name references are resolved at compile time, so a forward
// reference to a const declared further down the file fails with
// "undefined name". Keep this block above the first user.
const _MD_LINE_H            = 17
const _MD_PARA_GAP          = 6
const _MD_CODE_PAD          = 12
const _MD_LIST_INDENT       = 16
const _MD_PROSE_SCALE       = 0.46
const _MD_INLINE_CODE_SCALE = 0.46
const _MD_CODE_BLOCK_SCALE  = 0.42

fn drawChatTab(x, y, w, h) {
    let inputH = 88
    if agentPendingTool != null { inputH = 110 }

    // Thin toolbar above the conversation panel: session cost on the left,
    // Cancel + Clear on the right. Tucked into the top 26px so it doesn't
    // steal much vertical space.
    let toolbarH = 26
    fillC(theme["subtitleText"])
    let totalTok = sessionInTokens + sessionOutTokens
    if activeChatBackend == "ollama" {
        // Ollama is local — no per-token cost. Just show token totals.
        let costStr = "session · " + str(totalTok) + " tok · local (free)"
    } else {
        let cost = _estimateCost(config["claude"]["model"], sessionInTokens, sessionOutTokens)
        let costStr = "session · " + str(totalTok) + " tok · $" +
            substr(str(cost + 0.0001), 0, 6)
    }
    say(costStr, x + 6, y + 6, 0.4)

    // Cancel button — only visible while a turn is in flight.
    let btnRight = x + w
    if chatSending || agentPendingTool != null {
        if button("Cancel", btnRight - 80, y + 0, 70, 22, 0.42) { cancelChat() }
        btnRight = btnRight - 86
    }
    if len(chatMessages) > 0 || len(agentMessages) > 0 {
        if button("Clear", btnRight - 70, y + 0, 64, 22, 0.42) { clearChat() }
    }

    drawPanel(x, y + toolbarH, w, h - inputH - toolbarH)

    // ── Conversation area ─────────────────────────────────────────────────
    // Bottom-anchored render with pixel scroll offset. chatScroll is the
    // number of pixels lifted above the bottom — 0 = newest at bottom,
    // maxScroll = oldest at top. Mouse wheel adjusts chatScroll; sending
    // or receiving a message resets it to 0 (pin to bottom).
    let sbReserve = 12
    let innerX  = x + 18
    let innerW  = w - 36 - sbReserve
    let bubblePad = 10
    let gap     = 12
    let regionTop    = y + toolbarH + 14
    let regionBottom = y + h - inputH - 14
    let regionH      = regionBottom - regionTop

    if len(chatMessages) == 0 {
        fillC(theme["subtitleText"])
        let backendName = providerLabel(activeChatBackend)
        let msg = "Chat with " + backendName + ". Configure in Settings → " + backendName + "."
        if activeChatBackend == "claude" && len(config["claude"]["api_key"]) == 0 {
            msg = "⚠  Add your Anthropic API key in Settings → Claude to start chatting."
        }
        if activeChatBackend == "ollama" {
            let base = config["ollama"]["base_url"]
            if base == null || len(base) == 0 {
                msg = "⚠  Configure the Ollama base URL in Settings → Ollama to start chatting."
            }
        }
        let tw = textWidth(uiFont, msg, 0.46)
        say(msg, x + (w - tw) / 2, y + (h - inputH) / 2 - 8, 0.46)
    } else {
        // Pass 1 — measure every entry, hitting chatRenderCache where we
        // can. Tool entries are fixed-height; user/assistant bubbles use
        // the compiled-segments cache so we only re-parse/tokenise when
        // content length or render width has actually changed.
        let msgN = len(chatMessages)
        let msgSegs    = makeArray(msgN, null)
        let msgHeights = makeArray(msgN, 0)

        let bubbleMaxW = innerW - bubblePad * 2
        if chatRenderCacheWidth != bubbleMaxW {
            // Width changed → every cached layout is stale. Drop the lot.
            chatRenderCache      = makeArray(0)
            chatRenderCacheWidth = bubbleMaxW
        }
        // Grow the cache parallel array if chatMessages has new entries.
        if len(chatRenderCache) < msgN {
            let grown = makeArray(msgN, null)
            let ci = 0
            let cn = len(chatRenderCache)
            while ci < cn {
                grown[ci] = chatRenderCache[ci]
                ci = ci + 1
            }
            chatRenderCache = grown
        }

        let totalH = 0
        let i = 0
        while i < msgN {
            let m = chatMessages[i]
            if m["role"] == "tool" {
                msgHeights[i] = _TOOL_ENTRY_H
            } else {
                let contentLen = len(m["content"])
                let ce = chatRenderCache[i]
                if ce != null && ce["len"] == contentLen {
                    msgSegs[i]    = ce["segs"]
                    msgHeights[i] = ce["height"] + bubblePad * 2 + 20
                } else {
                    let compiled = compileSegs(m["content"], bubbleMaxW)
                    msgSegs[i]    = compiled["segs"]
                    msgHeights[i] = compiled["height"] + bubblePad * 2 + 20
                    chatRenderCache[i] = {
                        "len":    contentLen,
                        "segs":   compiled["segs"],
                        "height": compiled["height"],
                    }
                }
            }
            totalH = totalH + msgHeights[i] + gap
            i = i + 1
        }
        if totalH > 0 { totalH = totalH - gap }      // trim trailing gap

        let maxScroll = totalH - regionH
        if maxScroll < 0 { maxScroll = 0 }

        // Mouse wheel — only when pointer is inside the conversation region.
        let scrollDelta = mouseScrollY()
        if scrollDelta != 0.0 {
            let mx = mouseX()
            let my = mouseY()
            if mx >= innerX && mx <= innerX + innerW + sbReserve &&
               my >= regionTop && my <= regionBottom {
                chatScroll = chatScroll + int(scrollDelta * 30.0)
                if chatScroll < 0          { chatScroll = 0 }
                if chatScroll > maxScroll  { chatScroll = maxScroll }
            }
        }
        // Clamp again — maxScroll may have shrunk if a message was deleted.
        if chatScroll > maxScroll { chatScroll = maxScroll }
        if chatScroll < 0         { chatScroll = 0 }

        // Pass 2 — render top-down with clipping. cy0 is the y of the first
        // message; when chatScroll=0 it sits so the LAST message bottom
        // touches regionBottom. Scrolling up moves cy0 down.
        let cy0 = regionBottom - totalH + chatScroll
        pushClip(innerX, regionTop, innerW + sbReserve, regionH)

        let cy = cy0
        i = 0
        while i < msgN {
            let blockH = msgHeights[i]
            if cy + blockH < regionTop {
                cy = cy + blockH + gap
                i = i + 1
                continue
            }
            if cy > regionBottom { i = msgN   continue }   // break

            let m = chatMessages[i]
            if m["role"] == "tool" {
                drawToolEntry(m, innerX, cy, innerW)
            } else {
                let segs = msgSegs[i]
                let isUser = m["role"] == "user"
                let bubbleX = innerX
                let bubbleW = innerW
                _shadow(bubbleX, cy, bubbleW, blockH, 8)
                if isUser {
                    fillC(theme["wAccentBg"])
                } else {
                    fillC(theme["cardBg"])
                }
                roundedRect(bubbleX, cy, bubbleW, blockH, 8)
                _topHighlight(bubbleX, cy, bubbleW)
                if isUser {
                    fillC(theme["wAccent"])
                    say("You", bubbleX + bubblePad, cy + 6, 0.4)
                } else {
                    fillC(theme["accentBar"])
                    say(providerLabel(activeChatBackend), bubbleX + bubblePad, cy + 6, 0.4)
                }
                if hasKey(m, "isError") && m["isError"] {
                    fillC(theme["crit"])
                    drawWrappedPx(m["content"],
                                  bubbleX + bubblePad,
                                  cy + bubblePad + 16,
                                  innerW - bubblePad * 2,
                                  _MD_PROSE_SCALE, _MD_LINE_H)
                } else {
                    fillC(theme["cardText"])
                    drawSegments(segs,
                                 bubbleX + bubblePad,
                                 cy + bubblePad + 16,
                                 innerW - bubblePad * 2)
                }
                // Copy button — assistant bubbles only. Sits in the bubble's
                // bottom-right corner; flashes "✓ Copied" briefly on click.
                if !isUser {
                    let copyLabel = "Copy"
                    if copyFlashIdx == i && elapsedTime() - copyFlashAt < 2.0 {
                        copyLabel = "✓ Copied"
                    }
                    let cbX = bubbleX + bubbleW - 68
                    let cbY = cy + blockH - 22
                    if button(copyLabel, cbX, cbY, 60, 18, 0.36) {
                        copyToClipboard(m["content"])
                        copyFlashIdx = i
                        copyFlashAt  = elapsedTime()
                    }
                }
            }
            cy = cy + blockH + gap
            i = i + 1
        }

        popClip()

        // Scrollbar — drawn after the clipped content so it always shows.
        if maxScroll > 0 {
            let sbX = innerX + innerW + 4
            let sbW = 6
            let thumbH = int(float(regionH) * float(regionH) / float(totalH))
            if thumbH < 24 { thumbH = 24 }
            // 0 → thumb at bottom, maxScroll → thumb at top.
            let travel = regionH - thumbH
            let thumbOff = int(float(chatScroll) * float(travel) / float(maxScroll))
            let thumbY = regionBottom - thumbH - thumbOff
            fillC(theme["wTrack"])
            roundedRect(sbX, regionTop, sbW, regionH, sbW / 2)
            fillC(theme["wHandle"])
            roundedRect(sbX, thumbY, sbW, thumbH, sbW / 2)
        }
    }

    // ── Input strip OR approval card (when a tool is pending) ────────────
    let inputY = y + h - inputH + 4
    if agentPendingTool != null {
        drawApprovalCard(x, inputY, w, inputH - 4, agentPendingTool)
    } else {
        drawPanel(x, inputY, w, inputH - 4)

        // Top row: either the "Message" label, or — when there's a generated
        // image — Attach toggle + Critique quick-button. Click Attach to
        // include the current image with the next message; click Critique
        // to send a structured "what's good/bad" prompt with image attached
        // in one tap.
        if currentImage != null {
            let attachLbl = "📷 Attach image"
            if attachImage { attachLbl = "✓ image attached" }
            if button(attachLbl, x + 18, inputY + 4, 150, 22, 0.4) {
                attachImage = !attachImage
            }
            if button("Critique it", x + 174, inputY + 4, 110, 22, 0.4) {
                attachImage = true
                chatDraft = "Take a look at this image. What works, and what could be improved? Comment on composition, lighting, subject, and style. If you'd suggest a refined prompt, give me one I can use as-is."
                sendChat()
            }
        } else {
            fillC(theme["mainLabel"])
            say("Message", x + 18, inputY + 10, 0.42)
        }

        let btnW = 110
        let fieldW = w - 36 - btnW - 12
        let chatInputId = uiNextFieldID()
        chatDraft = textInput("", chatDraft, x + 18, inputY + 30, fieldW, 32, 0.5)
        // Re-apply focus on the frame *after* Send was clicked / Enter was
        // pressed. We use a deferred flag (not an inline uiSetFocus at the
        // call site) because the chat input's positional ID shifts as soon
        // as chatSending=true causes the Cancel button to appear — the ID
        // captured during the Send frame would point at the Cancel button
        // by the next frame. The flag lets us re-focus against THIS frame's
        // freshly computed chatInputId, which is always correct.
        if chatRefocusPending {
            uiSetFocus(chatInputId)
            chatRefocusPending = false
        }
        // Enter while focused = send. Scoped to this specific textInput so
        // Enter elsewhere (the prompt textArea, settings fields) is unaffected.
        if uiGetFocus() == chatInputId && keyPressed("ENTER") &&
           !chatSending && len(trim(chatDraft)) > 0 {
            sendChat()
        }
        let btnLabel = "Send"
        if chatSending { btnLabel = "…" }
        let canSend = !chatSending && len(trim(chatDraft)) > 0
        if canSend {
            if button(btnLabel, x + 18 + fieldW + 12, inputY + 30, btnW, 32, 0.5) {
                sendChat()
            }
        } else {
            fillC(theme["cardBg"])
            roundedRect(x + 18 + fieldW + 12, inputY + 30, btnW, 32, 6)
            fillC(theme["subtitleText"])
            let tw = textWidth(uiFont, btnLabel, 0.5)
            say(btnLabel, x + 18 + fieldW + 12 + (btnW - tw) / 2, inputY + 30 + 8, 0.5)
        }
    }
}


// ── Pixel-precise word-wrap helpers (used by chat bubbles) ────────────────
//
// drawWrapped (top of file) works in character counts which is fine for the
// crammed left-panel sidebar but is wrong for a chat bubble at variable
// widths. These versions measure with textWidth so wrapping happens exactly
// at the bubble edge.

fn countWrappedLines(s, maxW, scale) {
    let words = split(s, " ")
    let wi = 0
    let wn = len(words)
    let count = 0
    while wi < wn {
        let line = words[wi]
        wi = wi + 1
        while wi < wn && textWidth(uiFont, line + " " + words[wi], scale) <= maxW {
            line = line + " " + words[wi]
            wi = wi + 1
        }
        count = count + 1
    }
    if count == 0 { return 1 }
    return count
}

fn drawWrappedPx(s, x, y, maxW, scale, lineH) {
    let words = split(s, " ")
    let wi = 0
    let wn = len(words)
    let cy = y
    while wi < wn {
        let line = words[wi]
        wi = wi + 1
        while wi < wn && textWidth(uiFont, line + " " + words[wi], scale) <= maxW {
            line = line + " " + words[wi]
            wi = wi + 1
        }
        say(line, x, cy, scale)
        cy = cy + lineH
    }
    return cy
}


// ── Lightweight markdown for chat ────────────────────────────────────────
//
// Two passes:
//   parseMarkdown(s)  → block-level segments (paragraphs, code blocks, lists)
//   parseInline(s)    → inline runs for any non-code text
//
// Block segment shapes:
//   {"kind": "para", "runs": [...inline runs...]}
//   {"kind": "code", "lang": "python", "lines": ["line1", "line2"]}
//   {"kind": "list", "items": [[runs], [runs], ...]}
//
// Inline run shapes:
//   {"kind": "text",   "content": "..."}
//   {"kind": "bold",   "content": "..."}
//   {"kind": "italic", "content": "..."}
//   {"kind": "code",   "content": "..."}
//
// Bold and italic emphasise via colour (no bold/italic font is loaded).
// Inline code uses a tinted background pill in the same font. Code blocks
// switch to kLex's embedded monospace via text() so code is visually
// distinct from prose.

fn _findIdx(s, needle, from) {
    let n = len(s)
    let nl = len(needle)
    if nl == 0 || from < 0 || from + nl > n { return -1 }
    let i = from
    while i + nl <= n {
        if substr(s, i, i + nl) == needle { return i }
        i = i + 1
    }
    return -1
}

fn parseInline(s) {
    let runs = makeArray(0)
    let n = len(s)
    let i = 0
    let buf = ""
    while i < n {
        let c = substr(s, i, i + 1)
        // ** bold
        if c == "*" && i + 1 < n && substr(s, i + 1, i + 2) == "*" {
            let end = _findIdx(s, "**", i + 2)
            if end >= 0 {
                if len(buf) > 0 { runs = concat(runs, [{"kind": "text", "content": buf}])  buf = "" }
                runs = concat(runs, [{"kind": "bold", "content": substr(s, i + 2, end)}])
                i = end + 2
                continue
            }
        }
        // * italic (single)
        if c == "*" {
            let end = _findIdx(s, "*", i + 1)
            if end >= 0 {
                if len(buf) > 0 { runs = concat(runs, [{"kind": "text", "content": buf}])  buf = "" }
                runs = concat(runs, [{"kind": "italic", "content": substr(s, i + 1, end)}])
                i = end + 1
                continue
            }
        }
        // ` inline code
        if c == "`" {
            let end = _findIdx(s, "`", i + 1)
            if end >= 0 {
                if len(buf) > 0 { runs = concat(runs, [{"kind": "text", "content": buf}])  buf = "" }
                runs = concat(runs, [{"kind": "code", "content": substr(s, i + 1, end)}])
                i = end + 1
                continue
            }
        }
        buf = buf + c
        i = i + 1
    }
    if len(buf) > 0 { runs = concat(runs, [{"kind": "text", "content": buf}]) }
    if len(runs) == 0 { runs = [{"kind": "text", "content": ""}] }
    return runs
}

fn parseMarkdown(s) {
    let segments = makeArray(0)
    let lines = split(s, "\n")
    let n = len(lines)
    let i = 0
    let paraBuf = ""
    let listBuf = makeArray(0)

    while i < n {
        let line = lines[i]
        // Code fence
        if len(line) >= 3 && substr(line, 0, 3) == "```" {
            if len(paraBuf) > 0 {
                segments = concat(segments, [{"kind": "para", "runs": parseInline(paraBuf)}])
                paraBuf = ""
            }
            if len(listBuf) > 0 {
                segments = concat(segments, [{"kind": "list", "items": listBuf}])
                listBuf = makeArray(0)
            }
            let lang = trim(substr(line, 3))
            let codeLines = makeArray(0)
            i = i + 1
            while i < n && (len(lines[i]) < 3 || substr(lines[i], 0, 3) != "```") {
                codeLines = concat(codeLines, [lines[i]])
                i = i + 1
            }
            segments = concat(segments, [{"kind": "code", "lang": lang, "lines": codeLines}])
            if i < n { i = i + 1 }
            continue
        }
        // List item
        if (len(line) >= 2 && (substr(line, 0, 2) == "- " || substr(line, 0, 2) == "* ")) ||
           (len(line) >= 3 && substr(line, 0, 3) == "  -") {
            if len(paraBuf) > 0 {
                segments = concat(segments, [{"kind": "para", "runs": parseInline(paraBuf)}])
                paraBuf = ""
            }
            let itemText = substr(line, 2)
            listBuf = concat(listBuf, [parseInline(trim(itemText))])
            i = i + 1
            continue
        }
        // Blank line — flush paragraph + list
        if len(trim(line)) == 0 {
            if len(paraBuf) > 0 {
                segments = concat(segments, [{"kind": "para", "runs": parseInline(paraBuf)}])
                paraBuf = ""
            }
            if len(listBuf) > 0 {
                segments = concat(segments, [{"kind": "list", "items": listBuf}])
                listBuf = makeArray(0)
            }
            i = i + 1
            continue
        }
        // Regular text — accumulate into current paragraph
        if len(listBuf) > 0 {
            segments = concat(segments, [{"kind": "list", "items": listBuf}])
            listBuf = makeArray(0)
        }
        if len(paraBuf) > 0 { paraBuf = paraBuf + " " + line }
        else                { paraBuf = line }
        i = i + 1
    }
    if len(paraBuf) > 0 {
        segments = concat(segments, [{"kind": "para", "runs": parseInline(paraBuf)}])
    }
    if len(listBuf) > 0 {
        segments = concat(segments, [{"kind": "list", "items": listBuf}])
    }
    return segments
}


// ── Markdown layout + render ─────────────────────────────────────────────
//
// Each function returns the height it would occupy (measure) or the y past
// the last rendered pixel (draw). Heights are computed without rendering
// so bubbles can pre-size correctly before painting.
//
// (Constants moved above drawChatTab — see top of file. drawChatTab's
// error-bubble branch references _MD_PROSE_SCALE / _MD_LINE_H, and under
// --vm a function's references resolve when the function is compiled, so
// the consts must be declared before any caller is defined.)

// _codeLineH returns the per-line vertical advance for code-block text.
// Sized for the TTF mono font loaded into codeFont (24pt × code scale,
// with ~1.25 line height for breathing room). Minimum floor of 20 keeps
// the result comfortable even when codeFont falls back to uiFont and
// the scale dial gets cranked down.
fn _codeLineH() {
    let h = int(24.0 * _MD_CODE_BLOCK_SCALE * 1.4)
    if h < 11 { h = 11 }
    return h
}

// Token = a piece of an inline run ready to wrap. Each token is one word
// (or one inline-code chunk). drawTokens does the wrap walk.
//
// Two-pass implementation: count first, pre-allocate once, then fill by
// index. The previous concat-in-loop shape was O(n²) — a 500-word
// paragraph allocated 500 arrays of growing size every call. With
// tokens cached per message via compileSegs, this is now only called on
// cache miss, but the per-miss cost still dominates streaming frames so
// keeping it O(n) matters.
fn _tokenize(runs, scale) {
    let spaceW = textWidth(uiFont, " ", scale)
    let rn = len(runs)

    // Pass 1 — count tokens so we can makeArray() once.
    let total = 0
    let ri = 0
    while ri < rn {
        let r = runs[ri]
        if r["kind"] == "code" {
            total = total + 1
        } else {
            let words = split(r["content"], " ")
            let wn = len(words)
            let wi = 0
            while wi < wn {
                if len(words[wi]) > 0 { total = total + 1 }
                if wi < wn - 1 { total = total + 1 }   // inter-word space
                wi = wi + 1
            }
        }
        ri = ri + 1
    }

    // Pass 2 — fill the pre-sized array by index. No concat, no copies.
    let tokens = makeArray(total, null)
    let idx = 0
    ri = 0
    while ri < rn {
        let r = runs[ri]
        if r["kind"] == "code" {
            let w = textWidth(uiFont, r["content"], scale)
            tokens[idx] = {"kind": "code", "content": r["content"], "w": w + 6}
            idx = idx + 1
        } else {
            let words = split(r["content"], " ")
            let wn = len(words)
            let wi = 0
            while wi < wn {
                let word = words[wi]
                if len(word) > 0 {
                    let w = textWidth(uiFont, word, scale)
                    tokens[idx] = {"kind": r["kind"], "content": word, "w": w}
                    idx = idx + 1
                }
                if wi < wn - 1 {
                    tokens[idx] = {"kind": "space", "content": " ", "w": spaceW}
                    idx = idx + 1
                }
                wi = wi + 1
            }
        }
        ri = ri + 1
    }
    return tokens
}

fn _countRunLines(tokens, maxW) {
    let lines = 1
    let cx = 0.0
    let i = 0
    let n = len(tokens)
    while i < n {
        let t = tokens[i]
        if cx + t["w"] > maxW && cx > 0 {
            lines = lines + 1
            cx = 0.0
            if t["kind"] == "space" { i = i + 1   continue }
        }
        cx = cx + t["w"]
        i = i + 1
    }
    return lines
}

fn _drawTokens(tokens, x, y, maxW) {
    let cx = x
    let cy = y
    let i = 0
    let n = len(tokens)
    while i < n {
        let t = tokens[i]
        if cx + t["w"] > x + maxW && cx > x {
            cy = cy + _MD_LINE_H
            cx = x
            if t["kind"] == "space" { i = i + 1   continue }
        }
        let kind = t["kind"]
        if kind == "code" {
            // Inline code: tinted background pill behind monospace-styled text.
            fillC(theme["wTrack"])
            roundedRect(cx, cy - 2, t["w"], 16, 3)
            fillC(theme["accentBar"])
            say(t["content"], cx + 4, cy - 1, _MD_INLINE_CODE_SCALE)
            fillC(theme["cardText"])
        } else if kind == "bold" {
            fillC(theme["titleText"])
            say(t["content"], cx, cy, _MD_PROSE_SCALE)
            fillC(theme["cardText"])
        } else if kind == "italic" {
            fillC(theme["subtitleText"])
            say(t["content"], cx, cy, _MD_PROSE_SCALE)
            fillC(theme["cardText"])
        } else {
            // text / space — body colour
            say(t["content"], cx, cy, _MD_PROSE_SCALE)
        }
        cx = cx + t["w"]
        i = i + 1
    }
    return cy + _MD_LINE_H
}

// compileSegs is the single place markdown is parsed + tokenised +
// measured. Returns {segs, height} where every paragraph/list-item seg
// has its tokens and line count cached inline. drawSegments then reads
// those cached fields instead of re-tokenising — which used to happen
// twice per message per frame (once for measure, once for draw).
fn compileSegs(content, maxW) {
    let segs = parseMarkdown(content)
    let listMaxW = maxW - _MD_LIST_INDENT
    let height = 0
    let si = 0
    let sn = len(segs)
    while si < sn {
        let seg = segs[si]
        let kind = seg["kind"]
        if kind == "para" {
            let tokens = _tokenize(seg["runs"], _MD_PROSE_SCALE)
            let nLines = _countRunLines(tokens, maxW)
            seg["tokens"] = tokens
            seg["nLines"] = nLines
            height = height + _MD_LINE_H * nLines + _MD_PARA_GAP
        } else if kind == "code" {
            let lines = seg["lines"]
            let nLines = len(lines)
            if nLines == 0 { nLines = 1 }
            seg["nLines"] = nLines
            height = height + _MD_CODE_PAD * 2 + _codeLineH() * nLines + _MD_PARA_GAP
        } else if kind == "list" {
            let items = seg["items"]
            let iN = len(items)
            let itemTokens = makeArray(iN, null)
            let itemLines  = makeArray(iN, 0)
            let ii = 0
            while ii < iN {
                let tokens = _tokenize(items[ii], _MD_PROSE_SCALE)
                let nLines = _countRunLines(tokens, listMaxW)
                itemTokens[ii] = tokens
                itemLines[ii]  = nLines
                height = height + _MD_LINE_H * nLines
                ii = ii + 1
            }
            seg["itemTokens"] = itemTokens
            seg["itemLines"]  = itemLines
            height = height + _MD_PARA_GAP
        }
        segs[si] = seg
        si = si + 1
    }
    return {"segs": segs, "height": height}
}

fn drawSegments(segments, x, y, maxW) {
    let cy = y
    let si = 0
    let sn = len(segments)
    while si < sn {
        let seg = segments[si]
        let kind = seg["kind"]
        if kind == "para" {
            cy = _drawTokens(seg["tokens"], x, cy, maxW)
            cy = cy + _MD_PARA_GAP
        } else if kind == "code" {
            let lines = seg["lines"]
            let nLines = len(lines)
            if nLines == 0 { nLines = 1 }
            let codeLineH = _codeLineH()
            let blockH = _MD_CODE_PAD * 2 + codeLineH * nLines
            // Background panel — near-black so the code is visually distinct
            // from the chat panel (which is already a dark grey).
            fill(0.04, 0.04, 0.06, 1.0)
            noStroke()
            roundedRect(x, cy, maxW, blockH, 6)
            // Accent bar on the left — brand colour, signals "this is code"
            // at a glance.
            fillC(theme["accentBar"])
            roundedRect(x, cy, 3, blockH, 0)
            // Optional language tag in the top-right corner
            if len(seg["lang"]) > 0 {
                fillC(theme["dimLabel"])
                let tagW = textWidth(uiFont, seg["lang"], 0.36)
                say(seg["lang"], x + maxW - 8 - tagW, cy + 4, 0.36)
            }
            // Code lines — TTF monospace (codeFont). Was the embedded
            // bitmap text() builtin; that's still functional but visibly
            // rougher than the surrounding chrome.
            fill(0.85, 0.88, 0.95, 1.0)
            let ly = cy + _MD_CODE_PAD
            let li = 0
            while li < nLines {
                sayCode(lines[li], x + _MD_CODE_PAD + 6, ly, _MD_CODE_BLOCK_SCALE)
                ly = ly + codeLineH
                li = li + 1
            }
            cy = cy + blockH + _MD_PARA_GAP
            // Restore body fill so subsequent prose renders in normal text colour.
            fillC(theme["cardText"])
        } else if kind == "list" {
            let itemTokens = seg["itemTokens"]
            let iN = len(itemTokens)
            let ii = 0
            while ii < iN {
                fillC(theme["accentBar"])
                say("•", x + 4, cy, _MD_PROSE_SCALE)
                fillC(theme["cardText"])
                cy = _drawTokens(itemTokens[ii], x + _MD_LIST_INDENT, cy, maxW - _MD_LIST_INDENT)
                ii = ii + 1
            }
            cy = cy + _MD_PARA_GAP
        }
        si = si + 1
    }
    return cy
}


// ── Settings modal ─────────────────────────────────────────────────────────

fn drawSettingsModal(W, H) {
    // Dim the background. Skip drawing the main UI entirely so its widgets
    // don't compete for hover.
    fill(0, 0, 0, 0.55)
    noStroke()
    rect(0, 0, W, H)

    let modalW = 780
    // 500 (was 460) — the per-tab helper text drawn by drawXxxFields can
    // run to 5 wrapped lines (~75 px) on the A1111 tab; the old 460 had
    // it overlapping the Test button at the bottom-left. The footer and
    // testStatus Y positions below are computed as `modalY + modalH - N`
    // so they slide down with the new height — the only effect is extra
    // breathing room under the helper text.
    let modalH = 500
    let modalX = (W - modalW) / 2
    let modalY = (H - modalH) / 2

    drawPanel(modalX, modalY, modalW, modalH)

    // Header
    fillC(theme["titleText"])
    sayDisplay("Settings", modalX + 22, modalY + 16, 0.55)
    fillC(theme["subtitleText"])
    say("Provider configuration · saved to ~/.tadpole/config.json",
        modalX + 22, modalY + 50, 0.38)

    // Tabs row. Image-provider tabs PLUS a Claude tab for the assistant
    // (prompt-enhance + chat) config. Selecting an image-provider tab and
    // clicking Save also marks that tab as the active image provider; the
    // Claude tab is config-only and never changes the active image provider.
    let tabsY     = modalY + 80
    let tabIds    = settingsTabIds()
    let tabLabels = makeArray(len(tabIds), "")
    let activeIdx = 0
    let i = 0
    while i < len(tabIds) {
        tabLabels[i] = providerLabel(tabIds[i])
        if tabIds[i] == settingsTab { activeIdx = i }
        i = i + 1
    }
    let newIdx = tabs(modalX + 22, tabsY, modalW - 44, tabLabels, activeIdx, 0.45)
    if newIdx != activeIdx {
        settingsTab = tabIds[newIdx]
        testStatus  = ""
    }

    // Per-tab field area
    let fieldsX = modalX + 22
    let fieldsY = tabsY + 56
    let fieldsW = modalW - 44

    let cy = fieldsY
    if settingsTab == "aihorde"     { cy = drawAiHordeFields(fieldsX, cy, fieldsW) }
    if settingsTab == "huggingface" { cy = drawHuggingFaceFields(fieldsX, cy, fieldsW) }
    if settingsTab == "openai"      { cy = drawOpenAIFields(fieldsX, cy, fieldsW) }
    if settingsTab == "claude"      { cy = drawClaudeFields(fieldsX, cy, fieldsW) }
    if settingsTab == "ollama"      { cy = drawOllamaFields(fieldsX, cy, fieldsW) }
    if settingsTab == "a1111"       { cy = drawA1111Fields(fieldsX, cy, fieldsW) }

    // Test result line (just above the footer)
    if len(testStatus) > 0 {
        if testError { fillC(theme["crit"]) }
        else         { fillC(theme["low"])  }
        drawWrapped(testStatus, fieldsX, modalY + modalH - 90, 70, 0.42)
    }

    // Footer buttons
    let footY = modalY + modalH - 56
    if button("Test", fieldsX, footY, 100, 36, 0.5) {
        testActiveProviderKey()
    }
    if button("Cancel", fieldsX + fieldsW - 220, footY, 100, 36, 0.5) {
        // Reload from disk so any edits in-modal are discarded.
        config = loadConfig()
        activeProvider = config["active_provider"]
        settingsOpen = false
        testStatus   = ""
    }
    // "Make active for chat" button — only shown when the current tab is
    // a chat-backend tab that is NOT currently the active backend.
    if isChatBackend(settingsTab) && settingsTab != activeChatBackend {
        if button("Use for chat", fieldsX + 110, footY, 130, 36, 0.5) {
            // Backend switch invalidates the in-progress conversation
            // because each backend has its own message-shape (Anthropic
            // content blocks vs Ollama role:tool messages). Clear chat
            // and persist so the switch survives restart.
            _clearChatForBackendSwitch(settingsTab)
            config["active_chat_backend"] = settingsTab
            activeChatBackend = settingsTab
            let werr = saveConfigToDisk(config)
            if werr != null {
                testStatus = "Save failed: " + werr.message
                testError  = true
            } else {
                testStatus = providerLabel(settingsTab) + " is now the active chat backend. Chat history cleared."
                testError  = false
            }
        }
    }
    if button("Save", fieldsX + fieldsW - 110, footY, 110, 36, 0.55) {
        // Only image-provider tabs change which provider Generate uses.
        // Chat-backend tabs are config-only on Save — use the "Use for
        // chat" button to switch the active backend.
        if isImageProvider(settingsTab) {
            config["active_provider"] = settingsTab
            activeProvider = settingsTab
        }
        let werr = saveConfigToDisk(config)
        if werr != null {
            testStatus = "Save failed: " + werr.message
            testError  = true
        } else {
            settingsOpen = false
            testStatus   = ""
        }
    }
}


// _clearChatForBackendSwitch resets the conversation when the user moves
// from one chat backend to another. The agentMessages array is in the
// active backend's wire format; mixing shapes mid-conversation corrupts
// the next API call. Display chat is wiped too so the user has a clean
// slate matching the new backend's capabilities.
fn _clearChatForBackendSwitch(newBackend) {
    chatMessages      = makeArray(0)
    agentMessages     = makeArray(0)
    chatSending       = false
    agentPendingTool  = null
    agentPendingQueue = makeArray(0)
    agentResultsAccum = makeArray(0)
    agentStep         = 0
    chatRenderCache      = makeArray(0)
    chatRenderCacheWidth = -1
    chatGen           = chatGen + 1     // invalidate any in-flight stream
    sessionInTokens  = 0
    sessionOutTokens = 0
    _streamReset()
}


// drawOllamaFields renders the Ollama (local LLM) config panel.
fn drawOllamaFields(x, y, w) {
    let cy = y

    // Status row — is this the active chat backend right now?
    if activeChatBackend == "ollama" {
        fillC(theme["low"])
        say("● Active for chat", x, cy, 0.45)
    } else {
        fillC(theme["dimLabel"])
        say("Not currently the chat backend (click 'Use for chat' below to switch)", x, cy, 0.42)
    }
    cy = cy + 26

    fillC(theme["mainLabel"])
    say("Base URL", x, cy, 0.5)
    cy = cy + 22
    config["ollama"]["base_url"] =
        textInput("", config["ollama"]["base_url"], x, cy, w, 32, 0.5)
    cy = cy + 44

    fillC(theme["mainLabel"])
    say("Model", x, cy, 0.5)
    cy = cy + 22
    config["ollama"]["model"] =
        textInput("", config["ollama"]["model"], x, cy, w, 32, 0.5)
    cy = cy + 44

    // "think" toggle for reasoning models (qwen3, deepseek-r1, …). When
    // off, the model emits the answer directly instead of streaming a
    // chain-of-thought first — much faster + uses fewer tokens.
    let thinking = false
    if hasKey(config["ollama"], "think") { thinking = config["ollama"]["think"] }
    config["ollama"]["think"] = checkbox("Show chain-of-thought (reasoning models only)",
                                          x, cy, thinking, 0.45)
    cy = cy + 28

    // "tools" toggle. When on, tadPole sends MCP tool definitions with
    // every chat turn so the model can call them. Models without tool
    // support (phi4, smaller llama variants, etc.) 400 on requests that
    // include the "tools" field at all — turn this OFF for those.
    let toolsOn = true
    if hasKey(config["ollama"], "tools") { toolsOn = config["ollama"]["tools"] }
    config["ollama"]["tools"] = checkbox("Send tools to model (uncheck for non-tool models like phi4)",
                                          x, cy, toolsOn, 0.45)
    cy = cy + 28

    fillC(theme["subtitleText"])
    drawWrapped("Ollama runs locally (default http://localhost:11434). Pull a model first: `ollama pull <model>`. Tool-using models recommended: llama3.2, qwen2.5, mistral-nemo. Reasoning models (qwen3.x, deepseek-r1) work best with the chain-of-thought toggle OFF for tight Q&A. Non-tool models (phi4, gemma small, …) need the tools toggle OFF.",
                x, cy, 76, 0.42)
    return cy + 78
}

fn drawAiHordeFields(x, y, w) {
    let cy = y
    fillC(theme["mainLabel"])
    say("API key (optional)", x, cy, 0.5)
    cy = cy + 22
    config["aihorde"]["api_key"] =
        textInput("", config["aihorde"]["api_key"], x, cy, w, 32, 0.5)
    cy = cy + 44
    fillC(theme["subtitleText"])
    drawWrapped("Empty = anonymous queue (free, slower). Register at aihorde.net for a free key — keyed requests skip most of the queue.",
                x, cy, 80, 0.42)
    return cy + 60
}

fn drawHuggingFaceFields(x, y, w) {
    let cy = y
    fillC(theme["mainLabel"])
    say("API token (required)", x, cy, 0.5)
    cy = cy + 22
    config["huggingface"]["api_key"] =
        textInput("", config["huggingface"]["api_key"], x, cy, w, 32, 0.5)
    cy = cy + 44
    fillC(theme["mainLabel"])
    say("Model", x, cy, 0.5)
    cy = cy + 22
    let hfModels = [
        "black-forest-labs/FLUX.1-schnell",
        "black-forest-labs/FLUX.1-dev",
        "stabilityai/stable-diffusion-xl-base-1.0",
    ]
    config["huggingface"]["model"] =
        dropdown("", hfModels, x, cy, w, 0.5)
    // dropdown returns the selected item every frame; if a previously-saved
    // model isn't in the list it stays as whatever the user picked just now.
    cy = cy + 44
    fillC(theme["subtitleText"])
    drawWrapped("Free token at huggingface.co/settings/tokens. FLUX-schnell is fast and free-tier-friendly.",
                x, cy, 80, 0.42)
    return cy + 50
}

fn drawOpenAIFields(x, y, w) {
    let cy = y
    fillC(theme["mainLabel"])
    say("API key (required)", x, cy, 0.5)
    cy = cy + 22
    config["openai"]["api_key"] =
        textInput("", config["openai"]["api_key"], x, cy, w, 32, 0.5)
    cy = cy + 44
    let halfW = (w - 16) / 2
    fillC(theme["mainLabel"])
    say("Model",   x,             cy, 0.5)
    say("Quality", x + halfW + 16, cy, 0.5)
    cy = cy + 22
    config["openai"]["model"] =
        dropdown("", ["dall-e-3", "dall-e-2"], x, cy, halfW, 0.5)
    config["openai"]["quality"] =
        dropdown("", ["standard", "hd"], x + halfW + 16, cy, halfW, 0.5)
    cy = cy + 44
    fillC(theme["subtitleText"])
    drawWrapped("Get a key at platform.openai.com. DALL-E 3 standard ~$0.04/image, HD ~$0.08. DALL-E 2 is cheaper but lower quality.",
                x, cy, 80, 0.42)
    return cy + 60
}

fn drawClaudeFields(x, y, w) {
    let cy = y
    fillC(theme["mainLabel"])
    say("Anthropic API key (required)", x, cy, 0.5)
    cy = cy + 22
    config["claude"]["api_key"] =
        textInput("", config["claude"]["api_key"], x, cy, w, 32, 0.5)
    cy = cy + 44
    fillC(theme["mainLabel"])
    say("Model", x, cy, 0.5)
    cy = cy + 22
    let claudeModels = [
        "claude-haiku-4-5-20251001",
        "claude-sonnet-4-6",
        "claude-opus-4-7",
    ]
    config["claude"]["model"] =
        dropdown("", claudeModels, x, cy, w, 0.5)
    cy = cy + 44
    fillC(theme["subtitleText"])
    drawWrapped("Get a key at console.anthropic.com. Claude powers the ✨ Enhance Prompt button and the Chat tab. Haiku 4.5 is cheap & fast (default); Sonnet 4.6 balances quality and cost; Opus 4.7 is the most capable.",
                x, cy, 76, 0.42)
    return cy + 78
}

// drawA1111Fields renders the Local Stable Diffusion (AUTOMATIC1111 / Forge
// / reForge) config panel. Talks straight to /sdapi/v1 over HTTP — no API
// key, no Python bridge. The user only needs the daemon's URL and (optionally)
// a specific checkpoint title to pin.
fn drawA1111Fields(x, y, w) {
    let cy = y

    fillC(theme["mainLabel"])
    say("Base URL", x, cy, 0.5)
    cy = cy + 22
    config["a1111"]["base_url"] =
        textInput("", config["a1111"]["base_url"], x, cy, w, 32, 0.5)
    cy = cy + 44

    fillC(theme["mainLabel"])
    say("Model checkpoint (optional)", x, cy, 0.5)
    cy = cy + 22
    config["a1111"]["model"] =
        textInput("", config["a1111"]["model"], x, cy, w, 32, 0.5)
    cy = cy + 44

    // Generation knobs sit on one row to keep the panel compact.
    let thirdW = (w - 16) / 3
    fillC(theme["mainLabel"])
    say("Steps",     x,                  cy, 0.5)
    say("CFG",       x + thirdW + 8,     cy, 0.5)
    say("Sampler",   x + thirdW * 2 + 16, cy, 0.5)
    cy = cy + 22
    let stepsCur = config["a1111"]["steps"]
    let cfgCur   = config["a1111"]["cfg_scale"]
    let sampCur  = config["a1111"]["sampler"]
    let stepsStr = textInput("", str(stepsCur), x,                  cy, thirdW, 32, 0.5)
    let cfgStr   = textInput("", str(cfgCur),   x + thirdW + 8,     cy, thirdW, 32, 0.5)
    // MPS-safe samplers first (Euler, DDIM, UniPC).
    // DPM++ 2M / SDE and "Euler a" produce black images on Apple Silicon —
    // Forge issue #978. They work on CUDA/CPU.
    let samplers = ["Euler", "DDIM", "UniPC", "Euler a", "DPM++ 2M Karras", "DPM++ SDE Karras"]
    config["a1111"]["sampler"] =
        dropdown("", samplers, x + thirdW * 2 + 16, cy, thirdW, 0.5)
    cy = cy + 44

    // textInput hands back strings — parse defensively so a partly-typed value
    // doesn't crash a generation. Empty/bad strings keep the previous value.
    let parsedSteps, sErr = safe(int, stepsStr)
    if sErr == null && type(parsedSteps) == "INTEGER" && parsedSteps > 0 {
        config["a1111"]["steps"] = parsedSteps
    }
    let parsedCfg, cErr = safe(float, cfgStr)
    if cErr == null && type(parsedCfg) == "FLOAT" && parsedCfg > 0.0 {
        config["a1111"]["cfg_scale"] = parsedCfg
    }

    fillC(theme["subtitleText"])
    drawWrapped("AUTOMATIC1111 / Forge / reForge daemon — start with `./webui.sh --api`. Leave model empty to use whichever checkpoint the daemon currently has loaded; otherwise paste the daemon's exact title (e.g. 'sd_xl_base_1.0.safetensors [31e35c80fc]'). Click Test to verify reachability.",
                x, cy, 76, 0.42)
    return cy + 100
}


// ── Run ───────────────────────────────────────────────────────────────────

// Restore previous session if one exists. Runs after function defs so
// loadSession is in scope; silently degrades to empty conversation if the
// file is missing or corrupt.
loadSession()

// Best-effort frogMcp spawn. Runs in the background — Tadpole opens its
// window immediately whether or not MCP is reachable. If Python or the
// server's deps are missing the init fails silently (mcpStatus = "failed")
// and Tadpole keeps working with the built-in tool set.
_initMcpAsync()

// ── MCP server (lets Claude / external agents drive Tadpole) ──────────────
//
// Exposes a small read-only-leaning tool surface so an MCP client can
// observe Tadpole's state and queue messages. Tools run on the HTTP
// server's goroutines (see stdlib/mcp_server.lex threading note);
// handlers below are written to be quick + self-contained.
//
// Listens on 127.0.0.1:7778 (NOT 7777 — that's the default and we keep
// it free for ad-hoc test servers). Wire-up:
//
//   claude mcp add --transport sse tadpole http://127.0.0.1:7778/sse

fn _mcpToolChat(args) {
    if !hasKey(args, "message") {
        return {"ok": false, "error": "missing 'message' string argument"}
    }
    let msg = args["message"]
    if type(msg) != "STRING" || len(trim(msg)) == 0 {
        return {"ok": false, "error": "'message' must be a non-empty string"}
    }
    if chatSending {
        return {"ok": false, "error": "tadpole is currently processing another message — try again shortly"}
    }
    // Inject through the same entry point a UI Send click uses. sendChat()
    // reads chatDraft, validates, and kicks off the agentic loop on its
    // own goroutines — we return immediately with a hint to poll history
    // for the reply.
    chatDraft = msg
    sendChat()
    return {"ok": true, "queued": msg, "note": "poll list_history for the assistant reply once chat_sending=false"}
}

fn _mcpToolListHistory(args) {
    let limit = 50
    if hasKey(args, "limit") && type(args["limit"]) == "INTEGER" && args["limit"] > 0 {
        limit = args["limit"]
    }
    let n = len(chatMessages)
    let start = 0
    if n > limit { start = n - limit }
    // Build a defensive copy so concurrent UI-side mutation can't tear the
    // returned snapshot mid-serialisation.
    let out = makeArray(0)
    let i = start
    while i < n {
        out = concat(out, [chatMessages[i]])
        i = i + 1
    }
    return {"messages": out, "total": n, "returned": len(out)}
}

fn _mcpToolCurrentState(args) {
    return {
        "active_provider":     activeProvider,
        "active_chat_backend": activeChatBackend,
        "chat_message_count":  len(chatMessages),
        "agent_message_count": len(agentMessages),
        "chat_sending":        chatSending,
        "generating":          generating,
        "session_in_tokens":   sessionInTokens,
        "session_out_tokens":  sessionOutTokens,
    }
}

fn _mcpToolListProviders(args) {
    return {
        "image_providers": providerIds,
        "chat_backends":   chatBackendIds(),
    }
}

// ── Transform tool ─────────────────────────────────────────────────────────
// Bounded, stateless text transformation routed to the local ollama backend
// (Gemma-on-Metal in Karl's setup). Pinned to config["ollama"] regardless of
// activeChatBackend so it never accidentally hits a paid Claude key. The
// system prompt lives here (not in Go) so it can be tweaked without a
// rebuild — change the string, restart tadpole, done.

let TRANSFORM_SYSTEM_PROMPT = "You are a text transformation tool. " +
    "Apply the user's instruction to their input. " +
    "Output only the transformed text. " +
    "No preamble, no explanation, no markdown fences unless the instruction requires them."

let TRANSFORM_MAX_INPUT_BYTES = 32768

fn _mcpToolTransform(args) {
    if !hasKey(args, "input") || type(args["input"]) != "STRING" {
        return {"ok": false, "error": "input is required (string)"}
    }
    if !hasKey(args, "instruction") || type(args["instruction"]) != "STRING" {
        return {"ok": false, "error": "instruction is required (string)"}
    }
    let input       = args["input"]
    let instruction = args["instruction"]
    if len(input) > TRANSFORM_MAX_INPUT_BYTES {
        return {"ok": false, "error": "input too large (max 32 KB)"}
    }

    let maxTokens = 2048
    if hasKey(args, "max_tokens") && type(args["max_tokens"]) == "INTEGER" && args["max_tokens"] > 0 {
        maxTokens = args["max_tokens"]
    }
    let temperature = 0.2
    if hasKey(args, "temperature") {
        let t = args["temperature"]
        if type(t) == "FLOAT"   { temperature = t }
        if type(t) == "INTEGER" { temperature = float(t) }
    }

    let cfg = config["ollama"]
    if cfg == null || cfg["base_url"] == null || cfg["base_url"] == "" {
        return {"ok": false, "error": "ollama backend not configured"}
    }
    let model = cfg["model"]
    let oc    = ollama.newClient(cfg["base_url"], model)

    let startNs = _timeNanos()
    let resp, cerr = ollama.chat(oc, {
        "system":   TRANSFORM_SYSTEM_PROMPT,
        "messages": [{"role": "user", "content": "Instruction: " + instruction + "\n\nInput:\n" + input}],
        "options":  {"temperature": temperature, "num_predict": maxTokens},
    })
    let elapsedMs = (_timeNanos() - startNs) / 1000000

    if cerr != null {
        _appendTransformLog(instruction, len(input), 0, model, elapsedMs, cerr.message)
        return {"ok": false, "error": cerr.message, "model": model, "elapsed_ms": elapsedMs}
    }

    let text = ollama.textOf(resp)
    _appendTransformLog(instruction, len(input), len(text), model, elapsedMs, null)
    return {"ok": true, "output": text, "model": model, "elapsed_ms": elapsedMs}
}

fn _appendTransformLog(instruction, inLen, outLen, model, elapsedMs, errMsg) {
    let logPath = userHomeDir() + "/.tadpole/transform.log"
    let entry = {
        "ts":          _timeNanos() / 1000000,
        "instruction": instruction,
        "input_len":   inLen,
        "output_len":  outLen,
        "model":       model,
        "elapsed_ms":  elapsedMs,
    }
    if errMsg != null { entry["error"] = errMsg }
    let line, _ = safe(json.stringify, entry)
    if line == null { return null }
    let _, _ = safe(appendFile, logPath, line + "\n")
    return null
}


// ── generate_image MCP tool ───────────────────────────────────────────────
// Triggers tadpole's normal image-generation flow through whichever provider
// the user has selected in the UI. Refuses if a generation is already in
// flight (back-pressure mirrors _mcpToolChat). Width/height/provider are
// intentionally NOT exposed — they stay UI-controlled to keep the MCP
// surface minimal and the demo focused on the visible result.

fn _mcpToolGenerateImage(args) {
    if generating {
        return {"ok": false, "error": "tadpole is already generating an image"}
    }
    if !hasKey(args, "prompt") || type(args["prompt"]) != "STRING" {
        return {"ok": false, "error": "'prompt' (string) is required"}
    }
    let p = trim(args["prompt"])
    if len(p) == 0 {
        return {"ok": false, "error": "'prompt' must be non-empty"}
    }
    prompt = p
    if hasKey(args, "negative_prompt") && type(args["negative_prompt"]) == "STRING" {
        negPrompt = args["negative_prompt"]
    }
    startGeneration()
    return {"ok": true,
            "queued": p,
            "provider": activeProvider,
            "note": "poll current_state.generating; image lands in tadpole's panel when done"}
}


// ── tape_query MCP tool ───────────────────────────────────────────────────
// Reads tadpole's --record-tape JSONL file and returns the last N events,
// optionally filtered by `kind`. Path is whitelisted: only files under
// /tmp/ or ~/.tadpole/ with a .lextape suffix are accepted, so a hostile
// caller can't turn the MCP surface into an arbitrary file-reader.

fn _tapePathAllowed(path) {
    if !endsWith(path, ".lextape") { return false }
    if startsWith(path, "/tmp/") { return true }
    let homeDot = userHomeDir() + "/.tadpole/"
    if startsWith(path, homeDot) { return true }
    return false
}

fn _mcpToolTapeQuery(args) {
    let tapePath = "/tmp/tadpole.lextape"
    if hasKey(args, "tape_path") && type(args["tape_path"]) == "STRING" {
        tapePath = args["tape_path"]
    }
    if !_tapePathAllowed(tapePath) {
        return {"ok": false,
                "error": "tape_path not in whitelist (/tmp/*.lextape or ~/.tadpole/*.lextape)"}
    }
    let body, rerr = safe(readFile, tapePath)
    if rerr != null {
        return {"ok": false,
                "error": "tape unreadable at " + tapePath + ": " + rerr.message}
    }
    let kindFilter = ""
    if hasKey(args, "kind") && type(args["kind"]) == "STRING" {
        kindFilter = args["kind"]
    }
    let limit = 20
    if hasKey(args, "last_n") && type(args["last_n"]) == "INTEGER" && args["last_n"] > 0 {
        limit = args["last_n"]
    }

    let lines  = split(body, "\n")
    let nLines = len(lines)

    // First pass — count matching events so the output array can be sized
    // exactly. Avoids the push-in-loop O(n²) antipattern.
    let matched = 0
    let i = 0
    while i < nLines {
        let line = lines[i]
        if len(line) > 0 {
            let ev, perr = safe(json.parse, line)
            if perr == null && type(ev) == "HASH" {
                if kindFilter == "" || (hasKey(ev, "kind") && ev["kind"] == kindFilter) {
                    matched = matched + 1
                }
            }
        }
        i = i + 1
    }

    let take = limit
    if matched < take { take = matched }
    let out  = makeArray(take, null)
    let skip = matched - take
    let outIdx = 0
    let seen   = 0
    i = 0
    while i < nLines {
        let line = lines[i]
        if len(line) > 0 {
            let ev, perr = safe(json.parse, line)
            if perr == null && type(ev) == "HASH" {
                if kindFilter == "" || (hasKey(ev, "kind") && ev["kind"] == kindFilter) {
                    if seen >= skip && outIdx < take {
                        out[outIdx] = ev
                        outIdx = outIdx + 1
                    }
                    seen = seen + 1
                }
            }
        }
        i = i + 1
    }

    return {"ok": true,
            "tape_path":   tapePath,
            "kind_filter": kindFilter,
            "matched":     matched,
            "returned":    outIdx,
            "events":      out}
}


// ── set_right_tab MCP tool ────────────────────────────────────────────────
// Switches the right-hand tab. Indices match drawMainUI's tabs() call:
// 0=Image, 1=Adjust, 2=Chat. We accept the friendly string names and
// trim/lowercase so callers don't have to know the integer mapping.

fn _mcpToolSetRightTab(args) {
    if !hasKey(args, "name") || type(args["name"]) != "STRING" {
        return {"ok": false, "error": "'name' (string) is required: 'image', 'adjust', or 'chat'"}
    }
    let n = lower(trim(args["name"]))
    let idx = -1
    if n == "image"  { idx = 0 }
    if n == "adjust" { idx = 1 }
    if n == "chat"   { idx = 2 }
    if idx < 0 {
        return {"ok": false,
                "error": "unknown tab '" + args["name"] + "' — use 'image', 'adjust', or 'chat'"}
    }
    rightTab = idx
    return {"ok": true, "tab": n, "index": idx}
}


// ── set_theme MCP tool ────────────────────────────────────────────────────
// Swaps the active UI theme. We update `theme` and clear `themeApplied`
// so the next frame re-runs themes.applyTheme(theme) from the main render
// thread — keeps the uiTheme() call off the MCP handler's HTTP goroutine
// and avoids any race with the renderer.

fn _mcpToolSetTheme(args) {
    if !hasKey(args, "name") || type(args["name"]) != "STRING" {
        return {"ok": false, "error": "'name' (string) is required"}
    }
    let n = lower(trim(args["name"]))
    let picked = null
    if n == "dark"     { picked = themes.dark() }
    if n == "crimson"  { picked = themes.crimson() }
    if n == "midnight" { picked = themes.midnight() }
    if n == "forest"   { picked = themes.forest() }
    if n == "light"    { picked = themes.light() }
    if picked == null {
        return {"ok": false,
                "error": "unknown theme '" + args["name"] + "' — use dark/crimson/midnight/forest/light"}
    }
    theme        = picked
    themeApplied = false
    return {"ok": true, "theme": n}
}


// ── reset_adjust MCP tool ─────────────────────────────────────────────────
// Snaps every Adjust slider back to its zero-effect default. Mirrors the
// in-app Reset button.

fn _mcpToolResetAdjust(args) {
    resetAdjust()
    return {"ok": true}
}


// ── set_adjust MCP tool ───────────────────────────────────────────────────
// Set any subset of the Adjust-tab sliders in one call. Each value is
// clamped to the slider's own UI range so the caller can't push the image
// out of band. JSON sends both integers and floats for "number" schema
// types, so we coerce INTEGER → FLOAT before the clamp. After mutating
// the globals, _refreshAdjustPreview() updates the live preview on the
// next frame.

fn _clampFloat(v, lo, hi) {
    if v < lo { return lo }
    if v > hi { return hi }
    return v
}

fn _mcpToolSetAdjust(args) {
    if currentImage == null {
        return {"ok": false, "error": "no image loaded yet — generate one first"}
    }

    // Each slider is the same shape on purpose: a future bound change
    // touches exactly one block, no clever indirection to second-guess.

    if hasKey(args, "exposure") {
        let v = args["exposure"]
        if type(v) == "INTEGER" { v = float(v) }
        if type(v) != "FLOAT" {
            return {"ok": false, "error": "'exposure' must be a number"}
        }
        adjExposure = _clampFloat(v, -3.0, 3.0)
    }
    if hasKey(args, "contrast") {
        let v = args["contrast"]
        if type(v) == "INTEGER" { v = float(v) }
        if type(v) != "FLOAT" {
            return {"ok": false, "error": "'contrast' must be a number"}
        }
        adjContrast = _clampFloat(v, -1.0, 1.0)
    }
    if hasKey(args, "saturation") {
        let v = args["saturation"]
        if type(v) == "INTEGER" { v = float(v) }
        if type(v) != "FLOAT" {
            return {"ok": false, "error": "'saturation' must be a number"}
        }
        adjSaturation = _clampFloat(v, -1.0, 1.0)
    }
    if hasKey(args, "hue") {
        let v = args["hue"]
        if type(v) == "INTEGER" { v = float(v) }
        if type(v) != "FLOAT" {
            return {"ok": false, "error": "'hue' must be a number"}
        }
        adjHue = _clampFloat(v, -180.0, 180.0)
    }
    if hasKey(args, "vignette") {
        let v = args["vignette"]
        if type(v) == "INTEGER" { v = float(v) }
        if type(v) != "FLOAT" {
            return {"ok": false, "error": "'vignette' must be a number"}
        }
        adjVignette = _clampFloat(v, 0.0, 1.0)
    }
    if hasKey(args, "sepia") {
        let v = args["sepia"]
        if type(v) == "INTEGER" { v = float(v) }
        if type(v) != "FLOAT" {
            return {"ok": false, "error": "'sepia' must be a number"}
        }
        adjSepia = _clampFloat(v, 0.0, 1.0)
    }
    if hasKey(args, "brightness") {
        let v = args["brightness"]
        if type(v) == "INTEGER" { v = float(v) }
        if type(v) != "FLOAT" {
            return {"ok": false, "error": "'brightness' must be a number"}
        }
        adjBrightness = _clampFloat(v, -1.0, 1.0)
    }
    if hasKey(args, "gamma") {
        let v = args["gamma"]
        if type(v) == "INTEGER" { v = float(v) }
        if type(v) != "FLOAT" {
            return {"ok": false, "error": "'gamma' must be a number"}
        }
        adjGamma = _clampFloat(v, 0.4, 2.5)
    }
    if hasKey(args, "invert") {
        if type(args["invert"]) != "BOOLEAN" {
            return {"ok": false, "error": "'invert' must be a boolean"}
        }
        adjInvert = args["invert"]
    }
    if hasKey(args, "desaturate") {
        if type(args["desaturate"]) != "BOOLEAN" {
            return {"ok": false, "error": "'desaturate' must be a boolean"}
        }
        adjDesaturate = args["desaturate"]
    }

    // If More… was hidden but the caller touched one of the advanced
    // knobs, expand it so the user can see what changed in the UI.
    if hasKey(args, "brightness") || hasKey(args, "gamma") ||
       hasKey(args, "invert")     || hasKey(args, "desaturate") {
        adjShowMore = true
    }

    _refreshAdjustPreview()

    return {
        "ok": true,
        "values": {
            "exposure":   adjExposure,
            "contrast":   adjContrast,
            "saturation": adjSaturation,
            "hue":        adjHue,
            "vignette":   adjVignette,
            "sepia":      adjSepia,
            "brightness": adjBrightness,
            "gamma":      adjGamma,
            "invert":     adjInvert,
            "desaturate": adjDesaturate,
        },
    }
}


let tadpoleMcpSrv, tadpoleMcpErr = _mcpServeHTTP({
    "name":    "tadpole",
    "version": "0.1.0",
    "port":    7778,
    "tools": {
        "chat": {
            "description": "Send a message to Tadpole's agent. The message is queued through the normal chat send flow; poll list_history to see the assistant reply once chat_sending is false.",
            "schema": {
                "type": "object",
                "properties": {"message": {"type": "string", "description": "The user message text"}},
                "required": ["message"],
            },
            "handler": _mcpToolChat,
        },
        "list_history": {
            "description": "Return recent chat messages (most recent last). Optional 'limit' integer caps how many to return (default 50).",
            "schema": {
                "type": "object",
                "properties": {"limit": {"type": "integer", "description": "Maximum number of messages to return"}},
            },
            "handler": _mcpToolListHistory,
        },
        "current_state": {
            "description": "Snapshot of Tadpole's current provider, chat backend, message counts, send status, and session token usage.",
            "schema": {"type": "object"},
            "handler": _mcpToolCurrentState,
        },
        "list_providers": {
            "description": "List the image providers and chat backends Tadpole has configured.",
            "schema": {"type": "object"},
            "handler": _mcpToolListProviders,
        },
        "transform": {
            "description": "Bounded one-shot text transformation via the local ollama backend (Gemma-on-Metal). Stateless — no chat history, no agent loop. Use for prose tightening, summarising, reformatting, structured extraction. Input capped at 32 KB. Returns {{ok, output, model, elapsed_ms}} on success or {{ok:false, error}} on failure.",
            "schema": {
                "type": "object",
                "properties": {
                    "input":       {"type": "string",  "description": "The text to transform"},
                    "instruction": {"type": "string",  "description": "What to do with the input (e.g. 'tighten to one paragraph', 'convert to markdown table')"},
                    "max_tokens":  {"type": "integer", "description": "Output cap (default 2048)"},
                    "temperature": {"type": "number",  "description": "Sampling temperature (default 0.2)"},
                },
                "required": ["input", "instruction"],
            },
            "handler": _mcpToolTransform,
        },
        "generate_image": {
            "description": "Trigger an image generation in tadpole's active image provider (a1111 / aihorde / huggingface / openai — chosen in the UI). Returns immediately; poll current_state.generating for completion. The generated image appears in tadpole's image panel.",
            "schema": {
                "type": "object",
                "properties": {
                    "prompt":          {"type": "string", "description": "The image prompt"},
                    "negative_prompt": {"type": "string", "description": "Optional negative prompt (things to avoid)"},
                },
                "required": ["prompt"],
            },
            "handler": _mcpToolGenerateImage,
        },
        "tape_query": {
            "description": "Read events from tadpole's execution tape (JSONL causal log). Returns the last N events, optionally filtered by kind. Tape path defaults to /tmp/tadpole.lextape; pass tape_path to override (whitelisted to /tmp/*.lextape or ~/.tadpole/*.lextape).",
            "schema": {
                "type": "object",
                "properties": {
                    "kind":      {"type": "string",  "description": "Optional filter on event kind (e.g. 'async_done', 'ui_event', 'bridge_call')"},
                    "last_n":    {"type": "integer", "description": "Maximum number of events to return (default 20)"},
                    "tape_path": {"type": "string",  "description": "Optional override of the tape file path"},
                },
            },
            "handler": _mcpToolTapeQuery,
        },
        "set_right_tab": {
            "description": "Switch the right-hand tab in tadpole's main view. Useful for driving the UI from outside.",
            "schema": {
                "type": "object",
                "properties": {
                    "name": {"type": "string", "description": "Tab name: 'image', 'adjust', or 'chat' (case-insensitive)"},
                },
                "required": ["name"],
            },
            "handler": _mcpToolSetRightTab,
        },
        "set_theme": {
            "description": "Change tadpole's active UI theme. The whole app repaints in the new palette on the next frame.",
            "schema": {
                "type": "object",
                "properties": {
                    "name": {"type": "string", "description": "Theme name: dark, crimson, midnight, forest, or light"},
                },
                "required": ["name"],
            },
            "handler": _mcpToolSetTheme,
        },
        "set_adjust": {
            "description": "Set any subset of the Adjust-tab sliders in one call. Each slider value is clamped to its UI range (exposure -3..3, contrast/saturation/brightness -1..1, hue -180..180, vignette/sepia 0..1, gamma 0.4..2.5). invert/desaturate are booleans. After update, the live preview refreshes.",
            "schema": {
                "type": "object",
                "properties": {
                    "exposure":   {"type": "number",  "description": "Exposure in stops (-3..3)"},
                    "contrast":   {"type": "number",  "description": "Contrast (-1..1)"},
                    "saturation": {"type": "number",  "description": "Saturation (-1..1)"},
                    "hue":        {"type": "number",  "description": "Hue shift in degrees (-180..180)"},
                    "vignette":   {"type": "number",  "description": "Vignette strength (0..1)"},
                    "sepia":      {"type": "number",  "description": "Sepia strength (0..1)"},
                    "brightness": {"type": "number",  "description": "Brightness (-1..1)"},
                    "gamma":      {"type": "number",  "description": "Gamma (0.4..2.5)"},
                    "invert":     {"type": "boolean", "description": "Invert colours"},
                    "desaturate": {"type": "boolean", "description": "Fully desaturate (greyscale)"},
                },
            },
            "handler": _mcpToolSetAdjust,
        },
        "reset_adjust": {
            "description": "Snap every Adjust-tab slider back to its zero-effect default. Mirrors the in-app Reset button.",
            "schema": {"type": "object"},
            "handler": _mcpToolResetAdjust,
        },
    },
})
if tadpoleMcpErr != null {
    println("[mcp-server] failed to start: " + tadpoleMcpErr.message)
} else {
    println("[mcp-server] up on http://127.0.0.1:7778/sse — 11 tools exposed")
}

uiSetFont(uiFont)
window(1280, 820, "Tadpole — AI Image Generator", drawFrame)

if tadpoleMcpSrv != null {
    _mcpStopServer(tadpoleMcpSrv)
}

bridgeClose(bridge)
