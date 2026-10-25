package container

import (
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

// TestBuildTagFlagDefaultEmpty pins #767: the build command registers --tag
// with an empty default, so an unset flag is indistinguishable from "use the
// resolved container.tag" — the same precedence push and the MCP tools apply.
// A literal default would silently override ludus.yaml on every build.
func TestBuildTagFlagDefaultEmpty(t *testing.T) {
	flag := buildCmd.Flags().Lookup("tag")
	if flag == nil {
		t.Fatal("container build has no --tag flag")
	}
	if flag.DefValue != "" {
		t.Errorf("--tag default = %q, want empty so container.tag flows through", flag.DefValue)
	}
}

// TestRunBuildEffectiveTag pins #767 on the build side: the --tag flag wins,
// otherwise the resolved container.tag from ludus.yaml names the image
// (latest when nothing is configured).
func TestRunBuildEffectiveTag(t *testing.T) {
	for _, tt := range []struct {
		name    string
		flagTag string
		cfgTag  string
		want    string
	}{
		{name: "flag wins over config tag", flagTag: "candidate", cfgTag: "stable", want: "ludus-server:candidate"},
		{name: "config tag honored when flag unset", flagTag: "", cfgTag: "stable", want: "ludus-server:stable"},
		{name: "latest when nothing set", flagTag: "", cfgTag: "latest", want: "ludus-server:latest"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			resetContainerFlags(t)
			cfg, _ := containerBuildConfig(t)
			cfg.Container.Tag = tt.cfgTag
			globals.SetGlobals(t, cfg, globals.WithDryRun(true), globals.WithNoLogs(true))
			stubBuildPrereqTools(t)
			t.Chdir(t.TempDir())
			tag = tt.flagTag

			output, err := captureContainerStdout(t, func() error { return runBuild(buildCmd, nil) })
			if err != nil {
				t.Fatalf("runBuild() error = %v", err)
			}
			if want := "Container image built: " + tt.want; !strings.Contains(output, want) {
				t.Errorf("output missing %q:\n%s", want, output)
			}
		})
	}
}

// TestRunPushEffectiveTag pins #767's round trip: push resolves its tag with
// the same precedence as build, so a build/push pair can never reference an
// image built under a different tag. The dry-run echo exposes the effective
// tag on the docker tag command.
func TestRunPushEffectiveTag(t *testing.T) {
	for _, tt := range []struct {
		name     string
		pushFlag string
		cfgTag   string
		want     string
	}{
		{name: "flag wins over config tag", pushFlag: "candidate", cfgTag: "stable", want: "ludus-server:candidate"},
		{name: "config tag honored when flag unset", pushFlag: "", cfgTag: "stable", want: "ludus-server:stable"},
		{name: "latest when nothing set", pushFlag: "", cfgTag: "latest", want: "ludus-server:latest"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			resetContainerFlags(t)
			cfg := &config.Config{
				AWS: config.AWSConfig{
					AccountID:     "123456789012",
					Region:        "us-east-1",
					ECRRepository: "ludus-server",
				},
				Container: config.ContainerConfig{
					ImageName:  "ludus-server",
					Tag:        tt.cfgTag,
					ServerPort: 7777,
				},
			}
			globals.SetGlobals(t, cfg, globals.WithDryRun(true), globals.WithNoLogs(true))
			stubAWSEnv(t)
			testsupport.FakeTools(t, map[string]testsupport.ToolBehavior{
				"docker": {},
				"aws":    {Stdout: `{"Account":"123456789012","Arn":"arn:aws:iam::123456789012:user/test"}`},
			})
			t.Chdir(t.TempDir())
			pushTag = tt.pushFlag

			output, err := captureContainerStdout(t, func() error { return runPush(pushCmd, nil) })
			if err != nil {
				t.Fatalf("runPush() error = %v", err)
			}
			if want := "docker tag " + tt.want + " "; !strings.Contains(output, want) {
				t.Errorf("output missing %q:\n%s", want, output)
			}
		})
	}
}
