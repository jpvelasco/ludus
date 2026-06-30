//go:build windows

package prereq

import "testing"

// TestNeedsNewerMSVC pins the engine-version gate that selects the MSVC toolset.
// UE 5.7+ (incl. 5.8) needs MSVC 14.44; 5.6 and earlier use 14.38. A drift in
// the `minor >= 7` gate would silently mis-pin MSVC for 5.8 and break Windows
// container builds, so 5.8 is asserted explicitly here. Passing only a config
// version exercises DetectEngineVersion's config fallback (no engine tree needed).
func TestNeedsNewerMSVC(t *testing.T) {
	tests := []struct {
		configVersion string
		want          bool
	}{
		{"5.4.4", false},
		{"5.5.4", false},
		{"5.6.1", false},
		{"5.7.3", true},
		{"5.8.0", true},
		{"", false},
		{"garbage", false},
	}
	for _, tt := range tests {
		t.Run(tt.configVersion, func(t *testing.T) {
			if got := needsNewerMSVC("", tt.configVersion); got != tt.want {
				t.Errorf("needsNewerMSVC(%q) = %v, want %v", tt.configVersion, got, tt.want)
			}
		})
	}
}

// TestMSVCVersionForEngine confirms the gate maps to the right MSVC toolset
// string — the value actually written into BuildConfiguration.xml.
func TestMSVCVersionForEngine(t *testing.T) {
	tests := []struct {
		configVersion string
		want          string
	}{
		{"5.6.1", "14.38.33130"},
		{"5.7.3", "14.44.35207"},
		{"5.8.0", "14.44.35207"},
	}
	for _, tt := range tests {
		t.Run(tt.configVersion, func(t *testing.T) {
			if got := msvcVersionForEngine("", tt.configVersion); got != tt.want {
				t.Errorf("msvcVersionForEngine(%q) = %q, want %q", tt.configVersion, got, tt.want)
			}
		})
	}
}

// containsProductsStar reports whether args scope vswhere to all products,
// which is what makes the BuildTools edition discoverable.
func containsProductsStar(args []string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-products" && args[i+1] == "*" {
			return true
		}
	}
	return false
}

// TestVswhereArgsScopeAllProducts guards the #411 fix: both the instance-listing
// and component-checking vswhere invocations must pass "-products *", otherwise
// a headless VS 2022 Build Tools install is reported as "no Visual Studio detected".
func TestVswhereArgsScopeAllProducts(t *testing.T) {
	if !containsProductsStar(vswhereListArgs()) {
		t.Errorf("vswhereListArgs() = %v; missing -products *", vswhereListArgs())
	}
	reqArgs := vswhereRequiresArgs("Microsoft.VisualStudio.Component.VC.Tools.x86.x64")
	if !containsProductsStar(reqArgs) {
		t.Errorf("vswhereRequiresArgs() = %v; missing -products *", reqArgs)
	}
}

// TestVswhereRequiresArgsIncludesComponent confirms the component id is threaded
// into the -requires query.
func TestVswhereRequiresArgsIncludesComponent(t *testing.T) {
	const id = "Microsoft.VisualStudio.Component.VC.14.44.17.14.x86.x64"
	args := vswhereRequiresArgs(id)
	found := false
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-requires" && args[i+1] == id {
			found = true
		}
	}
	if !found {
		t.Errorf("vswhereRequiresArgs(%q) = %v; component id not present", id, args)
	}
}
