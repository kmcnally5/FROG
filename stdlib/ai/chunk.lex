// stdlib/ai/chunk.lex — text chunking for RAG ingestion.
// @module    chunk
// @version   1.0.0
// @since     klex 0.3.35
// @author    karl
// @summary   text chunking for RAG ingestion.
//
// Each strategy splits a text into chunks and returns an array of hashes:
//
//     {"index": int, "start": int, "end": int, "text": string}
//
// `start` and `end` are character offsets into the original text — useful
// for source-attribution / citation in RAG answers ("this came from chars
// 1234–1567 of foo.pdf").
//
// Strategies:
//
//   byChars(text, size, overlap)
//       Fixed-character-window chunks. Simple, fast, language-agnostic.
//       Use when you don't care about respecting natural boundaries.
//
//   byWords(text, size, overlap)
//       Fixed-word-count windows (size = word count, NOT char count).
//       Tokenises on whitespace so chunks land on word boundaries.
//
//   byParagraphs(text, maxChars)
//       Splits on blank lines (\n\n), then greedily merges consecutive
//       paragraphs until adding the next would exceed maxChars. Pass
//       maxChars=0 to disable merging and emit one chunk per paragraph.
//       Best default for prose corpora.
//
// Sizing notes:
//   Embedding APIs cap input at ~8k tokens ≈ 32k chars (English). A
//   typical RAG setup uses 500–1000 char chunks with 100–200 char overlap.
//   Larger chunks → richer context per hit but fewer distinct hits per
//   document. Tune to your retrieval pattern.
//
// Usage:
//   import "stdlib/ai/chunk.lex"        as chunk
//   import "stdlib/ai/vector_store.lex" as vs
//   import "stdlib/ai/ollama.lex"       as ollama
//
//   chunks = chunk.byParagraphs(longText, 800)
//   for ch in chunks {
//       v, _ = ollama.embed(c, ch["text"])
//       store.add("doc:" + str(ch["index"]), v["embeddings"][0], ch)
//   }


// byChars cuts text into fixed-size character windows with optional overlap.
fn byChars(text, size, overlap) {
    if size < 1                { return makeArray(0) }
    if overlap >= size         { overlap = size - 1 }
    if overlap < 0             { overlap = 0 }
    let n = len(text)
    if n == 0                  { return makeArray(0) }

    let stride = size - overlap

    // Pass 1: count chunks.
    let cn  = 0
    let pos = 0
    while pos < n {
        cn = cn + 1
        if pos + size >= n { break }
        pos = pos + stride
    }

    // Pass 2: fill.
    let out = makeArray(cn, null)
    let ci  = 0
    pos = 0
    while pos < n {
        let e = pos + size
        if e > n { e = n }
        out[ci] = {"index": ci, "start": pos, "end": e, "text": substr(text, pos, e)}
        ci = ci + 1
        if e >= n { break }
        pos = pos + stride
    }
    return out
}


// byWords cuts text into fixed-word-count windows with overlap measured
// in words (not characters). size = words per chunk; overlap = words shared
// between consecutive chunks.
fn byWords(text, size, overlap) {
    if size < 1                { return makeArray(0) }
    if overlap >= size         { overlap = size - 1 }
    if overlap < 0             { overlap = 0 }

    let toks = _tokenizeWords(text)
    let wn   = len(toks)
    if wn == 0                 { return makeArray(0) }

    let stride = size - overlap
    if stride < 1 { stride = 1 }

    // Pass 1: count chunks.
    let cn = 0
    let i  = 0
    while i < wn {
        cn = cn + 1
        if i + size >= wn { break }
        i = i + stride
    }

    // Pass 2: fill.
    let out = makeArray(cn, null)
    let ci  = 0
    i   = 0
    while i < wn {
        let endIdx = i + size
        if endIdx > wn { endIdx = wn }
        let sChar = toks[i]["start"]
        let eChar = toks[endIdx - 1]["end"]
        out[ci] = {
            "index": ci,
            "start": sChar,
            "end":   eChar,
            "text":  substr(text, sChar, eChar),
        }
        ci = ci + 1
        if endIdx >= wn { break }
        i = i + stride
    }
    return out
}


// byParagraphs splits text on blank lines (\n\n) and greedily groups
// consecutive paragraphs into chunks of up to maxChars. Pass maxChars=0
// (or negative) to emit one chunk per paragraph with no merging.
//
// A paragraph longer than maxChars becomes a single oversized chunk —
// pipe such chunks through byChars() afterwards if you need a strict cap.
fn byParagraphs(text, maxChars) {
    if len(text) == 0 { return makeArray(0) }

    let parts = split(text, "\n\n")
    let pn    = len(parts)

    // Walk the original text to find each paragraph's char offsets.
    let starts = makeArray(pn, 0)
    let ends   = makeArray(pn, 0)
    let pos    = 0
    let j      = 0
    while j < pn {
        starts[j] = pos
        ends[j]   = pos + len(parts[j])
        pos = pos + len(parts[j]) + 2     // skip the \n\n separator
        j = j + 1
    }

    // No-merge fast path — one chunk per paragraph.
    if maxChars <= 0 {
        let out = makeArray(pn, null)
        j = 0
        while j < pn {
            out[j] = {
                "index": j,
                "start": starts[j],
                "end":   ends[j],
                "text":  parts[j],
            }
            j = j + 1
        }
        return out
    }

    // Greedy merge — accumulate paragraphs until adding the next would
    // exceed maxChars. Pass 1 counts the resulting chunks.
    let cn     = 0
    let curLen = 0
    j      = 0
    while j < pn {
        let plen = len(parts[j])
        if curLen == 0 {
            curLen = plen
        } else if curLen + 2 + plen <= maxChars {
            curLen = curLen + 2 + plen
        } else {
            cn = cn + 1
            curLen = plen
        }
        j = j + 1
    }
    if curLen > 0 { cn = cn + 1 }

    // Pass 2: fill.
    let out        = makeArray(cn, null)
    let ci         = 0
    let chunkStart = starts[0]
    let chunkEnd   = ends[0]
    let chunkText  = parts[0]
    curLen     = len(parts[0])
    j          = 1
    while j < pn {
        let plen = len(parts[j])
        if curLen + 2 + plen <= maxChars {
            chunkEnd  = ends[j]
            chunkText = chunkText + "\n\n" + parts[j]
            curLen    = curLen + 2 + plen
        } else {
            out[ci] = {
                "index": ci,
                "start": chunkStart,
                "end":   chunkEnd,
                "text":  chunkText,
            }
            ci = ci + 1
            chunkStart = starts[j]
            chunkEnd   = ends[j]
            chunkText  = parts[j]
            curLen     = plen
        }
        j = j + 1
    }
    out[ci] = {
        "index": ci,
        "start": chunkStart,
        "end":   chunkEnd,
        "text":  chunkText,
    }
    return out
}


// _tokenizeWords returns [{start, end, text}, …] for every whitespace-
// delimited run in text. Whitespace = space, tab, newline, carriage return.
// Two passes: count word-runs first, pre-allocate, then fill.
fn _tokenizeWords(text) {
    let n  = len(text)
    if n == 0 { return makeArray(0) }

    // Pass 1: count words.
    let wn       = 0
    let inWord   = false
    let i        = 0
    while i < n {
        let c = substr(text, i, i + 1)
        let space = (c == " ") || (c == "\t") || (c == "\n") || (c == "\r")
        if !space && !inWord {
            wn = wn + 1
            inWord = true
        } else if space {
            inWord = false
        }
        i = i + 1
    }
    if wn == 0 { return makeArray(0) }

    // Pass 2: fill.
    let out      = makeArray(wn, null)
    let idx      = 0
    i        = 0
    let start    = 0
    inWord   = false
    while i < n {
        let c = substr(text, i, i + 1)
        let space = (c == " ") || (c == "\t") || (c == "\n") || (c == "\r")
        if !space {
            if !inWord {
                start = i
                inWord = true
            }
        } else {
            if inWord {
                out[idx] = {"start": start, "end": i, "text": substr(text, start, i)}
                idx = idx + 1
                inWord = false
            }
        }
        i = i + 1
    }
    if inWord {
        out[idx] = {"start": start, "end": n, "text": substr(text, start, n)}
    }
    return out
}
