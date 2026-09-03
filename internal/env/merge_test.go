package env

import "testing"

func mergeNames(ts []Tool) []string {
	names := make([]string, len(ts))
	for i, t := range ts {
		names[i] = t.Name
	}
	return names
}

func findTool(t *testing.T, ts []Tool, name string) Tool {
	t.Helper()
	for _, x := range ts {
		if x.Name == name {
			return x
		}
	}
	t.Fatalf("tool %q not found in %+v", name, ts)
	return Tool{}
}

func TestMergeUnionDifferentTools(t *testing.T) {
	base := []Tool{}
	local := []Tool{{Name: "node", Version: "22"}}
	remote := []Tool{{Name: "ripgrep", Version: "latest"}}

	merged, conflicts := Merge(base, local, remote)
	if len(conflicts) != 0 {
		t.Fatalf("conflicts = %+v, want none", conflicts)
	}
	if len(merged) != 2 {
		t.Fatalf("merged = %v, want union of 2", mergeNames(merged))
	}
}

func TestMergeNoChanges(t *testing.T) {
	base := []Tool{{Name: "node", Version: "22"}}
	merged, conflicts := Merge(base, base, base)
	if len(conflicts) != 0 || len(merged) != 1 || merged[0].Version != "22" {
		t.Fatalf("Merge() = %+v, conflicts %+v", merged, conflicts)
	}
}

func TestMergeOneSideVersionChange(t *testing.T) {
	base := []Tool{{Name: "node", Version: "20"}}
	local := []Tool{{Name: "node", Version: "22"}}
	remote := []Tool{{Name: "node", Version: "20"}}

	merged, conflicts := Merge(base, local, remote)
	if len(conflicts) != 0 {
		t.Fatalf("conflicts = %+v, want none", conflicts)
	}
	if got := findTool(t, merged, "node").Version; got != "22" {
		t.Fatalf("node version = %q, want 22 (local change)", got)
	}
}

func TestMergeRemoteVersionChange(t *testing.T) {
	base := []Tool{{Name: "node", Version: "20"}}
	local := []Tool{{Name: "node", Version: "20"}}
	remote := []Tool{{Name: "node", Version: "24"}}

	merged, conflicts := Merge(base, local, remote)
	if len(conflicts) != 0 {
		t.Fatalf("conflicts = %+v, want none", conflicts)
	}
	if got := findTool(t, merged, "node").Version; got != "24" {
		t.Fatalf("node version = %q, want 24 (remote change)", got)
	}
}

func TestMergeRemoteAddition(t *testing.T) {
	base := []Tool{{Name: "node", Version: "22"}}
	local := []Tool{{Name: "node", Version: "22"}}
	remote := []Tool{{Name: "node", Version: "22"}, {Name: "go", Version: "1.25"}}

	merged, conflicts := Merge(base, local, remote)
	if len(conflicts) != 0 {
		t.Fatalf("conflicts = %+v, want none", conflicts)
	}
	if len(merged) != 2 {
		t.Fatalf("merged = %v, want node+go", mergeNames(merged))
	}
}

func TestMergeConflictBothChanged(t *testing.T) {
	base := []Tool{{Name: "node", Version: "20"}}
	local := []Tool{{Name: "node", Version: "24"}}
	remote := []Tool{{Name: "node", Version: "22"}}

	merged, conflicts := Merge(base, local, remote)
	if len(conflicts) != 1 {
		t.Fatalf("conflicts = %+v, want 1", conflicts)
	}
	c := conflicts[0]
	if c.Name != "node" || c.Base.Version != "20" || c.Local.Version != "24" || c.Remote.Version != "22" {
		t.Fatalf("conflict = %+v", c)
	}
	// conflicted tool is excluded from merged; caller resolves then sets
	if len(merged) != 0 {
		t.Fatalf("merged = %v, want empty until resolved", mergeNames(merged))
	}
}

func TestMergeLocalRemoval(t *testing.T) {
	base := []Tool{{Name: "node", Version: "22"}, {Name: "go", Version: "1.25"}}
	local := []Tool{{Name: "go", Version: "1.25"}} // node uninstalled locally
	remote := base

	merged, conflicts := Merge(base, local, remote)
	if len(conflicts) != 0 {
		t.Fatalf("conflicts = %+v, want none", conflicts)
	}
	if len(merged) != 1 || merged[0].Name != "go" {
		t.Fatalf("merged = %v, want only go (node removed)", mergeNames(merged))
	}
}

func TestMergeRemovalVsChangeConflict(t *testing.T) {
	base := []Tool{{Name: "node", Version: "20"}}
	local := []Tool{}                               // removed locally
	remote := []Tool{{Name: "node", Version: "22"}} // changed remotely

	merged, conflicts := Merge(base, local, remote)
	if len(conflicts) != 1 || conflicts[0].Name != "node" {
		t.Fatalf("conflicts = %+v, want node removal-vs-change", conflicts)
	}
	if len(merged) != 0 {
		t.Fatalf("merged = %v, want empty", mergeNames(merged))
	}
}

func TestMergeOSEntryChange(t *testing.T) {
	base := []Tool{{Name: "docker", Version: "latest", OS: []string{"linux"}}}
	local := []Tool{{Name: "docker", Version: "latest", OS: []string{"linux", "macos"}}}
	remote := base

	merged, conflicts := Merge(base, local, remote)
	if len(conflicts) != 0 {
		t.Fatalf("conflicts = %+v, want none", conflicts)
	}
	got := findTool(t, merged, "docker")
	if len(got.OS) != 2 || got.OS[1] != "macos" {
		t.Fatalf("docker OS = %v, want [linux macos]", got.OS)
	}
}

func TestMergeOSConflict(t *testing.T) {
	base := []Tool{{Name: "docker", Version: "latest", OS: []string{"linux"}}}
	local := []Tool{{Name: "docker", Version: "latest", OS: []string{"linux", "macos"}}}
	remote := []Tool{{Name: "docker", Version: "latest", OS: []string{"linux/x64"}}}

	_, conflicts := Merge(base, local, remote)
	if len(conflicts) != 1 || conflicts[0].Name != "docker" {
		t.Fatalf("conflicts = %+v, want docker", conflicts)
	}
}

func TestMergeOptionOnlyRemoteEditTaken(t *testing.T) {
	base := []Tool{{Name: "node", Version: "22"}}
	local := []Tool{{Name: "node", Version: "22"}}
	remote := []Tool{{Name: "node", Version: "22", Options: map[string]any{"postinstall": "echo hi"}}}
	merged, conflicts := Merge(base, local, remote)
	if len(conflicts) != 0 {
		t.Fatalf("unexpected conflicts: %+v", conflicts)
	}
	if len(merged) != 1 || merged[0].Options["postinstall"] != "echo hi" {
		t.Fatalf("remote option edit must be taken, got %+v", merged)
	}
}

func TestMergeOptionBothChangedDifferentlyConflicts(t *testing.T) {
	base := []Tool{{Name: "node", Version: "22"}}
	local := []Tool{{Name: "node", Version: "22", Options: map[string]any{"postinstall": "a"}}}
	remote := []Tool{{Name: "node", Version: "22", Options: map[string]any{"postinstall": "b"}}}
	_, conflicts := Merge(base, local, remote)
	if len(conflicts) != 1 || conflicts[0].Name != "node" {
		t.Fatalf("option-only divergence must conflict, got %+v", conflicts)
	}
}
