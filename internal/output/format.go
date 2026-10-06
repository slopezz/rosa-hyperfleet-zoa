package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"
)

type Format string

const (
	FormatTable    Format = "table"
	FormatWide     Format = "wide"
	FormatJSON     Format = "json"
	FormatMarkdown Format = "markdown"
)

func ParseFormat(s string) Format {
	switch strings.ToLower(s) {
	case "json":
		return FormatJSON
	case "wide":
		return FormatWide
	case "markdown", "md":
		return FormatMarkdown
	default:
		return FormatTable
	}
}

func JSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func NewTable(w io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
}

func FormatDuration(ms *int64) string {
	if ms == nil {
		return "-"
	}
	m := *ms
	switch {
	case m < 1000:
		return fmt.Sprintf("%dms", m)
	case m < 60_000:
		return fmt.Sprintf("%.1fs", float64(m)/1000)
	case m < 3_600_000:
		return fmt.Sprintf("%dm%ds", m/60_000, (m%60_000)/1000)
	default:
		return fmt.Sprintf("%dh%dm", m/3_600_000, (m%3_600_000)/60_000)
	}
}

func FormatTimestamp(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.Format("2006-01-02 15:04:05")
}

func FormatBool(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func FormatBytes(b *int64) string {
	if b == nil {
		return "-"
	}
	return FormatBytesInt64(*b)
}

// FormatBytesInt64 renders a byte count as a compact human-readable size (B/K/M).
func FormatBytesInt64(n int64) string {
	switch {
	case n == 0:
		return "0B"
	case n < 1024:
		return fmt.Sprintf("%dB", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1fK", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1fM", float64(n)/(1024*1024))
	}
}

func Dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// ShortOperator extracts the session name from an AWS ARN or returns the
// input unchanged if it's not an assumed-role ARN.
// "arn:aws:sts::123:assumed-role/RoleName/slopezma" → "slopezma"
func ShortOperator(arn string) string {
	if arn == "" {
		return "-"
	}
	if i := strings.LastIndex(arn, "/"); i >= 0 && strings.Contains(arn, "assumed-role") {
		return arn[i+1:]
	}
	return arn
}

func Truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

func IsTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func PrintTAOutput(w io.Writer, raw string) {
	printTAOutputWithTerminal(w, raw, IsTerminal())
}

func printTAOutputWithTerminal(w io.Writer, raw string, terminal bool) {
	if raw == "" {
		return
	}

	if !terminal {
		fmt.Fprint(w, raw)
		if !strings.HasSuffix(raw, "\n") {
			fmt.Fprintln(w)
		}
		return
	}

	lines := strings.Split(strings.TrimRight(raw, "\n"), "\n")
	if len(lines) == 0 {
		return
	}

	// Try JSONL first: each line is a flat JSON object → render as table
	if rows, keys := parseJSONL(lines); rows != nil {
		tw := NewTable(w)
		header := make([]string, len(keys))
		for i, k := range keys {
			header[i] = strings.ToUpper(k)
		}
		fmt.Fprintln(tw, strings.Join(header, "\t"))
		for _, row := range rows {
			vals := make([]string, len(keys))
			for i, k := range keys {
				vals[i] = fmt.Sprintf("%v", row[k])
			}
			fmt.Fprintln(tw, strings.Join(vals, "\t"))
		}
		tw.Flush()
		return
	}

	// Try JSON array of flat objects → render as table (Table API output)
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
		if rows, keys := parseJSONArray(trimmed); rows != nil {
			renderTable(w, rows, keys)
			return
		}
	}

	// Single flat JSON object → render as key-value pairs (single resource get)
	if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
		keys := jsonKeysOrdered(trimmed)
		if len(keys) > 0 {
			var obj map[string]any
			if err := json.Unmarshal([]byte(trimmed), &obj); err == nil && isFlatObject(obj) {
				for _, k := range keys {
					fmt.Fprintf(w, "%-14s %v\n", strings.ToUpper(k)+":", obj[k])
				}
				return
			}
		}
		// Nested object (verbose) → pretty-print
		var parsed any
		if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
			enc := json.NewEncoder(w)
			enc.SetIndent("", "  ")
			_ = enc.Encode(parsed)
			return
		}
	}

	// Nested array (verbose list) → pretty-print
	if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
		var parsed any
		if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
			enc := json.NewEncoder(w)
			enc.SetIndent("", "  ")
			_ = enc.Encode(parsed)
			return
		}
	}

	// Plain text — pass through raw
	fmt.Fprint(w, raw)
	if !strings.HasSuffix(raw, "\n") {
		fmt.Fprintln(w)
	}
}

// parseJSONArray parses a JSON array string. If all elements are flat objects,
// returns them with keys in the order they appear in the first element's JSON.
func parseJSONArray(raw string) ([]map[string]any, []string) {
	// Extract key order from first object in the array
	idx := strings.Index(raw, "{")
	if idx < 0 {
		return nil, nil
	}
	closeBrace := strings.Index(raw[idx:], "}")
	if closeBrace < 0 {
		return nil, nil
	}
	firstObj := raw[idx : idx+closeBrace+1]
	keys := jsonKeysOrdered(firstObj)
	if len(keys) == 0 {
		return nil, nil
	}

	var arr []map[string]any
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		return nil, nil
	}
	if len(arr) == 0 {
		return nil, nil
	}
	for _, obj := range arr {
		if !isFlatObject(obj) {
			return nil, nil
		}
	}
	return arr, keys
}

func renderTable(w io.Writer, rows []map[string]any, keys []string) {
	tw := NewTable(w)
	header := make([]string, len(keys))
	for i, k := range keys {
		header[i] = strings.ToUpper(k)
	}
	fmt.Fprintln(tw, strings.Join(header, "\t"))
	for _, row := range rows {
		vals := make([]string, len(keys))
		for i, k := range keys {
			vals[i] = fmt.Sprintf("%v", row[k])
		}
		fmt.Fprintln(tw, strings.Join(vals, "\t"))
	}
	tw.Flush()
}

// parseJSONL checks if all lines are flat JSON objects (scalar values only).
// Returns parsed rows and ordered keys, or nil if not valid flat JSONL.
func parseJSONL(lines []string) ([]map[string]any, []string) {
	keys := jsonKeysOrdered(lines[0])
	if len(keys) == 0 {
		return nil, nil
	}

	rows := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			return nil, nil
		}
		if !isFlatObject(obj) {
			return nil, nil
		}
		rows = append(rows, obj)
	}
	return rows, keys
}

func isFlatObject(obj map[string]any) bool {
	for _, v := range obj {
		switch v.(type) {
		case map[string]any, []any:
			return false
		}
	}
	return true
}

func jsonKeysOrdered(line string) []string {
	dec := json.NewDecoder(strings.NewReader(line))
	t, err := dec.Token()
	if err != nil {
		return nil
	}
	if delim, ok := t.(json.Delim); !ok || delim != '{' {
		return nil
	}

	var keys []string
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			break
		}
		key, ok := t.(string)
		if !ok {
			break
		}
		keys = append(keys, key)
		// Skip the value
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			break
		}
	}
	return keys
}
