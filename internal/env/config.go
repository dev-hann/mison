package env

import (
	"bytes"
	"fmt"
	"sort"

	toml "github.com/BurntSushi/toml"
)

// Config is a parsed mise.toml file. It keeps the whole document so
// unknown sections survive modification. Use Parse to create one.
type Config struct {
	data map[string]any
}

// Parse decodes mise.toml content. It validates that every [tools]
// entry is well-formed (string version or table with version/os keys).
func Parse(data []byte) (*Config, error) {
	raw := map[string]any{}
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse mise.toml: %w", err)
	}
	c := &Config{data: raw}
	if _, err := c.tools(); err != nil {
		return nil, err
	}
	return c, nil
}

// Tools returns the [tools] table sorted by name.
func (c *Config) Tools() []Tool {
	table, _ := c.tools() // validated at Parse time
	names := make([]string, 0, len(table))
	for name := range table {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]Tool, 0, len(names))
	for _, name := range names {
		t, err := toTool(name, table[name])
		if err != nil {
			continue // unreachable: validated at Parse time
		}
		out = append(out, t)
	}
	return out
}

func (c *Config) tools() (map[string]any, error) {
	v, ok := c.data["tools"]
	if !ok || v == nil {
		return map[string]any{}, nil
	}
	table, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("parse mise.toml: [tools] is not a table")
	}
	for name, entry := range table {
		if _, err := toTool(name, entry); err != nil {
			return nil, err
		}
	}
	return table, nil
}

func toTool(name string, v any) (Tool, error) {
	switch t := v.(type) {
	case string:
		return Tool{Name: name, Version: t}, nil
	case map[string]any:
		version, _ := t["version"].(string)
		if version == "" {
			return Tool{}, fmt.Errorf("parse mise.toml: tool %q: missing version", name)
		}
		return Tool{Name: name, Version: version, OS: toStrings(t["os"])}, nil
	default:
		return Tool{}, fmt.Errorf("parse mise.toml: tool %q: invalid entry type %T", name, v)
	}
}

// SetTool adds or updates a tool in the [tools] table.
func (c *Config) SetTool(t Tool) {
	table := c.toolsTable()
	if len(t.OS) == 0 {
		table[t.Name] = t.Version
		return
	}
	table[t.Name] = map[string]any{
		"version": t.Version,
		"os":      toAny(t.OS),
	}
}

// RemoveTool deletes a tool. It reports whether the tool was present.
func (c *Config) RemoveTool(name string) bool {
	table := c.toolsTable()
	if _, ok := table[name]; !ok {
		return false
	}
	delete(table, name)
	return true
}

// Bytes re-encodes the whole document, preserving unknown sections.
// Comment and formatting loss is accepted in V1.
func (c *Config) Bytes() ([]byte, error) {
	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf)
	if err := enc.Encode(c.data); err != nil {
		return nil, fmt.Errorf("encode mise.toml: %w", err)
	}
	return buf.Bytes(), nil
}

func (c *Config) toolsTable() map[string]any {
	v, ok := c.data["tools"]
	if !ok {
		table := map[string]any{}
		c.data["tools"] = table
		return table
	}
	table, _ := v.(map[string]any)
	return table
}

func toAny(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

func toStrings(v any) []string {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		s, ok := item.(string)
		if !ok {
			return nil
		}
		out = append(out, s)
	}
	return out
}
