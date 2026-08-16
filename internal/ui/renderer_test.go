package ui

import (
	"bytes"
	"testing"
)

func TestRendererGoldenOutput(t *testing.T) {
	cases := []struct {
		name string
		draw func(r *Renderer)
		want string
	}{
		{"step", func(r *Renderer) { r.Step("Installing node") }, "✓ Installing node\n"},
		{"synced", func(r *Renderer) { r.Synced("Remote had new changes (node)") }, "↻ Remote had new changes (node)\n"},
		{"warn", func(r *Renderer) { r.Warn("kept") }, "⚠ kept\n"},
		{"fail", func(r *Renderer) { r.Fail("boom") }, "✗ boom\n"},
		{"line", func(r *Renderer) { r.Line("plain") }, "plain\n"},
		{"tool plain", func(r *Renderer) { r.ToolLine(MarkOK, "node", "") }, "✓ node\n"},
		{"tool detail", func(r *Renderer) { r.ToolLine(MarkFail, "node", "missing") }, "✗ node (missing)\n"},
	}
	for _, c := range cases {
		var buf bytes.Buffer
		c.draw(New(&buf))
		if buf.String() != c.want {
			t.Errorf("%s: got %q, want %q", c.name, buf.String(), c.want)
		}
	}
}
