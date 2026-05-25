// sd.lex — Stable Diffusion (AUTOMATIC1111 / Forge / Reforge) as a first-class
// @module    sd
// @version   1.0.0
// @since     klex 0.3.35
// @author    karl
// @summary   Stable Diffusion (AUTOMATIC1111 / Forge / Reforge) as a first-class
// kLex library.
//
// Pure-HTTP implementation built on _httpDo. Zero external dependencies —
// talks straight to a local A1111-compatible daemon over its /sdapi/v1 surface.
//
// Default host: http://127.0.0.1:7860. Pass a different baseUrl to newClient()
// if your daemon is running on another port or host.
//
// API surface (the common 90%):
//   sd.newClient(baseUrl)                       — Client
//   sd.generate(c, opts)            → bytes,err  — txt2img, returns first PNG
//   sd.generateBatch(c, opts)       → array,err  — txt2img, returns all PNGs
//   sd.img2img(c, opts)             → bytes,err  — img2img, init from bytes
//   sd.models(c)                    → array,err  — list checkpoints
//   sd.samplers(c)                  → array,err  — list samplers
//   sd.schedulers(c)                → array,err  — list schedulers
//   sd.setModel(c, title)           → null,err   — switch checkpoint
//   sd.progress(c)                  → hash,err   — {progress, eta_relative, ...}
//   sd.interrupt(c)                 → null,err   — cancel current generation
//   sd.options(c)                   → hash,err   — read current /sdapi/v1/options
//   sd.ping(c)                      → bool,err   — true if the daemon answers
//
// Returned image bytes feed directly into loadImage(bytes) — no disk round-trip.
//
// Reference: https://github.com/AUTOMATIC1111/stable-diffusion-webui/wiki/API
//
// Usage:
//   import "stdlib/ai/sd.lex" as sd
//
//   c = sd.newClient(null)
//   pngBytes, err = sd.generate(c, {
//       "prompt":          "a tadpole swimming in a watercolor pond",
//       "negative_prompt": "ugly, blurry, low quality",
//       "width":           768,
//       "height":          768,
//       "steps":           25,
//       "cfg_scale":       7.0,
//       "sampler_name":    "Euler a",
//       "seed":            -1,
//   })
//   if err != null { println(err.message)  return }
//   img = loadImage(pngBytes)
//   saveImage(img, "/tmp/out.png")


import "stdlib/json.lex" as json


// ── Constants ─────────────────────────────────────────────────────────────

const DEFAULT_BASE_URL = "http://127.0.0.1:7860"

// txt2img on a 768x768 SDXL at 25 steps on a mid-range GPU is typically
// 5–15 seconds. On CPU-only or first-load (model swap), it can easily push
// past a minute. 600s is generous but cheap — context-bound, only fires if
// something is genuinely wrong.
const DEFAULT_TIMEOUT_SEC = 600

// Short timeout for tiny status calls (progress, ping, list-models).
const FAST_TIMEOUT_SEC = 15


// ── Client ────────────────────────────────────────────────────────────────

// Client carries the daemon's base URL. A1111 has no API key by default;
// the field is reserved for setups behind nginx basic-auth.
struct Client {
    baseUrl, authHeader
}


// newClient creates a Client. baseUrl=null → DEFAULT_BASE_URL.
fn newClient(baseUrl) {
    let chosenBase = DEFAULT_BASE_URL
    if baseUrl != null && type(baseUrl) == "STRING" && len(baseUrl) > 0 {
        chosenBase = baseUrl
    }
    return Client { baseUrl: chosenBase, authHeader: null }
}


// newClientAuth creates a Client with a pre-built Authorization header value
// — e.g. "Basic dXNlcjpwYXNz" for the common nginx basic-auth setup, or
// "Bearer …" if you're fronting the daemon with something custom.
fn newClientAuth(baseUrl, authHeader) {
    let c = newClient(baseUrl)
    c.authHeader = authHeader
    return c
}


// ── Internal helpers ──────────────────────────────────────────────────────

fn _headers(c) {
    let h = {"Content-Type": "application/json"}
    if c.authHeader != null && type(c.authHeader) == "STRING" && len(c.authHeader) > 0 {
        h["Authorization"] = c.authHeader
    }
    return h
}


// _classifyHTTP turns a non-2xx status into a typed error. We hand the raw
// body back in the message because A1111 returns useful detail (missing model,
// VRAM OOM, bad sampler name, …) as a JSON {"error": "...", "detail": "..."}.
fn _classifyHTTP(status, body) {
    let code = "SD_HTTP_ERROR"
    let msg  = "Stable Diffusion API returned status " + str(status)
    if status == 404 {
        code = "SD_NOT_FOUND"
        msg  = "endpoint not found — is the daemon running with --api enabled?"
    } else if status == 401 || status == 403 {
        code = "SD_AUTH"
        msg  = "authentication failed (status " + str(status) + ")"
    } else if status >= 500 {
        code = "SD_SERVER"
        msg  = "Stable Diffusion server error (status " + str(status) + ")"
    }
    if body != null && type(body) == "STRING" && len(body) > 0 {
        msg = msg + " — " + body
    }
    return error(code, msg)
}


// _post issues a JSON POST. Returns (parsed_body, err). On non-2xx the
// parsed body is null and err is a typed error with the daemon's payload
// embedded in the message.
fn _post(c, path, payload, timeoutSec) {
    let bodyStr = json.stringify(payload)
    let url     = c.baseUrl + path
    let status, rbody, _, httpErr = _httpDo("POST", url, _headers(c), bodyStr, timeoutSec)
    if httpErr != null {
        return null, error("SD_NETWORK", "POST " + url + " failed: " + httpErr)
    }
    if status < 200 || status >= 300 {
        return null, _classifyHTTP(status, rbody)
    }
    let parsed, parseErr = json.parse(rbody)
    if parseErr != null {
        return null, error("SD_BAD_JSON", "could not parse response from " + path + ": " + parseErr.message)
    }
    return parsed, null
}


// _get issues a GET. Returns (parsed_body, err).
fn _get(c, path, timeoutSec) {
    let url = c.baseUrl + path
    let status, rbody, _, httpErr = _httpDo("GET", url, _headers(c), null, timeoutSec)
    if httpErr != null {
        return null, error("SD_NETWORK", "GET " + url + " failed: " + httpErr)
    }
    if status < 200 || status >= 300 {
        return null, _classifyHTTP(status, rbody)
    }
    if rbody == "" {
        return null, null
    }
    let parsed, parseErr = json.parse(rbody)
    if parseErr != null {
        return null, error("SD_BAD_JSON", "could not parse response from " + path + ": " + parseErr.message)
    }
    return parsed, null
}


// _resolveTimeout pulls opts["timeout_sec"] if present, otherwise the default.
// Accepts integer or float.
fn _resolveTimeout(opts) {
    if opts != null && type(opts) == "HASH" && hasKey(opts, "timeout_sec") {
        let t = opts["timeout_sec"]
        let tt = type(t)
        if tt == "INTEGER" || tt == "FLOAT" { return t }
    }
    return DEFAULT_TIMEOUT_SEC
}


// _decodeImages takes the "images" array from a txt2img/img2img response and
// returns (array_of_bytes, err). Each entry is base64-encoded PNG.
fn _decodeImages(images) {
    if images == null || type(images) != "ARRAY" {
        return null, error("SD_NO_IMAGES", "response did not contain an images array")
    }
    let n = len(images)
    if n == 0 {
        return null, error("SD_NO_IMAGES", "response images array was empty")
    }
    let out = makeArray(n, null)
    let i = 0
    while i < n {
        let entry = images[i]
        if type(entry) != "STRING" {
            return null, error("SD_BAD_IMAGE", "image " + str(i) + " was not a base64 string")
        }
        // A1111 sometimes prefixes with "data:image/png;base64," — strip it.
        let s = entry
        let comma = indexOf(s, ",")
        let prefix = "data:"
        if comma != -1 && len(s) > len(prefix) && substr(s, 0, len(prefix)) == prefix {
            s = substr(s, comma + 1, len(s))
        }
        let b, decErr = base64ToBytes(s)
        if decErr != null {
            return null, error("SD_BAD_IMAGE", "image " + str(i) + " base64 decode failed: " + decErr.message)
        }
        out[i] = b
        i = i + 1
    }
    return out, null
}


// ── Generation: txt2img ───────────────────────────────────────────────────

// generate runs txt2img and returns the first PNG as bytes. Use generateBatch
// if you set batch_size or n_iter > 1 and want every image back.
//
// opts is a hash. Any key A1111's /sdapi/v1/txt2img accepts is passed through
// verbatim — this is the full surface, not a curated subset. The most useful:
//   prompt           — required, the positive prompt
//   negative_prompt  — string, things to avoid
//   width, height    — integers (multiples of 8 — A1111 enforces)
//   steps            — integer, sampling steps (default 20)
//   cfg_scale        — float, classifier-free guidance (default 7.0)
//   sampler_name     — e.g. "Euler a", "DPM++ 2M Karras"
//   scheduler        — e.g. "Automatic", "Karras"
//   seed             — integer; -1 = random
//   subseed          — integer; -1 = none
//   batch_size       — images per batch (default 1)
//   n_iter           — number of batches (default 1)
//   restore_faces    — bool
//   tiling           — bool
//   timeout_sec      — kLex-only: per-call HTTP timeout override
//
// Returns (pngBytes, err). On any failure err is typed.
fn generate(c, opts) {
    let imgs, err = generateBatch(c, opts)
    if err != null { return null, err }
    return imgs[0], null
}


// generateBatch is like generate() but returns every image produced.
fn generateBatch(c, opts) {
    if opts == null || type(opts) != "HASH" {
        return null, error("SD_BAD_OPTS", "generate: opts must be a hash")
    }
    if !hasKey(opts, "prompt") {
        return null, error("SD_BAD_OPTS", "generate: opts.prompt is required")
    }

    // Build a payload hash from opts, dropping kLex-only keys and forcing
    // send_images=true / save_images=false so the daemon doesn't pollute its
    // output folder. We copy rather than mutate the caller's hash.
    let payload = {}
    for k, v in opts {
        if k != "timeout_sec" {
            payload[k] = v
        }
    }
    payload["send_images"] = true
    payload["save_images"] = false

    let timeoutSec = _resolveTimeout(opts)
    let resp, err  = _post(c, "/sdapi/v1/txt2img", payload, timeoutSec)
    if err != null { return null, err }

    if type(resp) != "HASH" || !hasKey(resp, "images") {
        return null, error("SD_NO_IMAGES", "txt2img response had no 'images' key")
    }
    return _decodeImages(resp["images"])
}


// ── Generation: img2img ───────────────────────────────────────────────────

// img2img runs img2img. Init images are passed as bytes via opts["init_image"]
// (single) or opts["init_images"] (array of bytes — they get base64-encoded
// inside this function so callers don't have to). All other A1111 img2img
// fields pass through verbatim — denoising_strength, mask, mask_blur, …
fn img2img(c, opts) {
    if opts == null || type(opts) != "HASH" {
        return null, error("SD_BAD_OPTS", "img2img: opts must be a hash")
    }
    if !hasKey(opts, "prompt") {
        return null, error("SD_BAD_OPTS", "img2img: opts.prompt is required")
    }

    let payload = {}
    let initImages = []
    for k, v in opts {
        if k == "timeout_sec" {
            // skip
        } else if k == "init_image" {
            // single bytes → array of one base64 string
            if type(v) != "BYTES" {
                return null, error("SD_BAD_OPTS", "img2img: init_image must be bytes")
            }
            initImages = [bytesToBase64(v)]
        } else if k == "init_images" {
            // array of bytes → array of base64 strings
            if type(v) != "ARRAY" {
                return null, error("SD_BAD_OPTS", "img2img: init_images must be an array of bytes")
            }
            let n = len(v)
            let encoded = makeArray(n, null)
            let i = 0
            while i < n {
                let item = v[i]
                if type(item) != "BYTES" {
                    return null, error("SD_BAD_OPTS", "img2img: init_images[" + str(i) + "] must be bytes")
                }
                encoded[i] = bytesToBase64(item)
                i = i + 1
            }
            initImages = encoded
        } else if k == "mask" {
            // mask as bytes → base64 string
            if type(v) != "BYTES" {
                return null, error("SD_BAD_OPTS", "img2img: mask must be bytes")
            }
            payload["mask"] = bytesToBase64(v)
        } else {
            payload[k] = v
        }
    }

    if len(initImages) == 0 {
        return null, error("SD_BAD_OPTS", "img2img: provide either init_image (bytes) or init_images (array of bytes)")
    }
    payload["init_images"] = initImages
    payload["send_images"] = true
    payload["save_images"] = false

    let timeoutSec = _resolveTimeout(opts)
    let resp, err  = _post(c, "/sdapi/v1/img2img", payload, timeoutSec)
    if err != null { return null, err }

    if type(resp) != "HASH" || !hasKey(resp, "images") {
        return null, error("SD_NO_IMAGES", "img2img response had no 'images' key")
    }
    let imgs, decErr = _decodeImages(resp["images"])
    if decErr != null { return null, decErr }
    return imgs[0], null
}


// ── Daemon introspection ─────────────────────────────────────────────────

// models lists installed checkpoints. Each entry is the A1111 model hash:
// { title, model_name, hash, sha256, filename, config }. The `title` field
// is what setModel() wants.
fn models(c) {
    return _get(c, "/sdapi/v1/sd-models", FAST_TIMEOUT_SEC)
}


// samplers lists available samplers. Each entry: { name, aliases, options }.
fn samplers(c) {
    return _get(c, "/sdapi/v1/samplers", FAST_TIMEOUT_SEC)
}


// schedulers lists available scheduler types (Automatic, Karras, …).
// New in A1111 1.9+. On older daemons this returns SD_NOT_FOUND.
fn schedulers(c) {
    return _get(c, "/sdapi/v1/schedulers", FAST_TIMEOUT_SEC)
}


// setModel switches the active checkpoint. `title` is the value of the
// `title` field returned by models() — typically looks like
// "sd_xl_base_1.0.safetensors [31e35c80fc]". Returns (null, err); the
// daemon may take 10–60s on a cold swap so we use the long timeout.
fn setModel(c, title) {
    if type(title) != "STRING" || len(title) == 0 {
        return null, error("SD_BAD_OPTS", "setModel: title must be a non-empty string")
    }
    let _, err = _post(c, "/sdapi/v1/options", {"sd_model_checkpoint": title}, DEFAULT_TIMEOUT_SEC)
    return null, err
}


// progress polls the current generation status. Returns a hash:
//   progress       — float 0.0..1.0
//   eta_relative   — float seconds remaining (estimate)
//   state          — { skipped, interrupted, job, job_count, sampling_step, … }
//   current_image  — base64 PNG of the current preview (may be empty string)
//   textinfo       — string status from the daemon
fn progress(c) {
    return _get(c, "/sdapi/v1/progress?skip_current_image=true", FAST_TIMEOUT_SEC)
}


// progressWithPreview is the same as progress() but asks the daemon for
// the in-flight preview image. Heavier — only call this if you actually
// render the preview.
fn progressWithPreview(c) {
    return _get(c, "/sdapi/v1/progress", FAST_TIMEOUT_SEC)
}


// interrupt cancels the in-flight generation. Returns (null, err).
fn interrupt(c) {
    let _, err = _post(c, "/sdapi/v1/interrupt", {}, FAST_TIMEOUT_SEC)
    return null, err
}


// options reads the full /sdapi/v1/options hash. Useful for inspecting the
// currently active model, VAE, upscalers, etc. Use setOption() to write a
// single key without clobbering everything else.
fn options(c) {
    return _get(c, "/sdapi/v1/options", FAST_TIMEOUT_SEC)
}


// setOption writes a single key into the daemon's options hash. Wraps the
// "send only the keys you want to change" idiom A1111's options endpoint
// requires (a full PUT would overwrite every other setting).
fn setOption(c, key, value) {
    if type(key) != "STRING" || len(key) == 0 {
        return null, error("SD_BAD_OPTS", "setOption: key must be a non-empty string")
    }
    let _, err = _post(c, "/sdapi/v1/options", {key: value}, FAST_TIMEOUT_SEC)
    return null, err
}


// ping returns true if the daemon answers a cheap GET within FAST_TIMEOUT_SEC,
// false otherwise. Doesn't raise — used by UIs to colour-code a status dot.
fn ping(c) {
    let _, err = _get(c, "/sdapi/v1/progress?skip_current_image=true", FAST_TIMEOUT_SEC)
    return err == null, null
}
