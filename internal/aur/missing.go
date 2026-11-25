package aur

import (
	"regexp"
	"strings"
)

var missingDepRes = []*regexp.Regexp{
	regexp.MustCompile(`unable to satisfy dependency '([^']+)'`),
	regexp.MustCompile(`cannot resolve "([^"]+)"`),
}

// MissingDeps reads pacman output for dependencies the install could not resolve.
func MissingDeps(log string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		out = append(out, name)
	}
	for _, re := range missingDepRes {
		for _, m := range re.FindAllStringSubmatch(log, -1) {
			add(m[1])
		}
	}
	return out
}
