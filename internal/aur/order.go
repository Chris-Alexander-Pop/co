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

// Bases returns one git repo per package base, dependency bases first.
// gcc14-libs, gcc14, and gcc14-fortran share the gcc14 repo. A name the RPC
// does not know is cloned under its own name.
func Bases(names []string, info map[string]Pkg) []string {
	seen := map[string]struct{}{}
	members := map[string][]string{}
	var bases []string
	for _, n := range names {
		b := baseOf(n, info)
		members[b] = append(members[b], n)
		if _, ok := seen[b]; ok {
			continue
		}
		seen[b] = struct{}{}
		bases = append(bases, b)
	}
	deps := map[string][]string{}
	for _, b := range bases {
		var d []string
		for _, n := range members[b] {
			for _, spec := range info[n].Deps {
				depBase := baseOf(DepName(spec), info)
				if depBase == b {
					continue
				}
				if _, ok := seen[depBase]; !ok {
					continue
				}
				d = append(d, depBase)
			}
		}
		deps[b] = d
	}
	return Order(bases, deps)
}

// WithDeps adds AUR packages that names depend on. lookup is the AUR RPC.
// A name it does not return is a repo package and is left for pacman.
// satisfied reports a dependency the repos already provide, including a
// different package such as sdl2-compat providing sdl2. Those stay with
// pacman. A package already in names is still built.
func WithDeps(names []string, info map[string]Pkg, lookup func([]string) (map[string]Pkg, error), satisfied func(string) bool) ([]string, map[string]Pkg, error) {
	in := map[string]bool{}
	for _, n := range names {
		in[n] = true
	}
	for step := 0; step < 8; step++ {
		var query []string
		seen := map[string]bool{}
		for _, n := range names {
			for _, spec := range info[n].Deps {
				d := DepName(spec)
				if in[d] || seen[d] {
					continue
				}
				if satisfied != nil && satisfied(spec) {
					continue
				}
				seen[d] = true
				query = append(query, d)
			}
		}
		if len(query) == 0 {
			return names, info, nil
		}
		more, err := lookup(query)
		if err != nil {
			return nil, nil, err
		}
		added := false
		for _, n := range query {
			pkg, ok := more[n]
			if !ok {
				continue
			}
			info[n] = pkg
			names = append(names, n)
			in[n] = true
			added = true
		}
		if !added {
			return names, info, nil
		}
	}
	return names, info, nil
}

func baseOf(name string, info map[string]Pkg) string {
	if p, ok := info[name]; ok && p.Base != "" {
		return p.Base
	}
	return name
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
