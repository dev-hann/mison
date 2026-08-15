package env

import "sort"

// Conflict describes a tool that local and remote changed differently.
// Zero Tool values mean the tool is absent (removed) on that side.
type Conflict struct {
	Name   string
	Base   Tool
	Local  Tool
	Remote Tool
}

// Merge performs a semantic 3-way merge of the [tools] table.
//
// For each tool, comparing base→local and base→remote:
//   - untouched            → keep base state
//   - one side changed     → take that side (addition, removal, or edit)
//   - both changed equally → take it once
//   - both changed apart   → Conflict, tool excluded from merged
//
// The caller resolves conflicts (user prompt or --ours/--theirs) and
// applies the winner via Config.SetTool / RemoveTool. This function is
// pure: it never touches git or files.
func Merge(base, local, remote []Tool) (merged []Tool, conflicts []Conflict) {
	bm := toToolMap(base)
	lm := toToolMap(local)
	rm := toToolMap(remote)

	names := map[string]struct{}{}
	for n := range bm {
		names[n] = struct{}{}
	}
	for n := range lm {
		names[n] = struct{}{}
	}
	for n := range rm {
		names[n] = struct{}{}
	}
	sorted := make([]string, 0, len(names))
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)

	for _, name := range sorted {
		b, bOK := bm[name]
		l, lOK := lm[name]
		r, rOK := rm[name]

		localChanged := entryChanged(b, bOK, l, lOK)
		remoteChanged := entryChanged(b, bOK, r, rOK)

		switch {
		case !localChanged && !remoteChanged:
			if bOK {
				merged = append(merged, b)
			}
		case localChanged && !remoteChanged:
			if lOK {
				merged = append(merged, l)
			}
		case !localChanged && remoteChanged:
			if rOK {
				merged = append(merged, r)
			}
		default: // both changed
			if lOK == rOK && (!lOK || toolEqual(l, r)) {
				if lOK {
					merged = append(merged, l)
				}
			} else {
				conflicts = append(conflicts, Conflict{Name: name, Base: b, Local: l, Remote: r})
			}
		}
	}
	return merged, conflicts
}

func entryChanged(b Tool, bOK bool, t Tool, tOK bool) bool {
	if bOK != tOK {
		return true // addition or removal
	}
	if !bOK {
		return false // absent on both sides
	}
	return !toolEqual(b, t)
}

func toolEqual(a, b Tool) bool {
	if a.Version != b.Version || len(a.OS) != len(b.OS) {
		return false
	}
	for i := range a.OS {
		if a.OS[i] != b.OS[i] {
			return false
		}
	}
	return true
}

func toToolMap(tools []Tool) map[string]Tool {
	m := make(map[string]Tool, len(tools))
	for _, t := range tools {
		m[t.Name] = t
	}
	return m
}
