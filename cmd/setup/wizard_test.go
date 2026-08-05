package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/config"
)

// engineChoiceRoot builds a temp root containing a working dir and an engine
// candidate with a Build.version, then chdirs into the working dir and points
// the home dir at an empty location. This makes scanEnginePaths deterministic
// (the controlled candidate is always candidates[0]).
func engineChoiceRoot(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	working := filepath.Join(root, "project")
	createDirectory(t, working)
	enginePath := createEngineCandidate(t, root, "UnrealEngine-5.8")
	writeBuildVersion(t, enginePath, 5, 8, 1)
	t.Chdir(working)
	setTestHome(t, filepath.Join(root, "empty-home"))
	return root, enginePath
}

// emptyEnvironment chdirs to an empty dir and points home at an empty location
// so scanEnginePaths finds no engine candidates.
func emptyEnvironment(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
	setTestHome(t, t.TempDir())
}

func TestCollectEngineAnswers(t *testing.T) {
	t.Run("empty path skips detection", func(t *testing.T) {
		emptyEnvironment(t)
		withScannerInput(t, "\n")
		var a setupAnswers
		collectEngineAnswers(&a, nil)
		if a.enginePath != "" || a.engineVersion != "" {
			t.Errorf("answers = %q, %q; want empty", a.enginePath, a.engineVersion)
		}
	})
	t.Run("path auto-detects version", func(t *testing.T) {
		_, enginePath := engineChoiceRoot(t)
		withScannerInput(t, "1\n")
		var a setupAnswers
		collectEngineAnswers(&a, nil)
		if a.enginePath != enginePath || a.engineVersion != "5.8.1" {
			t.Errorf("answers = %q, %q; want %q, 5.8.1", a.enginePath, a.engineVersion, enginePath)
		}
	})
}

func TestCollectProjectAnswers(t *testing.T) {
	t.Run("custom project", func(t *testing.T) {
		uproject := filepath.Join(t.TempDir(), "Game.uproject")
		createFile(t, uproject)
		withScannerInput(t, "MyGame\n"+uproject+"\n")
		var a setupAnswers
		a.enginePath = "/opt/engine"
		collectProjectAnswers(&a, nil)
		if a.projectName != "MyGame" || a.projectPath != uproject || a.contentSourcePath != "" {
			t.Errorf("answers = %q, %q, %q", a.projectName, a.projectPath, a.contentSourcePath)
		}
	})
	t.Run("lyra content fallback", func(t *testing.T) {
		setTestHome(t, t.TempDir())
		withScannerInput(t, "Lyra\n\n")
		var a setupAnswers
		a.enginePath = t.TempDir()
		collectProjectAnswers(&a, nil)
		if a.projectName != "Lyra" || a.projectPath != "" || a.contentSourcePath != "" {
			t.Errorf("answers = %q, %q, %q", a.projectName, a.projectPath, a.contentSourcePath)
		}
	})
}

func TestCollectAWSAnswers(t *testing.T) {
	t.Run("gamelift prompts instance", testCollectAWSGamelift)
	t.Run("binary skips instance", testCollectAWSBinary)
	t.Run("existing defaults", testCollectAWSExisting)
}

func testCollectAWSGamelift(t *testing.T) {
	forceNoAWS(t)
	withScannerInput(t, "eu-west-1\n123456789012\nc6i.2xlarge\n")
	var a setupAnswers
	a.deployTarget = "gamelift"
	collectAWSAnswers(&a, nil)
	if a.region != "eu-west-1" || a.accountID != "123456789012" || a.instanceType != "c6i.2xlarge" {
		t.Errorf("answers = %q, %q, %q", a.region, a.accountID, a.instanceType)
	}
}

func testCollectAWSBinary(t *testing.T) {
	forceNoAWS(t)
	withScannerInput(t, "\n\n")
	var a setupAnswers
	a.deployTarget = "binary"
	collectAWSAnswers(&a, nil)
	if a.instanceType != "c6i.large" || a.region != "us-east-1" {
		t.Errorf("answers = %q, %q", a.region, a.instanceType)
	}
}

func testCollectAWSExisting(t *testing.T) {
	forceNoAWS(t)
	existing := &config.Config{}
	existing.AWS.Region = "ap-south-1"
	existing.AWS.AccountID = "111122223333"
	existing.GameLift.InstanceType = "m5.large"
	withScannerInput(t, "\n\n\n")
	var a setupAnswers
	a.deployTarget = "gamelift"
	collectAWSAnswers(&a, existing)
	if a.region != "ap-south-1" || a.accountID != "111122223333" || a.instanceType != "m5.large" {
		t.Errorf("answers = %q, %q, %q", a.region, a.accountID, a.instanceType)
	}
}

func TestPrintSummaryInstanceType(t *testing.T) {
	output := captureSetupStdout(t, func() {
		printSummary(setupAnswers{
			cfgFile: "ludus.yaml", projectName: "Game",
			deployTarget: "gamelift", instanceType: "c6i.xlarge",
		})
	})
	if !strings.Contains(output, "Instance type:  c6i.xlarge") {
		t.Errorf("summary missing instance type:\n%s", output)
	}
}

func TestRunSetupFullWizard(t *testing.T) {
	engineChoiceRoot(t)
	forceNoAWS(t)
	withScannerInput(t, "1\nMyGame\n/custom/Game.uproject\n2\neu-central-1\n123456789012\nc6i.xlarge\ny\n")

	if err := runSetup(nil, nil); err != nil {
		t.Fatalf("runSetup() error: %v", err)
	}

	data, err := os.ReadFile("ludus.yaml")
	if err != nil {
		t.Fatalf("read ludus.yaml: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "UnrealEngine-5.8") || !strings.Contains(content, "sourcepath:") {
		t.Errorf("config missing engine source path:\n%s", content)
	}
	for _, want := range []string{
		"version: 5.8.1",
		"projectname: MyGame",
		"target: stack",
		"region: eu-central-1",
		"accountid: \"123456789012\"",
		"instancetype: c6i.xlarge",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("config missing %q:\n%s", want, content)
		}
	}
}

func TestRunSetupWriteDeclined(t *testing.T) {
	engineChoiceRoot(t)
	forceNoAWS(t)
	withScannerInput(t, "1\nMyGame\n/custom/Game.uproject\n2\neu-central-1\n123456789012\nc6i.xlarge\nn\n")

	if err := runSetup(nil, nil); err != nil {
		t.Fatalf("runSetup() error: %v", err)
	}
	if _, err := os.Stat("ludus.yaml"); err == nil {
		t.Error("expected no ludus.yaml when write declined")
	}
}

func TestRunSetupOverwriteDeclined(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)
	if err := os.WriteFile("ludus.yaml", []byte("existing: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	withScannerInput(t, "n\n")
	if err := runSetup(nil, nil); err != nil {
		t.Fatalf("runSetup() error: %v", err)
	}
	data, err := os.ReadFile("ludus.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "existing: true") {
		t.Errorf("config was overwritten:\n%s", data)
	}
}

func TestRunSetupOverwriteAccepted(t *testing.T) {
	root, _ := engineChoiceRoot(t)
	forceNoAWS(t)
	existing := filepath.Join(root, "project", "ludus.yaml")
	if err := os.WriteFile(existing, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	withScannerInput(t, "y\n1\nMyGame\n/custom/Game.uproject\n2\neu-central-1\n123456789012\nc6i.xlarge\ny\n")

	if err := runSetup(nil, nil); err != nil {
		t.Fatalf("runSetup() error: %v", err)
	}

	data, err := os.ReadFile(existing)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "UnrealEngine-5.8") || !strings.Contains(content, "target: stack") {
		t.Errorf("config missing wizard values:\n%s", content)
	}
}
