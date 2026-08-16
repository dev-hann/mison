package gitclient

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dev-hann/mison/internal/env"
)

// keepLocal / acceptRemote are simple resolvers for tests.
func keepLocal(cs []env.Conflict) ([]env.Tool, error) {
	out := make([]env.Tool, len(cs))
	for i, c := range cs {
		if c.Local.Name == "" {
			continue // removal wins on local side
		}
		out[i] = c.Local
	}
	return out, nil
}

func acceptRemote(cs []env.Conflict) ([]env.Tool, error) {
	out := make([]env.Tool, len(cs))
	for i, c := range cs {
		if c.Remote.Name == "" {
			continue
		}
		out[i] = c.Remote
	}
	return out, nil
}

// newBareRemote creates a bare repo seeded with an initial commit
// (mirrors `mison init`, which pushes an empty declaration first).
func newBareRemote(t *testing.T) string {
	t.Helper()
	parent := t.TempDir()
	dir := filepath.Join(parent, "remote.git")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, parent, "init", "--bare", "-b", "main", dir)

	seed := filepath.Join(parent, "seed")
	git(t, parent, "clone", dir, "seed")
	git(t, seed, "config", "user.email", "test@mison")
	git(t, seed, "config", "user.name", "mison-test")
	if err := os.WriteFile(filepath.Join(seed, "mise.toml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, seed, "add", "-A")
	git(t, seed, "commit", "-m", "mison: init environment")
	git(t, seed, "push", "origin", "HEAD:main")
	return dir
}

// newClone clones the remote into a working dir with test identity.
func newClone(t *testing.T, remote, name string) *Git {
	t.Helper()
	parent := t.TempDir()
	git(t, parent, "clone", remote, name)
	dir := filepath.Join(parent, name)
	git(t, dir, "config", "user.email", "test@mison")
	git(t, dir, "config", "user.name", "mison-test")
	return New(dir)
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=mison-test", "GIT_AUTHOR_EMAIL=test@mison",
		"GIT_COMMITTER_NAME=mison-test", "GIT_COMMITTER_EMAIL=test@mison",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func writeToml(t *testing.T, g *Git, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(g.dir, "mise.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readRemoteToml(t *testing.T, remote string) string {
	t.Helper()
	parent := t.TempDir()
	peek := filepath.Join(parent, "peek")
	cmd := exec.Command("git", "clone", remote, peek)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("clone remote: %v\n%s", err, out)
	}
	data, err := os.ReadFile(filepath.Join(peek, "mise.toml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestSmartPushClean(t *testing.T) {
	remote := newBareRemote(t)
	g := newClone(t, remote, "a")

	writeToml(t, g, "[tools]\nnode = \"22\"\n")
	if _, err := g.SmartPush("install: node", keepLocal); err != nil {
		t.Fatalf("SmartPush() error = %v", err)
	}

	got := readRemoteToml(t, remote)
	if !strings.Contains(got, `node = "22"`) {
		t.Fatalf("remote mise.toml = %q", got)
	}
}

func TestSmartPushSkipsEmptyCommit(t *testing.T) {
	remote := newBareRemote(t)
	g := newClone(t, remote, "a")
	writeToml(t, g, "[tools]\nnode = \"22\"\n")
	if _, err := g.SmartPush("install: node", keepLocal); err != nil {
		t.Fatal(err)
	}

	// second push with no changes must not fail
	if _, err := g.SmartPush("install: nothing", keepLocal); err != nil {
		t.Fatalf("SmartPush() on clean tree error = %v", err)
	}
}

func TestSmartPushDivergedAutoMerge(t *testing.T) {
	remote := newBareRemote(t)
	a := newClone(t, remote, "a")
	b := newClone(t, remote, "b")

	// A installs node and pushes
	writeToml(t, a, "[tools]\nnode = \"22\"\n")
	if _, err := a.SmartPush("install: node", keepLocal); err != nil {
		t.Fatal(err)
	}

	// B (stale, no node) installs ripgrep and pushes — must auto-merge
	writeToml(t, b, "[tools]\nripgrep = \"latest\"\n")
	merged, err := b.SmartPush("install: ripgrep", keepLocal)
	if err != nil {
		t.Fatalf("SmartPush() error = %v", err)
	}
	if len(merged) != 1 || merged[0] != "node" {
		t.Errorf("merged notice = %v, want [node]", merged)
	}

	got := readRemoteToml(t, remote)
	for _, want := range []string{`node = "22"`, `ripgrep = "latest"`} {
		if !strings.Contains(got, want) {
			t.Errorf("remote missing %q:\n%s", want, got)
		}
	}

	// B's local declaration must match the merged result
	localData, _ := os.ReadFile(filepath.Join(b.dir, "mise.toml"))
	if !strings.Contains(string(localData), `node = "22"`) {
		t.Errorf("B local missing node after merge:\n%s", localData)
	}
}

func TestSmartPushConflictResolvedLocal(t *testing.T) {
	remote := newBareRemote(t)
	a := newClone(t, remote, "a")
	b := newClone(t, remote, "b")

	writeToml(t, a, "[tools]\nnode = \"20\"\n")
	if _, err := a.SmartPush("install: node 20", keepLocal); err != nil {
		t.Fatal(err)
	}

	// B, still at empty base, sets node=24 → conflict
	writeToml(t, b, "[tools]\nnode = \"24\"\n")
	if _, err := b.SmartPush("install: node 24", keepLocal); err != nil {
		t.Fatal(err)
	}

	got := readRemoteToml(t, remote)
	if !strings.Contains(got, `node = "24"`) {
		t.Fatalf("remote should keep local 24:\n%s", got)
	}
}

func TestSmartPushConflictResolvedRemote(t *testing.T) {
	remote := newBareRemote(t)
	a := newClone(t, remote, "a")
	b := newClone(t, remote, "b")

	writeToml(t, a, "[tools]\nnode = \"22\"\n")
	if _, err := a.SmartPush("install: node 22", keepLocal); err != nil {
		t.Fatal(err)
	}

	writeToml(t, b, "[tools]\nnode = \"24\"\n")
	if _, err := b.SmartPush("install: node 24", acceptRemote); err != nil {
		t.Fatal(err)
	}

	got := readRemoteToml(t, remote)
	if !strings.Contains(got, `node = "22"`) {
		t.Fatalf("remote should accept remote 22:\n%s", got)
	}
}

func TestSmartPullFastForward(t *testing.T) {
	remote := newBareRemote(t)
	a := newClone(t, remote, "a")
	b := newClone(t, remote, "b")

	writeToml(t, a, "[tools]\nnode = \"22\"\n")
	if _, err := a.SmartPush("install: node", keepLocal); err != nil {
		t.Fatal(err)
	}

	merged, err := b.SmartPull(keepLocal)
	if err != nil {
		t.Fatalf("SmartPull() error = %v", err)
	}
	if len(merged) != 1 || merged[0] != "node" {
		t.Errorf("merged notice = %v, want [node]", merged)
	}

	localData, _ := os.ReadFile(filepath.Join(b.dir, "mise.toml"))
	if !strings.Contains(string(localData), `node = "22"`) {
		t.Fatalf("B local after pull:\n%s", localData)
	}
}

func TestSmartPullUpToDate(t *testing.T) {
	remote := newBareRemote(t)
	g := newClone(t, remote, "a")
	writeToml(t, g, "[tools]\nnode = \"22\"\n")
	if _, err := g.SmartPush("install: node", keepLocal); err != nil {
		t.Fatal(err)
	}

	merged, err := g.SmartPull(keepLocal)
	if err != nil {
		t.Fatalf("SmartPull() error = %v", err)
	}
	if len(merged) != 0 {
		t.Errorf("merged notice = %v, want empty", merged)
	}
}

func TestSmartPullDivergedWithLocalPending(t *testing.T) {
	// local has an unpushed commit, remote moved too — pull merges
	// both ways and pushes the reconciliation (sync semantics)
	remote := newBareRemote(t)
	a := newClone(t, remote, "a")
	b := newClone(t, remote, "b")

	writeToml(t, a, "[tools]\nnode = \"22\"\n")
	if _, err := a.SmartPush("install: node", keepLocal); err != nil {
		t.Fatal(err)
	}

	writeToml(t, b, "[tools]\nripgrep = \"latest\"\n")
	git(t, b.dir, "add", "-A")
	git(t, b.dir, "commit", "-m", "install: ripgrep")

	if _, err := b.SmartPull(keepLocal); err != nil {
		t.Fatalf("SmartPull() error = %v", err)
	}

	// both sides' tools survive locally and on remote
	localData, _ := os.ReadFile(filepath.Join(b.dir, "mise.toml"))
	for _, want := range []string{"node", "ripgrep"} {
		if !strings.Contains(string(localData), want) {
			t.Errorf("B local missing %s:\n%s", want, localData)
		}
	}
	remoteToml := readRemoteToml(t, remote)
	if !strings.Contains(remoteToml, "ripgrep") {
		t.Errorf("remote missing ripgrep after reconciled pull:\n%s", remoteToml)
	}
}

func TestIsRepo(t *testing.T) {
	remote := newBareRemote(t)
	g := newClone(t, remote, "a")
	if !g.IsRepo() {
		t.Fatal("IsRepo() = false for clone")
	}

	plain := New(t.TempDir())
	if plain.IsRepo() {
		t.Fatal("IsRepo() = true for plain dir")
	}
}
