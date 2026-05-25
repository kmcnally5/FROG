# Tadpole

A kLex-native desktop app for AI-driven image generation and agentic
chat. Runs on **macOS, Linux, and Windows** out of the box.

## What it does

- **Image generation** across five providers in a single UI:
  - **Local SD** (AUTOMATIC1111 / Forge / reForge) over pure HTTP — no
    Python bridge, no extra config beyond pointing at your local daemon
  - **AI Horde** — free, anonymous works
  - **Hugging Face** — free tier (token required)
  - **OpenAI** DALL·E — paid, best quality for prompt adherence
- **Agentic chat** backed by Claude or Ollama (qwen3 / llama3.1+ for
  tool use). Built-in tools for file reading, directory listing, HTTP
  fetch, file writing, shell, and launch — all OS-aware.
- **Non-destructive image adjustments** — Adjust tab with 10 real-time
  sliders (exposure, contrast, saturation, hue, vignette, sepia,
  brightness, gamma, invert, desaturate). Preview updates live; Apply
  bakes the result into the main image.
- **Themes** — five built-in UI palettes: `dark`, `crimson`, `midnight`,
  `forest`, `light`. Switchable from the Settings modal or via MCP.
- **Tadpole MCP server** — Tadpole exposes itself as an MCP server on
  `http://127.0.0.1:7778/sse` with 11 tools. Any external AI agent
  (Claude Code, another Tadpole, a script) can drive image generation,
  adjust sliders, send chat messages, query the execution tape, and more
  — without touching the keyboard.
- **Optional frogMcp integration** — drops in [`frogMcp`](snowball/frogMcp/)
  for first-class kLex-language navigation (`klex_search`,
  `klex_describe_symbol`, `klex_list_builtins`, …). Tadpole spawns the
  server automatically if it's discoverable.
- **Prompt caching** wired through Anthropic's `cache_control:
  ephemeral` markers — system prompt + tool definitions cached for
  ~90% input-token discount on repeated turns within 5 minutes.

## Quick start

```sh
# From the kLex repo root:
./klex examples/tadPole/tadPole.lex
```

That's the whole launch line. Tadpole opens a window. First-run defaults
ship with AI Horde as the provider — you can generate anonymously with
no setup. Open **Settings** (gear icon) to add API keys for other
providers.

## Per-OS notes

### macOS

Works as-is. No prerequisites beyond the `klex` binary and (optionally)
Python 3.10+ for frogMcp / the Python bridge providers.

System fonts (Tahoma / SF / Arial) are picked up automatically.

The `launch` tool uses `open` and honours the `app` argument — Claude
can open files with a specific app:
```
launch(app="Visual Studio Code", path="~/notes.md")
```

### Linux

Install a sans-serif font package (Tadpole auto-detects DejaVu and
Liberation):

```sh
# Debian / Ubuntu
sudo apt install fonts-dejavu fonts-liberation
# Fedora
sudo dnf install dejavu-sans-fonts liberation-fonts
```

The `launch` tool uses `xdg-open` — file associations come from your
desktop environment. The `app` argument is ignored on Linux (the
desktop handler is fixed by mimetype association).

The `shell` tool runs commands through `sh -c`.

### Windows

System fonts (Segoe UI / Arial / Tahoma) are picked up automatically.

The `launch` tool uses `cmd /c start` — opens files in their default
handler. The `app` argument is ignored on Windows.

The `shell` tool runs commands through `cmd /c` — **note this means
POSIX syntax (pipes, redirects, glob expansion) won't work**. Claude
sees the active shell in its system prompt and should adapt.

If `python` / `py` are on PATH, the Python bridge providers (Hugging
Face, OpenAI, AI Horde) and frogMcp will work. If not, Local SD is
still fully functional — it's pure HTTP, no Python required.

## Providers

### AI Horde (default, free, no key required)

Works out of the box. Anonymous queue is slow (30s – 3 min) but free.
Add a key in **Settings → AI Horde** to skip the back of the queue.

### Hugging Face

1. Get a token at https://huggingface.co/settings/tokens
2. **Settings → Hugging Face → API key** → paste → Save

### OpenAI

1. Get a key at https://platform.openai.com/api-keys
2. **Settings → OpenAI → API key** → paste → Save

### Local SD (AUTOMATIC1111 / Forge / reForge)

The fastest, most-private option once set up — runs on your own
hardware.

1. Install Forge: `git clone https://github.com/lllyasviel/stable-diffusion-webui-forge`
2. Download a model into `models/Stable-diffusion/` — **JuggernautXL v9**
   from RunDiffusion on Hugging Face is the recommended photorealism
   model:
   ```sh
   curl -L -o juggernautXL_v9.safetensors \
     "https://huggingface.co/RunDiffusion/Juggernaut-XL-v9/resolve/main/Juggernaut-XL_v9_RunDiffusionPhoto_v2.safetensors"
   ```
   (~6.6GB — `aria2c -x 16 -s 16` is dramatically faster than curl on
   slow links.)
3. Launch Forge with API enabled:
   ```sh
   cd stable-diffusion-webui-forge && ./webui.sh --api
   ```
   On Windows: `webui-user.bat` (edit to add `--api` to the COMMANDLINE_ARGS).
4. In Tadpole: **Settings → Local SD (a1111)** → Test → Save.
5. Switch the main panel provider to **Local SD**.

#### Apple Silicon (M-series) gotcha

DPM++ samplers and **"Euler a"** produce solid-black images on MPS
([Forge issue #978](https://github.com/lllyasviel/stable-diffusion-webui-forge/issues/978)).
Tadpole's defaults are MPS-safe: **Euler / DDIM / UniPC**. The Web UI
masks the bug; the API doesn't.

### Claude (chat backend)

1. Get a key at https://console.anthropic.com/settings/keys
2. **Settings → Claude → API key** → paste → Save

### Ollama (chat backend)

1. Install Ollama: https://ollama.com/download
2. Pull a tool-capable model: `ollama pull qwen3` (or `llama3.1:8b` etc.)
3. **Settings → Ollama → Base URL** (`http://127.0.0.1:11434`) →
   pick model from dropdown → Save → **"Use for chat"**

## Optional: frogMcp for kLex-aware chat

[frogMcp](snowball/frogMcp/) is an MCP server that exposes
13 tools for navigating a kLex codebase (`klex_search`,
`klex_describe_symbol`, `klex_list_builtins`, …). When wired in,
Claude can answer kLex questions with authoritative data from the real
source index instead of hallucinating builtin signatures.

**Tadpole auto-discovers** frogMcp at startup. It checks (in order):

1. `$KLEX_HOME/snowball/frogMcp/server/server.py`
2. `$KLEX_PATH/snowball/frogMcp/server/server.py`
3. `<tadPole-dir>/../../../snowball/frogMcp/server/server.py` (dev checkout)
4. `~/.tadpole/frogmcp/server.py` (user install)

The first one that exists wins. **If none found, MCP is skipped
silently** — Tadpole still works fine with its built-in tools.

**Setup (one-time):**

```sh
# From the kLex repo root:
pip install --user 'mcp>=1.0' 'watchdog>=3.0'
./snowball/frogMcp/indexer/frogMcp-indexer   # builds index.json
```

That's it. Restart Tadpole — `mcpStatus` becomes `ready` within ~2s of
the window opening. Try asking Claude *"What does the kLex builtin
makeArray do?"* — it'll call `klex_describe_symbol` directly.

## Tadpole as an MCP server

Tadpole starts its own MCP server on `http://127.0.0.1:7778/sse` at
launch. Any MCP-capable client — Claude Code, another script, a second
Tadpole — can connect and drive it remotely. No configuration required;
the server comes up automatically and shuts down cleanly when the window
closes.

**11 tools exposed:**

| Tool | What it does |
|---|---|
| `chat` | Inject a message into the chat agent — same path as clicking Send in the UI. Returns immediately; poll `list_history` for the reply. |
| `list_history` | Return recent chat messages (most-recent-last). Optional `limit` cap. |
| `current_state` | Snapshot: active provider, chat backend, message counts, send/generating flags, session token usage. |
| `list_providers` | List all configured image providers and chat backends. |
| `generate_image` | Trigger a generation on the currently selected provider. Accepts `prompt` and optional `negative_prompt`. Returns immediately; poll `current_state.generating`. |
| `transform` | Stateless one-shot text transformation via the local Ollama backend (Gemma). No chat history, no agent loop. Capped at 32 KB input. |
| `tape_query` | Read events from Tadpole's `--record-tape` execution log. Filter by event kind; return last N events. Path whitelisted to `/tmp/*.lextape` or `~/.tadpole/*.lextape`. |
| `set_right_tab` | Switch the right panel to `image`, `adjust`, or `chat`. |
| `set_theme` | Change the active UI theme: `dark`, `crimson`, `midnight`, `forest`, or `light`. Repaints on the next frame. |
| `set_adjust` | Set any subset of the Adjust-tab sliders in one call. Values are clamped to their UI ranges. Live preview refreshes immediately. |
| `reset_adjust` | Snap every Adjust slider back to its zero-effect default. |

**Connecting from Claude Code:**

Add to your `claude_mcp_config.json` (or `~/.claude/mcp_config.json`):

```json
{
  "mcpServers": {
    "tadpole": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-sse-client", "http://127.0.0.1:7778/sse"]
    }
  }
}
```

Tadpole must already be running before Claude Code connects.

**Using `tape_query` with `--record-tape`:**

Run Tadpole with the tape flag to capture a full execution log:

```sh
./klex --record-tape=/tmp/tadpole.lextape examples/tadPole/tadPole.lex
```

Then from any connected MCP client:

```
tape_query(kind="bridge_call", last_n=10)
tape_query(kind="async_done", last_n=20)
tape_query(kind="error")
```

This gives an external agent a real-time, causal view of everything
Tadpole's runtime is doing — bridge calls to Python providers, async
task completions, UI events — without any screen-scraping or polling.

---

## Configuration

Per-user settings (API keys, default provider, etc.) live at:

- macOS / Linux: `~/.tadpole/config.json`
- Windows: `%USERPROFILE%\.tadpole\config.json`

Plus `~/.tadpole/last_session.json` for chat history. Both are JSON —
safe to inspect, edit, or delete.

## Troubleshooting

### "no usable font found"

Install a sans-serif font (see the Linux per-OS notes above).

### Tadpole opens but Claude says "no Claude API key configured"

Open **Settings → Claude** and paste your key. The error message is
correct — chat won't work without it.

### Local SD generates black images

Your sampler isn't MPS-safe — switch to **Euler / DDIM / UniPC** in
**Settings → Local SD**. See the Apple Silicon gotcha above.

### MCP status says "disabled"

frogMcp wasn't found at any of the search paths. Either install it
(see the frogMcp section), or set `KLEX_HOME` to your kLex repo root.
Tadpole continues to work with built-in tools either way.

### MCP status says "failed"

Python wasn't found, or the `mcp` / `watchdog` packages aren't
installed. Run `pip install --user 'mcp>=1.0' 'watchdog>=3.0'` and
restart Tadpole.

### Tadpole MCP server (port 7778) not reachable

Check the terminal where you launched Tadpole — on success you'll see:

```
[mcp-server] up on http://127.0.0.1:7778/sse — 11 tools exposed
```

If you see `[mcp-server] failed to start: ...`, the port is likely
already in use. Close any other running Tadpole instance and restart.

## Architecture

```
┌──────────────────────────────────────────────────────────────────────┐
│ Tadpole (tadPole.lex)                                                │
│                                                                      │
│  ┌─────────────┐  ┌────────────────┐  ┌───────────┐  ┌───────────┐  │
│  │ Image gen   │  │ Agentic chat   │  │ Adjust    │  │ Settings  │  │
│  │             │  │ (Claude/Ollama)│  │ tab       │  │ (modal)   │  │
│  └──────┬──────┘  └───────┬────────┘  └─────┬─────┘  └───────────┘  │
│         │                 │                 │                        │
│  ┌──────┴──────┐  ┌───────┴──────┐  ┌───────┴──────┐                │
│  │ sd.lex      │  │ Built-in     │  │ MCP client   │                │
│  │ (HTTP)      │  │ tools        │  │ (→ frogMcp)  │                │
│  └──────┬──────┘  └───────┬──────┘  └───────┬──────┘                │
│         │                 │                 │                        │
│  ┌──────┴──────────────────────────────────────────────────────────┐ │
│  │  Tadpole MCP server  http://127.0.0.1:7778/sse  (11 tools)     │ │
│  └─────────────────────────────────────────────────────────────────┘ │
└───────────┬─────────────────┬──────────────────┬────────────────────┘
            │                 │                  │
   ┌────────▼───────┐ ┌───────▼──────┐  ┌────────▼───────┐
   │ Forge / Horde  │ │ OS shell /   │  │ frogMcp        │
   │ / HF / OpenAI  │ │ filesystem / │  │ (Python MCP    │
   │ (Python bridge │ │ HTTP         │  │  server)       │
   │  or pure HTTP) │ └──────────────┘  └────────────────┘
   └────────────────┘
          ▲
   ┌──────┴────────────────────────────┐
   │ External MCP clients              │
   │ (Claude Code, scripts, agents)    │
   └───────────────────────────────────┘
```

## License

Same as kLex.
