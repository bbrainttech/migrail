package cli

import (
	"bytes"
	"runtime"
	"runtime/debug"
	"testing"
)

func TestResolveBuildInfo(t *testing.T) {
	t.Parallel()

	platform := runtime.GOOS + "/" + runtime.GOARCH

	tests := []struct {
		name      string
		linked    linkedValues
		debugInfo *debug.BuildInfo
		want      buildInfo
	}{
		{
			name:   "release build uses linked values",
			linked: linkedValues{Version: "0.1.0", Commit: "9fa5811", Date: "2026-09-17T10:00:00Z"},
			debugInfo: debugBuildInfo("(devel)", map[string]string{
				"vcs.revision": "ffffffffffffffffffffffffffffffffffffffff",
				"vcs.time":     "2020-01-01T00:00:00Z",
			}),
			want: buildInfo{
				Version:   "0.1.0",
				Commit:    "9fa5811",
				Date:      "2026-09-17T10:00:00Z",
				GoVersion: "go1.26.4",
				Platform:  platform,
			},
		},
		{
			name:      "go install uses module version",
			debugInfo: debugBuildInfo("v0.1.0", nil),
			want: buildInfo{
				Version:   "v0.1.0",
				GoVersion: "go1.26.4",
				Platform:  platform,
			},
		},
		{
			name: "local build falls back to vcs stamp",
			debugInfo: debugBuildInfo("(devel)", map[string]string{
				"vcs.revision": "9fa58110f2b3c4d5e6f708192a3b4c5d6e7f8091",
				"vcs.time":     "2026-09-17T10:00:00Z",
				"vcs.modified": "true",
			}),
			want: buildInfo{
				Version:   "dev",
				Commit:    "9fa58110f2b3-dirty",
				Date:      "2026-09-17T10:00:00Z",
				GoVersion: "go1.26.4",
				Platform:  platform,
			},
		},
		{
			name: "local pseudo-version is reported as dev",
			debugInfo: debugBuildInfo("v0.0.0-20260917002814-9fa5811116f1", map[string]string{
				"vcs.revision": "9fa5811116f1aaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			}),
			want: buildInfo{
				Version:   "dev",
				Commit:    "9fa5811116f1",
				GoVersion: "go1.26.4",
				Platform:  platform,
			},
		},
		{
			name: "missing build info",
			want: buildInfo{
				Version:   "dev",
				GoVersion: runtime.Version(),
				Platform:  platform,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := resolveBuildInfo(tt.linked, tt.debugInfo); got != tt.want {
				t.Errorf("resolveBuildInfo() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestBuildInfoWrite(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		info buildInfo
		want string
	}{
		{
			name: "all fields",
			info: buildInfo{
				Version:   "0.1.0",
				Commit:    "9fa5811",
				Date:      "2026-09-17T10:00:00Z",
				GoVersion: "go1.26.4",
				Platform:  "darwin/arm64",
			},
			want: "migrail 0.1.0\n" +
				"  commit 9fa5811\n" +
				"  date   2026-09-17T10:00:00Z\n" +
				"  go     go1.26.4 darwin/arm64\n",
		},
		{
			name: "unknown commit and date are omitted",
			info: buildInfo{Version: "dev", GoVersion: "go1.26.4", Platform: "linux/amd64"},
			want: "migrail dev\n" +
				"  go     go1.26.4 linux/amd64\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer
			if err := tt.info.write(&out); err != nil {
				t.Fatalf("write() error = %v", err)
			}

			if out.String() != tt.want {
				t.Errorf("write() output:\n%s\nwant:\n%s", out.String(), tt.want)
			}
		})
	}
}

func debugBuildInfo(mainVersion string, settings map[string]string) *debug.BuildInfo {
	info := &debug.BuildInfo{
		GoVersion: "go1.26.4",
		Main:      debug.Module{Path: "github.com/bbrainttech/migrail", Version: mainVersion},
	}

	for key, value := range settings {
		info.Settings = append(info.Settings, debug.BuildSetting{Key: key, Value: value})
	}

	return info
}
