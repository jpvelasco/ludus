package deploy

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	deployapi "github.com/jpvelasco/ludus/internal/deploy"
	"github.com/jpvelasco/ludus/internal/state"
)

func TestDestroyPreviewPreservesProfileFiles(t *testing.T) {
	t.Chdir(t.TempDir())
	previous := state.ActiveProfile()
	state.SetProfile("preview")
	t.Cleanup(func() { state.SetProfile(previous) })
	cfg := config.Defaults()
	cfg.EC2Fleet.S3Bucket = "configured-purge-bucket"
	globals.SetGlobals(t, cfg, globals.WithDryRun(true), globals.WithJSONOutput(true))
	saveDeployFlags(t)
	destroyAllTgts, destroyPurge, destroyYes = true, true, true
	createPreviewSnapshotFiles(t)
	before := snapshotPreviewFiles(t)
	var runErr error
	out := captureStdout(func() { runErr = runDestroy(newCommand(), nil) })
	if runErr != nil {
		t.Fatal(runErr)
	}
	var plan deployapi.DestroyPlan
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Profile != "preview" || plan.Purge[1].Identifier != "configured-purge-bucket" {
		t.Fatalf("wrong profile/purge scope: %+v", plan)
	}
	if !reflect.DeepEqual(before, snapshotPreviewFiles(t)) {
		t.Fatal("preview modified state, cache, logs, runtime, or output files")
	}
}

func createPreviewSnapshotFiles(t *testing.T) {
	t.Helper()
	st := &state.State{
		Deploy:   &state.DeployState{TargetName: "stack"},
		Fleet:    &state.FleetState{StackName: "recorded-custom-stack"},
		EC2Fleet: &state.EC2FleetState{FleetID: "ec2-fleet", BuildID: "build", S3Bucket: "archive-bucket", S3Key: "server.zip"},
		Anywhere: &state.AnywhereState{PID: 2147483647, WrapperConfigDir: ".runtime"},
	}
	if err := state.Save(st); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{".ludus/cache.json", ".ludus/logs/keep.log", ".runtime/server", "dist/server/keep"} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func snapshotPreviewFiles(t *testing.T) map[string]string {
	t.Helper()
	root, err := os.OpenRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	files := map[string]string{}
	err = filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			files[path+"/"] = ""
			return nil
		}
		data, err := root.ReadFile(path)
		files[path] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestDestroyPreviewRejectsMalformedState(t *testing.T) {
	t.Chdir(t.TempDir())
	previous := state.ActiveProfile()
	state.SetProfile("")
	t.Cleanup(func() { state.SetProfile(previous) })
	globals.SetGlobals(t, config.Defaults(), globals.WithDryRun(true))
	saveDeployFlags(t)
	if err := os.Mkdir(".ludus", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".ludus/state.json", []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runDestroy(newCommand(), nil); err == nil {
		t.Fatal("malformed state treated as an empty deployment")
	}
}
