package ui

import (
	"strings"
	"testing"
	"time"
)

func TestBoardShowsCoresQueueAndETA(t *testing.T) {
	Enabled = true
	got := Board(Machine{CPUs: 16, Busy: 12, Running: 1, Queued: 2}, []Lane{
		{
			Mode: "run", Name: "zen-kernel", Status: "running", Threads: 12,
			Pct: 27, Elapsed: 8*time.Minute + 5*time.Second, ETA: 22 * time.Minute, ETAKnown: true,
			Compiled: 3429, Verb: "CC", File: "lib/842/842_compress.o",
			Spark: []float64{1, 3, 5, 4}, PerSec: 14,
		},
		{
			Mode: "wait", Name: "hyprland", Status: "queued", Threads: 8, Position: 1,
			Detail: "needs 8 cores, 4 free",
		},
		{
			Mode: "wait", Name: "neovim", Status: "queued", Threads: 4, Position: 2,
			Detail: "in line",
		},
		{
			Mode: "done", Name: "hyprlock", Status: "succeeded", Elapsed: 4*time.Minute + 12*time.Second,
		},
	})
	for _, want := range []string{"12/16 cores", "-j12", "eta 22m", "14/s", "842_compress.o", "hyprland", "needs 8 cores", "neovim", "hyprlock", "done"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q\n%s", want, got)
		}
	}
}
