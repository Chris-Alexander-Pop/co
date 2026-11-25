package queue

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Chris-Alexander-Pop/co/internal/archive"
)

// CommandArtifact is the tarball of binaries a command job produced.
const CommandArtifact = "bins.tar.gz"

func command(argv []string, env []string) (*exec.Cmd, error) {
	name := argv[0]
	if !strings.Contains(name, "/") {
		if p := findInPath(name, env); p != "" {
			name = p
		} else {
			return nil, fmt.Errorf("%s: command not found", argv[0])
		}
	}
	return exec.Command(name, argv[1:]...), nil
}

func findInPath(name string, env []string) string {
	for _, e := range env {
		k, v, ok := strings.Cut(e, "=")
		if !ok || k != "PATH" {
			continue
		}
		for _, dir := range strings.Split(v, string(os.PathListSeparator)) {
			p := filepath.Join(dir, name)
			st, err := os.Stat(p)
			if err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
				return p
			}
		}
	}
	return ""
}

func (q *Queue) stageCommandOutputs(job *Job) error {
	src := q.SrcDir(job.ID)
	rels, err := outputRels(src)
	if err != nil || len(rels) == 0 {
		return err
	}
	if err := os.MkdirAll(q.artifactDir(job.ID), 0o755); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(q.artifactDir(job.ID), CommandArtifact))
	if err != nil {
		return err
	}
	packErr := archive.PackPaths(src, rels, f)
	closeErr := f.Close()
	if packErr != nil {
		return packErr
	}
	return closeErr
}

func outputRels(dir string) ([]string, error) {
	var rels []string
	entries, err := os.ReadDir(filepath.Join(dir, "co-bin"))
	if err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			rels = append(rels, filepath.Join("co-bin", e.Name()))
		}
	}
	for _, profile := range []string{"release", "debug"} {
		d := filepath.Join(dir, "target", profile)
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if strings.HasSuffix(name, ".d") || strings.HasSuffix(name, ".rlib") || strings.HasSuffix(name, ".rmeta") {
				continue
			}
			info, err := e.Info()
			if err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
				continue
			}
			rels = append(rels, filepath.Join("target", profile, name))
		}
	}
	return rels, nil
}
