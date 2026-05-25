// tapetool — kLex .lextape utility.
//
// A .lextape is a JSON-Lines record of every agentic-hook event a kLex
// program produced during a run. The runtime writes it when invoked
// with --record-tape=PATH; tapetool wraps that flow and gives you ways
// to inspect, replay, diff, and mutate tapes after the fact.
//
// Format spec: docs/AGENTIC_HOOKS_ROADMAP.md
//
// Subcommands (v1 ships record + show; play/diff/mutate are roadmap):
//
//	tapetool record [--output FILE] [--klex BIN] [--vm] <program.lex> [args...]
//	    Invokes the kLex binary with --record-tape=FILE wrapping it.
//	    Defaults: --output → ./<program>-<timestamp>.lextape; --klex → ./klex.
//
//	tapetool show <FILE> [--filter KIND] [--from N] [--to M]
//	    Pretty-prints the tape. --filter limits to one event kind;
//	    --from/--to restrict the event ID range.
//
// All commands exit non-zero on read/write errors. JSON output mode
// (--json) emits a parseable summary instead of the human report.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}
	sub := os.Args[1]
	args := os.Args[2:]

	switch sub {
	case "record":
		runRecord(args)
	case "show":
		runShow(args)
	case "-h", "--help", "help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "tapetool: unknown subcommand %q\n\n", sub)
		printUsage()
		os.Exit(2)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `tapetool — kLex .lextape utility.

USAGE
  tapetool record [--output FILE] [--klex BIN] [--vm] <program.lex> [args...]
  tapetool show   <FILE> [--filter KIND] [--from N] [--to M] [--json]
  tapetool help

For the format spec and roadmap (play/diff/mutate), see
docs/AGENTIC_HOOKS_ROADMAP.md.`)
}

// ── record ───────────────────────────────────────────────────────────────

func runRecord(args []string) {
	fs := flag.NewFlagSet("record", flag.ExitOnError)
	output := fs.String("output", "", "tape output path (default: <program>-<timestamp>.lextape next to the program)")
	klexBin := fs.String("klex", "", "path to the kLex binary (default: ./klex)")
	useVM := fs.Bool("vm", false, "pass --vm to the kLex binary (bytecode VM)")
	_ = fs.Parse(args)

	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprintln(os.Stderr, "tapetool record: missing <program.lex>")
		os.Exit(2)
	}
	program := rest[0]
	programArgs := rest[1:]

	tapePath := *output
	if tapePath == "" {
		base := strings.TrimSuffix(filepath.Base(program), filepath.Ext(program))
		ts := time.Now().Format("20060102-150405")
		tapePath = filepath.Join(filepath.Dir(program), fmt.Sprintf("%s-%s.lextape", base, ts))
	}

	bin := *klexBin
	if bin == "" {
		bin = "./klex"
	}

	cmdArgs := []string{"--record-tape=" + tapePath}
	if *useVM {
		cmdArgs = append(cmdArgs, "--vm")
	}
	cmdArgs = append(cmdArgs, program)
	cmdArgs = append(cmdArgs, programArgs...)

	fmt.Fprintf(os.Stderr, "tapetool: recording → %s\n", tapePath)
	fmt.Fprintf(os.Stderr, "tapetool: %s %s\n", bin, strings.Join(cmdArgs, " "))

	cmd := exec.Command(bin, cmdArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "tapetool: program exited with error: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "tapetool: wrote %s\n", tapePath)
}

// ── show ─────────────────────────────────────────────────────────────────

// tapeRecord is a generic envelope matching every line shape (header /
// event / footer). Fields are pulled out as needed in the renderer.
type tapeRecord struct {
	Type     string                 `json:"type"`
	ID       *uint64                `json:"id,omitempty"`
	CausedBy *uint64                `json:"caused_by,omitempty"`
	TMs      *int64                 `json:"t_ms,omitempty"`
	Kind     string                 `json:"kind,omitempty"`
	Data     map[string]interface{} `json:"data,omitempty"`
	// Header fields
	TapeVersion        int                    `json:"tape_version,omitempty"`
	KlexVersion        string                 `json:"klex_version,omitempty"`
	Program            string                 `json:"program,omitempty"`
	ProgramSha         string                 `json:"program_sha256,omitempty"`
	StartedAt          string                 `json:"started_at,omitempty"`
	Args               []string               `json:"args,omitempty"`
	GestureAggregation bool                   `json:"gesture_aggregation,omitempty"`
	// Footer fields
	EndedAt      string         `json:"ended_at,omitempty"`
	DurationMs   int64          `json:"duration_ms,omitempty"`
	EventCount   int            `json:"event_count,omitempty"`
	CountsByKind map[string]int `json:"counts_by_kind,omitempty"`
}

func runShow(args []string) {
	fs := flag.NewFlagSet("show", flag.ExitOnError)
	filterKind := fs.String("filter", "", "show only events with this kind (e.g. ui_event, async_spawn)")
	fromID := fs.Uint64("from", 0, "start from event id N (inclusive)")
	toID := fs.Uint64("to", 0, "stop at event id M (inclusive); 0 = no limit")
	asJSON := fs.Bool("json", false, "emit summary as JSON instead of human report")
	_ = fs.Parse(args)

	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprintln(os.Stderr, "tapetool show: missing <FILE>")
		os.Exit(2)
	}
	path := rest[0]

	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tapetool show: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	var (
		header  *tapeRecord
		footer  *tapeRecord
		events  []tapeRecord
		scanner = bufio.NewScanner(f)
	)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024*16)

	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var rec tapeRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			fmt.Fprintf(os.Stderr, "tapetool show: line %d: %v\n", lineNo, err)
			os.Exit(1)
		}
		switch rec.Type {
		case "header":
			h := rec
			header = &h
		case "footer":
			ft := rec
			footer = &ft
		case "event":
			if *filterKind != "" && rec.Kind != *filterKind {
				continue
			}
			if rec.ID != nil {
				if *fromID > 0 && *rec.ID < *fromID {
					continue
				}
				if *toID > 0 && *rec.ID > *toID {
					continue
				}
			}
			events = append(events, rec)
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "tapetool show: scan: %v\n", err)
		os.Exit(1)
	}

	if *asJSON {
		emitJSONSummary(header, footer, events)
		return
	}
	renderShow(header, footer, events)
}

func renderShow(header, footer *tapeRecord, events []tapeRecord) {
	fmt.Println("╔══════════════════════════════════════════════════════╗")
	fmt.Println("║  .lextape — tape inspector                            ║")
	fmt.Println("╚══════════════════════════════════════════════════════╝")
	fmt.Println()

	if header != nil {
		fmt.Println("HEADER")
		fmt.Printf("  program:        %s\n", header.Program)
		fmt.Printf("  program_sha:    %s\n", shortSha(header.ProgramSha))
		fmt.Printf("  klex_version:   %s\n", header.KlexVersion)
		fmt.Printf("  tape_version:   %d\n", header.TapeVersion)
		fmt.Printf("  started_at:     %s\n", header.StartedAt)
		fmt.Printf("  gesture_aggregation: %v\n", header.GestureAggregation)
		if len(header.Args) > 0 {
			fmt.Printf("  args:           %v\n", header.Args)
		}
		fmt.Println()
	}

	if len(events) > 0 {
		fmt.Printf("EVENTS (%d shown)\n", len(events))
		fmt.Println()
		for _, e := range events {
			cause := "    "
			if e.CausedBy != nil && *e.CausedBy != 0 {
				cause = fmt.Sprintf("←#%-2d", *e.CausedBy)
			}
			fmt.Printf("  #%-5d %s  %-12s  %s\n", deref(e.ID), cause, e.Kind, formatEventData(e.Kind, e.Data))
		}
		fmt.Println()
	} else {
		fmt.Println("EVENTS: (none, after filters)")
		fmt.Println()
	}

	if footer != nil {
		fmt.Println("FOOTER")
		fmt.Printf("  ended_at:       %s\n", footer.EndedAt)
		fmt.Printf("  duration_ms:    %d\n", footer.DurationMs)
		fmt.Printf("  event_count:    %d\n", footer.EventCount)
		if len(footer.CountsByKind) > 0 {
			fmt.Println("  by kind:")
			ks := keysSorted(footer.CountsByKind)
			for _, k := range ks {
				fmt.Printf("    %-15s %d\n", k, footer.CountsByKind[k])
			}
		}
		fmt.Println()
	} else {
		fmt.Println("FOOTER: (none — recording may have ended abnormally)")
		fmt.Println()
	}
}

func emitJSONSummary(header, footer *tapeRecord, events []tapeRecord) {
	out := map[string]interface{}{
		"header":      header,
		"footer":      footer,
		"event_count": len(events),
		"events":      events,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(out)
}

// formatEventData renders the kind-specific payload compactly.
func formatEventData(kind string, data map[string]interface{}) string {
	if data == nil {
		return ""
	}
	switch kind {
	case "ui_event":
		return fmt.Sprintf("%s %s:%s  value=%v",
			data["kind"], data["widget"], data["label"], data["value"])
	case "async_spawn":
		return fmt.Sprintf("#%v %v (argc=%v)",
			data["task_id"], data["fn"], data["argc"])
	case "async_done":
		ok := data["ok"]
		dur := data["duration_ms"]
		base := fmt.Sprintf("#%v %vms ok=%v", data["task_id"], dur, ok)
		if e, hasE := data["error"].(map[string]interface{}); hasE && e != nil {
			base += fmt.Sprintf("  ← %v: %v", e["kind"], e["message"])
		}
		return base
	case "bridge_call":
		ok := data["ok"]
		return fmt.Sprintf("%v(%v args) %vms ok=%v",
			data["fn"], data["argc"], data["duration_ms"], ok)
	case "error":
		return fmt.Sprintf("[%v] %v", data["kind"], data["message"])
	}
	// Fallback: dump the JSON.
	b, _ := json.Marshal(data)
	return string(b)
}

// ── helpers ──────────────────────────────────────────────────────────────

func deref(p *uint64) uint64 {
	if p == nil {
		return 0
	}
	return *p
}

func shortSha(s string) string {
	if len(s) > 12 {
		return s[:12] + "…"
	}
	return s
}

func keysSorted(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// strconv import kept to silence unused warning if future subcommands
// (mutate, diff) need flag-int parsing.
var _ = strconv.Itoa
