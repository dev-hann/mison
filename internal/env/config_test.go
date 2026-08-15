package env

import (
	"reflect"
	"testing"
)

func TestParseSimpleTools(t *testing.T) {
	src := `[tools]
node = "22"
python = "3.13"
`
	c, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	tools := c.Tools()
	if len(tools) != 2 {
		t.Fatalf("Tools() length = %d, want 2", len(tools))
	}
	want0 := Tool{Name: "node", Version: "22"}
	if !reflect.DeepEqual(tools[0], want0) {
		t.Errorf("tools[0] = %+v, want %+v", tools[0], want0)
	}
	want1 := Tool{Name: "python", Version: "3.13"}
	if !reflect.DeepEqual(tools[1], want1) {
		t.Errorf("tools[1] = %+v, want %+v", tools[1], want1)
	}
}

func TestParseToolWithOptions(t *testing.T) {
	src := `[tools]
node = "22"
docker = { version = "latest", os = ["linux"] }
hk = { version = "latest", os = ["macos/arm64"] }
`
	c, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	tools := c.Tools()
	if len(tools) != 3 {
		t.Fatalf("Tools() length = %d, want 3", len(tools))
	}
	// sorted by name: docker, hk, node
	if tools[0].Name != "docker" || tools[0].Version != "latest" {
		t.Errorf("docker = %+v", tools[0])
	}
	if len(tools[0].OS) != 1 || tools[0].OS[0] != "linux" {
		t.Errorf("docker OS = %v, want [linux]", tools[0].OS)
	}
	if len(tools[1].OS) != 1 || tools[1].OS[0] != "macos/arm64" {
		t.Errorf("hk OS = %v, want [macos/arm64]", tools[1].OS)
	}
	if tools[2].Name != "node" {
		t.Errorf("node = %+v", tools[2])
	}
}

func TestParseEmptyFile(t *testing.T) {
	cases := map[string]string{
		"empty":       "",
		"no tools":    "[env]\nFOO = \"bar\"\n",
		"empty tools": "[tools]\n",
	}
	for name, src := range cases {
		c, err := Parse([]byte(src))
		if err != nil {
			t.Fatalf("%s: Parse() error = %v", name, err)
		}
		if got := c.Tools(); len(got) != 0 {
			t.Errorf("%s: Tools() length = %d, want 0", name, len(got))
		}
	}
}

func TestParseIgnoresOtherSections(t *testing.T) {
	src := `[tools]
node = "22"

[env]
FOO = "bar"

[tasks.dev]
run = "echo hi"

[settings]
jobs = 4
`
	c, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	tools := c.Tools()
	if len(tools) != 1 || tools[0].Name != "node" {
		t.Fatalf("Tools() = %+v, want only node", tools)
	}
}

func TestParseInvalidTOML(t *testing.T) {
	if _, err := Parse([]byte(`[tools\nnode =`)); err == nil {
		t.Fatal("Parse() expected error for invalid TOML")
	}
}

func TestParseInvalidToolValue(t *testing.T) {
	src := `[tools]
node = 22
`
	if _, err := Parse([]byte(src)); err == nil {
		t.Fatal("Parse() expected error for non-string version")
	}
}
