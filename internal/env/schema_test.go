package env

import (
	"strings"
	"testing"
)

func TestParseRejectsFutureSchema(t *testing.T) {
	src := `[_.mison]
schema = 2

[tools]
node = "22"
`
	_, err := Parse([]byte(src))
	if err == nil {
		t.Fatal("Parse() expected error for future schema")
	}
	if !strings.Contains(err.Error(), "upgrade mison") {
		t.Fatalf("error should tell the user to upgrade: %v", err)
	}
}

func TestParseAcceptsCurrentAndMissingSchema(t *testing.T) {
	cases := map[string]string{
		"no schema field": `[tools]
node = "22"
`,
		"schema 1": `[_.mison]
schema = 1

[tools]
node = "22"
`,
	}
	for name, src := range cases {
		c, err := Parse([]byte(src))
		if err != nil {
			t.Fatalf("%s: Parse() error = %v", name, err)
		}
		if got := len(c.Tools()); got != 1 {
			t.Fatalf("%s: tools = %d", name, got)
		}
	}
}

func TestStampSchemaWritesInertKey(t *testing.T) {
	c := parseOrDie(t, "[tools]\nnode = \"22\"\n")

	if err := c.StampSchema(); err != nil {
		t.Fatalf("StampSchema() error = %v", err)
	}

	out := string(c.mustBytes(t))
	if !strings.Contains(out, "[_.mison]") || !strings.Contains(out, "schema = 1") {
		t.Fatalf("schema stamp missing:\n%s", out)
	}

	// stamping must survive round-trips and not disturb tools
	re := parseOrDie(t, out)
	if err := re.StampSchema(); err != nil {
		t.Fatalf("re-stamp error = %v", err)
	}
	if len(re.Tools()) != 1 {
		t.Fatalf("tools disturbed: %+v", re.Tools())
	}
}

func TestStampSchemaPreservesOtherInertData(t *testing.T) {
	src := `[_.mison]
schema = 1
note = "user data"

[tools]
node = "22"
`
	c := parseOrDie(t, src)
	if err := c.StampSchema(); err != nil {
		t.Fatal(err)
	}

	out := string(c.mustBytes(t))
	if !strings.Contains(out, `note = "user data"`) {
		t.Fatalf("sibling inert keys must survive stamping:\n%s", out)
	}
}
