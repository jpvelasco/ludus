//go:build darwin

package prereq

import (
	"testing"

	"github.com/jpvelasco/ludus/internal/dockerbuild"
)

func TestPlatformChecks_SkipsXcodeForContainerBackends(t *testing.T) {
	tests := []struct {
		name      string
		backend   string
		wantXcode bool
	}{
		{name: "docker skips xcode", backend: dockerbuild.BackendDocker, wantXcode: false},
		{name: "podman skips xcode", backend: dockerbuild.BackendPodman, wantXcode: false},
		{name: "native runs xcode", backend: "native", wantXcode: true},
		{name: "empty runs xcode", backend: "", wantXcode: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Checker{Backend: tt.backend}
			results := c.platformChecks()

			var sawXcode bool
			for _, r := range results {
				if r.Name == "Xcode" {
					sawXcode = true
				}
			}
			if sawXcode != tt.wantXcode {
				t.Errorf("backend %q: Xcode check present = %v, want %v", tt.backend, sawXcode, tt.wantXcode)
			}
		})
	}
}
