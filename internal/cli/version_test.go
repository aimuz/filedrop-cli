package cli

import (
	"runtime/debug"
	"testing"
)

func TestBuildFromInfo(t *testing.T) {
	original := Version
	t.Cleanup(func() { Version = original })
	Version = "dev"

	local := []debug.BuildSetting{{Key: "vcs.revision", Value: "36eee353973d"}}
	dirty := append(local, debug.BuildSetting{Key: "vcs.modified", Value: "true"})
	cases := []struct {
		name         string
		info         *debug.BuildInfo
		wantVersion  string
		wantRevision string
	}{
		{"module install", &debug.BuildInfo{Main: debug.Module{Version: "v1.2.3"}}, "v1.2.3", ""},
		{"local source build", &debug.BuildInfo{Main: debug.Module{Version: "v0.0.0-20260101000000-36eee353973d"}, Settings: local}, "dev", "36eee35"},
		{"local dirty build", &debug.BuildInfo{Main: debug.Module{Version: "v0.0.0-20260101000000-36eee353973d"}, Settings: dirty}, "dev", "36eee35-dirty"},
		{"devel", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, "dev", ""},
		{"empty", &debug.BuildInfo{}, "dev", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			version, revision := buildFromInfo(tc.info)
			if version != tc.wantVersion || revision != tc.wantRevision {
				t.Fatalf("got (%q, %q), want (%q, %q)", version, revision, tc.wantVersion, tc.wantRevision)
			}
		})
	}
}

func TestResolvedBuildPrefersInjected(t *testing.T) {
	original := Version
	t.Cleanup(func() { Version = original })
	Version = "v9.9.9"
	version, revision := resolvedBuild()
	if version != "v9.9.9" || revision != "" {
		t.Fatalf("got (%q, %q)", version, revision)
	}
}

func TestVersionLine(t *testing.T) {
	if got := versionLine("v1.2.3", ""); got != "filedrop v1.2.3" {
		t.Fatalf("got %q", got)
	}
	if got := versionLine("dev", "36eee35-dirty"); got != "filedrop dev (36eee35-dirty)" {
		t.Fatalf("got %q", got)
	}
}
