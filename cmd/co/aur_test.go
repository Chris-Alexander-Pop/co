package main

import "testing"

func TestSkipAndContinue(t *testing.T) {
	stop := []string{"", "n", "N", "no", "No", "NO", "yeah", "yep", " "}
	for _, answer := range stop {
		if skipAndContinue(answer) {
			t.Fatalf("answer %q should stop", answer)
		}
	}
	for _, answer := range []string{"y", "Y", "yes", "YES", " Yes "} {
		if !skipAndContinue(answer) {
			t.Fatalf("answer %q should skip", answer)
		}
	}
}
