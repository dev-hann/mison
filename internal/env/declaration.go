// Package env reads and writes the environment declaration (mise.toml).
package env

// Tool is a single entry in the [tools] table. Options carries every
// non-version/os key of a table entry (postinstall, depends, ...);
// nil means a bare string entry with no options.
type Tool struct {
	Name    string
	Version string
	OS      []string // optional os restriction, e.g. ["linux"], ["macos/arm64"]
	Options map[string]any
}

// PathBackend returns the machine-local path backing this tool (mise's
// path backend), or "" for registry-backed tools. Path-backed entries
// only work where the path exists — they are not portable.
func (t Tool) PathBackend() string {
	p, _ := t.Options["path"].(string)
	return p
}

// Declaration is the parsed [tools] section of mise.toml.
type Declaration struct {
	Tools []Tool
}
