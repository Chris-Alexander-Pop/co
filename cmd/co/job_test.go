package main

import (
	"testing"

	"github.com/Chris-Alexander-Pop/co/internal/queue"
)

func TestNewestJob(t *testing.T) {
	jobs := []queue.Job{
		{ID: "1", Name: "zen-kernel"},
		{ID: "2", Name: "zen-kernel"},
		{ID: "3", Name: "other"},
	}
	if got := newestJob(append([]queue.Job(nil), jobs...), ""); got == nil || got.ID != "3" {
		t.Fatalf("newest: %v", got)
	}
	if got := newestJob(append([]queue.Job(nil), jobs...), "zen-kernel"); got == nil || got.ID != "2" {
		t.Fatalf("named: %v", got)
	}
	if got := newestJob(append([]queue.Job(nil), jobs...), "missing"); got != nil {
		t.Fatalf("missing: %v", got)
	}
}
