package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"
)

const (
	reset  = "\033[0m"
	bold   = "\033[1m"
	red    = "\033[31m"
	green  = "\033[32m"
	yellow = "\033[33m"
	cyan   = "\033[36m"
	gray   = "\033[90m"
)

func colorStatus(status string) string {
	switch status {
	case "success":
		return green + status + reset
	case "failed":
		return red + status + reset
	case "running":
		return cyan + status + reset
	case "pending":
		return yellow + status + reset
	case "cancelled", "superseded", "skipped":
		return gray + status + reset
	default:
		return status
	}
}

func stepIcon(status string) string {
	switch status {
	case "success":
		return green + "✓" + reset
	case "failed":
		return red + "✗" + reset
	case "running":
		return cyan + "●" + reset
	case "skipped", "cancelled":
		return gray + "–" + reset
	default:
		return gray + "○" + reset
	}
}

func printTable(headers []string, rows [][]string) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, bold+strings.Join(headers, "\t")+reset)
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	w.Flush()
}

func printJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(v) //nolint:errcheck
}

func fmtDuration(start, end *time.Time) string {
	if start == nil || end == nil {
		return "—"
	}
	d := end.Sub(*start).Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func clearScreen() {
	fmt.Print("\033[2J\033[H")
}
