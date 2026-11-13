package cli

import (
	"runtime/debug"
	"testing"
)

func TestVersionFromBuildInfo(t *testing.T) {
	original := Version
	t.Cleanup(func() { Version = original })
	Version = "dev"

	local := []debug.BuildSetting{{Key: "vcs.revision", Value: "abc123"}}
	cases := []struct {
		name string
		info *debug.BuildInfo
		want string
	}{
		{"module install", &debug.BuildInfo{Main: debug.Module{Version: "v1.2.3"}}, "v1.2.3"},
		{"local source build", &debug.BuildInfo{Main: debug.Module{Version: "v0.0.0-20260101000000-abc123"}, Settings: local}, "dev"},
		{"devel", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, "dev"},
		{"empty", &debug.BuildInfo{}, "dev"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := versionFromBuildInfo(tc.info); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestResolvedVersionPrefersInjected(t *testing.T) {
	original := Version
	t.Cleanup(func() { Version = original })
	Version = "v9.9.9"
	if got := resolvedVersion(); got != "v9.9.9" {
		t.Fatalf("got %q", got)
	}
}
