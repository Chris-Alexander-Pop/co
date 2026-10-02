package progress

import (
	"testing"
	"time"
)

func TestPriorUsesLastSpeedUntilThisRunSettles(t *testing.T) {
	r := NewRate()
	r.UsePrior(Prior{Files: 1000, Duration: 100 * time.Second})
	now := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	eta, ok := r.Note(now, 20, 5, 2*time.Second)
	if !ok {
		t.Fatal("expected an eta from the last run")
	}
	// Last run was 10 files/s. 980 left. This run is too young to trust.
	if eta < 90*time.Second || eta > 100*time.Second {
		t.Fatalf("eta %s", eta)
	}
}

func TestPriorAdjustsToThisSessionsAverage(t *testing.T) {
	r := NewRate()
	r.UsePrior(Prior{Files: 1000, Duration: 100 * time.Second})
	now := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	// 400 files in 20s is 20/s, twice the last run. 600 left -> 30s.
	eta, ok := r.Note(now, 400, 40, 20*time.Second)
	if !ok {
		t.Fatal("expected an eta")
	}
	if eta < 28*time.Second || eta > 32*time.Second {
		t.Fatalf("eta %s", eta)
	}
	if r.PerSec() < 19 || r.PerSec() > 21 {
		t.Fatalf("speed %v", r.PerSec())
	}
}

func TestSlowerSessionLengthensTheETA(t *testing.T) {
	r := NewRate()
	r.UsePrior(Prior{Files: 1000, Duration: 100 * time.Second})
	now := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	// 200 files in 40s is 5/s, half the last run. 800 left -> 160s.
	eta, ok := r.Note(now, 200, 20, 40*time.Second)
	if !ok {
		t.Fatal("expected an eta")
	}
	if eta < 150*time.Second || eta > 170*time.Second {
		t.Fatalf("eta %s", eta)
	}
}

func TestNoPriorUsesSessionAverage(t *testing.T) {
	r := NewRate()
	now := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	if _, ok := r.Note(now, 100, 10, time.Second); ok {
		t.Fatal("a one second sample should not guess")
	}
	// 200 files in 10s is 20/s. 20% means about 1000 total, 800 left, 40s.
	eta, ok := r.Note(now.Add(10*time.Second), 200, 20, 10*time.Second)
	if !ok {
		t.Fatal("expected an eta")
	}
	if eta < 35*time.Second || eta > 45*time.Second {
		t.Fatalf("eta %s", eta)
	}
}

func TestFlatCompileDoesNotInventAnETA(t *testing.T) {
	r := NewRate()
	now := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	if _, ok := r.Note(now, 0, 0, 0); ok {
		t.Fatal("nothing compiled")
	}
	if _, ok := r.Note(now.Add(time.Second), 0, 0, time.Second); ok {
		t.Fatal("still nothing compiled")
	}
}
