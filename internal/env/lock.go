package env

import (
	"fmt"
	"strings"

	toml "github.com/BurntSushi/toml"
)

// ParseLock reads a mise.lock file and returns, per tool, the set of
// platform keys it resolved builds for (e.g. "macos-arm64",
// "linux-x64"). Platform sub-tables use literal dotted keys
// ("platforms.linux-x64"), so a platform the tool does not publish
// for is simply an absent key — that absence is the "not for this
// machine" signal. Tools missing from the lock have no map entry.
func ParseLock(data []byte) (map[string][]string, error) {
	raw := map[string]any{}
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse mise.lock: %w", err)
	}
	out := map[string][]string{}
	tools, _ := raw["tools"].(map[string]any)
	for name, entries := range tools {
		list, ok := entries.([]any)
		if !ok {
			continue
		}
		for _, e := range list {
			entry, ok := e.(map[string]any)
			if !ok {
				continue
			}
			for key := range entry {
				if platform, ok := strings.CutPrefix(key, "platforms."); ok {
					out[name] = appendUnique(out[name], platform)
				}
			}
		}
	}
	return out, nil
}

func appendUnique(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}
