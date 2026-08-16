package usecase

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dev-hann/mison/internal/env"
	"github.com/dev-hann/mison/internal/repo/gitrepo"
	"github.com/dev-hann/mison/internal/repo/miserepo"
	"github.com/dev-hann/mison/internal/service"
)

// keepLocal / acceptRemote are simple resolvers for tests; a removal
// (empty tool on that side) wins over the other side's edit.
func keepLocal(cs []env.Conflict) ([]env.Tool, error) {
	var out []env.Tool
	for _, c := range cs {
		if t := pickSide(c.Local, c.Remote); t.Name != "" {
			out = append(out, t)
		}
	}
	return out, nil
}

func acceptRemote(cs []env.Conflict) ([]env.Tool, error) {
	var out []env.Tool
	for _, c := range cs {
		if t := pickSide(c.Remote, c.Local); t.Name != "" {
			out = append(out, t)
		}
	}
	return out, nil
}

// newEngineAt builds a real-gitsvc engine over dir.
func newEngineAt(dir string) *Engine {
	return NewEngine(gitrepo.New(service.New(), dir))
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
func newClone(t *testing.T, remote, name string) *Engine {
	t.Helper()
	parent := t.TempDir()
	git(t, parent, "clone", remote, name)
	dir := filepath.Join(parent, name)
	git(t, dir, "config", "user.email", "test@mison")
	git(t, dir, "config", "user.name", "mison-test")
	return newEngineAt(dir)
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

func writeToml(t *testing.T, e *Engine, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(e.Dir(), "mise.toml"), []byte(content), 0o644); err != nil {
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

func TestPlanSyncTable(t *testing.T) {
	cases := []struct {
		name             string
		head, remote, mb string
		hasRemote        bool
		want             SyncAction
	}{
		{"no remote", "a", "", "", false, ActionSeedRemote},
		{"equal", "a", "a", "a", true, ActionPush},
		{"ahead", "b", "a", "a", true, ActionPush},
		{"behind", "a", "b", "a", true, ActionFastForward},
		{"diverged", "a", "b", "c", true, ActionMerge},
		{"unrelated histories", "a", "b", "", true, ActionMerge},
	}
	for _, c := range cases {
		if got := PlanSync(c.head, c.remote, c.mb, c.hasRemote).Action; got != c.want {
			t.Errorf("%s: PlanSync() = %v, want %v", c.name, got, c.want)
		}
	}
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
	localData, _ := os.ReadFile(filepath.Join(b.Dir(), "mise.toml"))
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

	localData, _ := os.ReadFile(filepath.Join(b.Dir(), "mise.toml"))
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
	git(t, b.Dir(), "add", "-A")
	git(t, b.Dir(), "commit", "-m", "install: ripgrep")

	if _, err := b.SmartPull(keepLocal); err != nil {
		t.Fatalf("SmartPull() error = %v", err)
	}

	// both sides' tools survive locally and on remote
	localData, _ := os.ReadFile(filepath.Join(b.Dir(), "mise.toml"))
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

	plain := newEngineAt(t.TempDir())
	if plain.IsRepo() {
		t.Fatal("IsRepo() = true for plain dir")
	}
}

// newCloneBare clones without git identity (simulates a fresh machine
// with no global git config).
func newCloneBare(t *testing.T, remote, name string) *Engine {
	t.Helper()
	parent := t.TempDir()
	git(t, parent, "clone", remote, name)
	return newEngineAt(filepath.Join(parent, name))
}

func TestConnectFreshDir(t *testing.T) {
	remote := newBareRemote(t)
	a := newClone(t, remote, "a")
	writeToml(t, a, "[tools]\nnode = \"22\"\n")
	if _, err := a.SmartPush("install: node", keepLocal); err != nil {
		t.Fatal(err)
	}

	// a directory that already holds mise.toml but no git repo
	dir := filepath.Join(t.TempDir(), "env")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	g := newEngineAt(dir)

	if err := g.Connect(remote); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	if !g.IsRepo() {
		t.Fatal("IsRepo() = false after Connect")
	}
	data, err := os.ReadFile(filepath.Join(dir, "mise.toml"))
	if err != nil || !strings.Contains(string(data), `node = "22"`) {
		t.Fatalf("declaration not checked out: %v %q", err, data)
	}
}

func TestSmartPushWithoutGitIdentity(t *testing.T) {
	remote := newBareRemote(t)
	g := newCloneBare(t, remote, "b")

	writeToml(t, g, "[tools]\nripgrep = \"latest\"\n")
	if _, err := g.SmartPush("install: ripgrep", keepLocal); err != nil {
		t.Fatalf("SmartPush() without identity error = %v", err)
	}
	got := readRemoteToml(t, remote)
	if !strings.Contains(got, "ripgrep") {
		t.Fatalf("remote missing ripgrep:\n%s", got)
	}
}

func TestSyncStatusUpToDate(t *testing.T) {
	remote := newBareRemote(t)
	g := newClone(t, remote, "a")
	writeToml(t, g, "[tools]\nnode = \"22\"\n")
	if _, err := g.SmartPush("install: node", keepLocal); err != nil {
		t.Fatal(err)
	}

	info, err := g.SyncStatus()
	if err != nil {
		t.Fatalf("SyncStatus() error = %v", err)
	}
	if info.State != SyncUpToDate {
		t.Errorf("State = %v, want SyncUpToDate", info.State)
	}
}

func TestSyncStatusBehind(t *testing.T) {
	remote := newBareRemote(t)
	a := newClone(t, remote, "a")
	b := newClone(t, remote, "b")

	writeToml(t, a, "[tools]\nnode = \"22\"\ngo = \"1.25\"\n")
	if _, err := a.SmartPush("install: node go", keepLocal); err != nil {
		t.Fatal(err)
	}

	info, err := b.SyncStatus()
	if err != nil {
		t.Fatalf("SyncStatus() error = %v", err)
	}
	if info.State != SyncBehind {
		t.Errorf("State = %v, want SyncBehind", info.State)
	}
	if len(info.RemoteAdded) != 2 {
		t.Errorf("RemoteAdded = %v, want [go node]", info.RemoteAdded)
	}
}

func TestSyncStatusAhead(t *testing.T) {
	remote := newBareRemote(t)
	g := newClone(t, remote, "a")
	writeToml(t, g, "[tools]\nnode = \"22\"\n")
	git(t, g.Dir(), "add", "-A")
	git(t, g.Dir(), "commit", "-m", "offline edit")

	info, err := g.SyncStatus()
	if err != nil {
		t.Fatalf("SyncStatus() error = %v", err)
	}
	if info.State != SyncAhead {
		t.Errorf("State = %v, want SyncAhead", info.State)
	}
}

func TestSyncStatusDiverged(t *testing.T) {
	remote := newBareRemote(t)
	a := newClone(t, remote, "a")
	b := newClone(t, remote, "b")

	writeToml(t, a, "[tools]\nnode = \"22\"\n")
	if _, err := a.SmartPush("install: node", keepLocal); err != nil {
		t.Fatal(err)
	}

	writeToml(t, b, "[tools]\nripgrep = \"latest\"\n")
	git(t, b.Dir(), "add", "-A")
	git(t, b.Dir(), "commit", "-m", "local edit")

	info, err := b.SyncStatus()
	if err != nil {
		t.Fatalf("SyncStatus() error = %v", err)
	}
	if info.State != SyncDiverged {
		t.Errorf("State = %v, want SyncDiverged", info.State)
	}
}

func TestSmartPullPreservesManualEdits(t *testing.T) {
	remote := newBareRemote(t)
	a := newClone(t, remote, "a")
	b := newClone(t, remote, "b")

	// A pushes a new tool
	writeToml(t, a, "[tools]\nnode = \"22\"\n")
	if _, err := a.SmartPush("install: node", keepLocal); err != nil {
		t.Fatal(err)
	}

	// B edits mise.toml by hand (uncommitted) — DESIGN.md Case F
	writeToml(t, b, "[tools]\njq = \"latest\"\n")

	if _, err := b.SmartPull(keepLocal); err != nil {
		t.Fatalf("SmartPull() error = %v", err)
	}

	// both B's manual edit and A's remote tool must survive
	localData, err := os.ReadFile(filepath.Join(b.Dir(), "mise.toml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"jq", "node"} {
		if !strings.Contains(string(localData), want) {
			t.Errorf("B local missing %s after pull with manual edit:\n%s", want, localData)
		}
	}
	// and the merged result must be pushed
	remoteToml := readRemoteToml(t, remote)
	if !strings.Contains(remoteToml, "jq") {
		t.Errorf("remote missing manual edit:\n%s", remoteToml)
	}
}

func TestOwnedToolsFiltersBySourceAndActivity(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	ours := "/home/u/.config/mise/config.toml"
	foreign := "/home/hann/.config/mise/config.toml"
	entries := []miserepo.Entry{
		{Name: "node", Version: "22.23.2", Active: true, Source: ours},
		{Name: "go", Version: "1.25.1", Active: true, Source: ours},
		{Name: "node", Version: "20.1.0", Active: false, Source: ours}, // inactive
		{Name: "docker", Version: "1.0.0", Active: true, Source: foreign},
	}

	got := OwnedTools(entries, "/home/u")
	if len(got) != 2 || got[0].Name != "go" || got[1].Name != "node" {
		t.Fatalf("OwnedTools() = %+v, want [go node]", got)
	}
}

func TestOwnedToolsMatchesDeclarationPathDirectly(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	entries := []miserepo.Entry{
		{Name: "node", Version: "22.23.2", Active: true, Source: "/home/u/.mison/env/mise.toml"},
	}

	got := OwnedTools(entries, "/home/u")
	if len(got) != 1 || got[0].Name != "node" {
		t.Fatalf("OwnedTools() = %+v, want node via direct path", got)
	}
}

func TestSmartPullRejectsFutureSchemaWithoutTouchingWorktree(t *testing.T) {
	remote := newBareRemote(t)
	a := newClone(t, remote, "a")
	b := newClone(t, remote, "b")

	// A writes a schema-2 declaration (as a future mison would)
	writeToml(t, a, "[_.mison]\nschema = 2\n\n[tools]\nnode = \"22\"\n")
	git(t, a.Dir(), "add", "-A")
	git(t, a.Dir(), "commit", "-m", "future schema")
	git(t, a.Dir(), "push", "origin", "HEAD:main")

	// B has local content that must remain untouched
	writeToml(t, b, "[tools]\njq = \"latest\"\n")

	_, err := b.SmartPull(keepLocal)
	if err == nil || !strings.Contains(err.Error(), "upgrade mison") {
		t.Fatalf("SmartPull() error = %v, want schema guard error", err)
	}

	// the worktree must be unchanged — no reset, no data loss
	localData, _ := os.ReadFile(filepath.Join(b.Dir(), "mise.toml"))
	if !strings.Contains(string(localData), "jq") {
		t.Fatalf("local declaration must be untouched by guard:\n%s", localData)
	}
	if strings.Contains(string(localData), `node = "22"`) {
		t.Fatalf("future-schema remote must not be checked out:\n%s", localData)
	}
}

func TestSmartPushRefusesToPushOntoFutureSchemaRemote(t *testing.T) {
	remote := newBareRemote(t)
	a := newClone(t, remote, "a")
	b := newClone(t, remote, "b")

	writeToml(t, a, "[_.mison]\nschema = 2\n\n[tools]\nnode = \"22\"\n")
	git(t, a.Dir(), "add", "-A")
	git(t, a.Dir(), "commit", "-m", "future schema")
	git(t, a.Dir(), "push", "origin", "HEAD:main")

	writeToml(t, b, "[tools]\nripgrep = \"latest\"\n")
	if _, err := b.SmartPush("install: ripgrep", keepLocal); err == nil {
		t.Fatal("SmartPush() must refuse when the remote uses a future schema")
	} else if !strings.Contains(err.Error(), "upgrade mison") {
		t.Fatalf("error = %v, want schema guard error", err)
	}

	// the remote keeps A's content only
	got := readRemoteToml(t, remote)
	if strings.Contains(got, "ripgrep") {
		t.Fatalf("push must not have landed:\n%s", got)
	}
}

func TestSaveConfigStampsSchemaInFlows(t *testing.T) {
	f, _, _ := newTestFlows(t)
	if err := f.RunInstall([]string{"node"}, "", PolicyAsk); err != nil {
		t.Fatal(err)
	}
	toml := readToml(t, f)
	if !strings.Contains(toml, "[_.mison]") || !strings.Contains(toml, "schema = 1") {
		t.Fatalf("flows must stamp the schema on save:\n%s", toml)
	}
}
