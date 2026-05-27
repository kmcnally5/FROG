#!/usr/bin/env python3
"""
ai_image_bridge.py — kLex bridge with pluggable AI image providers.

Currently wired:
  aihorde       — free, crowdsourced Stable Diffusion. Anonymous works.
  huggingface   — free tier inference API for FLUX / SDXL etc. Requires token.
  openai        — DALL-E 3 / DALL-E 2. Paid (~$0.04/image standard).

Wire surface (kLex side):
  bridgeCall(b, "generate",  [provider, prompt, width, height, opts]) → bytes
  bridgeCall(b, "test_key",  [provider, opts])                        → string
  bridgeCall(b, "backend",   [])                                      → string
  bridgeCall(b, "providers", [])                                      → array

`opts` is a hash carrying provider-specific knobs (api_key, model, quality,
…). Each provider unpacks the keys it cares about; missing keys fall back to
sensible defaults. Image bytes come back through the binary capability so
loadImage() can decode them straight off the wire.
"""

import base64
import json
import os
import ssl
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

from klex_bridge import handler, serve, notify


# ── SSL context (macOS python.org Python ships no CA bundle) ───────────────

def _make_ssl_context():
    try:
        import certifi
        return ssl.create_default_context(cafile=certifi.where())
    except ImportError:
        return ssl.create_default_context()

_SSL_CONTEXT = _make_ssl_context()


# ── HTTP helpers ───────────────────────────────────────────────────────────

_USER_AGENT     = "kLex-tadpole/0.2"
_HTTP_TIMEOUT_S = 60


def _http(method, url, body=None, headers=None, want="json", timeout=_HTTP_TIMEOUT_S):
    h = {"User-Agent": _USER_AGENT}
    if headers:
        h.update(headers)
    data = None
    if body is not None:
        if isinstance(body, (bytes, bytearray)):
            data = bytes(body)
        else:
            data = json.dumps(body).encode("utf-8")
            h.setdefault("Content-Type", "application/json")
    req = urllib.request.Request(url, data=data, headers=h, method=method)
    try:
        with urllib.request.urlopen(req, timeout=timeout, context=_SSL_CONTEXT) as resp:
            raw = resp.read()
            ctype = resp.headers.get("Content-Type", "")
    except urllib.error.HTTPError as e:
        body_text = ""
        try:
            body_text = e.read().decode("utf-8", errors="replace")
        except Exception:
            pass
        raise RuntimeError(f"HTTP {e.code} {e.reason} from {url}: {body_text[:300]}") from e
    except urllib.error.URLError as e:
        raise RuntimeError(f"network error reaching {url}: {e.reason}") from e

    if want == "bytes":
        return raw, ctype
    if want == "json":
        return json.loads(raw) if raw else {}
    return raw


# ── Provider: AI Horde ─────────────────────────────────────────────────────

_HORDE_BASE       = "https://aihorde.net/api/v2"
_HORDE_POLL_S     = 2.0
_HORDE_MAX_WAIT_S = 300

def _generate_aihorde(prompt, width, height, opts):
    api_key = (opts.get("api_key") or "").strip() or "0000000000"
    # AI Horde requires dimensions divisible by 64 in [64, 3072].
    w = max(64, min(3072, (width  // 64) * 64))
    h = max(64, min(3072, (height // 64) * 64))

    submit = _http("POST", f"{_HORDE_BASE}/generate/async",
                   body={
                       "prompt": prompt,
                       "params": {"width": w, "height": h, "steps": 20,
                                  "sampler_name": "k_euler"},
                       "models": ["stable_diffusion"],
                       "nsfw": False,
                       "trusted_workers": False,
                   },
                   headers={"apikey": api_key})
    job_id = submit.get("id")
    if not job_id:
        raise RuntimeError(f"AI Horde did not return a job id: {submit}")
    notify({"phase": "queued", "job_id": job_id})

    started = time.time()
    while True:
        if time.time() - started > _HORDE_MAX_WAIT_S:
            try:
                _http("DELETE", f"{_HORDE_BASE}/generate/status/{job_id}")
            except Exception:
                pass
            raise RuntimeError(f"AI Horde job timed out after {_HORDE_MAX_WAIT_S}s")
        time.sleep(_HORDE_POLL_S)
        check = _http("GET", f"{_HORDE_BASE}/generate/check/{job_id}")
        notify({"phase": "polling",
                "wait":    check.get("wait_time", 0),
                "pos":     check.get("queue_position", 0),
                "elapsed": int(time.time() - started)})
        if check.get("faulted"):
            raise RuntimeError("AI Horde worker reported the job faulted")
        if check.get("done"):
            break

    status = _http("GET", f"{_HORDE_BASE}/generate/status/{job_id}")
    gens = status.get("generations") or []
    if not gens:
        raise RuntimeError(f"AI Horde returned no generations: {status}")
    img = gens[0].get("img", "")
    if not img:
        raise RuntimeError("AI Horde returned a generation with no image")
    if img.startswith("http://") or img.startswith("https://"):
        raw, _ = _http("GET", img, want="bytes")
        return raw
    return base64.b64decode(img)


def _test_aihorde(opts):
    api_key = (opts.get("api_key") or "").strip()
    if not api_key:
        return "OK (anonymous — keyed mode not configured)"
    res = _http("GET", f"{_HORDE_BASE}/find_user", headers={"apikey": api_key})
    name = res.get("username") or "?"
    kudos = res.get("kudos", 0)
    return f"OK · {name} · {kudos} kudos"


# ── Provider: Hugging Face Inference ───────────────────────────────────────

_HF_DEFAULT_MODEL = "black-forest-labs/FLUX.1-schnell"

def _generate_huggingface(prompt, width, height, opts):
    api_key = (opts.get("api_key") or "").strip()
    if not api_key:
        raise RuntimeError("Hugging Face: api_key is required (free signup at huggingface.co)")
    model = (opts.get("model") or _HF_DEFAULT_MODEL).strip() or _HF_DEFAULT_MODEL

    # HF inference: dimensions must be multiples of 8 for most diffusion models.
    w = max(64, min(2048, (width  // 8) * 8))
    h = max(64, min(2048, (height // 8) * 8))

    notify({"phase": "requesting", "model": model})
    raw, ctype = _http(
        "POST", f"https://api-inference.huggingface.co/models/{model}",
        body={"inputs": prompt, "parameters": {"width": w, "height": h}},
        headers={"Authorization": f"Bearer {api_key}", "Accept": "image/png"},
        want="bytes",
        timeout=120,
    )
    if "json" in ctype:
        # HF sometimes responds with a JSON error body even on 200.
        try:
            err = json.loads(raw)
        except Exception:
            err = {"raw": raw[:200].decode("utf-8", errors="replace")}
        raise RuntimeError(f"Hugging Face returned JSON, not an image: {err}")
    if len(raw) < 256:
        raise RuntimeError(f"Hugging Face returned {len(raw)} bytes — likely an error response")
    return raw


def _test_huggingface(opts):
    api_key = (opts.get("api_key") or "").strip()
    if not api_key:
        raise RuntimeError("api_key is required")
    res = _http("GET", "https://huggingface.co/api/whoami-v2",
                headers={"Authorization": f"Bearer {api_key}"})
    name = res.get("name") or res.get("fullname") or "?"
    return f"OK · {name}"


# ── Provider: OpenAI DALL-E ────────────────────────────────────────────────

# DALL-E 3 only supports 1024×1024, 1024×1792, 1792×1024.
# DALL-E 2 supports 256, 512, 1024 square.
_DALLE3_SIZES = ["1024x1024", "1024x1792", "1792x1024"]
_DALLE2_SIZES = ["256x256", "512x512", "1024x1024"]

def _pick_dalle_size(model, width, height):
    if model == "dall-e-2":
        # snap to nearest of 256/512/1024 square
        target = max(width, height)
        if target < 384:
            return "256x256"
        if target < 768:
            return "512x512"
        return "1024x1024"
    # dall-e-3
    if width > height * 1.2:
        return "1792x1024"
    if height > width * 1.2:
        return "1024x1792"
    return "1024x1024"


def _generate_openai(prompt, width, height, opts):
    api_key = (opts.get("api_key") or "").strip()
    if not api_key:
        raise RuntimeError("OpenAI: api_key is required")
    model   = (opts.get("model") or "dall-e-3").strip() or "dall-e-3"
    quality = (opts.get("quality") or "standard").strip() or "standard"
    size    = _pick_dalle_size(model, width, height)

    body = {
        "model":           model,
        "prompt":          prompt,
        "n":               1,
        "size":            size,
        "response_format": "b64_json",
    }
    if model == "dall-e-3":
        body["quality"] = quality   # "standard" or "hd"

    notify({"phase": "requesting", "model": model, "size": size, "quality": quality})
    res = _http("POST", "https://api.openai.com/v1/images/generations",
                body=body,
                headers={"Authorization": f"Bearer {api_key}"},
                timeout=120)
    data = res.get("data") or []
    if not data or "b64_json" not in data[0]:
        raise RuntimeError(f"OpenAI response had no image data: {res}")
    return base64.b64decode(data[0]["b64_json"])


def _test_openai(opts):
    api_key = (opts.get("api_key") or "").strip()
    if not api_key:
        raise RuntimeError("api_key is required")
    # Cheap call — just list models. Validates the key without burning a generation.
    _ = _http("GET", "https://api.openai.com/v1/models",
              headers={"Authorization": f"Bearer {api_key}"},
              timeout=15)
    return "OK · key valid"


# ── Claude (Anthropic) — text helpers, not an image provider ───────────────

_CLAUDE_BASE = "https://api.anthropic.com/v1/messages"
_CLAUDE_VER  = "2023-06-01"

_ENHANCE_SYSTEM = (
    "You rewrite short image-generation prompts to be vivid and detailed. "
    "Return ONLY the rewritten prompt — no preamble, no quotes, no explanation. "
    "Keep it under 80 words. Add specifics like lighting, art style, composition, "
    "mood, camera framing. Preserve the user's intent exactly. Never add safety "
    "disclaimers or refusals."
)

def _claude_call(opts, system, messages, max_tokens=1024):
    """Single Claude messages-API call. Returns the assistant's text. Accepts
    a list of {role, content} dicts in `messages`."""
    api_key = (opts.get("api_key") or "").strip()
    if not api_key:
        raise RuntimeError("Claude: api_key is required (anthropic.com/settings/keys)")
    model = (opts.get("model") or "claude-haiku-4-5-20251001").strip() or "claude-haiku-4-5-20251001"

    body = {
        "model":      model,
        "max_tokens": max_tokens,
        "messages":   messages,
    }
    if system:
        body["system"] = system

    res = _http(
        "POST", _CLAUDE_BASE,
        body=body,
        headers={
            "x-api-key":         api_key,
            "anthropic-version": _CLAUDE_VER,
        },
        timeout=60,
    )
    content = res.get("content") or []
    parts = [c.get("text", "") for c in content if c.get("type") == "text"]
    out = "".join(parts).strip()
    if not out:
        raise RuntimeError(f"Claude returned no text content: {res}")
    return out


@handler(
    args=[("prompt", "string"), ("opts", "hash")],
    returns="string",
)
def enhance_prompt(prompt, opts):
    """Run the user's rough prompt through Claude to produce a more detailed
    image-generation prompt. Returns the enhanced prompt text only."""
    if not prompt or not prompt.strip():
        raise ValueError("enhance_prompt: prompt cannot be empty")
    notify({"phase": "claude_request", "task": "enhance"})
    return _claude_call(
        opts,
        system=_ENHANCE_SYSTEM,
        messages=[{"role": "user", "content": prompt.strip()}],
        max_tokens=400,
    )


@handler(
    args=[("messages", "array"), ("opts", "hash")],
    returns="string",
)
def claude_chat(messages, opts):
    """Send a conversation to Claude and return the assistant reply. `messages`
    is an array of {role, content} hashes — kLex side maintains conversation
    state, this handler is stateless. Returns the assistant's reply text."""
    if not messages:
        raise ValueError("claude_chat: at least one message is required")
    # Coerce kLex hash entries into the dict shape Claude expects.
    msgs = []
    for m in messages:
        role = (m.get("role") or "user").strip()
        content = m.get("content") or m.get("text") or ""
        if not content:
            continue
        msgs.append({"role": role, "content": content})
    if not msgs:
        raise ValueError("claude_chat: messages had no usable content")
    notify({"phase": "claude_request", "task": "chat"})
    return _claude_call(opts, system=None, messages=msgs, max_tokens=1024)


@handler(
    args=[("messages", "array"), ("tools", "array"), ("opts", "hash")],
    returns="hash",
)
def claude_step(messages, tools, opts):
    """One agentic step. Sends messages + tool definitions to Claude and
    returns either a final text response or the first tool_use block Claude
    wants to invoke. The caller is responsible for executing the tool and
    re-calling with the tool_result appended to the messages array.

    Return shape:
      {"kind": "text", "content": "...", "stop_reason": "..."}
      {"kind": "tool_use", "id": "...", "name": "...", "input": {...},
       "assistant_content": [...content blocks verbatim...]}

    `messages` accepts both shapes Anthropic understands:
      {"role": "user", "content": "plain text"}
      {"role": "user", "content": [{"type": "tool_result", ...}, ...]}
      {"role": "assistant", "content": [{"type": "text", ...}, {"type": "tool_use", ...}]}
    """
    api_key = (opts.get("api_key") or "").strip()
    if not api_key:
        raise RuntimeError("Claude: api_key is required")
    model = (opts.get("model") or "claude-haiku-4-5-20251001").strip() or "claude-haiku-4-5-20251001"

    msgs = []
    for m in messages:
        role = (m.get("role") or "user").strip()
        content = m.get("content")
        if content is None:
            continue
        msgs.append({"role": role, "content": content})
    if not msgs:
        raise ValueError("claude_step: at least one message required")

    body = {
        "model":      model,
        "max_tokens": 4096,
        "messages":   msgs,
    }
    if tools:
        body["tools"] = tools
    system = opts.get("system")
    if system:
        body["system"] = system

    notify({"phase": "claude_request", "task": "agent_step"})
    res = _http("POST", _CLAUDE_BASE, body=body,
                headers={"x-api-key": api_key, "anthropic-version": _CLAUDE_VER},
                timeout=120)

    content = res.get("content") or []
    stop_reason = res.get("stop_reason", "")
    usage = res.get("usage") or {}
    usage_out = {
        "input_tokens":  int(usage.get("input_tokens", 0)),
        "output_tokens": int(usage.get("output_tokens", 0)),
        "model":         model,
    }

    # If Claude wants a tool, return the first tool_use block (Anthropic
    # currently emits one per response when in agent mode; we'd loop client-
    # side in any case).
    for block in content:
        if block.get("type") == "tool_use":
            return {
                "kind":              "tool_use",
                "id":                block.get("id", ""),
                "name":              block.get("name", ""),
                "input":             block.get("input") or {},
                "assistant_content": content,
                "stop_reason":       stop_reason,
                "usage":             usage_out,
            }

    # Otherwise concat all text blocks for a final response.
    text_parts = [b.get("text", "") for b in content if b.get("type") == "text"]
    return {
        "kind":        "text",
        "content":     "".join(text_parts).strip(),
        "stop_reason": stop_reason,
        "usage":       usage_out,
    }


@handler(args=[("opts", "hash")], returns="string")
def test_claude(opts):
    """Validate a Claude API key with a minimal 1-token request. Cheap but
    real — verifies the key works, the model is accessible, and the org has
    credit."""
    api_key = (opts.get("api_key") or "").strip()
    if not api_key:
        raise RuntimeError("api_key is required")
    model = (opts.get("model") or "claude-haiku-4-5-20251001").strip() or "claude-haiku-4-5-20251001"
    _ = _http(
        "POST", _CLAUDE_BASE,
        body={
            "model":      model,
            "max_tokens": 1,
            "messages":   [{"role": "user", "content": "ok"}],
        },
        headers={"x-api-key": api_key, "anthropic-version": _CLAUDE_VER},
        timeout=15,
    )
    return f"OK · {model}"


# ── Dispatcher table ───────────────────────────────────────────────────────

_PROVIDERS = {
    "aihorde": {
        "label":    "AI Horde",
        "generate": _generate_aihorde,
        "test":     _test_aihorde,
        "fields":   ["api_key"],
    },
    "huggingface": {
        "label":    "Hugging Face",
        "generate": _generate_huggingface,
        "test":     _test_huggingface,
        "fields":   ["api_key", "model"],
    },
    "openai": {
        "label":    "OpenAI (DALL-E)",
        "generate": _generate_openai,
        "test":     _test_openai,
        "fields":   ["api_key", "model", "quality"],
    },
}


# ── Public handlers ────────────────────────────────────────────────────────

@handler(
    args=[
        ("provider", "string"),
        ("prompt",   "string"),
        ("width",    "int"),
        ("height",   "int"),
        ("opts",     "hash"),
    ],
    returns="bytes",
)
def generate(provider, prompt, width, height, opts):
    """Generate an image using `provider`. Each provider unpacks the keys it
    needs from `opts`; missing keys fall back to defaults. Returns raw image
    bytes (PNG / JPEG / WebP — provider chooses) ready for loadImage(bytes)
    on the kLex side."""
    if not prompt or not prompt.strip():
        raise ValueError("generate: prompt cannot be empty")
    p = _PROVIDERS.get(provider)
    if not p:
        raise ValueError(f"generate: unknown provider {provider!r}")
    started = time.time()
    notify({"phase": "submit", "provider": provider})
    data = p["generate"](prompt.strip(), width, height, opts or {})
    elapsed_ms = int((time.time() - started) * 1000)
    notify({"phase": "done", "bytes": len(data), "elapsed_ms": elapsed_ms,
            "provider": provider})
    if len(data) < 256:
        raise RuntimeError(f"{provider}: returned {len(data)} bytes — too small to be an image")
    return data


@handler(
    args=[("provider", "string"), ("opts", "hash")],
    returns="string",
)
def test_key(provider, opts):
    """Validate the credentials for `provider` without generating an image.
    Returns a one-line status string (e.g. "OK · username · 250 kudos") on
    success, raises on failure. Used by the Settings → Test button."""
    p = _PROVIDERS.get(provider)
    if not p:
        raise ValueError(f"test_key: unknown provider {provider!r}")
    return p["test"](opts or {})


@handler(args=[], returns="array")
def providers():
    """Return the list of provider ids the bridge supports. UI tabs read this
    so adding a backend on the Python side surfaces automatically without a
    kLex-side code change."""
    return list(_PROVIDERS.keys())


@handler(args=[], returns="string")
def backend():
    return "tadpole multi-provider (aihorde · huggingface · openai)"


if __name__ == "__main__":
    serve()
