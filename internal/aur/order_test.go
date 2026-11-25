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

func TestBasesSplitsShareOneRepo(t *testing.T) {
	names := []string{"gcc14-fortran", "gcc14-libs", "gcc14", "flutter-bin"}
	info := map[string]Pkg{
		"gcc14-libs":    {Name: "gcc14-libs", Base: "gcc14", Deps: []string{"glibc>=2.27"}},
		"gcc14":         {Name: "gcc14", Base: "gcc14", Deps: []string{"gcc14-libs"}},
		"gcc14-fortran": {Name: "gcc14-fortran", Base: "gcc14", Deps: []string{"gcc14"}},
		"flutter-bin":   {Name: "flutter-bin", Base: "flutter-bin"},
	}
	got := Bases(names, info)
	want := []string{"flutter-bin", "gcc14"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestWithDepsAddsAUROnly(t *testing.T) {
	names := []string{"jlink-software-and-documentation", "lib32-openal", "python-pyocd"}
	info := map[string]Pkg{
		"jlink-software-and-documentation": {Name: "jlink-software-and-documentation", Base: "jlink-software-and-documentation", Deps: []string{"libudev0-shim"}},
		"lib32-openal":                     {Name: "lib32-openal", Base: "lib32-openal", Deps: []string{"lib32-glibc", "lib32-portaudio"}},
		"python-pyocd":                     {Name: "python-pyocd", Base: "python-pyocd", Deps: []string{"python-pylink-square"}},
	}
	lookup := func(query []string) (map[string]Pkg, error) {
		known := map[string]Pkg{
			"libudev0-shim":        {Name: "libudev0-shim", Base: "libudev0-shim", Deps: []string{"systemd-libs"}},
			"lib32-portaudio":      {Name: "lib32-portaudio", Base: "lib32-portaudio", Deps: []string{"lib32-jack"}},
			"python-pylink-square": {Name: "python-pylink-square", Base: "python-pylink-square", Deps: []string{"jlink-software-and-documentation"}},
		}
		out := map[string]Pkg{}
		for _, n := range query {
			if p, ok := known[n]; ok {
				out[n] = p
			}
		}
		return out, nil
	}
	got, info, err := WithDeps(names, info, lookup, nil)
	if err != nil {
		t.Fatal(err)
	}
	order := Bases(got, info)
	pos := map[string]int{}
	for i, n := range order {
		pos[n] = i
	}
	before := [][2]string{
		{"libudev0-shim", "jlink-software-and-documentation"},
		{"jlink-software-and-documentation", "python-pylink-square"},
		{"python-pylink-square", "python-pyocd"},
		{"lib32-portaudio", "lib32-openal"},
	}
	for _, pair := range before {
		if pos[pair[0]] >= pos[pair[1]] {
			t.Fatalf("%s should come before %s in %v", pair[0], pair[1], order)
		}
	}
	for _, n := range order {
		if n == "lib32-jack" || n == "systemd-libs" {
			t.Fatalf("repo package %s was queued", n)
		}
	}
}

func TestWithDepsIncludesCheckDep(t *testing.T) {
	names := []string{"python-pylink-square"}
	info := map[string]Pkg{
		"python-pylink-square": {Name: "python-pylink-square", Base: "python-pylink-square", Deps: []string{"python-mock"}},
	}
	lookup := func(query []string) (map[string]Pkg, error) {
		if len(query) != 1 || query[0] != "python-mock" {
			t.Fatalf("query %v", query)
		}
		return map[string]Pkg{
			"python-mock": {Name: "python-mock", Base: "python-mock"},
		}, nil
	}
	got, info, err := WithDeps(names, info, lookup, nil)
	if err != nil {
		t.Fatal(err)
	}
	order := Bases(got, info)
	if len(order) != 2 || order[0] != "python-mock" || order[1] != "python-pylink-square" {
		t.Fatalf("%v", order)
	}
}

func TestBasesOrdersAcrossRepos(t *testing.T) {
	names := []string{"flutter-bin", "unionfs-fuse"}
	info := map[string]Pkg{
		"flutter-bin":  {Name: "flutter-bin", Base: "flutter-bin", Deps: []string{"unionfs-fuse"}},
		"unionfs-fuse": {Name: "unionfs-fuse", Base: "unionfs-fuse"},
	}
	got := Bases(names, info)
	want := []string{"unionfs-fuse", "flutter-bin"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestWithDepsSkipsRepoProvide(t *testing.T) {
	names := []string{"cemu"}
	info := map[string]Pkg{
		"cemu": {Name: "cemu", Base: "cemu", Deps: []string{"sdl2"}},
	}
	lookup := func(query []string) (map[string]Pkg, error) {
		t.Fatalf("lookup %v", query)
		return nil, nil
	}
	got, _, err := WithDeps(names, info, lookup, func(spec string) bool { return spec == "sdl2" })
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "cemu" {
		t.Fatalf("%v", got)
	}
}

func TestWithDepsKeepsExplicitUpgrade(t *testing.T) {
	names := []string{"sdl2", "cemu"}
	info := map[string]Pkg{
		"sdl2": {Name: "sdl2", Base: "sdl2"},
		"cemu": {Name: "cemu", Base: "cemu", Deps: []string{"sdl2"}},
	}
	lookup := func(query []string) (map[string]Pkg, error) {
		t.Fatalf("lookup %v", query)
		return nil, nil
	}
	got, _, err := WithDeps(names, info, lookup, func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "sdl2" || got[1] != "cemu" {
		t.Fatalf("%v", got)
	}
}
