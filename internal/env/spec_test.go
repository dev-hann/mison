package env

import "testing"

func TestParseToolSpecNameOnly(t *testing.T) {
	name, version, err := ParseToolSpec("node")
	if err != nil {
		t.Fatalf("ParseToolSpec() error = %v", err)
	}
	if name != "node" || version != "latest" {
		t.Fatalf("got (%q, %q), want (node, latest)", name, version)
	}
}

func TestParseToolSpecNameAtVersion(t *testing.T) {
	name, version, err := ParseToolSpec("node@22")
	if err != nil {
		t.Fatalf("ParseToolSpec() error = %v", err)
	}
	if name != "node" || version != "22" {
		t.Fatalf("got (%q, %q), want (node, 22)", name, version)
	}
}

func TestParseToolSpecFullVersion(t *testing.T) {
	name, version, err := ParseToolSpec("python@3.13.1")
	if err != nil {
		t.Fatalf("ParseToolSpec() error = %v", err)
	}
	if name != "python" || version != "3.13.1" {
		t.Fatalf("got (%q, %q), want (python, 3.13.1)", name, version)
	}
}

func TestParseToolSpecEmptyVersion(t *testing.T) {
	if _, _, err := ParseToolSpec("node@"); err == nil {
		t.Fatal("ParseToolSpec() expected error for empty version")
	}
}

func TestParseToolSpecEmptyName(t *testing.T) {
	for _, bad := range []string{"", "@22", "node@@22"} {
		if _, _, err := ParseToolSpec(bad); err == nil {
			t.Fatalf("ParseToolSpec(%q) expected error", bad)
		}
	}
}

func TestAppliesTo(t *testing.T) {
	cases := []struct {
		os     []string
		goos   string
		goarch string
		want   bool
	}{
		{nil, "linux", "amd64", true},
		{[]string{"linux"}, "linux", "amd64", true},
		{[]string{"linux"}, "darwin", "arm64", false},
		{[]string{"macos"}, "darwin", "arm64", true},
		{[]string{"macos"}, "linux", "amd64", false},
		{[]string{"macos/arm64"}, "darwin", "arm64", true},
		{[]string{"macos/arm64"}, "darwin", "amd64", false},
		{[]string{"linux/x64"}, "linux", "amd64", true},
		{[]string{"linux/x64"}, "linux", "arm64", false},
		{[]string{"x64"}, "linux", "amd64", false}, // bare arch is not a valid spec
	}
	for _, c := range cases {
		tool := Tool{Name: "t", Version: "1", OS: c.os}
		if got := tool.AppliesTo(c.goos, c.goarch); got != c.want {
			t.Errorf("OS=%v on %s/%s: AppliesTo() = %v, want %v", c.os, c.goos, c.goarch, got, c.want)
		}
	}
}
