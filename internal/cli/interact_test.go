package cli

import (
	"strings"
	"testing"

	"github.com/dev-hann/mison/internal/env"
	"github.com/dev-hann/mison/internal/mise"
)

// fakeReport records one-way notifications; fakeAsk records blocking
// questions. Tests assert which interaction-port calls a business flow
// made — never how they were rendered.
type fakeReport struct{ calls []string }

func (f *fakeReport) Step(msg string)   { f.calls = append(f.calls, "step:"+msg) }
func (f *fakeReport) Synced(msg string) { f.calls = append(f.calls, "synced:"+msg) }
func (f *fakeReport) Warn(msg string)   { f.calls = append(f.calls, "warn:"+msg) }
func (f *fakeReport) Fail(msg string)   { f.calls = append(f.calls, "fail:"+msg) }
func (f *fakeReport) Line(msg string)   { f.calls = append(f.calls, "line:"+msg) }
func (f *fakeReport) ToolLine(mark, name, detail string) {
	f.calls = append(f.calls, "tool:"+mark+":"+name+":"+detail)
}

func (f *fakeReport) has(prefix string) bool {
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

type fakeAsk struct {
	confirm   bool
	confirmQs []string
	conflicts []string
	tool      env.Tool
}

func (f *fakeAsk) Confirm(q string) bool {
	f.confirmQs = append(f.confirmQs, q)
	return f.confirm
}
func (f *fakeAsk) ResolveConflict(c env.Conflict) (env.Tool, error) {
	f.conflicts = append(f.conflicts, c.Name)
	return f.tool, nil
}

func wireFakes(app *App) (*fakeReport, *fakeAsk) {
	rep := &fakeReport{}
	ask := &fakeAsk{}
	app.UI = rep
	app.Ask = ask
	return rep, ask
}

func TestUninstallFlowAsksConfirmationBeforeRemoving(t *testing.T) {
	repo := &fakeRepo{isRepo: true}
	app, fm, _ := newTestAppWith(t, repo)
	rep, ask := wireFakes(app)
	ask.confirm = true

	if err := app.RunInstall([]string{"node"}, "", PolicyAsk); err != nil {
		t.Fatal(err)
	}
	fm.installed = []mise.Tool{{Name: "node", Version: "22.1.0"}}
	repo.pushes = nil
	rep.calls = nil

	if err := app.RunUninstall([]string{"node"}, false, PolicyAsk); err != nil {
		t.Fatalf("RunUninstall() error = %v", err)
	}
	if len(ask.confirmQs) != 1 || !strings.Contains(ask.confirmQs[0], "Remove node") {
		t.Errorf("questions = %v, want one 'Remove node...'", ask.confirmQs)
	}
	if !rep.has("step:Removing node") {
		t.Errorf("calls = %v, want removal step after confirmation", rep.calls)
	}
	if len(repo.pushes) != 1 {
		t.Errorf("pushes = %v, want auto-push after confirmed removal", repo.pushes)
	}
}

func TestUninstallFlowDeclinedStopsBeforeAnyChange(t *testing.T) {
	app, _, _ := newTestApp(t)
	rep, ask := wireFakes(app)
	ask.confirm = false

	if err := app.RunInstall([]string{"node"}, "", PolicyAsk); err != nil {
		t.Fatal(err)
	}
	rep.calls = nil

	if err := app.RunUninstall([]string{"node"}, false, PolicyAsk); err != nil {
		t.Fatalf("RunUninstall() error = %v", err)
	}
	if len(ask.confirmQs) == 0 {
		t.Fatal("no confirmation asked")
	}
	if rep.has("step:Removing") {
		t.Errorf("declined confirmation must stop the flow, calls = %v", rep.calls)
	}
}

func TestSyncFlowNotifiesRemoteMergeViaSyncedPort(t *testing.T) {
	repo := &fakeRepo{isRepo: true, mergedOn: []string{"node"}}
	app, _, _ := newTestAppWith(t, repo)
	rep, _ := wireFakes(app)

	if err := app.RunInstall([]string{"rg"}, "", PolicyAsk); err != nil {
		t.Fatal(err)
	}
	rep.calls = nil

	if err := app.RunSync(false, PolicyAsk); err != nil {
		t.Fatalf("RunSync() error = %v", err)
	}
	if !rep.has("synced:New changes: node") {
		t.Errorf("remote merge must surface through Reporter.Synced, calls = %v", rep.calls)
	}
}

func TestConflictResolutionRoutesThroughPrompter(t *testing.T) {
	repo := &fakeRepo{isRepo: true, conflict: &env.Conflict{
		Name: "node", Base: envTool("node", "20"),
		Local: envTool("node", "24"), Remote: envTool("node", "22"),
	}}
	app, _, _ := newTestAppWith(t, repo)
	_, ask := wireFakes(app)
	ask.tool = envTool("node", "24")

	if err := app.RunInstall([]string{"rg"}, "", PolicyAsk); err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	if len(ask.conflicts) != 1 || ask.conflicts[0] != "node" {
		t.Errorf("conflicts = %v, want [node] routed through Prompter", ask.conflicts)
	}
	if len(repo.pushes) != 1 {
		t.Errorf("pushes = %v, want push after resolution", repo.pushes)
	}
}
