package env

import "strings"

// State is the comparison result between declared and installed tools.
type State int

// Tool comparison states.
const (
	StateOK State = iota
	StateMissing
	StateMismatch
)

// ToolStatus is the diff result for one declared tool.
type ToolStatus struct {
	Tool      Tool
	Installed string // actual installed version, empty when missing
	State     State
}

// Diff compares declared tools against installed ones.
//
// Rules:
//   - not installed               → StateMissing
//   - exact or prefix version hit → StateOK ("22" matches "22.11.0")
//   - "latest"                    → StateOK (staleness is mise's domain)
//   - otherwise                   → StateMismatch
//
// Installed tools missing from the declaration are not reported here;
// orphan detection happens in the sync pipeline with OS context.
func Diff(declared []Tool, installed []Tool) []ToolStatus {
	installedByName := make(map[string]string, len(installed))
	for _, t := range installed {
		installedByName[t.Name] = t.Version
	}

	out := make([]ToolStatus, 0, len(declared))
	for _, d := range declared {
		st := ToolStatus{Tool: d}
		v, ok := installedByName[d.Name]
		switch {
		case !ok:
			st.State = StateMissing
		case versionsMatch(d.Version, v):
			st.State = StateOK
			st.Installed = v
		default:
			st.State = StateMismatch
			st.Installed = v
		}
		out = append(out, st)
	}
	return out
}

func versionsMatch(declared, installed string) bool {
	if declared == "latest" {
		return true
	}
	return installed == declared || strings.HasPrefix(installed, declared+".")
}
