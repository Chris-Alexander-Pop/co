package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Chris-Alexander-Pop/co/internal/archive"
	"github.com/Chris-Alexander-Pop/co/internal/client"
	"github.com/Chris-Alexander-Pop/co/internal/queue"
	"github.com/Chris-Alexander-Pop/co/internal/ui"
)

func splitCommand(args []string) (flags, cmd []string, ok bool) {
	for i, a := range args {
		if a == "--command" {
			return args[:i], args[i+1:], true
		}
	}
	return nil, nil, false
}

func cmdCommand(args []string) error {
	flags, cmd, ok := splitCommand(args)
	if !ok || len(cmd) == 0 {
		return fmt.Errorf("co --command needs the command to run")
	}
	modeFlag, rest, err := flagFallback(flags)
	if err != nil {
		return err
	}
	bg := false
	for _, a := range rest {
		if a == "--no-wait" || a == "--bg" {
			bg = true
			continue
		}
		return fmt.Errorf("unknown flag %q", a)
	}
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	cfg, err := load()
	if err != nil {
		return err
	}
	_, cli, err := reach(cfg, modeFlag)
	if err != nil {
		return err
	}
	if cli == nil {
		return runArgv(wd, cmd)
	}
	ui.Step("packing %s", filepath.Base(wd))
	var buf bytes.Buffer
	if err := archive.PackFilter(wd, &buf, skipHeavy); err != nil {
		return err
	}
	job, err := cli.SubmitCommand(filepath.Base(wd), cfg.Jobs, cmd, buf.Bytes())
	if err != nil {
		return err
	}
	noteSame(job)
	if bg {
		hintQueued(job.ID)
		return nil
	}
	done, err := waitJob(cli, job.ID)
	if err != nil {
		return err
	}
	return fetchCommandOutputs(cli, done.ID, wd)
}

func runArgv(dir string, argv []string) error {
	ui.Step("%s", strings.Join(argv, " "))
	c := exec.Command(argv[0], argv[1:]...)
	c.Dir = dir
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin
	if err := c.Run(); err != nil {
		return fmt.Errorf("%s: %w", strings.Join(argv, " "), err)
	}
	return nil
}

func skipHeavy(rel string, isDir bool) bool {
	if !isDir {
		return false
	}
	switch filepath.Base(rel) {
	case "target", "node_modules", ".cache":
		return true
	default:
		return false
	}
}

func fetchCommandOutputs(cli *client.Client, id, wd string) error {
	tarPath := filepath.Join(wd, queue.CommandArtifact)
	err := cli.Download(id, queue.CommandArtifact, tarPath)
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			ui.Info("no binaries to install")
			return nil
		}
		return err
	}
	f, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	dest, err := os.MkdirTemp("", "co-bins-")
	if err != nil {
		f.Close()
		os.Remove(tarPath)
		return err
	}
	unpackErr := archive.Unpack(f, dest)
	f.Close()
	os.Remove(tarPath)
	if unpackErr != nil {
		os.RemoveAll(dest)
		return unpackErr
	}
	err = placeOutputs(dest, wd)
	os.RemoveAll(dest)
	return err
}

func placeOutputs(root, wd string) error {
	binDir, err := localBin()
	if err != nil {
		return err
	}
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, "co-bin/") {
			ui.Step("install %s", filepath.Base(path))
			return copyOut(path, filepath.Join(binDir, filepath.Base(path)), info.Mode()|0o755)
		}
		if strings.HasPrefix(rel, "target/") {
			ui.Step("pull %s", rel)
			return copyOut(path, filepath.Join(wd, filepath.FromSlash(rel)), info.Mode())
		}
		return nil
	})
}

func localBin() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	cargo := filepath.Join(home, ".cargo", "bin")
	if st, err := os.Stat(cargo); err == nil && st.IsDir() {
		return cargo, nil
	}
	local := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(local, 0o755); err != nil {
		return "", err
	}
	return local, nil
}

func copyOut(src, dst string, mode os.FileMode) error {
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
