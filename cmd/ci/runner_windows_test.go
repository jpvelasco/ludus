//go:build windows

package ci

import (
	"strings"
	"testing"
)

func TestRunnerCommandsRejectWindows(t *testing.T) {
	tests := []struct {
		name string
		run  func() error
		want string
	}{
		{name: "install", run: func() error { return runInstall(installCmd, nil) }, want: "runner install is only supported on Linux"},
		{name: "status", run: func() error { return runStatus(statusCmd, nil) }, want: "runner status is only supported on Linux"},
		{name: "uninstall", run: func() error { return runUninstall(uninstallCmd, nil) }, want: "runner uninstall is only supported on Linux"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}
