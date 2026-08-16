package env

import "fmt"

// SchemaVersion is the declaration format version this mison writes
// and understands. Bump ONLY on incompatible changes to how mison
// interprets mise.toml: newer mison must keep reading older schemas
// (backward duty), older mison must reject newer ones (forward guard).
const SchemaVersion = 1

// StampSchema records the current schema version inside mise's
// official inert key [_.mison] — a section mise never parses. Missing
// or equal versions pass; nothing is removed.
func (c *Config) StampSchema() error {
	inert, ok := c.data["_"].(map[string]any)
	if !ok {
		if existing, exists := c.data["_"]; exists {
			return fmt.Errorf("[_] is present but not a table (type %T)", existing)
		}
		inert = map[string]any{}
		c.data["_"] = inert
	}

	mison, ok := inert["mison"].(map[string]any)
	if !ok {
		if existing, exists := inert["mison"]; exists && existing != nil {
			return fmt.Errorf("[_.mison] is present but not a table (type %T)", existing)
		}
		mison = map[string]any{}
		inert["mison"] = mison
	}
	mison["schema"] = SchemaVersion
	return nil
}

// checkSchema rejects declarations written for a newer mison. A missing
// field means schema 1 (pre-guard files are legacy-compatible).
func checkSchema(data map[string]any) error {
	inert, ok := data["_"].(map[string]any)
	if !ok {
		return nil
	}
	mison, ok := inert["mison"].(map[string]any)
	if !ok {
		return nil
	}
	v, ok := mison["schema"]
	if !ok {
		return nil
	}
	schema, ok := v.(int64)
	if !ok {
		// tolerate alternate numeric encodings from other TOML writers
		if f, isF := v.(float64); isF {
			schema = int64(f)
			ok = true
		}
	}
	if !ok {
		return nil // unparsable → treat as unknown/legacy, don't block
	}
	if schema > SchemaVersion {
		return fmt.Errorf(
			"declaration uses schema %d, this mison supports %d — upgrade mison",
			schema, SchemaVersion)
	}
	return nil
}
