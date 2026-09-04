package gitrepo

import (
	"fmt"
	"strings"
	"testing"
)

type fakeRunner struct {
	calls   []string
	results map[string]string
}

func (f *fakeRunner) Run(_ []string, name string, args ...string) (string, error) {
	key := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, key)
	if out, ok := f.results[key]; ok {
		return out, nil
	}
	return "", fmt.Errorf("unexpected command: %s", key)
}

func (f *fakeRunner) RunTTY(_ []string, _ string, _ ...string) error { return nil }

func TestLatestReleaseTag(t *testing.T) {
	fr := &fakeRunner{results: map[string]string{
		"sh -c curl -fsSL https://api.github.com/repos/dev-hann/mison/releases/latest": `{"tag_name": "v0.5.0", "name": "v0.5.0"}`,
	}}
	c := NewGitHub(fr, "/home/u")

	tag, err := c.LatestReleaseTag("dev-hann/mison")
	if err != nil {
		t.Fatalf("LatestReleaseTag() error = %v", err)
	}
	if tag != "v0.5.0" {
		t.Fatalf("tag = %q, want v0.5.0", tag)
	}
}

func TestRunMisonInstallerCommandShape(t *testing.T) {
	fr := &fakeRunner{results: map[string]string{
		"sh -c curl -fsSL https://raw.githubusercontent.com/dev-hann/mison/main/scripts/install.sh | sh": "mison: installed\n",
	}}
	c := NewGitHub(fr, "/home/u")

	if err := c.RunMisonInstaller(); err != nil {
		t.Fatalf("RunMisonInstaller() error = %v", err)
	}
	if len(fr.calls) != 1 || !strings.Contains(fr.calls[0], "install.sh") {
		t.Fatalf("calls = %v, want install.sh run", fr.calls)
	}
}
