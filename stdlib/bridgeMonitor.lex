// bridgeMonitor.lex — live bridge utilisation dashboard module.
//
// Passive observer — generates no traffic of its own. Import this module
// and call watch(bridge) with any bridge your application already holds.
// The dashboard redraws every 2 seconds until killed (Ctrl-C).
//
// Metrics shown each tick:
//   - calls/sec, inflight, interval error rate
//   - TX → outbound KB/s + activity bar
//   - RX ← inbound  KB/s + activity bar
//   - lifetime totals (calls, failures, bytes, error rate)
//   - per-function rolling p50 / p95 / p99 latency
//   - last 8 bridge call events (live, colour-coded)
//
// Usage:
//   import "stdlib/bridgeMonitor.lex" as monitor
//
//   let bridge, err = nativeBridge("node", ["myBridge.js"])
//   if err != null { return }
//
//   monitor.watch(bridge)                    // foreground — blocks
//   async(fn() { monitor.watch(bridge) })    // background — non-blocking
//
// Note: watch() registers agent.onBridgeCall internally. If your program
// already uses that hook, call watch() first or merge the handlers.

import "stdlib/agent.lex" as agent

// ── formatting helpers ────────────────────────────────────────────────────

fn pad2(n) {
    if n < 10 { return "0" + str(n) }
    return str(n)
}

fn padL(s, n) {
    let st = str(s)
    while len(st) < n { st = " " + st }
    return st
}

fn padR(s, n) {
    let st = str(s)
    while len(st) < n { st = st + " " }
    return st
}

fn fmt1(f) {
    if f < 0.0 { f = 0.0 }
    let whole = int(f)
    let frac  = int((f - float(whole)) * 10.0)
    if frac < 0 { frac = 0 }
    if frac > 9 { frac = 9 }
    return str(whole) + "." + str(frac)
}

fn fmtBps(bps) {
    if bps < 1.0       { return "0 B/s" }
    if bps < 1024.0    { return fmt1(bps) + " B/s" }
    if bps < 1048576.0 { return fmt1(bps / 1024.0) + " KB/s" }
    return fmt1(bps / 1048576.0) + " MB/s"
}

fn fmtBytes(b) {
    let bf = float(b)
    if bf < 1024.0    { return str(b) + " B" }
    if bf < 1048576.0 { return fmt1(bf / 1024.0) + " KB" }
    return fmt1(bf / 1048576.0) + " MB"
}

fn bar(pct, w) {
    if pct > 100 { pct = 100 }
    if pct < 0   { pct = 0 }
    let filled = int(float(pct) * float(w) / 100.0)
    let s = ""
    let i = 0
    while i < w {
        if i < filled { s = s + "█" } else { s = s + "░" }
        i = i + 1
    }
    return s
}

fn sep() {
    println(color_cyan() + "══════════════════════════════════════════════════════════════════" + color_reset())
}

fn thin() {
    println(color_dim() + "───────────────────────────────────────────────────────────────────" + color_reset())
}

// ── watch(bridge) ─────────────────────────────────────────────────────────
// Attach the dashboard to bridge and loop forever. Call from async() if you
// need the monitor to run alongside your application's own code.

fn watch(bridge) {

    // Event channel — the hook fires from any goroutine making bridge calls.
    // Channel is a reference type so sends from the hook land here safely.
    let evtCh = channel(1000)

    agent.onBridgeCall(fn(evt) {
        let failed = !evt["ok"] || evt["user_error"] != null
        let icon = "✓"
        if failed { icon = "✗" }
        let line = icon + " " + evt["fn"] + "(" + str(evt["argc"]) + " args) " + str(evt["duration_ms"]) + "ms"
        if !evt["ok"] {
            let e = evt["error"]
            line = line + "  ← " + e["kind"] + ": " + e["message"]
        }
        if evt["user_error"] != null {
            let e = evt["user_error"]
            line = line + "  ← " + e["code"] + ": " + e["message"]
        }
        send(evtCh, {"ok": !failed, "text": line})
    })

    let startNs   = _timeNanos()
    let prevNs    = startNs
    let prevCalls = 0
    let prevFail  = 0
    let prevSent  = 0
    let prevRecv  = 0
    let peakTx    = 0.001
    let peakRx    = 0.001

    let evtBuf   = makeArray(8, null)
    let evtHead  = 0
    let evtCount = 0

    while true {

        // snapshot metrics
        let m     = bridgeMetrics(bridge)
        let nowNs = _timeNanos()

        let elapsedSec = float(nowNs - prevNs) / 1000000000.0
        if elapsedSec < 0.001 { elapsedSec = 0.001 }

        let dCalls = m["calls_total"]    - prevCalls
        let dFail  = m["calls_failed"]   - prevFail
        let dSent  = m["bytes_sent"]     - prevSent
        let dRecv  = m["bytes_received"] - prevRecv

        let callsPerSec = float(dCalls) / elapsedSec
        let txBps       = float(dSent)  / elapsedSec
        let rxBps       = float(dRecv)  / elapsedSec

        // Decay peak 10% per tick — tracks recent activity, not all-time high.
        // Bar empties immediately when the bridge goes idle.
        peakTx = peakTx * 0.90
        peakRx = peakRx * 0.90
        if peakTx < 0.001 { peakTx = 0.001 }
        if peakRx < 0.001 { peakRx = 0.001 }
        if txBps > peakTx { peakTx = txBps }
        if rxBps > peakRx { peakRx = rxBps }

        let txBarPct = int(txBps / peakTx * 100.0)
        let rxBarPct = int(rxBps / peakRx * 100.0)

        // interval error rate
        let errPct = 0.0
        if dCalls > 0 {
            errPct = float(dFail) / float(dCalls) * 100.0
        }

        // lifetime error rate
        let lifeErrPct = 0.0
        if m["calls_total"] > 0 {
            lifeErrPct = float(m["calls_failed"]) / float(m["calls_total"]) * 100.0
        }

        // drain event channel into ring buffer
        let newEvt = recvNonBlock(evtCh)
        while newEvt != null {
            evtBuf[evtHead] = newEvt
            evtHead = (evtHead + 1) % 8
            if evtCount < 8 { evtCount = evtCount + 1 }
            newEvt = recvNonBlock(evtCh)
        }

        // uptime
        let uptimeSec = int(float(_timeNanos() - startNs) / 1000000000.0)
        let uptimeH   = uptimeSec / 3600
        let uptimeM   = (uptimeSec - uptimeH * 3600) / 60
        let uptimeS   = uptimeSec - uptimeH * 3600 - uptimeM * 60

        // current time
        let yr, mo, dy, hr, mn, sc, unix, wd = _timeNow()
        let ts = pad2(hr) + ":" + pad2(mn) + ":" + pad2(sc)
        let ut = pad2(uptimeH) + ":" + pad2(uptimeM) + ":" + pad2(uptimeS)

        // ── render ────────────────────────────────────────────────────────
        print(chr(27) + "[2J" + chr(27) + "[H")

        sep()
        println(color_bold() + color_cyan() +
            "  BRIDGE MONITOR" +
            "                            " + ts + "   up " + ut +
            color_reset())
        sep()

        println(color_bold() + "  THROUGHPUT  (2s interval)" + color_reset())

        let errColor = color_green()
        if errPct > 5.0 { errColor = color_red() }

        println("  Calls/sec  " + padL(fmt1(callsPerSec), 6) +
                "    Inflight  "  + padL(str(m["calls_inflight"]), 3) +
                "    Error rate  " + errColor + padL(fmt1(errPct), 5) + "%" + color_reset())
        println("")

        println("  TX  →  " + padL(fmtBps(txBps), 10) + "  [" + bar(txBarPct, 20) + "]  outbound →")
        println("  RX  ←  " + padL(fmtBps(rxBps), 10) + "  [" + bar(rxBarPct, 20) + "]  inbound  ←")

        thin()
        println(color_bold() + "  LIFETIME TOTALS" + color_reset())

        let lifeErrColor = color_green()
        if lifeErrPct > 5.0 { lifeErrColor = color_red() }

        println("  Calls  " + padL(str(m["calls_total"]),  8) +
                "   Failed  "   + padL(str(m["calls_failed"]), 6) +
                "   Streams  "  + padL(str(m["streams_total"]), 4) +
                "   Error rate  " + lifeErrColor + padL(fmt1(lifeErrPct), 5) + "%" + color_reset())
        println("  Sent     " + padL(fmtBytes(m["bytes_sent"]),     9) +
                "                  Received  " + padL(fmtBytes(m["bytes_received"]), 9))

        thin()
        println(color_bold() + "  PER-FUNCTION  (rolling 256-sample window)" + color_reset())
        println(color_dim() +
            "  " + padR("function", 18) +
            padL("calls",  7) +
            padL("errors", 8) +
            padL("p50ms",  8) +
            padL("p95ms",  8) +
            padL("p99ms",  8) +
            color_reset())

        let pf = m["per_function"]
        if pf != null {
            for fnName in keys(pf) {
                let fd      = pf[fnName]
                let errStr  = padL(str(fd["errors"]), 8)
                let errCell = errStr
                if fd["errors"] > 0 {
                    errCell = color_red() + errStr + color_reset()
                }
                println("  " + padR(fnName, 18) +
                        padL(str(fd["count"]),  7) +
                        errCell                    +
                        padL(fmt1(fd["p50_ms"]), 8) +
                        padL(fmt1(fd["p95_ms"]), 8) +
                        padL(fmt1(fd["p99_ms"]), 8))
            }
        } else {
            println("  " + color_dim() + "(waiting for first call...)" + color_reset())
        }

        thin()
        println(color_bold() + "  RECENT EVENTS" + color_reset())

        if evtCount == 0 {
            println("  " + color_dim() + "(waiting for events...)" + color_reset())
        } else {
            let displayStart = 0
            if evtCount >= 8 { displayStart = evtHead }
            let i = 0
            while i < evtCount {
                let idx   = (displayStart + i) % 8
                let entry = evtBuf[idx]
                if entry != null {
                    if entry["ok"] {
                        println("  " + color_green() + entry["text"] + color_reset())
                    } else {
                        println("  " + color_red() + entry["text"] + color_reset())
                    }
                }
                i = i + 1
            }
        }

        sep()

        // update prev tracking
        prevNs    = nowNs
        prevCalls = m["calls_total"]
        prevFail  = m["calls_failed"]
        prevSent  = m["bytes_sent"]
        prevRecv  = m["bytes_received"]

        sleep(2000)
    }
}
