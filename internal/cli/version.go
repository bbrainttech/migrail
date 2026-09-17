package cli

import (
	"fmt"
	"io"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"
)

var (
	version = ""
	commit  = ""
	date    = ""
)

const (
	versionCommandName  = "version"
	shortCommitLength   = 12
	developmentVersion  = "dev"
	unreleasedVersion   = "(devel)"
	pseudoVersionPrefix = "v0.0.0-"
)

type buildInfo struct {
	Version   string
	Commit    string
	Date      string
	GoVersion string
	Platform  string
}

type linkedValues struct {
	Version string
	Commit  string
	Date    string
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   versionCommandName,
		Short: "Print the migrail version and build details",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			debugInfo, _ := debug.ReadBuildInfo()
			info := resolveBuildInfo(linkedValues{Version: version, Commit: commit, Date: date}, debugInfo)

			if err := info.write(cmd.OutOrStdout()); err != nil {
				return internalError(fmt.Errorf("write version: %w", err))
			}

			return nil
		},
	}
}

func resolveBuildInfo(linked linkedValues, debugInfo *debug.BuildInfo) buildInfo {
	info := buildInfo{
		Version:   linked.Version,
		Commit:    linked.Commit,
		Date:      linked.Date,
		GoVersion: runtime.Version(),
		Platform:  runtime.GOOS + "/" + runtime.GOARCH,
	}

	if debugInfo == nil {
		return info.withDefaults()
	}

	info.GoVersion = debugInfo.GoVersion

	if info.Version == "" && isReleaseModuleVersion(debugInfo.Main.Version) {
		info.Version = debugInfo.Main.Version
	}

	settings := vcsSettings(debugInfo)
	if info.Commit == "" {
		info.Commit = shortCommit(settings["vcs.revision"], settings["vcs.modified"] == "true")
	}

	if info.Date == "" {
		info.Date = settings["vcs.time"]
	}

	return info.withDefaults()
}

func isReleaseModuleVersion(moduleVersion string) bool {
	isLocal := moduleVersion == "" || moduleVersion == unreleasedVersion
	isPseudo := strings.HasPrefix(moduleVersion, pseudoVersionPrefix) || strings.HasSuffix(moduleVersion, "+dirty")

	return !isLocal && !isPseudo
}

func (b buildInfo) withDefaults() buildInfo {
	if b.Version == "" {
		b.Version = developmentVersion
	}

	return b
}

func vcsSettings(debugInfo *debug.BuildInfo) map[string]string {
	settings := make(map[string]string, len(debugInfo.Settings))
	for _, setting := range debugInfo.Settings {
		settings[setting.Key] = setting.Value
	}

	return settings
}

func shortCommit(revision string, modified bool) string {
	if revision == "" {
		return ""
	}

	if len(revision) > shortCommitLength {
		revision = revision[:shortCommitLength]
	}

	if modified {
		revision += "-dirty"
	}

	return revision
}

func (b buildInfo) write(w io.Writer) error {
	lines := []struct {
		label string
		value string
	}{
		{label: "commit", value: b.Commit},
		{label: "date", value: b.Date},
		{label: "go", value: b.GoVersion + " " + b.Platform},
	}

	if _, err := fmt.Fprintf(w, "migrail %s\n", b.Version); err != nil {
		return err
	}

	for _, line := range lines {
		if line.value == "" {
			continue
		}

		if _, err := fmt.Fprintf(w, "  %-7s%s\n", line.label, line.value); err != nil {
			return err
		}
	}

	return nil
}
