package prereq

import (
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/testsupport"
)

func TestQEMUEmulationRemedy(t *testing.T) {
	for _, tt := range []struct{ arch, handler string }{
		{"arm64", "qemu-aarch64"}, {"aarch64", "qemu-aarch64"},
		{"amd64", "qemu-x86_64"}, {"x86_64", "qemu-x86_64"},
	} {
		t.Run(tt.arch, func(t *testing.T) {
			remedy := qemuEmulationRemedy(tt.arch)
			for _, want := range []string{"Debian/Ubuntu", "qemu-user-static binfmt-support", "update-binfmts --enable " + tt.handler} {
				if !strings.Contains(remedy, want) {
					t.Errorf("remedy missing %q: %s", want, remedy)
				}
			}
			if strings.Contains(remedy, "--privileged") || strings.Contains(remedy, "tonistiigi") {
				t.Error("remedy still recommends a privileged mutable image")
			}
		})
	}
}

func TestRuntimeChecksUseDistroQEMURemedy(t *testing.T) {
	testsupport.FakeTools(t, map[string]testsupport.ToolBehavior{"docker": {}, "podman": {}})
	for _, result := range []CheckResult{
		checkBuildxEmulation("emulation", "docker", "arm64", "linux/arm64"),
		checkPodmanEmulation("emulation", "arm64", "linux/arm64"),
	} {
		if !strings.Contains(result.Message, "update-binfmts --enable qemu-aarch64") {
			t.Errorf("runtime check omitted distro remedy: %+v", result)
		}
	}
}
