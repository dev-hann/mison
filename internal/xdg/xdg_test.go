package xdg

import "testing"

func TestConfigDirEnv(t *testing.T) {
	cases := map[struct{ home, xdg string }]string{
		{"/home/u", ""}:         "/home/u/.config",
		{"/home/u", "/xdg/cfg"}: "/xdg/cfg",
	}
	for in, want := range cases {
		if got := ConfigDirEnv(in.home, in.xdg); got != want {
			t.Errorf("ConfigDirEnv(%q, %q) = %q, want %q", in.home, in.xdg, got, want)
		}
	}
}

func TestDataDirEnv(t *testing.T) {
	cases := map[struct{ home, xdg string }]string{
		{"/home/u", ""}:          "/home/u/.local/share",
		{"/home/u", "/xdg/data"}: "/xdg/data",
	}
	for in, want := range cases {
		if got := DataDirEnv(in.home, in.xdg); got != want {
			t.Errorf("DataDirEnv(%q, %q) = %q, want %q", in.home, in.xdg, got, want)
		}
	}
}

func TestMiseShimsAndBin(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	if got := MiseShims("/home/u"); got != "/home/u/.local/share/mise/shims" {
		t.Errorf("MiseShims() = %q", got)
	}
	if got := MiseBin("/home/u"); got != "/home/u/.local/bin" {
		t.Errorf("MiseBin() = %q", got)
	}
}
