package detector

import (
	"errors"
	"testing"
)

func TestDetectReturnsValidOSArch(t *testing.T) {
	info := Detect()
	validOS := map[string]bool{"darwin": true, "linux": true, "windows": true}
	validArch := map[string]bool{"arm64": true, "amd64": true, "386": true}

	if !validOS[info.OS] {
		t.Errorf("OS = %q, want a known GOOS", info.OS)
	}
	if !validArch[info.Arch] {
		t.Errorf("Arch = %q, want a known GOARCH", info.Arch)
	}
}

func TestIsMiseInstalledFound(t *testing.T) {
	got := IsMiseInstalled(func(name string) (string, error) {
		if name != "mise" {
			t.Errorf("lookup name = %q, want mise", name)
		}
		return "/home/u/.local/bin/mise", nil
	})
	if !got {
		t.Fatal("IsMiseInstalled() = false, want true")
	}
}

func TestIsMiseInstalledNotFound(t *testing.T) {
	got := IsMiseInstalled(func(string) (string, error) {
		return "", errors.New("exec: not found")
	})
	if got {
		t.Fatal("IsMiseInstalled() = true, want false")
	}
}

func TestMiseBinaryPathUsesHome(t *testing.T) {
	got := MiseBinaryPath("/home/u")
	want := "/home/u/.local/bin/mise"
	if got != want {
		t.Fatalf("MiseBinaryPath() = %q, want %q", got, want)
	}
}

func TestShimPathUsesHome(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	got := ShimPath("/home/u")
	want := "/home/u/.local/share/mise/shims"
	if got != want {
		t.Fatalf("ShimPath() = %q, want %q", got, want)
	}
}

func TestShimPathRespectsEnvOverride(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/xdg/data")
	if got := ShimPath("/home/u"); got != "/xdg/data/mise/shims" {
		t.Fatalf("ShimPath() = %q, want /xdg/data/mise/shims", got)
	}
	t.Setenv("XDG_DATA_HOME", "")
	if got := ShimPath("/home/u"); got != "/home/u/.local/share/mise/shims" {
		t.Fatalf("ShimPath() fallback = %q", got)
	}
}
