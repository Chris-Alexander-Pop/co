// Package deps installs tools a remote command needs.
// The builder is Arch. Package names are Arch package names.
package deps

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

const (
	// RustupHome is the toolchain. It stays in the image, not on a volume.
	RustupHome = "/opt/rustup"
	// CargoHome is the cargo bin directory and registry.
	CargoHome = "/opt/cargo"
	// GoPath is the persistent Go workspace on the builder.
	GoPath = "/var/cache/co/go"
	// GoCache is the persistent Go build cache.
	GoCache = "/var/cache/co/gocache"
)

// CargoBin is where cargo install writes binaries.
func CargoBin() string {
	return filepath.Join(CargoHome, "bin")
}

// MergeEnv returns base with the keys in set replaced.
func MergeEnv(base []string, set map[string]string) []string {
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
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out = append(out, k+"="+set[k])
	}
	return out
}

// RustEnv is the environment a rustup or cargo invocation should see.
func RustEnv() []string {
	path := os.Getenv("PATH")
	prefix := filepath.Join(CargoHome, "bin")
	if !strings.Contains(path, prefix) {
		path = prefix + string(os.PathListSeparator) + path
	}
	return MergeEnv(os.Environ(), map[string]string{
		"PATH":        path,
		"RUSTUP_HOME": RustupHome,
		"CARGO_HOME":  CargoHome,
	})
}

// toolPkg maps a binary name to the Arch package that provides it.
var toolPkg = map[string]string{
	"cargo":      "rustup",
	"rustc":      "rustup",
	"rustdoc":    "rustup",
	"rustup":     "rustup",
	"wasm-opt":   "binaryen",
	"protoc":     "protobuf",
	"pkg-config": "pkgconf",
	"nasm":       "nasm",
	"cmake":      "cmake",
	"go":         "go",
	"clang":      "clang",
	"clang++":    "clang",
	"cc":         "gcc",
	"gcc":        "gcc",
	"g++":        "gcc",
	"node":       "nodejs",
	"nodejs":     "nodejs",
	"npm":        "npm",
	"python":     "python",
	"python3":    "python",
	"make":       "make",
}

// pcPkg maps a pkg-config module name to an Arch package.
var pcPkg = map[string]string{
	"libudev":        "systemd",
	"libsystemd":     "systemd",
	"openssl":        "openssl",
	"libssl":         "openssl",
	"libcrypto":      "openssl",
	"alsa":           "alsa-lib",
	"vulkan":         "vulkan-headers",
	"x11":            "libx11",
	"freetype2":      "freetype2",
	"libpng":         "libpng",
	"wayland-client": "wayland",
	"xkbcommon":      "libxkbcommon",
	"dbus-1":         "dbus",
}

// linkPkg maps an -l name to an Arch package.
var linkPkg = map[string]string{
	"ssl":    "openssl",
	"crypto": "openssl",
	"udev":   "systemd",
	"z":      "zlib",
	"bz2":    "bzip2",
	"lzma":   "xz",
	"stdc++": "gcc-libs",
	"clang":  "clang",
}

var linkSkip = map[string]bool{
	"m": true, "c": true, "dl": true, "rt": true, "pthread": true,
	"gcc": true, "gcc_s": true,
}

var notAPackage = map[string]bool{
	"sh": true, "bash": true, "sudo": true, "echo": true, "true": true, "false": true,
}

var (
	reNotFound = regexp.MustCompile(`([A-Za-z0-9+_.-]+): command not found`)
	rePkg      = regexp.MustCompile(`(?:No package '|Package ')([^']+)' not found`)
	reLink     = regexp.MustCompile(`cannot find -l([A-Za-z0-9_+\-]+)`)
)

// Mentioned returns tool names referenced by a command line.
func Mentioned(argv []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, field := range strings.Fields(strings.Join(argv, " ")) {
		base := filepath.Base(strings.Trim(field, "'\"`;|&"))
		if _, ok := toolPkg[base]; !ok || seen[base] {
			continue
		}
		seen[base] = true
		out = append(out, base)
	}
	return out
}

// MissingPackages reads a failed build log and returns Arch packages that
// would satisfy the missing commands, pkg-config modules, and libraries.
func MissingPackages(log string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(pkg string) {
		if pkg == "" || seen[pkg] {
			return
		}
		seen[pkg] = true
		out = append(out, pkg)
	}
	for _, m := range reNotFound.FindAllStringSubmatch(log, -1) {
		name := m[1]
		if notAPackage[name] {
			continue
		}
		if pkg, ok := toolPkg[name]; ok {
			add(pkg)
			continue
		}
		add(name)
	}
	for _, m := range rePkg.FindAllStringSubmatch(log, -1) {
		name := m[1]
		if pkg, ok := pcPkg[name]; ok {
			add(pkg)
			continue
		}
		add(name)
	}
	for _, m := range reLink.FindAllStringSubmatch(log, -1) {
		name := m[1]
		if linkSkip[name] {
			continue
		}
		if pkg, ok := linkPkg[name]; ok {
			add(pkg)
			continue
		}
		add(name)
	}
	if strings.Contains(log, "libclang") {
		add("clang")
	}
	return out
}

var pacmanMu sync.Mutex

// Install asks pacman for pkgs. Concurrent callers wait, because pacman
// locks the database.
func Install(pkgs []string, logf func(string)) error {
	if len(pkgs) == 0 {
		return nil
	}
	pacmanMu.Lock()
	defer pacmanMu.Unlock()
	return run(pacmanArgs(pkgs), logf)
}

func pacmanArgs(pkgs []string) []string {
	args := []string{"pacman", "-S", "--needed", "--noconfirm"}
	return append(args, pkgs...)
}
