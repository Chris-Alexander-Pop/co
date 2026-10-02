package fallback

import "testing"

func TestChoose(t *testing.T) {
	got, err := Choose("", "local")
	if err != nil || got != Local {
		t.Fatalf("config local: %q %v", got, err)
	}
	got, err = Choose("cancel", "local")
	if err != nil || got != Cancel {
		t.Fatalf("flag wins: %q %v", got, err)
	}
	if _, err := Choose("cloud", "local"); err == nil {
		t.Fatal("expected error")
	}
}
