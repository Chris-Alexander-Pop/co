package aur

import "testing"

func TestParseQua(t *testing.T) {
	got := ParseQua("foo 1-1 -> 2-1\n\nbar 3-1 -> 4-1\n")
	if len(got) != 2 || got[0] != "foo" || got[1] != "bar" {
		t.Fatalf("%v", got)
	}
}
