package testsupport

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFakeEngineTreeCreatesRequiredFiles(t *testing.T) {
	root := FakeEngineTree(t)

	// Check required files exist
	requiredFiles := []string{
		"Setup.bat",
		"Setup.sh",
		"GenerateProjectFiles.bat",
		"GenerateProjectFiles.sh",
		filepath.Join("Engine", "Build", "BatchFiles", "Build.bat"),
		filepath.Join("Engine", "Build", "BatchFiles", "Build.sh"),
		filepath.Join("Engine", "Build", "BatchFiles", "RunUAT.bat"),
		filepath.Join("Engine", "Build", "BatchFiles", "RunUAT.sh"),
		filepath.Join("Engine", "Build", "Build.version"),
	}

	for _, file := range requiredFiles {
		path := filepath.Join(root, file)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("required file not created: %s", file)
		}
	}
}

func TestFakeEngineTreeWithoutSetup(t *testing.T) {
	root := FakeEngineTree(t, WithoutSetup())

	// Check Setup files do not exist
	setupBat := filepath.Join(root, "Setup.bat")
	if _, err := os.Stat(setupBat); !os.IsNotExist(err) {
		t.Error("Setup.bat should not exist with WithoutSetup()")
	}

	setupSh := filepath.Join(root, "Setup.sh")
	if _, err := os.Stat(setupSh); !os.IsNotExist(err) {
		t.Error("Setup.sh should not exist with WithoutSetup()")
	}

	// Other files should still exist
	generateBat := filepath.Join(root, "GenerateProjectFiles.bat")
	if _, err := os.Stat(generateBat); os.IsNotExist(err) {
		t.Error("GenerateProjectFiles.bat should still exist")
	}
}

func TestFakeEngineTreeWithVersion(t *testing.T) {
	tests := []struct {
		name      string
		version   string
		wantMajor int
		wantMinor int
		wantPatch int
	}{
		{
			name:      "version 5.7.3",
			version:   "5.7.3",
			wantMajor: 5,
			wantMinor: 7,
			wantPatch: 3,
		},
		{
			name:      "version 5.6.1",
			version:   "5.6.1",
			wantMajor: 5,
			wantMinor: 6,
			wantPatch: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := FakeEngineTree(t, WithVersion(tt.version))

			// Read Build.version
			versionPath := filepath.Join(root, "Engine", "Build", "Build.version")
			data, err := os.ReadFile(versionPath)
			if err != nil {
				t.Fatalf("failed to read Build.version: %v", err)
			}

			var versionData map[string]int
			if err := json.Unmarshal(data, &versionData); err != nil {
				t.Fatalf("failed to parse Build.version: %v", err)
			}

			if versionData["MajorVersion"] != tt.wantMajor {
				t.Errorf("MajorVersion = %d, want %d", versionData["MajorVersion"], tt.wantMajor)
			}
			if versionData["MinorVersion"] != tt.wantMinor {
				t.Errorf("MinorVersion = %d, want %d", versionData["MinorVersion"], tt.wantMinor)
			}
			if versionData["PatchVersion"] != tt.wantPatch {
				t.Errorf("PatchVersion = %d, want %d", versionData["PatchVersion"], tt.wantPatch)
			}
		})
	}
}

func TestFakeEngineTreeWithLinuxToolchain(t *testing.T) {
	toolchainVersion := "v26_clang-20.1.8-rockylinux8"
	root := FakeEngineTree(t, WithLinuxToolchain(toolchainVersion))

	// Check toolchain directory was created
	toolchainPath := filepath.Join(root, "Engine", "Extras", "ThirdPartyNotUE", "SDKs", "HostLinux", "Linux_x64", toolchainVersion)
	if _, err := os.Stat(toolchainPath); os.IsNotExist(err) {
		t.Errorf("toolchain directory not created: %s", toolchainPath)
	}
}

func TestFakeProject(t *testing.T) {
	uprojectPath := FakeProject(t, "TestGame")

	// Check .uproject file exists
	if _, err := os.Stat(uprojectPath); os.IsNotExist(err) {
		t.Fatalf("uproject file not created: %s", uprojectPath)
	}

	// Check Content directory exists
	contentDir := filepath.Dir(uprojectPath)
	contentDir = filepath.Join(contentDir, "Content")
	if _, err := os.Stat(contentDir); os.IsNotExist(err) {
		t.Errorf("Content directory not created: %s", contentDir)
	}

	// Parse .uproject to verify it's valid JSON
	data, err := os.ReadFile(uprojectPath)
	if err != nil {
		t.Fatalf("failed to read uproject: %v", err)
	}

	var uprojectData map[string]interface{}
	if err := json.Unmarshal(data, &uprojectData); err != nil {
		t.Fatalf("failed to parse uproject: %v", err)
	}

	// Verify required fields
	if uprojectData["FileVersion"] == nil {
		t.Error("FileVersion not in uproject")
	}
	if uprojectData["EngineAssociation"] == nil {
		t.Error("EngineAssociation not in uproject")
	}
}
