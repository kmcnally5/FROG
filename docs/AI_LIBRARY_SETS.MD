# kLex AI Libraries

A complete reference for the AI libraries shipped in `stdlib/ai/`. Covers the easy-path facade, all provider libraries, the RAG pipeline, structured output, image generation, the eval framework, and the design pattern for adding new providers.

---

## Quick Start

For most use cases, import `quick.lex`:

```
import "stdlib/ai/quick.lex" as ai

ans = ai.ask("What is the capital of France?")?
println(ans)
```

`quick.lex` defaults to Claude (reads `ANTHROPIC_API_KEY`). Switch to a local Ollama model with one call:

```
ai.useOllama("llama3.2")
ans = ai.ask("Summarise this text...")?
```

No client construction, no response parsing, no boilerplate. The full provider libraries (`anthropic.lex`, `ollama.lex`, etc.) are available when you need the complete API surface.

---

## Library Layout

```
stdlib/ai/
├── ai_common.lex      — shared types (Message, Usage) + cost ledger
├── anthropic.lex      — Anthropic Claude — reference implementation
├── ollama.lex         — local LLMs via Ollama HTTP
├── voyage.lex         — Voyage AI embeddings
├── sd.lex             — Stable Diffusion / AUTOMATIC1111 image generation
├── quick.lex          — friendly one-liner facade (wraps all providers)
├── schema.lex         — JSON Schema builder for structured output
├── vector_store.lex   — in-memory K-NN store for RAG
├── chunk.lex          — text chunking for RAG ingestion
└── evals.lex          — parallel AI eval framework
```

---

## Why Library Sets

The temptation when adding LLMs to a language is to build one unified primitive:

```
result = ai.ask("Hello")   // works on any provider, magically
```

That sounds clean. It isn't. Every provider has a distinct feature surface — Anthropic's batches API, Ollama's local quirks and reasoning-model thinking traces, Voyage's document/query embedding modes, Stable Diffusion's txt2img/img2img/progress pipeline. Any "unified" wrapper either:

- **Picks the lowest common denominator** — you lose most of each provider's value
- **Bolts on feature flags** — the abstraction leaks immediately
- **Falls behind** — providers ship faster than abstractions can follow

kLex takes the opposite stance: **one library per provider, each exposing that provider's full surface in kLex-native shapes.** Cohesion comes from shared *types* (`ai.Message`, `ai.Usage`), not a shared *interface*. `quick.lex` exists as a deliberate convenience layer on top — not a replacement for the real APIs.

---

## ai_common.lex — Shared Vocabulary

```
import "stdlib/ai/ai_common.lex" as ai
```

Shared types and helpers every provider library imports. No HTTP, no provider logic, no agent loops.

### Types

```
struct Message { role, content }
struct Usage   { input_tokens, output_tokens, cost_usd }
```

`content` is either a plain string or an array of content blocks (mixed text + image, tool I/O, etc.). Provider libraries translate this to their wire format internally.

### Message constructors

| Function | Returns |
|---|---|
| `userMsg(content)` | `Message { role: "user", content }` |
| `assistantMsg(content)` | `Message { role: "assistant", content }` |

### Content-block helpers

| Function | Returns |
|---|---|
| `textBlock(text)` | `{ type: "text", text }` |
| `imageBlock(base64Data, mediaType)` | `{ type: "image", source: { type: "base64", ... } }` |

### Cost ledger

Process-wide spend tracking. All provider libraries (`anthropic.lex`, `voyage.lex`) call `_record()` automatically after every API call so the ledger stays current without manual bookkeeping. Ollama records zero-cost token counts (it's local).

| Function | Description |
|---|---|
| `budget(limitUsd)` | Set a hard spend limit in USD for this process |
| `budgetExceeded()` | Returns `true` if accumulated spend has passed the limit |
| `spent()` | Returns total USD spent so far as a float |
| `usage()` | Returns `{ total_input_tokens, total_output_tokens, total_cost_usd, calls }` |
| `resetUsage()` | Reset all counters and the budget limit |
| `_record(model, inputTok, outputTok, inputRate, outputRate)` | Internal — called by provider libs |
| `_budgetError()` | Internal — returns the typed `AI_BUDGET_EXCEEDED` error |

---

## quick.lex — Easy-Path Facade

```
import "stdlib/ai/quick.lex" as ai
```

Wraps the five most-used patterns behind simple entry points. Provider-switches are global — `useClaude()` / `useOllama()` flips every subsequent call. Default is Claude.

All public functions return `(value, err)` tuples. Use the `?` propagation operator for clean error handling.

### Provider switching

| Function | Description |
|---|---|
| `useClaude(modelName = null)` | Switch to Anthropic. `null` uses `KLEX_AI_MODEL` env var or `claude-opus-4-7` |
| `useOllama(modelName = null)` | Switch to local Ollama. `null` uses `KLEX_AI_MODEL` or `llama3.2` |
| `currentProvider()` | Returns `"claude"` or `"ollama"` |
| `allowOllamaTools(allow)` | Opt in to `executeQuery()` on Ollama. Off by default — gated because older models silently ignore tool definitions |

### Core API

| Function | Returns | Description |
|---|---|---|
| `ask(question, opts = null)` | `(string, err)` | Fire-and-forget Q&A. opts: `system`, `max_tokens`, `temperature`, `model` |
| `executeQuery(question, tools, opts = null)` | `(string, err)` | Natural-language tool dispatch. `tools` may be a single function, a `{name: fn}` hash, or a full tool-spec array |
| `extract(text, jsonSchema)` | `(hash, err)` | Structured output matching a JSON Schema (build with `schema.lex`) |
| `classify(text, labels)` | `(string, err)` | Returns one of the supplied string labels |
| `askCached(question, opts = null)` | `(string, err)` | Same as `ask()`, with an in-memory exact-match cache keyed by `(provider, model, question, opts)` |
| `askDocument(path, question, opts = null)` | `(string, err)` | RAG answer grounded in a local file or PDF |

### Cache management

| Function | Description |
|---|---|
| `clearCache()` | Flush the `askCached()` response cache |
| `cacheSize()` | Number of cached responses |
| `clearDocumentCache()` | Flush the `askDocument()` vector-store cache |
| `documentCacheSize()` | Number of cached document stores |

### askDocument pipeline

`askDocument` implements a full RAG pipeline behind a single function call:

1. Extract text — PDFs via `stdlib/pdf.lex`, plain text via `readFile`
2. Chunk into paragraph-sized pieces (default 800 chars) using `chunk.byParagraphs`
3. Embed each chunk via Ollama (default `mxbai-embed-large`)
4. Embed the question, retrieve the top-K most similar chunks
5. Build a grounded prompt and ask the active chat provider

The per-document vector store is cached by `(path, embedModel, chunkSize)` and invalidated when the file's mtime changes — repeat questions on the same document are cheap.

Options hash:

| Key | Default | Description |
|---|---|---|
| `embedModel` | `"mxbai-embed-large"` | Ollama model for embeddings. Pull once: `ollama pull mxbai-embed-large` |
| `embedHost` | `null` (localhost:11434) | Ollama daemon host for embeddings |
| `chunkSize` | `800` | Characters per chunk |
| `topK` | `4` | How many chunks to feed the answering model |
| `system` | RAG default | Override the system prompt |
| `max_tokens` | `1024` | Answer length cap |

---

## anthropic.lex — Anthropic Claude

```
import "stdlib/ai/anthropic.lex"    as claude
import "stdlib/ai/ai_common.lex"    as ai

c = claude.newClient(env("ANTHROPIC_API_KEY"), "claude-opus-4-7")
resp, err = claude.messages(c, {
    "system":   "You are concise.",
    "messages": [ai.userMsg("What is kLex?")],
})
if err != null { println(err.message)  return }
println(claude.textOf(resp))
```

Pure-HTTP implementation. Zero external dependencies — no Python or Node bridge. Reference implementation for the provider-library pattern.

**Models (as of klex 0.3.35):** `claude-opus-4-7`, `claude-opus-4-5`, `claude-sonnet-4-6`, `claude-sonnet-4-5`, `claude-haiku-4-5`, `claude-3-5-sonnet`, `claude-3-5-haiku`, `claude-3-opus`. Pricing is built in; the cost ledger updates automatically.

### Client

```
c = claude.newClient(apiKey, model)
// model = null falls back to DEFAULT_MODEL (claude-opus-4-7)
```

### Core API

| Function | Returns | Description |
|---|---|---|
| `messages(c, opts)` | `(resp, err)` | `/v1/messages` — canonical chat completion |
| `stream(c, opts)` | `(channel, err)` | Streaming chat via SSE. Channel yields event hashes (see below) |
| `complete(c, jsonSchema, prompt)` | `(hash, err)` | Structured output using a JSON Schema |
| `completeWith(c, jsonSchema, prompt, opts)` | `(hash, err)` | `complete()` with full option control |
| `countTokens(c, opts)` | `(int, err)` | `/v1/messages/count_tokens` — token budget pre-check |
| `listModels(c)` | `(array, err)` | Enumerate available models on this API key |
| `runAgent(c, opts, maxSteps)` | `(resp, conv, err)` | Multi-turn tool-use loop with automatic dispatch |

**`messages()` opts hash** — all optional except `messages`:

| Key | Description |
|---|---|
| `messages` | **Required.** Array of `ai.Message` or message hashes |
| `system` | System prompt string |
| `model` | Override client default |
| `max_tokens` | Default: 4096 |
| `temperature` | Float 0.0–1.0 |
| `top_p`, `top_k` | Sampling controls |
| `stop_sequences` | Array of strings |
| `tools` | Array of tool hashes (include `handler` for `runAgent`) |
| `tool_choice` | `{"type": "auto"}` or `{"type": "tool", "name": "..."}` |
| `metadata` | Passthrough hash (`user_id`, etc.) |

### Streaming events

`stream()` returns a channel; iterate with `for evt in ch`. Each event is a hash with a `type` field:

| `type` | Fields | Description |
|---|---|---|
| `"text"` | `text` | A chunk of the assistant's text response |
| `"tool_use"` | `id`, `name` | A tool call has begun |
| `"tool_input"` | `partial_json` | Streaming fragment of a tool's input JSON |
| `"done"` | `stop_reason`, `usage` | Terminal event |
| `"error"` | `message` | Request or stream error |

### runAgent — tool-use loop

Each tool hash must include:

```
{
    "name":         "functionName",
    "description":  "What this tool does",
    "input_schema": { ... },   // JSON Schema
    "handler":      fn(inputHash) { ... },   // kLex function
}
```

`runAgent` strips `handler` before sending to the API (Anthropic rejects unknown fields), dispatches tool calls automatically, and loops until `stop_reason != "tool_use"` or `maxSteps` is reached. Pass `maxSteps = 0` to use the default (12).

Returns `(finalResponse, conversation, err)`. `conversation` is the full accumulated message history.

### Response accessors

| Function | Description |
|---|---|
| `textOf(resp)` | Concatenated text from all `text` content blocks |
| `toolCallsOf(resp)` | Array of `{type, id, name, input}` tool-use blocks |
| `stopReasonOf(resp)` | `"end_turn"`, `"max_tokens"`, `"stop_sequence"`, `"tool_use"` |
| `usageOf(resp)` | `{input_tokens, output_tokens, cost_usd}` |
| `toolResultBlock(toolUseId, content, isError)` | Build a `tool_result` block for a follow-up message |

### Message Batches API

Submit up to 100,000 requests asynchronously at 50% of the regular Messages API cost. Batches complete within 24 hours. Ideal for evals, dataset generation, and throughput-tolerant workloads.

| Function | Returns | Description |
|---|---|---|
| `buildBatchRequest(customId, opts)` | hash | Shape one `{custom_id, params}` request for `createBatch()` |
| `createBatch(c, requests)` | `(batch, err)` | Submit the batch |
| `getBatch(c, batchId)` | `(batch, err)` | Current status |
| `awaitBatch(c, batchId, opts)` | `(batch, err)` | Poll until terminal state. opts: `pollIntervalMs` (default 30000), `deadlineMs`, `onPoll` callback |
| `batchResults(c, batchId)` | `(array, err)` | Fetch parsed JSONL results. Each entry: `{custom_id, result}` where `result.type` is `"succeeded"`, `"errored"`, `"canceled"`, or `"expired"` |
| `listBatches(c, opts)` | `(array, err)` | Enumerate batches. opts: `limit`, `before_id`, `after_id` |
| `cancelBatch(c, batchId)` | `(batch, err)` | Cancel a batch still processing |
| `deleteBatch(c, batchId)` | `(null, err)` | Remove an ended batch |

### Typed error codes

`ANTHROPIC_AUTH`, `ANTHROPIC_FORBIDDEN`, `ANTHROPIC_NOT_FOUND`, `ANTHROPIC_RATE_LIMIT`, `ANTHROPIC_SERVER`, `ANTHROPIC_NETWORK`, `ANTHROPIC_BAD_ARGS`, `ANTHROPIC_PARSE`, `ANTHROPIC_NO_STRUCTURED_OUTPUT`, `ANTHROPIC_MAX_STEPS`, `ANTHROPIC_BATCH_NOT_READY`, `ANTHROPIC_BATCH_DEADLINE`

---

## ollama.lex — Local LLMs via Ollama

```
import "stdlib/ai/ollama.lex"     as ollama
import "stdlib/ai/ai_common.lex"  as ai

c = ollama.newClient(null, "llama3.2")   // null = http://localhost:11434
resp, err = ollama.chat(c, {
    "messages": [ai.userMsg("Why is the sky blue?")],
})
if err != null { println(err.message)  return }
println(ollama.textOf(resp))
```

Pure-HTTP implementation. Talks directly to a running Ollama daemon. No API key. Zero cost. Start the daemon: `ollama serve`. Pull a model: `ollama pull llama3.2`.

Default host: `http://localhost:11434`. Override via `baseUrl` at construction or via the `OLLAMA_HOST` env var at the call site. Default timeout: 300 s (generous for cold-loading large models).

### Client

```
c = ollama.newClient(baseUrl, model)
// baseUrl = null → DEFAULT_BASE_URL
// model   = null → DEFAULT_MODEL (llama3.2)
```

### Core API

| Function | Returns | Description |
|---|---|---|
| `chat(c, opts)` | `(resp, err)` | `/api/chat` — canonical chat completion (non-streaming) |
| `generate(c, opts)` | `(resp, err)` | `/api/generate` — single-prompt completion, no history |
| `embed(c, input)` | `(resp, err)` | `/api/embed` — embedding vectors. `input` may be string or array of strings |
| `stream(c, opts)` | `(channel, err)` | Streaming chat via NDJSON. Same event shape as `anthropic.stream()` |
| `complete(c, jsonSchema, prompt)` | `(hash, err)` | Structured output using Ollama's native `format` parameter |
| `completeWith(c, jsonSchema, prompt, opts)` | `(hash, err)` | `complete()` with full option control |
| `runAgent(c, opts, maxSteps)` | `(resp, conv, err)` | Multi-turn tool-use loop. Accepts both Anthropic-shape and Ollama-shape tool hashes |
| `listModels(c)` | `(array, err)` | `/api/tags` — locally installed models |
| `ps(c)` | `(array, err)` | `/api/ps` — models currently loaded in memory |
| `show(c, modelName)` | `(hash, err)` | `/api/show` — full model metadata including capabilities |
| `hasCapability(showResp, capName)` | `bool` | True if the `show()` response lists the given capability |

**`chat()` opts hash** — all optional except `messages`:

| Key | Description |
|---|---|
| `messages` | **Required.** Array of `ai.Message` or message hashes |
| `system` | System prompt (auto-prepended as a `system` role message) |
| `model` | Override client default |
| `format` | `"json"` or a JSON Schema hash for structured output |
| `options` | Ollama generation params hash: `temperature`, `num_predict`, `top_p`, `top_k`, `num_ctx`, `seed`, etc. |
| `keep_alive` | Model unload time (string or int) |
| `think` | `true` to enable chain-of-thought on reasoning models (qwen3, deepseek-r1, etc.) |
| `tools` | Array of tool hashes |
| `timeout_sec` | Per-call HTTP timeout override (default 300) |

### Capabilities (from show())

Pass the `show()` response to `hasCapability()` before sending requests the model would silently ignore:

```
info, err = ollama.show(c, null)
if err == null && ollama.hasCapability(info, "tools") {
    // safe to pass tools to chat() / stream()
}
```

Known capability names: `"completion"`, `"tools"`, `"vision"`, `"audio"`, `"thinking"`, `"embedding"`, `"insert"`.

### Reasoning models

Models like `qwen3`, `deepseek-r1`, and `phi4-reasoning` emit a chain-of-thought phase before the answer. Set `"think": true` in opts to enable it. In streaming, reasoning content arrives as `"thinking"` events; the final answer arrives as `"text"` events.

### Streaming events

Same shape as `anthropic.lex` for consumer portability:

| `type` | Fields | Description |
|---|---|---|
| `"text"` | `text` | Chunk of the final answer |
| `"thinking"` | `text` | Chain-of-thought fragment (reasoning models only) |
| `"tool_use"` | `name`, `input` | Tool call. Ollama emits the full input hash at once (not streamed by delta) |
| `"done"` | `done_reason`, `usage` | Terminal event |
| `"error"` | `message` | Request or stream error |

### Response accessors

| Function | Description |
|---|---|
| `textOf(resp)` | Assistant text from `chat()` response |
| `toolCallsOf(resp)` | Array of `{function: {name, arguments}}` tool-call hashes |
| `doneReasonOf(resp)` | `"stop"`, `"length"`, `"tool_calls"` |
| `usageOf(resp)` | `{input_tokens, output_tokens, cost_usd: null}` |

### Typed error codes

`OLLAMA_BAD_ARGS`, `OLLAMA_HTTP_ERROR`, `OLLAMA_NOT_FOUND`, `OLLAMA_SERVER`, `OLLAMA_OFFLINE`, `OLLAMA_NETWORK`, `OLLAMA_PARSE`, `OLLAMA_NO_STRUCTURED_OUTPUT`, `OLLAMA_INVALID_JSON`, `OLLAMA_MAX_STEPS`

---

## voyage.lex — Voyage AI Embeddings

```
import "stdlib/ai/voyage.lex"       as voyage
import "stdlib/ai/vector_store.lex" as vs

c     = voyage.newClient(env("VOYAGE_API_KEY"), "voyage-3")
store = vs.newStore()

resp, err = voyage.embed(c, "the quick brown fox")
if err != null { println(err.message)  return }
store.add("doc0", resp["embeddings"][0], {"text": "the quick brown fox"})
```

Voyage is Anthropic's officially recommended embedding provider. Anthropic does not ship its own embedding model; a Claude-primary stack pairs Anthropic for chat with Voyage for vectors.

Pure-HTTP implementation. Zero external dependencies.

**Models:** `voyage-3` (default), `voyage-3-lite`, `voyage-3-large`, `voyage-code-3`, `voyage-finance-2`, `voyage-multilingual-2`, `voyage-law-2`.

The response shape (`{embeddings: [[…], …]}`) matches `ollama.embed` so the same `vector_store` calls work unchanged regardless of which provider produced the vector.

### Client

```
c = voyage.newClient(apiKey, model)
// model = null → DEFAULT_MODEL (voyage-3)
```

### API

| Function | Returns | Description |
|---|---|---|
| `embed(c, input)` | `(resp, err)` | Embed a string or array of strings |
| `embedWith(c, input, opts)` | `(resp, err)` | `embed()` with per-call options |
| `usageOf(resp)` | hash | `{input_tokens, output_tokens, cost_usd: null}` |
| `embeddingsOf(resp)` | array | `[[float, …], …]` — convenience accessor |

**`embedWith()` opts hash:**

| Key | Description |
|---|---|
| `model` | Override client default |
| `input_type` | `"document"` or `"query"` — Voyage tunes the vector for retrieval pair sides |
| `truncation` | Bool — silently truncate oversized inputs rather than error |
| `output_dtype` | `"float"`, `"int8"`, `"uint8"`, `"binary"`, `"ubinary"` |
| `output_dimension` | Reduced-dimension vector size (model-dependent) |

Use `"input_type": "document"` when embedding corpus text, and `"input_type": "query"` when embedding search queries for best retrieval quality.

### Typed error codes

`VOYAGE_AUTH`, `VOYAGE_RATE_LIMIT`, `VOYAGE_HTTP_ERROR`, `VOYAGE_NETWORK`, `VOYAGE_BAD_ARGS`

---

## chunk.lex — Text Chunking for RAG

```
import "stdlib/ai/chunk.lex" as chunk
```

Splits text into chunks for embedding and retrieval. Each strategy returns an array of hashes:

```
{"index": int, "start": int, "end": int, "text": string}
```

`start` and `end` are character offsets into the original text — useful for source attribution and citation.

### Strategies

| Function | Description |
|---|---|
| `byChars(text, size, overlap)` | Fixed-character windows. Simple, fast, language-agnostic |
| `byWords(text, size, overlap)` | Fixed-word-count windows. Chunks land on word boundaries. `size` = word count |
| `byParagraphs(text, maxChars)` | Split on blank lines, greedily merge up to `maxChars`. Pass `maxChars=0` for one chunk per paragraph. Best default for prose |

### Sizing guide

Embedding APIs cap input at ~8k tokens ≈ 32k chars (English). A typical RAG setup uses 500–1000 char chunks with 100–200 char overlap. Larger chunks give richer context per hit; smaller chunks give more distinct hits. Tune to your retrieval pattern.

For paragraphs longer than `maxChars`, `byParagraphs` emits an oversized single chunk. Pass oversized chunks through `byChars()` afterwards if a strict cap is required.

---

## vector_store.lex — In-Memory K-NN Store

```
import "stdlib/ai/vector_store.lex" as vs

store = vs.newStore()
store.add("doc0", embeddingVector, {"text": "...", "source": "file.txt"})

hits = store.search(queryVector, 5)
for h in hits {
    println(str(h["score"]) + "  " + h["metadata"]["text"])
}
```

A dependency-free K-nearest-neighbour store for RAG, semantic search, and clustering. Vectors come from any embedding source — `ollama.embed`, `voyage.embed`, or any provider that returns a float array.

Storage is linear-scan. Suitable for up to ~10k entries with typical embedding dimensions (768–1024). For larger corpora a hosted vector DB (Chroma, Qdrant) is more appropriate.

Three Go builtins back the math: `cosineSim(a, b)`, `dotProduct(a, b)`, `vecNorm(v)`.

### API

Create a store with `vs.newStore()`. The returned `VectorStore` object has these methods:

| Method | Returns | Description |
|---|---|---|
| `store.add(id, vector, metadata)` | null | Insert or replace an entry. `metadata` is any kLex value |
| `store.delete(id)` | bool | Remove by id. Returns true if found |
| `store.get(id)` | hash or null | Fetch `{id, vector, metadata}` by id |
| `store.search(queryVector, k)` | array | Top-k by cosine similarity. Each result: `{id, score, metadata}` |
| `store.searchScored(queryVector, minScore)` | array | All entries with `score >= minScore`, sorted descending |
| `store.count()` | int | Number of stored entries |
| `store.ids()` | array | All stored ids in insertion order |
| `store.clear()` | null | Wipe all entries |

---

## schema.lex — JSON Schema Builder

```
import "stdlib/ai/schema.lex" as schema

PersonSchema = schema.object({
    "name":  schema.string("Full name"),
    "age":   schema.integer("Age in years", 0, 150),
    "email": schema.string("Email address"),
})

// Pass directly to complete() on any provider
person, err = claude.complete(c, PersonSchema, "Generate a fake profile")
println(person["name"])
```

Builds JSON Schema 2020-12 hashes for use with provider `complete()` / `completeWith()` functions. All builders are pure functions — no side effects, no global state. Hashes compose recursively.

### Builders

| Function | Description |
|---|---|
| `string(description = null)` | String-type field |
| `integer(description = null, minimum = null, maximum = null)` | Integer field with optional bounds |
| `number(description = null, minimum = null, maximum = null)` | Float field with optional bounds |
| `boolean(description = null)` | Boolean field |
| `array(items, description = null)` | Array where every element matches `items` schema |
| `object(props, required = null, description = null)` | Object with named properties. Default: all properties required |
| `enumOf(values, description = null)` | String constrained to one of `values` |
| `nullable(inner)` | Allow null in addition to the inner type |

`object()` defaults all properties to required — the safer behaviour for AI prompts where silent omissions produce confusing output. Pass an explicit `required` array to mark specific fields as optional:

```
schema.object({
    "name":  schema.string(),
    "email": schema.string(),
}, ["name"])   // only name is required; email is optional
```

---

## sd.lex — Stable Diffusion Image Generation

```
import "stdlib/ai/sd.lex" as sd

c = sd.newClient(null)   // null = http://127.0.0.1:7860
pngBytes, err = sd.generate(c, {
    "prompt":          "a watercolor landscape at sunset",
    "negative_prompt": "ugly, blurry, low quality",
    "width":           768,
    "height":          768,
    "steps":           25,
    "cfg_scale":       7.0,
    "sampler_name":    "Euler a",
    "seed":            -1,
})
if err != null { println(err.message)  return }
img = loadImage(pngBytes)
saveImage(img, "/tmp/out.png")
```

Pure-HTTP implementation against the AUTOMATIC1111 / Forge / Reforge `/sdapi/v1` surface. No external dependencies. Returned image bytes feed directly into `loadImage(bytes)` — no disk round-trip required.

Default host: `http://127.0.0.1:7860`. The daemon must be started with `--api` enabled.

### Client

```
c = sd.newClient(baseUrl)                  // no auth
c = sd.newClientAuth(baseUrl, authHeader)  // nginx basic-auth or Bearer token
```

### Generation

| Function | Returns | Description |
|---|---|---|
| `generate(c, opts)` | `(bytes, err)` | txt2img — returns the first PNG as bytes |
| `generateBatch(c, opts)` | `(array, err)` | txt2img — returns every PNG in the batch |
| `img2img(c, opts)` | `(bytes, err)` | img2img — initialise from `opts["init_image"]` (bytes) or `opts["init_images"]` (array of bytes) |

**Common `generate()` opts:**

| Key | Description |
|---|---|
| `prompt` | **Required.** Positive prompt |
| `negative_prompt` | Things to avoid |
| `width`, `height` | Integers (multiples of 8) |
| `steps` | Sampling steps (default 20) |
| `cfg_scale` | Classifier-free guidance (default 7.0) |
| `sampler_name` | e.g. `"Euler a"`, `"DPM++ 2M Karras"` |
| `scheduler` | e.g. `"Automatic"`, `"Karras"` |
| `seed` | `-1` = random |
| `batch_size` | Images per batch (default 1) |
| `n_iter` | Number of batches (default 1) |
| `timeout_sec` | Per-call HTTP timeout override (default 600) |

Any key accepted by A1111's `/sdapi/v1/txt2img` or `/sdapi/v1/img2img` passes through verbatim.

### Daemon introspection

| Function | Returns | Description |
|---|---|---|
| `models(c)` | `(array, err)` | Installed checkpoints. Each entry: `{title, model_name, hash, sha256, …}` |
| `samplers(c)` | `(array, err)` | Available samplers |
| `schedulers(c)` | `(array, err)` | Available schedulers (A1111 1.9+) |
| `setModel(c, title)` | `(null, err)` | Switch active checkpoint. `title` from `models()` |
| `progress(c)` | `(hash, err)` | `{progress, eta_relative, state, textinfo}` |
| `progressWithPreview(c)` | `(hash, err)` | Same but includes in-flight preview PNG |
| `interrupt(c)` | `(null, err)` | Cancel the in-flight generation |
| `options(c)` | `(hash, err)` | Full `/sdapi/v1/options` hash |
| `setOption(c, key, value)` | `(null, err)` | Write a single option key without clobbering others |
| `ping(c)` | `(bool, err)` | `true` if the daemon answers within the fast timeout |

### Typed error codes

`SD_NETWORK`, `SD_HTTP_ERROR`, `SD_NOT_FOUND`, `SD_AUTH`, `SD_SERVER`, `SD_BAD_JSON`, `SD_BAD_OPTS`, `SD_NO_IMAGES`, `SD_BAD_IMAGE`

---

## evals.lex — AI Eval Framework

```
import "stdlib/ai/evals.lex"    as evals
import "stdlib/ai/ollama.lex"   as ollama
import "stdlib/ai/ai_common.lex" as ai

cases = [
    {"input": "What is 2+2?",        "expected": "4"},
    {"input": "Capital of France?",  "expected": "Paris"},
]

results = evals.run(cases, fn(tc) {
    resp, _ = ollama.chat(c, {"messages": [ai.userMsg(tc["input"])]})
    return ollama.textOf(resp)
}, evals.contains, {"workers": 4})

evals.report(results)
```

Parallel eval harness built on kLex's native async + channel primitives. Case execution runs across N workers. Results are returned in case order regardless of completion order.

Each result hash: `{idx, case, actual, score, pass, error, duration_ms}`.

### Running evals

| Function | Returns | Description |
|---|---|---|
| `run(cases, runFn, scoreFn, opts)` | array | Parallel execution. opts: `workers` (default 4), `passThreshold` (default 1.0) |
| `runSerial(cases, runFn, scoreFn, opts)` | array | Sequential execution. Useful for deterministic logging or rate-limited APIs |
| `compare(cases, runners, scoreFn, opts)` | hash | Run cases through multiple named runners. Returns `{runnerName: results_array}` |

**`runFn` signature:** `fn(testCase) -> any`

**`scoreFn` signature:** `fn(actual, testCase) -> bool | float`

Scorer return values: `true/false` for pass/fail; a float `[0.0, 1.0]` for partial credit (passes if `score >= passThreshold`). Errors from either function are caught and recorded as failures without crashing the entire eval.

### Built-in scorers

| Scorer | Description |
|---|---|
| `exactMatch(actual, tc)` | `actual == tc["expected"]`. Strict equality |
| `contains(actual, tc)` | `tc["expected"]` appears anywhere in `actual`. Case-insensitive |
| `containsAll(actual, tc)` | Every string in the `tc["expected"]` array appears in `actual`. Case-insensitive |
| `containsAny(actual, tc)` | At least one string in the `tc["expected"]` array appears in `actual`. Case-insensitive |

### Reporting

| Function | Description |
|---|---|
| `report(results)` | Print pass/fail summary to stdout. Shows first 5 failing cases with their actual outputs |
| `compareReport(compareResults)` | Print a side-by-side scoreboard for a `compare()` result |

---

## The RAG Pipeline End-to-End

Combining `chunk.lex`, an embedding provider, `vector_store.lex`, and a chat provider gives a complete retrieval-augmented generation pipeline. `quick.askDocument()` implements this automatically; this section shows the explicit form for custom control:

```
import "stdlib/ai/chunk.lex"        as chunk
import "stdlib/ai/ollama.lex"       as ollama
import "stdlib/ai/vector_store.lex" as vs
import "stdlib/ai/ai_common.lex"    as ai

embedC = ollama.newClient(null, "mxbai-embed-large")
chatC  = ollama.newClient(null, "llama3.2")

// 1. Load and chunk the document
text   = readFile("docs/manual.md")?
chunks = chunk.byParagraphs(text, 800)

// 2. Embed each chunk and store
store = vs.newStore()
for ch in chunks {
    resp, _ = ollama.embed(embedC, ch["text"])
    store.add("c" + str(ch["index"]), resp["embeddings"][0], ch)
}

// 3. Query time: embed the question, retrieve top-4 chunks
question = "How do I configure the timeout?"
qResp, _ = ollama.embed(embedC, question)
hits     = store.search(qResp["embeddings"][0], 4)

// 4. Build the grounded prompt
parts = makeArray(len(hits), "")
for i, h in hits {
    parts[i] = "[chunk " + str(h["metadata"]["index"]) + "]\n" + h["metadata"]["text"]
}
context = join(parts, "\n\n")

// 5. Ask the chat provider
resp, err = ollama.chat(chatC, {
    "system":   "Answer using only the provided excerpts. Cite chunk indices.",
    "messages": [ai.userMsg("Excerpts:\n\n" + context + "\n\nQuestion: " + question)],
})
println(ollama.textOf(resp))
```

Swap `ollama.embed` for `voyage.embed` and `ollama.chat` for `claude.messages` for a production cloud-backed pipeline.

---

## Structured Output

Pass a `schema.lex` hash to `complete()` on any provider for type-safe structured responses:

```
import "stdlib/ai/schema.lex"    as schema
import "stdlib/ai/anthropic.lex" as claude

EventSchema = schema.object({
    "name":     schema.string("Event name"),
    "date":     schema.string("ISO 8601 date"),
    "location": schema.string("City or venue"),
    "public":   schema.boolean("Whether the event is open to the public"),
})

c = claude.newClient(env("ANTHROPIC_API_KEY"), null)
event, err = claude.complete(c, EventSchema, "Next Olympics opening ceremony details")
println(event["name"])
println(event["date"])
```

The two providers implement structured output differently under the hood:

- **Anthropic** — uses a synthetic single-purpose tool with `tool_choice` forced to it. The tool's `input` field becomes the structured response. Guarantees conformance for all Claude 3+ models.
- **Ollama** — uses the native `format` parameter (JSON Schema document) combined with a system-prompt restatement for less-capable models. Output is stripped of markdown fences before JSON parsing.

Both return `(parsedHash, err)` with the same call signature. `completeWith(c, schema, prompt, opts)` provides additional control (system prompt, temperature, model override).

---

## Authoring a New Provider Library

Follow `anthropic.lex` as the reference. The pattern:

### 1. Imports

```
import "stdlib/rest.lex"         as rest
import "stdlib/json.lex"         as json
import "stdlib/ai/ai_common.lex" as ai
```

Do not import other provider libraries. They are siblings, not dependencies.

### 2. Constants

```
const API_BASE_URL  = "https://..."
const DEFAULT_MODEL = "..."
```

Configuration that is fixed for the provider. User-tunable values go into the Client struct or per-call opts.

### 3. Client struct

```
struct Client { apiKey, model, baseUrl }

fn newClient(apiKey, model) { ... }
```

### 4. Core API functions

One function per endpoint, matching the provider's own terminology. Each returns `(result, err)`:

- `messages()` for Anthropic
- `chat()` for Ollama
- `generate()` for completion-style APIs

### 5. Accessor helpers

Wire-format access is awkward. Provide:

- `textOf(resp)` — extract the assistant's text
- `toolCallsOf(resp)` — extract tool calls
- `usageOf(resp)` — return `{input_tokens, output_tokens, cost_usd}` hash
- `stopReasonOf(resp)` — termination reason

These are pure functions over the response hash. No new HTTP calls.

### 6. Typed errors

```
PROVIDER_AUTH, PROVIDER_RATE_LIMIT, PROVIDER_SERVER, PROVIDER_NETWORK, ...
```

Callers branch on `err.code` without parsing strings.

### 7. Cost ledger integration

Call `ai._record(model, inputTok, outputTok, inputRate, outputRate)` after every successful API call so `ai.spent()` / `ai.usage()` / `ai.budgetExceeded()` reflect this provider's calls. Rates are USD per 1M tokens. Local/free providers pass `0.0` for both rates.

### 8. Agent loop (when applicable)

Providers with tool/function calling should provide `runAgent(client, opts, maxSteps)`:

- Accept tool hashes with a `handler` field (a kLex function)
- Strip `handler` before the wire call (providers reject unknown fields)
- Loop: call API → dispatch tool calls → feed results back → next turn
- Terminate when stop reason is not tool-use, or `maxSteps` is reached
- Return `(finalResponse, conversation, err)`

### 9. Demo scripts

Demo scripts should check for the relevant env var and exit cleanly if it is absent:

```
apiKey = env("PROVIDER_API_KEY")
if apiKey == null || len(apiKey) == 0 {
    println("demo: PROVIDER_API_KEY not set — skipping.")
    _osExit(0)
}
```

---

## Anti-Patterns

- **A `unified()` wrapper that abstracts providers.** This is the lowest-common-denominator trap.
- **Provider-specific code in `ai_common.lex`.** That file is shared *vocabulary*, not shared *implementation*.
- **Importing one provider library from another.** They are siblings. Genuinely shared logic belongs in `ai_common.lex` (data level only) or a private helper.
- **Returning raw HTTP errors as strings.** Use `error(code, message)` with structured codes.
- **Stateful module-level globals.** Libraries must be stateless beyond the Client struct. Multiple clients (different keys, different models) must coexist in one process.

---

## Current Status

| Library | Status | Summary |
|---|---|---|
| `ai_common.lex` | Shipped | Shared types, message/block constructors, process-wide cost ledger |
| `anthropic.lex` | Shipped — reference | `messages`, `stream` (SSE), `complete`, `countTokens`, `listModels`, `runAgent`, Message Batches API |
| `ollama.lex` | Shipped | `chat`, `generate`, `embed`, `stream` (NDJSON), `complete`, `runAgent`, `show`, `hasCapability`, `listModels`, `ps` |
| `voyage.lex` | Shipped | `embed`, `embedWith`, `usageOf`, `embeddingsOf`. Preferred embedding partner for Claude-primary stacks |
| `sd.lex` | Shipped | AUTOMATIC1111 / Forge / Reforge image generation — `generate`, `generateBatch`, `img2img`, model/sampler introspection |
| `quick.lex` | Shipped | Easy-path facade — `ask`, `executeQuery`, `extract`, `classify`, `askCached`, `askDocument`, provider switching, document cache |
| `schema.lex` | Shipped | JSON Schema builder — `string`, `integer`, `number`, `boolean`, `array`, `object`, `enumOf`, `nullable` |
| `vector_store.lex` | Shipped | In-memory K-NN store — `newStore`, `add`, `delete`, `get`, `search`, `searchScored`, `count`, `clear`, `ids` |
| `chunk.lex` | Shipped | Text chunking — `byChars`, `byWords`, `byParagraphs` |
| `evals.lex` | Shipped | Parallel eval harness — `run`, `runSerial`, `compare`, built-in scorers, `report`, `compareReport` |

---

## See Also

- [`stdlib/ai/anthropic.lex`](../stdlib/ai/anthropic.lex) — reference implementation
- [`stdlib/ai/ai_common.lex`](../stdlib/ai/ai_common.lex) — shared vocabulary and cost ledger
- [`stdlib/pdf.lex`](../stdlib/pdf.lex) — PDF text extraction, used by `askDocument`
- [`stdlib/rest.lex`](../stdlib/rest.lex) — JSON REST foundation all pure-HTTP libraries build on
- [`snowball/tadPole/tadPole.lex`](examples/tadPole/tadPole.lex) — multi-provider AI image generator using `anthropic.lex`, `ollama.lex`, and `sd.lex`
