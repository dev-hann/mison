package usecase

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dev-hann/mison/internal/env"
)

const testLock = `lockfile_version = 1

[[tools.btop]]
version = "1.0"

[tools.btop."platforms.linux-x64"]
url = "https://x"

[tools.btop."platforms.linux-arm64"]
url = "https://x"

[[tools.delta]]
version = "0.19"

[tools.delta."platforms.macos-arm64"]
url = "https://x"

[tools.delta."platforms.linux-x64"]
url = "https://x"
`

func writeLock(t *testing.T, f *Flows, content string) {
	t.Helper()
	if err := os.WriteFile(f.layout().MiseLock, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPlatformScopeSplitsByLock(t *testing.T) {
	f, _, _ := newTestFlows(t)
	if _, err := f.layout().Ensure(); err != nil {
		t.Fatal(err)
	}
	writeLock(t, f, testLock)

	declared := []env.Tool{
		{Name: "delta", Version: "latest"},    // has macos-arm64 -> in scope
		{Name: "btop", Version: "latest"},     // linux-only per lock -> skipped
		{Name: "unlocked", Version: "latest"}, // absent from lock -> fallback in scope
	}
	inScope, skipped := f.platformScope(declared)
	if len(inScope) != 2 || inScope[0].Name != "delta" || inScope[1].Name != "unlocked" {
		t.Fatalf("inScope = %+v", inScope)
	}
	if len(skipped) != 1 || skipped[0].Tool.Name != "btop" || skipped[0].Result != SkippedPlatform {
		t.Fatalf("skipped = %+v", skipped)
	}
	if !strings.Contains(skipped[0].Detail, "linux-x64") {
		t.Fatalf("skip detail must list supported platforms: %q", skipped[0].Detail)
	}
}

func TestPlatformScopeFallbackWithoutLock(t *testing.T) {
	f, _, _ := newTestFlows(t)
	if _, err := f.layout().Ensure(); err != nil {
		t.Fatal(err)
	}

	declared := []env.Tool{{Name: "delta", Version: "latest"}}
	inScope, skipped := f.platformScope(declared)
	if len(inScope) != 1 || len(skipped) != 0 {
		t.Fatalf("no lock → everything in scope, got %+v / %+v", inScope, skipped)
	}
}

func TestPlatformScopeHonorsManualOSField(t *testing.T) {
	f, _, _ := newTestFlows(t)
	if _, err := f.layout().Ensure(); err != nil {
		t.Fatal(err)
	}
	declared := []env.Tool{{Name: "docker", Version: "latest", OS: []string{"linux"}}}

	inScope, skipped := f.platformScope(declared)
	if len(inScope) != 0 || len(skipped) != 1 || skipped[0].Result != SkippedPlatform {
		t.Fatalf("manual os field must skip on darwin: %+v / %+v", inScope, skipped)
	}
}

func TestAttemptToolOutcomes(t *testing.T) {
	f, fm, _ := newTestFlows(t)
	fm.execFailArg = "bogus"

	ok := f.attemptTool(env.Tool{Name: "delta", Version: "latest"})
	if ok.Result != Applied {
		t.Fatalf("delta = %+v, want Applied", ok)
	}
	bad := f.attemptTool(env.Tool{Name: "bogus", Version: "latest"})
	if bad.Result != Failed || !strings.Contains(bad.Detail, "bogus") {
		t.Fatalf("bogus = %+v, want Failed with detail", bad)
	}
}

func TestAttemptSpecUsesExplicitVersion(t *testing.T) {
	f, fm, _ := newTestFlows(t)

	out := f.attemptSpec("btop", "1.0.0")
	if out.Result != Applied {
		t.Fatalf("attemptSpec = %+v", out)
	}
	if len(fm.execCalls) != 1 || fm.execCalls[0] != "install btop@1.0.0" {
		t.Fatalf("execCalls = %v, want [install btop@1.0.0]", fm.execCalls)
	}
}

func TestReportOutcomesRendersAllClasses(t *testing.T) {
	f, _, out := newTestFlows(t)
	outcomes := []ToolOutcome{
		{Tool: env.Tool{Name: "delta", Version: "latest"}, Result: Applied},
		{Tool: env.Tool{Name: "btop", Version: "latest"}, Result: SkippedPlatform, Detail: "linux-x64, linux-arm64"},
		{Tool: env.Tool{Name: "bogus", Version: "latest"}, Result: Failed, Detail: errors.New("not found").Error()},
	}

	failed := f.reportOutcomes(outcomes)
	if failed != 1 {
		t.Fatalf("failed = %d, want 1", failed)
	}
	got := out.String()
	for _, want := range []string{"✓ delta", "btop", "not for this platform", "✗ bogus", "not found"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func TestMachinePlatformKey(t *testing.T) {
	cases := map[[2]string]string{
		{"darwin", "arm64"}: "macos-arm64",
		{"darwin", "amd64"}: "macos-x64",
		{"linux", "amd64"}:  "linux-x64",
		{"linux", "arm64"}:  "linux-arm64",
	}
	for in, want := range cases {
		if got := machinePlatformKey(in[0], in[1]); got != want {
			t.Errorf("machinePlatformKey(%v) = %q, want %q", in, got, want)
		}
	}
}

var _ = filepath.Join // keep import when fixtures change shape
