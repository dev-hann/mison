package env

import "testing"

func TestDiffAllInstalled(t *testing.T) {
	declared := []Tool{{Name: "node", Version: "22"}, {Name: "go", Version: "1.25"}}
	installed := []Tool{{Name: "node", Version: "22"}, {Name: "go", Version: "1.25"}}

	got := Diff(declared, installed)
	if len(got) != 2 || got[0].State != StateOK || got[1].State != StateOK {
		t.Fatalf("Diff() = %+v, want all OK", got)
	}
}

func TestDiffMissing(t *testing.T) {
	declared := []Tool{{Name: "node", Version: "22"}, {Name: "opencode", Version: "latest"}}
	installed := []Tool{{Name: "node", Version: "22"}}

	got := Diff(declared, installed)
	if len(got) != 2 || got[1].State != StateMissing {
		t.Fatalf("Diff() = %+v, want opencode missing", got)
	}
}

func TestDiffPrefixMatch(t *testing.T) {
	declared := []Tool{{Name: "node", Version: "22"}}
	installed := []Tool{{Name: "node", Version: "22.11.0"}}

	got := Diff(declared, installed)
	if got[0].State != StateOK {
		t.Fatalf("Diff() = %+v, want OK for prefix match", got)
	}
}

func TestDiffMismatch(t *testing.T) {
	declared := []Tool{{Name: "node", Version: "24"}}
	installed := []Tool{{Name: "node", Version: "22.11.0"}}

	got := Diff(declared, installed)
	if got[0].State != StateMismatch {
		t.Fatalf("Diff() = %+v, want Mismatch", got)
	}
	if got[0].Installed != "22.11.0" {
		t.Fatalf("Installed = %q, want 22.11.0", got[0].Installed)
	}
}

func TestDiffLatestNeverStringMismatch(t *testing.T) {
	// "latest" cannot be compared mechanically; staleness detection
	// belongs to mise. Any installed version reads as OK here.
	declared := []Tool{{Name: "neovim", Version: "latest"}}
	installed := []Tool{{Name: "neovim", Version: "0.9.5"}}

	got := Diff(declared, installed)
	if got[0].State != StateOK {
		t.Fatalf("Diff() = %+v, want OK for latest", got)
	}
}

func TestDiffEmptyDeclared(t *testing.T) {
	got := Diff(nil, []Tool{{Name: "node", Version: "22"}})
	if len(got) != 0 {
		t.Fatalf("Diff() = %+v, want empty", got)
	}
}
