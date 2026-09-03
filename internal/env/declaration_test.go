package env

import "testing"

func TestPathBackendDetection(t *testing.T) {
	local := Tool{Name: "mytool", Version: "1.0", Options: map[string]any{"path": "/Users/me/dev/mytool"}}
	if got := local.PathBackend(); got != "/Users/me/dev/mytool" {
		t.Fatalf("PathBackend() = %q, want the path option", got)
	}
	registry := Tool{Name: "npm:prettier", Version: "3"}
	if got := registry.PathBackend(); got != "" {
		t.Fatalf("registry-backed tools have no path backend, got %q", got)
	}
	bare := Tool{Name: "node", Version: "22"}
	if got := bare.PathBackend(); got != "" {
		t.Fatalf("bare entries have no path backend, got %q", got)
	}
}
