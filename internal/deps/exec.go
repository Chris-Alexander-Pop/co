package deps

import (
	"os"
	"os/exec"
	"strings"
)

// Ensure installs tools named by argv when they are not already on PATH.
// cargo and rustc pull in a stable toolchain with the wasm32 target.
func Ensure(argv []string, logf func(string)) error {
	rust := false
	var rest []string
	for _, tool := range Mentioned(argv) {
		switch tool {
		case "cargo", "rustc", "rustdoc", "rustup":
			rust = true
		default:
			rest = append(rest, tool)
		}
	}
	if rust {
		if err := ensureRust(logf); err != nil {
			return err
		}
	}
	for _, tool := range rest {
		if err := ensureTool(tool, logf); err != nil {
			return err
		}
	}
	return nil
}

func ensureTool(tool string, logf func(string)) error {
	if _, err := look(tool); err == nil {
		return nil
	}
	pkg := toolPkg[tool]
	if pkg == "" {
		pkg = tool
	}
	return Install([]string{pkg}, logf)
}

func ensureRust(logf func(string)) error {
	if rustReady() {
		return nil
	}
	if _, err := look("rustup"); err != nil {
		if err := Install([]string{"rustup"}, logf); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(CargoHome, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(RustupHome, 0o755); err != nil {
		return err
	}
	if err := runEnv(RustEnv(), []string{"rustup", "default", "stable"}, logf); err != nil {
		return err
	}
	return runEnv(RustEnv(), []string{"rustup", "target", "add", "wasm32-unknown-unknown"}, logf)
}

func rustReady() bool {
	if _, err := look("rustc"); err != nil {
		return false
	}
	cmd := exec.Command("rustup", "target", "list", "--installed")
	cmd.Env = RustEnv()
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "wasm32-unknown-unknown")
}

func look(name string) (string, error) {
	if strings.Contains(name, "/") {
		_, err := os.Stat(name)
		return name, err
	}
	return exec.LookPath(name)
}

func run(args []string, logf func(string)) error {
	return runEnv(os.Environ(), args, logf)
}

func runEnv(env, args []string, logf func(string)) error {
	if logf != nil {
		logf(strings.Join(args, " "))
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if logf != nil && len(out) > 0 {
		logf(string(out))
	}
	return err
}
