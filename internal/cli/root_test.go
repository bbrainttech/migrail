package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestExecuteExitCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{
			name:       "version succeeds",
			args:       []string{"version"},
			wantCode:   exitOK,
			wantStdout: "migrail ",
		},
		{
			name:       "unknown command is a usage error",
			args:       []string{"deploy"},
			wantCode:   exitUsage,
			wantStderr: "Run 'migrail --help' for usage.",
		},
		{
			name:       "unknown flag is a usage error",
			args:       []string{"version", "--bogus"},
			wantCode:   exitUsage,
			wantStderr: "unknown flag: --bogus",
		},
		{
			name:       "extra argument is a usage error",
			args:       []string{"version", "extra"},
			wantCode:   exitUsage,
			wantStderr: "migrail: ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer

			code := Execute(context.Background(), tt.args, &stdout, &stderr)

			if code != tt.wantCode {
				t.Fatalf("exit code = %d, want %d (stderr: %q)", code, tt.wantCode, stderr.String())
			}

			if !strings.Contains(stdout.String(), tt.wantStdout) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tt.wantStdout)
			}

			if !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}

func TestExecuteInterrupted(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var stdout, stderr bytes.Buffer

	if code := Execute(ctx, []string{"deploy"}, &stdout, &stderr); code != exitInterrupted {
		t.Fatalf("exit code = %d, want %d", code, exitInterrupted)
	}
}
