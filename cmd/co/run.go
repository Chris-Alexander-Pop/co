package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Chris-Alexander-Pop/co/internal/archive"
	"github.com/Chris-Alexander-Pop/co/internal/client"
	"github.com/Chris-Alexander-Pop/co/internal/config"
	"github.com/Chris-Alexander-Pop/co/internal/fallback"
	"github.com/Chris-Alexander-Pop/co/internal/queue"
	"github.com/Chris-Alexander-Pop/co/internal/ui"
)

func reach(cfg *config.Config, flag string) (mode string, cli *client.Client, err error) {
	mode, err = fallback.Choose(flag, cfg.Fallback)
	if err != nil {
		return "", nil, err
	}
	cli = client.New(cfg.URL, cfg.Token)
	if err := cli.Healthy(); err != nil {
		ui.Info("builder unavailable: %s", err)
		if mode == fallback.Cancel {
			return mode, nil, fmt.Errorf("builder unavailable, fallback is cancel")
		}
		ui.Info("building on this machine")
		return mode, nil, nil
	}
	return mode, cli, nil
}

func packCwd() ([]byte, error) {
	var buf bytes.Buffer
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	if err := archive.Pack(wd, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func noteSame(job *queue.Job) {
	if job != nil && job.Reused {
		ui.Info("job %s already has this source", job.ID)
	}
}

func waitJob(cli *client.Client, id string) (*queue.Job, error) {
	job, err := follow(cli, id)
	if err != nil {
		return job, err
	}
	return job, nil
}

func downloadPkgs(cli *client.Client, id, dest string) ([]string, error) {
	names, err := cli.Artifacts(id)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, name := range names {
		path := filepath.Join(dest, name)
		ui.Step("pull %s", name)
		if err := cli.Download(id, name, path); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, nil
}

type outputError struct {
	err error
	out string
}

func (e *outputError) Error() string { return e.err.Error() }
func (e *outputError) Unwrap() error { return e.err }

func commandOutput(err error) string {
	var oe *outputError
	if errors.As(err, &oe) {
		return oe.out
	}
	return ""
}

func installPkgs(paths []string) error {
	if len(paths) == 0 {
		return fmt.Errorf("no packages were built")
	}
	if twins, err := repoPkgsFirst(paths); err != nil {
		ui.Info("provider check: %s", err)
	} else if len(twins) > 0 {
		ui.Step("install %s", strings.Join(twins, " "))
		if err := pacmanSync(twins); err != nil {
			return err
		}
	}
	args := append([]string{"pacman", "-U", "--noconfirm"}, paths...)
	cmd := exec.Command("sudo", args...)
	var buf bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &buf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &buf)
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return &outputError{err: err, out: buf.String()}
	}
	return nil
}

func pacmanSync(pkgs []string) error {
	args := append([]string{"pacman", "-S", "--needed", "--noconfirm"}, pkgs...)
	cmd := exec.Command("sudo", args...)
	var buf bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &buf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &buf)
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return &outputError{err: err, out: buf.String()}
	}
	return nil
}

// repoPkgsFirst finds lib32 packages that should be installed from the repos
// before pacman -U. A virtual dependency such as lib32-jack has two providers.
// pacman --noconfirm takes the first, lib32-jack2, which conflicts with
// pipewire-jack. The lib32 twin of the provider already installed is used instead.
func repoPkgsFirst(paths []string) ([]string, error) {
	var deps []string
	seenDep := map[string]bool{}
	for _, path := range paths {
		got, err := pkgDepends(path)
		if err != nil {
			return nil, err
		}
		for _, dep := range got {
			if seenDep[dep] {
				continue
			}
			seenDep[dep] = true
			deps = append(deps, dep)
		}
	}
	missing, err := missingPkgs(deps)
	if err != nil || len(missing) == 0 {
		return nil, err
	}
	provides, err := readProvides("/var/lib/pacman/local")
	if err != nil {
		return nil, err
	}
	var twins []string
	seen := map[string]bool{}
	for _, dep := range missing {
		twin := lib32Twin(dep, provides)
		if twin == "" || seen[twin] || !repoHas(twin) {
			continue
		}
		seen[twin] = true
		twins = append(twins, twin)
	}
	sort.Strings(twins)
	return twins, nil
}

func pkgDepends(path string) ([]string, error) {
	out, err := exec.Command("bsdtar", "-xOf", path, ".PKGINFO").Output()
	if err != nil {
		return nil, err
	}
	return parseDepends(string(out)), nil
}

func parseDepends(pkginfo string) []string {
	var out []string
	for _, line := range strings.Split(pkginfo, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "depend = ") {
			continue
		}
		out = append(out, depName(strings.TrimPrefix(line, "depend = ")))
	}
	return out
}

func depName(spec string) string {
	for _, cut := range []string{">=", "<=", "=", ">", "<"} {
		if i := strings.Index(spec, cut); i >= 0 {
			return strings.TrimSpace(spec[:i])
		}
	}
	return strings.TrimSpace(spec)
}

// lib32Twin returns the lib32 package matching an installed provider.
// lib32-jack with pipewire-jack installed returns lib32-pipewire-jack.
func lib32Twin(dep string, provides map[string][]string) string {
	if !strings.HasPrefix(dep, "lib32-") {
		return ""
	}
	base := strings.TrimPrefix(dep, "lib32-")
	var hits []string
	for name, provs := range provides {
		if name == base {
			continue
		}
		for _, p := range provs {
			if p == base {
				twin := "lib32-" + name
				if twin != dep {
					hits = append(hits, twin)
				}
				break
			}
		}
	}
	sort.Strings(hits)
	if len(hits) == 0 {
		return ""
	}
	return hits[0]
}

func readProvides(root string) (map[string][]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(root, e.Name(), "desc"))
		if err != nil {
			continue
		}
		name, provs := parseDesc(string(b))
		if name != "" {
			out[name] = provs
		}
	}
	return out, nil
}

func parseDesc(desc string) (string, []string) {
	var name string
	var provs []string
	section := ""
	for _, line := range strings.Split(desc, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			section = ""
			continue
		}
		if strings.HasPrefix(line, "%") && strings.HasSuffix(line, "%") {
			section = line
			continue
		}
		switch section {
		case "%NAME%":
			if name == "" {
				name = line
			}
		case "%PROVIDES%":
			provs = append(provs, depName(line))
		}
	}
	return name, provs
}

func missingPkgs(deps []string) ([]string, error) {
	if len(deps) == 0 {
		return nil, nil
	}
	cmd := exec.Command("pacman", append([]string{"-T"}, deps...)...)
	out, err := cmd.Output()
	if err == nil {
		return nil, nil
	}
	var missing []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			missing = append(missing, line)
		}
	}
	if len(missing) == 0 {
		return nil, err
	}
	return missing, nil
}

func repoHas(name string) bool {
	return exec.Command("pacman", "-Si", name).Run() == nil
}

func runLocal(name string, env []string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if env != nil {
		cmd.Env = env
	}
	return cmd.Run()
}
