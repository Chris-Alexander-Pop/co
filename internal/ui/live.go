package ui

import (
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	cAmber = "\033[38;5;215m"
	cFill  = "\033[38;5;172m"
	cEmpty = "\033[38;5;238m"
)

// Logo prints the co banner. The mark is a diamond, not a side bar.
func Logo(tagline string) {
	if !Enabled {
		fmt.Fprintln(Out, "co — "+tagline)
		return
	}
	fmt.Fprintln(Out)
	fmt.Fprintf(Out, "  %s %s%s%s\n", wrap(cAmber, "◆"), bold, wrap(brand, "co"), reset)
	fmt.Fprintf(Out, "  %s\n", Dim(tagline))
}

// Heading prints a section title over a dotted rule.
func Heading(title string) {
	fmt.Fprintln(Out)
	if !Enabled {
		fmt.Fprintf(Out, "  %s\n", title)
		return
	}
	fmt.Fprintf(Out, "  %s%s%s\n", bold, title, reset)
	fmt.Fprintf(Out, "  %s\n", Dim(strings.Repeat("· ", 14)))
}

// Bar is a 28-cell meter. pct is 0 to 100.
func Bar(pct int) string {
	const width = 28
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := pct * width / 100
	empty := width - filled
	if !Enabled {
		return fmt.Sprintf("[%s] %3d%%", strings.Repeat("#", filled)+strings.Repeat("-", empty), pct)
	}
	return wrap(cFill, strings.Repeat("█", filled)) + wrap(cEmpty, strings.Repeat("░", empty)) + "  " + Bold(fmt.Sprintf("%d%%", pct))
}

// Live is the build panel. It is always 6 lines so a redraw stays put.
func Live(name, status string, elapsed time.Duration, pct int, verb, file string, compiled int) string {
	var b strings.Builder
	mark := "*"
	if Enabled {
		mark = wrap(cAmber, "◆")
	}
	state := statusLabel(status)
	fmt.Fprintf(&b, "  %s %s   %s\n", mark, Bold(name), state)
	meta := FormatElapsed(elapsed)
	if compiled > 0 {
		meta += "   " + fmt.Sprintf("%d compiled", compiled)
	}
	fmt.Fprintf(&b, "  %s\n", Dim(meta))
	fmt.Fprintf(&b, "\n")
	fmt.Fprintf(&b, "  %s\n", Bar(pct))
	detail := "waiting"
	if file != "" {
		detail = strings.TrimSpace(verb + "  " + file)
	} else if status == "succeeded" {
		detail = "ready"
	} else if status == "failed" {
		detail = "failed"
	}
	fmt.Fprintf(&b, "  %s\n", Dim(detail))
	fmt.Fprintf(&b, "\n")
	return b.String()
}

func statusLabel(status string) string {
	switch status {
	case "succeeded":
		return Green("done")
	case "failed":
		return Red("failed")
	case "queued":
		return wrap(cAmber, "queued")
	case "running":
		return wrap(cAmber, "running")
	default:
		if status == "" {
			return Dim("pending")
		}
		return Dim(status)
	}
}

// FormatElapsed renders a duration as 42s, 18m 42s, or 1h 02m.
func FormatElapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	s := (d - m*time.Minute) / time.Second
	if h > 0 {
		return fmt.Sprintf("%dh %02dm", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %02ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// Machine is the builder the board is looking at.
type Machine struct {
	CPUs    int
	Busy    int
	Running int
	Queued  int
}

// Lane is one job on the board.
type Lane struct {
	Mode     string
	Name     string
	Status   string
	Verb     string
	File     string
	Detail   string
	Spark    []float64
	PerSec   float64
	Pct      int
	Threads  int
	Compiled int
	Position int
	Elapsed  time.Duration
	ETA      time.Duration
	ETAKnown bool
}

// Board is the queue screen: cores, running builds, and who is waiting.
func Board(m Machine, lanes []Lane) string {
	var b strings.Builder
	mark := "*"
	if Enabled {
		mark = wrap(cAmber, "◆")
	}
	meta := ""
	switch {
	case m.Running > 0 && m.Queued > 0:
		meta = fmt.Sprintf("%d running    %d waiting", m.Running, m.Queued)
	case m.Running > 0:
		meta = fmt.Sprintf("%d running", m.Running)
	case m.Queued > 0:
		meta = fmt.Sprintf("%d waiting", m.Queued)
	case len(lanes) > 0:
		meta = "idle"
	}
	fmt.Fprintf(&b, "  %s %s", mark, Bold("co"))
	if meta != "" {
		fmt.Fprintf(&b, "    %s", Dim(meta))
	}
	fmt.Fprintf(&b, "\n")
	if strip := coreStrip(m.CPUs, m.Busy); strip != "" {
		fmt.Fprintf(&b, "  %s\n", strip)
	}
	fmt.Fprintf(&b, "\n")

	var running, waiting, done []Lane
	for _, lane := range lanes {
		switch lane.Mode {
		case "wait":
			waiting = append(waiting, lane)
		case "done":
			done = append(done, lane)
		default:
			running = append(running, lane)
		}
	}
	if len(running) == 0 && len(waiting) == 0 && len(done) == 0 {
		fmt.Fprintf(&b, "  %s\n\n", Dim("queue empty"))
		return b.String()
	}
	for _, lane := range running {
		writeRunning(&b, lane)
	}
	if len(waiting) > 0 {
		fmt.Fprintf(&b, "  %s\n", Dim("waiting"))
		for _, lane := range waiting {
			writeWaiting(&b, lane)
		}
		fmt.Fprintf(&b, "\n")
	}
	if len(done) > 0 {
		fmt.Fprintf(&b, "  %s\n", Dim("finished"))
		for _, lane := range done {
			writeDone(&b, lane)
		}
		fmt.Fprintf(&b, "\n")
	}
	return b.String()
}

func writeRunning(b *strings.Builder, lane Lane) {
	fmt.Fprintf(b, "  %s", Bold(fit(lane.Name, 28)))
	if lane.Threads > 0 {
		fmt.Fprintf(b, "  %s", wrap(cAmber, fmt.Sprintf("-j%d", lane.Threads)))
	}
	fmt.Fprintf(b, "\n")
	fmt.Fprintf(b, "  %s\n", Bar(lane.Pct))
	meta := FormatElapsed(lane.Elapsed)
	if lane.ETAKnown {
		meta += "    eta " + FormatElapsed(lane.ETA)
	}
	if lane.PerSec >= 1 {
		meta += fmt.Sprintf("    %.0f/s", lane.PerSec)
	}
	if spark := sparkLine(lane.Spark); spark != "" {
		meta += "    " + spark
	}
	if lane.Compiled > 0 {
		meta += "    " + fmt.Sprintf("%d compiled", lane.Compiled)
	}
	fmt.Fprintf(b, "  %s\n", Dim(meta))
	detail := "starting"
	if lane.File != "" {
		detail = strings.TrimSpace(lane.Verb + "  " + shortPath(lane.File))
	} else if lane.Status == "succeeded" {
		detail = "ready"
	} else if lane.Status == "failed" {
		detail = "failed"
	}
	fmt.Fprintf(b, "  %s\n\n", Dim(detail))
}

func writeWaiting(b *strings.Builder, lane Lane) {
	pos := lane.Position
	if pos < 1 {
		pos = 1
	}
	name := fit(lane.Name, 28)
	threads := ""
	if lane.Threads > 0 {
		threads = fmt.Sprintf("-j%d", lane.Threads)
	}
	fmt.Fprintf(b, "  %s  %s  %s\n", Dim(fmt.Sprintf("%2d", pos)), name, Dim(threads))
	if lane.Detail != "" {
		fmt.Fprintf(b, "      %s\n", Dim(lane.Detail))
	}
}

func writeDone(b *strings.Builder, lane Lane) {
	state := statusLabel(lane.Status)
	fmt.Fprintf(b, "  %s  %s  %s\n", fit(lane.Name, 28), Dim(FormatElapsed(lane.Elapsed)), state)
}

func coreStrip(cpus, busy int) string {
	if cpus < 1 {
		return ""
	}
	if busy < 0 {
		busy = 0
	}
	if busy > cpus {
		busy = cpus
	}
	width := cpus
	if width > 32 {
		width = 32
	}
	filled := busy * width / cpus
	empty := width - filled
	label := fmt.Sprintf("%d/%d cores", busy, cpus)
	if !Enabled {
		return fmt.Sprintf("[%s] %s", strings.Repeat("#", filled)+strings.Repeat("-", empty), label)
	}
	return wrap(cFill, strings.Repeat("█", filled)) + wrap(cEmpty, strings.Repeat("░", empty)) + "  " + Dim(label)
}

func sparkLine(rates []float64) string {
	if len(rates) == 0 {
		return ""
	}
	max := 0.0
	for _, v := range rates {
		if v > max {
			max = v
		}
	}
	if max <= 0 {
		return ""
	}
	blocks := []rune("▁▂▃▄▅▆▇█")
	out := make([]rune, len(rates))
	for i, v := range rates {
		n := int(v / max * float64(len(blocks)-1))
		if n < 0 {
			n = 0
		}
		if n >= len(blocks) {
			n = len(blocks) - 1
		}
		out[i] = blocks[n]
	}
	if !Enabled {
		return string(out)
	}
	return wrap(cAmber, string(out))
}

func fit(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		if n < 2 {
			return string(r[:n])
		}
		return string(r[:n-1]) + "…"
	}
	return s + strings.Repeat(" ", n-len(r))
}

func shortPath(p string) string {
	p = strings.TrimSpace(p)
	if len(p) <= 52 {
		return p
	}
	i := strings.LastIndex(p, "/")
	if i >= 0 && i < len(p)-1 {
		p = p[i+1:]
	}
	if len(p) > 52 {
		return "…" + p[len(p)-51:]
	}
	return p
}

// Paint reprints block over the previous one. It returns how many lines block occupies.
func Paint(out io.Writer, prev int, block string) int {
	n := strings.Count(block, "\n")
	if Enabled && prev > 0 {
		fmt.Fprintf(out, "\033[%dA\033[J", prev)
	}
	fmt.Fprint(out, block)
	return n
}
