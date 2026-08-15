// Package env reads and writes the environment declaration (mise.toml).
package env

// Tool is a single entry in the [tools] table.
type Tool struct {
	Name    string
	Version string
	OS      []string // optional os restriction, e.g. ["linux"], ["macos/arm64"]
}

// Declaration is the parsed [tools] section of mise.toml.
type Declaration struct {
	Tools []Tool
}
