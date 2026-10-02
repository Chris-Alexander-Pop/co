// Package strategy loads per-project build and install commands.
package strategy

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// File is one project. dir is the directory you run co from.
// Build commands run on the builder. Install commands run on this machine
// after pull paths have been copied back into that directory.
type File struct {
	Dir     string   `yaml:"dir"`
	Skip    []string `yaml:"skip"`
	Build   []string `yaml:"build"`
	Pull    []string `yaml:"pull"`
	Install []string `yaml:"install"`
	path    string
}

// ProjectsDir is ~/.config/co/projects.
func ProjectsDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "co", "projects"), nil
}

// Find returns the strategy whose dir is cwd. The file name is used when dir is empty.
func Find(projectsDir, cwd string) (*File, error) {
	cwd, err := filepath.Abs(cwd)
	if err != nil {
		return nil, err
	}
	if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
		cwd = resolved
	}
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var found *File
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		path := filepath.Join(projectsDir, e.Name())
		f, err := load(path)
		if err != nil {
			return nil, err
		}
		ok, err := f.matches(cwd, strings.TrimSuffix(e.Name(), ".yaml"))
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("two strategies match %s (%s and %s)", cwd, found.path, f.path)
		}
		found = f
	}
	if found != nil && len(found.Build) == 0 {
		return nil, fmt.Errorf("%s has no build commands", found.path)
	}
	return found, nil
}

func load(path string) (*File, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f File
	if err := yaml.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	f.path = path
	return &f, nil
}

func (f *File) matches(cwd, stem string) (bool, error) {
	if f.Dir == "" {
		return filepath.Base(cwd) == stem, nil
	}
	dir, err := filepath.Abs(f.Dir)
	if err != nil {
		return false, err
	}
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	return dir == cwd, nil
}

// Skip reports whether rel should be left out of the upload.
func (f *File) SkipRel(rel string, isDir bool) bool {
	base := filepath.Base(rel)
	for _, pat := range f.Skip {
		ok, err := filepath.Match(pat, base)
		if err == nil && ok {
			return true
		}
	}
	return false
}

// Expand replaces {jobs} and {kernelrelease} in each command.
func Expand(lines []string, jobs int, kernelRelease string) []string {
	out := make([]string, len(lines))
	replacer := strings.NewReplacer(
		"{jobs}", strconv.Itoa(jobs),
		"{kernelrelease}", kernelRelease,
	)
	for i, line := range lines {
		out[i] = replacer.Replace(line)
	}
	return out
}
