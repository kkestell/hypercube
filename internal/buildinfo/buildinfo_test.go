package buildinfo

import (
	"runtime/debug"
	"testing"
)

func TestDescribe(t *testing.T) {
	for _, tt := range []struct {
		name, version, commit, want string
	}{
		{"checkout", "v0.0.0-20261104222841-96d0992419c4", "96d0992419c449689c4c4cd076a0950fd03810a2", "Hypercube v0.0.0-20261104222841-96d0992419c4 (96d0992419c449689c4c4cd076a0950fd03810a2)"},
		{"modified", "v0.1.0-beta.1+dirty", "abc123", "Hypercube v0.1.0-beta.1+dirty (abc123)"},
		{"release", "v0.1.0-beta.1", "abc123", "Hypercube v0.1.0-beta.1 (abc123)"},
		{"no git", "(devel)", "", "Hypercube devel"},
		{"no metadata", "", "", "Hypercube devel"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			info := &debug.BuildInfo{Main: debug.Module{Version: tt.version}}
			if tt.commit != "" {
				info.Settings = []debug.BuildSetting{{Key: "vcs.revision", Value: tt.commit}}
			}
			if got := describe(info); got != tt.want {
				t.Fatalf("identity = %q, want %q", got, tt.want)
			}
		})
	}
}
