package config

import "testing"

func newTestConfig() *Config {
	return &Config{
		AWS: AWSConfig{
			Region: "us-east-1",
			Tags:   map[string]string{"ManagedBy": "ludus", "Env": "prod"},
		},
		CI: CIConfig{
			RunnerLabels: []string{"self-hosted", "linux"},
		},
		Game: GameConfig{
			Arch:        "amd64",
			ProjectName: "Lyra",
			ContentValidation: &ContentValidationConfig{
				ContentMarkerFile: "Content/Default.uasset",
				PluginContentDirs: []string{"ShooterCore", "ShooterMaps"},
			},
		},
		GameLift: GameLiftConfig{InstanceType: "c6i.large"},
	}
}

func TestClone_ScalarFields(t *testing.T) {
	orig := newTestConfig()
	cp := orig.Clone()

	if cp.AWS.Region != "us-east-1" {
		t.Errorf("Region = %q, want %q", cp.AWS.Region, "us-east-1")
	}
	if cp.GameLift.InstanceType != "c6i.large" {
		t.Errorf("InstanceType = %q, want %q", cp.GameLift.InstanceType, "c6i.large")
	}
}

func TestClone_MapIsolation(t *testing.T) {
	orig := newTestConfig()
	cp := orig.Clone()

	cp.AWS.Tags["Env"] = "staging"
	cp.AWS.Tags["New"] = "value"

	if orig.AWS.Tags["Env"] != "prod" {
		t.Errorf("original Tags[Env] mutated: got %q", orig.AWS.Tags["Env"])
	}
	if _, ok := orig.AWS.Tags["New"]; ok {
		t.Error("original Tags gained key from clone mutation")
	}
}

func TestClone_SliceIsolation(t *testing.T) {
	orig := newTestConfig()
	cp := orig.Clone()

	cp.CI.RunnerLabels[0] = "changed"
	cp.CI.RunnerLabels = append(cp.CI.RunnerLabels, "extra")

	if orig.CI.RunnerLabels[0] != "self-hosted" {
		t.Errorf("original RunnerLabels[0] mutated: got %q", orig.CI.RunnerLabels[0])
	}
	if len(orig.CI.RunnerLabels) != 2 {
		t.Errorf("original RunnerLabels length changed: got %d", len(orig.CI.RunnerLabels))
	}
}

func TestClone_PointerIsolation(t *testing.T) {
	orig := newTestConfig()
	cp := orig.Clone()

	cp.Game.ContentValidation.ContentMarkerFile = "changed.uasset"
	cp.Game.ContentValidation.PluginContentDirs[0] = "Modified"

	if orig.Game.ContentValidation.ContentMarkerFile != "Content/Default.uasset" {
		t.Errorf("original ContentMarkerFile mutated: got %q", orig.Game.ContentValidation.ContentMarkerFile)
	}
	if orig.Game.ContentValidation.PluginContentDirs[0] != "ShooterCore" {
		t.Errorf("original PluginContentDirs[0] mutated: got %q", orig.Game.ContentValidation.PluginContentDirs[0])
	}
}

func TestClone_NilFieldsStayNil(t *testing.T) {
	bare := &Config{}
	cp := bare.Clone()

	if cp.AWS.Tags != nil {
		t.Error("expected nil Tags in clone of bare config")
	}
	if cp.CI.RunnerLabels != nil {
		t.Error("expected nil RunnerLabels in clone of bare config")
	}
	if cp.Game.ContentValidation != nil {
		t.Error("expected nil ContentValidation in clone of bare config")
	}
}
