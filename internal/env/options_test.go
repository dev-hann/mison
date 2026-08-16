package env

import (
	"strings"
	"testing"
)

func TestSetToolPreservesExistingOptions(t *testing.T) {
	src := `[tools]
node = { version = "22", postinstall = "corepack enable", os = ["linux"] }
`
	c := parseOrDie(t, src)

	// version bump must keep postinstall and os
	c.SetTool(Tool{Name: "node", Version: "24", OS: []string{"linux"}})

	out := string(c.mustBytes(t))
	if !strings.Contains(out, `postinstall = "corepack enable"`) {
		t.Fatalf("postinstall lost on version change:\n%s", out)
	}
	if !strings.Contains(out, `os = ["linux"]`) {
		t.Fatalf("os lost on version change:\n%s", out)
	}

	re := parseOrDie(t, out)
	tools := re.Tools()
	if len(tools) != 1 || tools[0].Version != "24" {
		t.Fatalf("round-trip tools = %+v", tools)
	}
}

func TestSetToolAddsOSWhileKeepingOptions(t *testing.T) {
	src := `[tools]
node = { version = "22", postinstall = "corepack enable" }
`
	c := parseOrDie(t, src)

	// adding an os restriction must not drop postinstall
	c.SetTool(Tool{Name: "node", Version: "22", OS: []string{"linux"}})

	out := string(c.mustBytes(t))
	if !strings.Contains(out, "postinstall") {
		t.Fatalf("postinstall lost when adding os:\n%s", out)
	}
	if !strings.Contains(out, `os = ["linux"]`) {
		t.Fatalf("os missing:\n%s", out)
	}
}

func TestSetToolRemovesOnlyOS(t *testing.T) {
	src := `[tools]
node = { version = "22", postinstall = "corepack enable", os = ["linux"] }
`
	c := parseOrDie(t, src)

	// no OS in the incoming tool → drop os, keep everything else
	c.SetTool(Tool{Name: "node", Version: "22"})

	out := string(c.mustBytes(t))
	if strings.Contains(out, "os =") {
		t.Fatalf("os should be removed:\n%s", out)
	}
	if !strings.Contains(out, "postinstall") {
		t.Fatalf("postinstall lost when removing os:\n%s", out)
	}
}

func TestSetToolPlainToStringKeepsSimplicity(t *testing.T) {
	src := "[tools]\nnode = \"22\"\n"
	c := parseOrDie(t, src)

	// plain → plain stays a bare string, no table inflation
	c.SetTool(Tool{Name: "node", Version: "24"})

	out := string(c.mustBytes(t))
	if !strings.Contains(out, `node = "24"`) || strings.Contains(out, "[tools.node]") {
		t.Fatalf("plain tool should stay a string:\n%s", out)
	}
}
