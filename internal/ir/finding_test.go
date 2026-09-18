package ir

import "testing"

func TestSeverityFailsAt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		severity Severity
		failOn   Severity
		want     bool
	}{
		{SeverityError, SeverityError, true},
		{SeverityWarning, SeverityError, false},
		{SeverityWarning, SeverityWarning, true},
		{SeverityNotice, SeverityNotice, true},
		{SeverityError, "never", false},
		{SeverityNotice, "never", false},
		{SeverityOff, SeverityNotice, false},
	}

	for _, tt := range tests {
		if got := tt.severity.FailsAt(tt.failOn); got != tt.want {
			t.Errorf("%s.FailsAt(%s) = %v, want %v", tt.severity, tt.failOn, got, tt.want)
		}
	}
}
