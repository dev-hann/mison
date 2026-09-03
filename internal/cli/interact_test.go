package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dev-hann/mison/internal/env"
)

func conflict() env.Conflict {
	return env.Conflict{
		Name:   "node",
		Local:  env.Tool{Name: "node", Version: "22"},
		Remote: env.Tool{Name: "node", Version: "20"},
	}
}

func TestResolveConflictShowsAbortOption(t *testing.T) {
	var out bytes.Buffer
	ui := NewTermUI(&out, strings.NewReader("1\n"))

	tool, err := ui.ResolveConflict(conflict())
	if err != nil {
		t.Fatalf("ResolveConflict() error = %v", err)
	}
	if tool.Version != "22" {
		t.Fatalf("input 1 must keep local, got %q", tool.Version)
	}
	if !strings.Contains(out.String(), "[3] abort") {
		t.Fatalf("prompt must show the abort option:\n%s", out.String())
	}
}

func TestResolveConflictPicksRemote(t *testing.T) {
	var out bytes.Buffer
	ui := NewTermUI(&out, strings.NewReader("2\n"))

	tool, err := ui.ResolveConflict(conflict())
	if err != nil {
		t.Fatalf("ResolveConflict() error = %v", err)
	}
	if tool.Version != "20" {
		t.Fatalf("input 2 must accept remote, got %q", tool.Version)
	}
}

func TestResolveConflictAbort(t *testing.T) {
	for _, input := range []string{"3\n", "\n", "x\n"} {
		var out bytes.Buffer
		ui := NewTermUI(&out, strings.NewReader(input))

		_, err := ui.ResolveConflict(conflict())
		if err == nil || !strings.Contains(err.Error(), "unpushed") {
			t.Fatalf("input %q must abort with unpushed notice, got: %v", strings.TrimSpace(input), err)
		}
	}
}
