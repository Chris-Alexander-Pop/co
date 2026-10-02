// Package aur orders AUR package names so a dependency is built before the package that needs it.
package aur

import (
	"sort"
	"strings"
)

// DepName strips a version constraint from an AUR depends entry.
func DepName(spec string) string {
	spec = strings.TrimSpace(spec)
	for _, cut := range []string{">=", "<=", "=", ">", "<"} {
		if i := strings.Index(spec, cut); i >= 0 {
			return spec[:i]
		}
	}
	return spec
}

// Order returns names so that if a package in the set depends on another package in the set,
// the dependency comes first. Names not mentioned as dependencies keep a stable order.
func Order(names []string, deps map[string][]string) []string {
	inSet := map[string]struct{}{}
	for _, n := range names {
		inSet[n] = struct{}{}
	}
	indeg := map[string]int{}
	next := map[string][]string{}
	for _, n := range names {
		indeg[n] = 0
	}
	for _, n := range names {
		for _, spec := range deps[n] {
			dep := DepName(spec)
			if _, ok := inSet[dep]; !ok || dep == n {
				continue
			}
			next[dep] = append(next[dep], n)
			indeg[n]++
		}
	}
	var ready []string
	for _, n := range names {
		if indeg[n] == 0 {
			ready = append(ready, n)
		}
	}
	sort.Strings(ready)
	var out []string
	for len(ready) > 0 {
		n := ready[0]
		ready = ready[1:]
		out = append(out, n)
		sort.Strings(next[n])
		for _, m := range next[n] {
			indeg[m]--
			if indeg[m] == 0 {
				ready = append(ready, m)
				sort.Strings(ready)
			}
		}
	}
	if len(out) == len(names) {
		return out
	}
	seen := map[string]struct{}{}
	for _, n := range out {
		seen[n] = struct{}{}
	}
	var rest []string
	for _, n := range names {
		if _, ok := seen[n]; !ok {
			rest = append(rest, n)
		}
	}
	sort.Strings(rest)
	return append(out, rest...)
}
