package aur

import (
	"reflect"
	"testing"
)

func TestDepName(t *testing.T) {
	if DepName("hyprlang>=0.6") != "hyprlang" {
		t.Fatal(DepName("hyprlang>=0.6"))
	}
}

func TestOrder(t *testing.T) {
	names := []string{"hyprland", "aquamarine", "hyprlock"}
	deps := map[string][]string{
		"hyprland": {"aquamarine>=0.4", "pixman"},
		"hyprlock": {"hyprland"},
	}
	got := Order(names, deps)
	want := []string{"aquamarine", "hyprland", "hyprlock"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}
