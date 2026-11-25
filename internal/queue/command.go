package queue

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Chris-Alexander-Pop/co/internal/deps"
)

func runCommand(job *Job, dir string, logf func(string)) error {
	if len(job.Build) == 0 || job.Build[0] == "" {
		return fmt.Errorf("no command")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	_ = os.MkdirAll(deps.GoPath, 0o755)
	_ = os.MkdirAll(deps.GoCache, 0o755)
	if err := deps.Ensure(job.Build, logf); err != nil {
		return err
	}
	before := snapBins(deps.CargoBin(), filepath.Join(deps.GoPath, "bin"))
	installed := map[string]bool{}
	env := buildEnv(job.Jobs)
	var last error
	for attempt := 0; attempt < 4; attempt++ {
		buf := &logBuf{logf: logf}
		cmd, err := command(job.Build, env)
		if err != nil {
			buf.add(err.Error() + "\n")
			last = err
		} else {
			cmd.Dir = dir
			cmd.Env = env
			last = runLogged(cmd, buf.add)
		}
		if last == nil {
			return copyChangedBins(filepath.Join(dir, "co-bin"), before, deps.CargoBin(), filepath.Join(deps.GoPath, "bin"))
		}
		pkgs := freshPackages(deps.MissingPackages(buf.text), installed)
		if len(pkgs) == 0 {
			return last
		}
		logf("installing " + joinWords(pkgs))
		if err := deps.Install(pkgs, logf); err != nil {
			logf(err.Error())
			return last
		}
	}
	return last
}

func freshPackages(pkgs []string, seen map[string]bool) []string {
	var out []string
	for _, pkg := range pkgs {
		if seen[pkg] {
			continue
		}
		seen[pkg] = true
		out = append(out, pkg)
	}
	return out
}

func joinWords(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += " "
		}
		out += p
	}
	return out
}

type logBuf struct {
	logf func(string)
	text string
}

func (b *logBuf) add(s string) {
	b.text += s
	if b.logf != nil {
		b.logf(s)
	}
}

type binSnap struct {
	size int64
	mod  int64
}

func snapBins(dirs ...string) map[string]binSnap {
	out := map[string]binSnap{}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			info, err := e.Info()
			if err != nil || e.IsDir() || !info.Mode().IsRegular() {
				continue
			}
			out[filepath.Join(dir, e.Name())] = binSnap{size: info.Size(), mod: info.ModTime().UnixNano()}
		}
	}
	return out
}

func copyChangedBins(dest string, before map[string]binSnap, dirs ...string) error {
	now := snapBins(dirs...)
	var n int
	for path, st := range now {
		if old, ok := before[path]; ok && old == st {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if n == 0 {
			if err := os.MkdirAll(dest, 0o755); err != nil {
				return err
			}
		}
		n++
		if err := copyFile(path, filepath.Join(dest, filepath.Base(path)), info.Mode()); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode.Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
