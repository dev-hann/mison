package env

import (
	"fmt"
	"strings"
)

// ParseToolSpec splits "name[@version]" into name and version,
// defaulting the version to "latest".
func ParseToolSpec(spec string) (name, version string, err error) {
	if spec == "" {
		return "", "", fmt.Errorf("empty tool spec")
	}
	parts := strings.SplitN(spec, "@", 2)
	name = parts[0]
	if name == "" {
		return "", "", fmt.Errorf("invalid tool spec %q: empty name", spec)
	}
	if len(parts) == 1 {
		return name, "latest", nil
	}
	version = parts[1]
	if version == "" || strings.Contains(version, "@") {
		return "", "", fmt.Errorf("invalid tool spec %q: empty version", spec)
	}
	return name, version, nil
}

// AppliesTo reports whether the tool should install on goos/goarch,
// following mise's os field semantics. No restriction means all platforms.
func (t Tool) AppliesTo(goos, goarch string) bool {
	if len(t.OS) == 0 {
		return true
	}
	for _, entry := range t.OS {
		osName, arch, hasArch := strings.Cut(entry, "/")
		if osName != "macos" && osName != "linux" && osName != "windows" {
			continue
		}
		if osName == "macos" && goos != "darwin" {
			continue
		}
		if osName == "linux" && goos != "linux" {
			continue
		}
		if !hasArch {
			return true
		}
		if arch == "x64" && goarch == "amd64" {
			return true
		}
		if arch == "arm64" && goarch == "arm64" {
			return true
		}
	}
	return false
}
