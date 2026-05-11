package hookaudit

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// MaxLines is the maximum number of lines kept in the audit log.
const MaxLines = 1000

// DefaultTail is the default number of events shown.
const DefaultTail = 20

// LogDir returns the directory for the audit log file.
func LogDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", "snip")
}

// LogPath returns the full path to the audit log file.
func LogPath() string {
	dir := LogDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "hook-audit.log")
}

// Event represents a single hook audit log entry.
//
// Agent identifies which integration produced the event ("claude-code",
// "cursor", "codex", ...). Omitted on disk for legacy log lines and read
// back as the empty string.
type Event struct {
	Timestamp time.Time `json:"timestamp"`
	Command   string    `json:"command"`
	Base      string    `json:"base"`
	Matched   bool      `json:"matched"`
	Rewritten bool      `json:"rewritten"`
	Agent     string    `json:"agent,omitempty"`
}

// Append writes an event to the audit log as a JSONL line.
// Best-effort: errors are silently ignored.
func Append(e Event) {
	path := LogPath()
	if path == "" {
		return
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}

	data, err := json.Marshal(e)
	if err != nil {
		return
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()

	// Read existing lines to check rotation.
	lines, _ := readLines(f)
	lines = append(lines, string(data))

	// Rotate: keep only the last MaxLines.
	if len(lines) > MaxLines {
		lines = lines[len(lines)-MaxLines:]
	}

	// Rewrite the file.
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return
	}
	if err := f.Truncate(0); err != nil {
		return
	}
	for _, line := range lines {
		_, _ = fmt.Fprintln(f, line)
	}
}

// ReadEvents reads all events from the audit log.
func ReadEvents() ([]Event, error) {
	path := LogPath()
	if path == "" {
		return nil, fmt.Errorf("determine log path: cannot find home directory")
	}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open audit log: %w", err)
	}
	defer func() { _ = f.Close() }()

	return ParseEvents(f)
}

// ParseEvents reads JSONL events from a reader.
func ParseEvents(r io.Reader) ([]Event, error) {
	var events []Event
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var e Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue // skip malformed lines
		}
		events = append(events, e)
	}
	if err := scanner.Err(); err != nil {
		return events, fmt.Errorf("read audit log: %w", err)
	}
	return events, nil
}

// Clear removes the audit log file.
func Clear() error {
	path := LogPath()
	if path == "" {
		return fmt.Errorf("determine log path: cannot find home directory")
	}
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("clear audit log: %w", err)
	}
	return nil
}

// FormatTable formats events as a human-readable table written to w.
func FormatTable(w io.Writer, events []Event, tail int) {
	if tail <= 0 {
		tail = DefaultTail
	}

	// Take only the last N events.
	if len(events) > tail {
		events = events[len(events)-tail:]
	}

	_, _ = fmt.Fprintln(w, "snip hook-audit - recent hook activity")
	_, _ = fmt.Fprintln(w)

	if len(events) == 0 {
		_, _ = fmt.Fprintln(w, "No events recorded.")
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintln(w, "Set SNIP_HOOK_AUDIT=1 to enable logging.")
		return
	}

	// Header
	_, _ = fmt.Fprintf(w, "%-20s %-24s %-8s %-9s %s\n", "Time", "Command", "Base", "Matched", "Rewritten")

	for _, e := range events {
		ts := e.Timestamp.Local().Format("2006-01-02 15:04")
		cmd := truncate(e.Command, 24)
		matched := boolYesNo(e.Matched)
		rewritten := boolYesNo(e.Rewritten)
		_, _ = fmt.Fprintf(w, "%-20s %-24s %-8s %-9s %s\n", ts, cmd, e.Base, matched, rewritten)
	}

	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(w, "Showing last %d events. Set SNIP_HOOK_AUDIT=1 to enable logging.\n", tail)
}

// Run is the entry point for the hook-audit subcommand.
func Run(args []string) error {
	tail := DefaultTail
	clear := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--tail":
			if i+1 >= len(args) {
				return fmt.Errorf("--tail requires a value")
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n <= 0 {
				return fmt.Errorf("--tail value must be a positive integer: %s", args[i])
			}
			tail = n
		case "--clear":
			clear = true
		default:
			// Check for --tail=N form.
			if strings.HasPrefix(args[i], "--tail=") {
				val := strings.TrimPrefix(args[i], "--tail=")
				n, err := strconv.Atoi(val)
				if err != nil || n <= 0 {
					return fmt.Errorf("--tail value must be a positive integer: %s", val)
				}
				tail = n
			} else {
				return fmt.Errorf("unknown flag: %s", args[i])
			}
		}
	}

	if clear {
		if err := Clear(); err != nil {
			return err
		}
		fmt.Println("Audit log cleared.")
		return nil
	}

	events, err := ReadEvents()
	if err != nil {
		return err
	}

	FormatTable(os.Stdout, events, tail)
	return nil
}

// Enabled returns true if SNIP_HOOK_AUDIT=1 is set.
func Enabled() bool {
	return os.Getenv("SNIP_HOOK_AUDIT") == "1"
}

// readLines reads all lines from a file (must be seeked to start).
func readLines(f *os.File) ([]string, error) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines, scanner.Err()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

func boolYesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
