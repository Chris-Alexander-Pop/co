package aur

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type rpcResp struct {
	Results []struct {
		Name         string   `json:"Name"`
		PackageBase  string   `json:"PackageBase"`
		Depends      []string `json:"Depends"`
		MakeDepends  []string `json:"MakeDepends"`
		CheckDepends []string `json:"CheckDepends"`
	} `json:"results"`
}

// Pkg is one AUR package. Base is the git repo. Split packages share a base.
type Pkg struct {
	Name string
	Base string
	Deps []string
}

// Info asks the AUR for names. Packages the RPC does not know are omitted.
func Info(names []string) (map[string]Pkg, error) {
	out := map[string]Pkg{}
	if len(names) == 0 {
		return out, nil
	}
	q := url.Values{}
	for _, n := range names {
		q.Add("arg[]", n)
	}
	res, err := http.Get("https://aur.archlinux.org/rpc/v5/info?" + q.Encode())
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("aur rpc %s", res.Status)
	}
	var body rpcResp
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return nil, err
	}
	for _, r := range body.Results {
		// The builder's makepkg.conf leaves check on, so checkdepends are
		// installed too. python-mock is an AUR checkdepend of pylink.
		deps := append([]string{}, r.Depends...)
		deps = append(deps, r.MakeDepends...)
		deps = append(deps, r.CheckDepends...)
		base := r.PackageBase
		if base == "" {
			base = r.Name
		}
		out[r.Name] = Pkg{Name: r.Name, Base: base, Deps: deps}
	}
	return out, nil
}

// Deps asks the AUR for depends of names. Missing packages get an empty list.
func Deps(names []string) (map[string][]string, error) {
	info, err := Info(names)
	if err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for _, n := range names {
		out[n] = info[n].Deps
	}
	return out, nil
}

// Upgrade is one line of `yay -Qua`: name, then the version being installed.
type Upgrade struct {
	Name string
	New  string
}

// ParseUpgrades reads `yay -Qua` lines: "name old -> new".
func ParseUpgrades(text string) []Upgrade {
	var out []Upgrade
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		u := Upgrade{Name: fields[0]}
		for i, f := range fields {
			if f == "->" && i+1 < len(fields) {
				u.New = fields[i+1]
			}
		}
		out = append(out, u)
	}
	return out
}

// ParseQua reads `yay -Qua` lines: "name old -> new".
func ParseQua(text string) []string {
	ups := ParseUpgrades(text)
	names := make([]string, len(ups))
	for i, u := range ups {
		names[i] = u.Name
	}
	return names
}

// ArtifactHasVersion reports whether a built package file is pkg at version.
// topgrade-17.12.3-1-x86_64.pkg.tar.zst matches topgrade 17.12.3-1.
func ArtifactHasVersion(pkg, version, filename string) bool {
	if pkg == "" || version == "" {
		return false
	}
	return strings.HasPrefix(filename, pkg+"-"+version+"-")
}
