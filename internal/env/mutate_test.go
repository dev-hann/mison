package env

import (
	"strings"
	"testing"
)

func parseOrDie(t *testing.T, src string) *Config {
	t.Helper()
	c, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	return c
}

func TestSetToolNew(t *testing.T) {
	c := parseOrDie(t, "[tools]\nnode = \"22\"\n")
	c.SetTool(Tool{Name: "go", Version: "1.25"})

	re := parseOrDie(t, string(c.mustBytes(t)))
	tools := re.Tools()
	if len(tools) != 2 || tools[0].Name != "go" || tools[0].Version != "1.25" {
		t.Fatalf("round-trip tools = %+v", tools)
	}
}

func TestSetToolUpdatesVersion(t *testing.T) {
	c := parseOrDie(t, "[tools]\nnode = \"22\"\n")
	c.SetTool(Tool{Name: "node", Version: "24"})

	re := parseOrDie(t, string(c.mustBytes(t)))
	tools := re.Tools()
	if len(tools) != 1 || tools[0].Version != "24" {
		t.Fatalf("round-trip tools = %+v", tools[0])
	}
}

func TestSetToolWithOS(t *testing.T) {
	c := parseOrDie(t, "")
	c.SetTool(Tool{Name: "docker", Version: "latest", OS: []string{"linux"}})

	out := string(c.mustBytes(t))
	if !strings.Contains(out, `os = ["linux"]`) {
		t.Fatalf("Bytes() missing os field:\n%s", out)
	}

	re := parseOrDie(t, out)
	tools := re.Tools()
	if len(tools) != 1 || tools[0].OS[0] != "linux" {
		t.Fatalf("round-trip tools = %+v", tools)
	}
}

func TestSetToolPlainHasNoOSKey(t *testing.T) {
	c := parseOrDie(t, "")
	c.SetTool(Tool{Name: "node", Version: "22"})

	out := string(c.mustBytes(t))
	if strings.Contains(out, "os") {
		t.Fatalf("plain tool should not carry os key:\n%s", out)
	}
	if !strings.Contains(out, "node = \"22\"") {
		t.Fatalf("plain tool should serialize as string:\n%s", out)
	}
}

func TestRemoveTool(t *testing.T) {
	c := parseOrDie(t, "[tools]\nnode = \"22\"\ngo = \"1.25\"\n")
	if !c.RemoveTool("node") {
		t.Fatal("RemoveTool(node) = false, want true")
	}

	re := parseOrDie(t, string(c.mustBytes(t)))
	tools := re.Tools()
	if len(tools) != 1 || tools[0].Name != "go" {
		t.Fatalf("round-trip tools = %+v", tools)
	}
}

func TestRemoveToolAbsent(t *testing.T) {
	c := parseOrDie(t, "[tools]\nnode = \"22\"\n")
	if c.RemoveTool("python") {
		t.Fatal("RemoveTool(python) = true, want false")
	}
}

func TestBytesPreservesUnknownSections(t *testing.T) {
	src := `[tools]
node = "22"

[env]
FOO = "bar"

[tasks.dev]
run = "echo hi"
`
	c := parseOrDie(t, src)
	c.SetTool(Tool{Name: "go", Version: "1.25"})
	out := string(c.mustBytes(t))

	for _, want := range []string{"FOO", "bar", "run", "echo hi", "go"} {
		if !strings.Contains(out, want) {
			t.Errorf("Bytes() lost %q:\n%s", want, out)
		}
	}
}

func (c *Config) mustBytes(t *testing.T) []byte {
	t.Helper()
	b, err := c.Bytes()
	if err != nil {
		t.Fatalf("Bytes() error = %v", err)
	}
	return b
}
