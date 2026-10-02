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
		Depends      []string `json:"Depends"`
		MakeDepends  []string `json:"MakeDepends"`
		CheckDepends []string `json:"CheckDepends"`
	} `json:"results"`
}

// Deps asks the AUR for depends of names. Missing packages get an empty list.
func Deps(names []string) (map[string][]string, error) {
	out := map[string][]string{}
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
	for _, n := range names {
		out[n] = nil
	}
	for _, r := range body.Results {
		var deps []string
		deps = append(deps, r.Depends...)
		deps = append(deps, r.MakeDepends...)
		deps = append(deps, r.CheckDepends...)
		out[r.Name] = deps
	}
	return out, nil
}

// ParseQua reads `yay -Qua` lines: "name old -> new".
func ParseQua(text string) []string {
	var names []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		names = append(names, fields[0])
	}
	return names
}
