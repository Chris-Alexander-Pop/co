package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Chris-Alexander-Pop/co/internal/archive"
	"github.com/Chris-Alexander-Pop/co/internal/client"
	"github.com/Chris-Alexander-Pop/co/internal/ui"
)

// seedDirs is the isolated cache tree used by --seed-cache builds.
type seedDirs struct {
	Root    string
	Ccache  string
	GoCache string
	Cargo   string
}

func seedRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	cache := os.Getenv("XDG_CACHE_HOME")
	if cache == "" {
		cache = filepath.Join(home, ".cache")
	}
	return filepath.Join(cache, "co", "seed"), nil
}

func prepareSeed() (*seedDirs, []string, error) {
	root, err := seedRoot()
	if err != nil {
		return nil, nil, err
	}
	d := &seedDirs{
		Root:    root,
		Ccache:  filepath.Join(root, "ccache"),
		GoCache: filepath.Join(root, "gocache"),
		Cargo:   filepath.Join(root, "cargo"),
	}
	for _, dir := range []string{d.Ccache, d.GoCache, filepath.Join(d.Cargo, "bin")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, nil, err
		}
	}
	env := seedEnviron(d)
	return d, env, nil
}

func seedEnviron(d *seedDirs) []string {
	path := os.Getenv("PATH")
	for _, prefix := range []string{"/usr/lib/ccache/bin", "/usr/lib/ccache"} {
		if st, err := os.Stat(prefix); err == nil && st.IsDir() {
			path = prefix + string(os.PathListSeparator) + path
			break
		}
	}
	base := os.Environ()
	set := map[string]string{
		"PATH":        path,
		"CCACHE_DIR":  d.Ccache,
		"GOCACHE":     d.GoCache,
		"CARGO_HOME":  d.Cargo,
		"CCACHE_MAXSIZE": "20G",
	}
	skip := map[string]bool{}
	for k := range set {
		skip[k] = true
	}
	out := make([]string, 0, len(base)+len(set))
	for _, e := range base {
		k, _, ok := strings.Cut(e, "=")
		if ok && skip[k] {
			continue
		}
		out = append(out, e)
	}
	keys := []string{"PATH", "CCACHE_DIR", "CCACHE_MAXSIZE", "GOCACHE", "CARGO_HOME"}
	for _, k := range keys {
		out = append(out, k+"="+set[k])
	}
	return out
}

func uploadSeed(cli *client.Client, d *seedDirs) error {
	ccache, err := packIfPresent(d.Ccache)
	if err != nil {
		return fmt.Errorf("pack ccache: %w", err)
	}
	gocache, err := packIfPresent(d.GoCache)
	if err != nil {
		return fmt.Errorf("pack gocache: %w", err)
	}
	cargo, err := packCargo(d.Cargo)
	if err != nil {
		return fmt.Errorf("pack cargo: %w", err)
	}
	if len(ccache) == 0 && len(gocache) == 0 && len(cargo) == 0 {
		ui.Info("no cache to upload")
		return nil
	}
	ui.Step("upload cache to builder")
	res, err := cli.UploadCache(ccache, gocache, cargo)
	if err != nil {
		return err
	}
	for _, p := range res.Parts {
		ui.Info("%s %d bytes", p.Name, p.Bytes)
	}
	return nil
}

func packIfPresent(dir string) ([]byte, error) {
	st, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !st.IsDir() {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}
	var buf bytes.Buffer
	if err := archive.Pack(dir, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func packCargo(cargoHome string) ([]byte, error) {
	var rels []string
	for _, name := range []string{"registry", "git"} {
		if st, err := os.Stat(filepath.Join(cargoHome, name)); err == nil && st.IsDir() {
			rels = append(rels, name)
		}
	}
	if len(rels) == 0 {
		return nil, nil
	}
	var buf bytes.Buffer
	if err := archive.PackPaths(cargoHome, rels, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func pruneSeed(d *seedDirs) error {
	if d == nil || d.Root == "" {
		return nil
	}
	ui.Step("prune local seed cache")
	return os.RemoveAll(d.Root)
}

func flagBuildOpts(args []string) (seed, bg bool, rest []string, err error) {
	for _, a := range args {
		switch a {
		case "--seed-cache":
			seed = true
		case "--bg", "--no-wait":
			bg = true
		default:
			rest = append(rest, a)
		}
	}
	if seed && bg {
		return false, false, nil, fmt.Errorf("--seed-cache cannot be used with --bg")
	}
	return seed, bg, rest, nil
}
