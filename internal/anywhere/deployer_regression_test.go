package anywhere

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/smithy-go"
	"github.com/jpvelasco/ludus/internal/runner"
	"github.com/jpvelasco/ludus/internal/state"
	"github.com/jpvelasco/ludus/internal/wrapper"
)

var errFleetBoom = &smithy.GenericAPIError{Code: "InternalServiceException", Message: "boom"}

// stubWrapperBinary pre-creates the cached game-server-wrapper binary so
// createResources can run without cloning or building the real wrapper.
func stubWrapperBinary(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	setTestHome(t, home)
	cacheDir := filepath.Join(home, ".cache", "ludus", "game-server-wrapper")
	binary := wrapper.BinaryPath(cacheDir, runtime.GOOS, runtime.GOARCH)
	if err := os.MkdirAll(filepath.Dir(binary), 0755); err != nil {
		t.Fatalf("creating wrapper cache dir: %v", err)
	}
	if err := os.WriteFile(binary, []byte("stub"), 0755); err != nil {
		t.Fatalf("writing wrapper binary: %v", err)
	}
}

// setTestHome points the platform's home-directory resolution at dir:
// os.UserHomeDir reads HOME on Unix and USERPROFILE on Windows, so both are
// set — the wrapper cache and the scoped config directories derive from it.
func setTestHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}

// stubServerBuild creates a packaged server build directory containing the
// given executable file in the host platform's Binaries directory and returns
// the build directory.
func stubServerBuild(t *testing.T, executable string) string {
	t.Helper()
	buildDir := t.TempDir()
	platformDir, _ := hostServerPlatformDir()
	binDir := filepath.Join(buildDir, "Lyra", "Binaries", platformDir)
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("creating binaries dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(binDir, executable), []byte("binary"), 0755); err != nil {
		t.Fatalf("writing server binary: %v", err)
	}
	return buildDir
}

// hostExecutableName returns the bare executable filename for this host: the
// discovery only accepts entries carrying the platform's executable suffix
// (".exe" on a Windows host), so a bare name must include it.
func hostExecutableName(target string) string {
	_, exeSuffix := hostServerPlatformDir()
	return target + exeSuffix
}

// shippingExecutableName returns the Shipping-suffixed executable filename
// for this host's Binaries platform directory (e.g. "LyraServer-Linux-Shipping"
// on Linux amd64, "LyraServer-Win64-Shipping.exe" on Windows).
func shippingExecutableName(target string) string {
	platformDir, exeSuffix := hostServerPlatformDir()
	return target + "-" + platformDir + "-Shipping" + exeSuffix
}

func newRegressionDeployer(t *testing.T, fake gameLiftAPI, opts DeployOptions) *TargetAdapter {
	t.Helper()
	deployer := NewDeployer(opts, aws.Config{}, runner.NewRunner(false, true))
	if fake != nil {
		deployer.glClient = fake
	}
	return NewTargetAdapter(deployer)
}

// TestCreateResourcesRollsBackLocationOnFleetFailure pins #719: when fleet
// creation fails after a fresh location was created, rollback must still run
// (fleetID is empty) and delete the location GameLift actually created —
// the normalized custom- name, not the configured one.
func TestCreateResourcesRollsBackLocationOnFleetFailure(t *testing.T) {
	stubWrapperBinary(t)
	fake := &fakeGameLift{createFleetErr: errFleetBoom}
	a := newRegressionDeployer(t, fake, DeployOptions{
		LocationName:   "ludus-dev", // no custom- prefix: needs normalization
		IPAddress:      "127.0.0.1",
		ServerBuildDir: stubServerBuild(t, hostExecutableName("LyraServer")),
		ProjectName:    "Lyra",
		ServerTarget:   "LyraServer",
	})

	_, err := a.createResources(context.Background())
	if err == nil {
		t.Fatal("createResources() error = nil, want fleet creation failure")
	}
	if !fake.createdLocation {
		t.Fatal("expected the location to be created first")
	}
	if !fake.deletedLocation {
		t.Error("rollback must delete the created location even though no fleet ID exists")
	}
	if fake.deleteLocationName != "custom-ludus-dev" {
		t.Errorf("DeleteLocation used %q, want normalized custom-ludus-dev", fake.deleteLocationName)
	}
	if fake.deletedFleet {
		t.Error("rollback must not attempt to delete a fleet that was never created")
	}
}

// TestCreateResourcesRollsBackNormalizedLocationOnRegisterFailure pins the
// second half of #719: a failure after fleet creation with an unprefixed
// configured location name must ask deletion of the normalized name.
func TestCreateResourcesRollsBackNormalizedLocationOnRegisterFailure(t *testing.T) {
	stubWrapperBinary(t)
	fake := &fakeGameLift{registerErr: errRegisterBoom}
	a := newRegressionDeployer(t, fake, DeployOptions{
		LocationName:   "ludus-dev",
		IPAddress:      "127.0.0.1",
		ServerBuildDir: stubServerBuild(t, hostExecutableName("LyraServer")),
		ProjectName:    "Lyra",
		ServerTarget:   "LyraServer",
	})

	_, err := a.createResources(context.Background())
	if err == nil {
		t.Fatal("createResources() error = nil, want registration failure")
	}
	if !fake.deletedFleet {
		t.Error("rollback must delete the created fleet")
	}
	if !fake.deletedLocation {
		t.Error("rollback must delete the created location")
	}
	if fake.deleteLocationName != "custom-ludus-dev" {
		t.Errorf("DeleteLocation used %q, want normalized custom-ludus-dev", fake.deleteLocationName)
	}
}

// TestCreateResourcesValidatesServerExecutableEarly pins #744: a deployment
// against a build directory with no server executable must fail before any
// cloud resource is created.
func TestCreateResourcesValidatesServerExecutableEarly(t *testing.T) {
	stubWrapperBinary(t)
	fake := &fakeGameLift{}
	a := newRegressionDeployer(t, fake, DeployOptions{
		LocationName:   "custom-loc",
		IPAddress:      "127.0.0.1",
		ServerBuildDir: t.TempDir(), // empty build dir: no executable
		ProjectName:    "Lyra",
		ServerTarget:   "LyraServer",
	})

	_, err := a.createResources(context.Background())
	if err == nil {
		t.Fatal("createResources() error = nil, want missing-executable failure")
	}
	if !strings.Contains(err.Error(), "server executable not found") {
		t.Errorf("error = %v, want a missing server executable error", err)
	}
	if fake.createdLocation {
		t.Error("no cloud resource may be created before the executable is validated")
	}
}

// TestResolveServerExecutablePathShippingSuffix pins the discovery half of
// #744: after a Shipping package, the suffixed executable must be selected.
func TestResolveServerExecutablePathShippingSuffix(t *testing.T) {
	buildDir := stubServerBuild(t, shippingExecutableName("LyraServer"))
	d := NewDeployer(DeployOptions{
		ServerBuildDir: buildDir,
		ProjectName:    "Lyra",
		ServerTarget:   "LyraServer",
	}, aws.Config{}, runner.NewRunner(false, true))

	got, err := d.resolveServerExecutablePath()
	if err != nil {
		t.Fatalf("resolveServerExecutablePath() error = %v", err)
	}
	if want := shippingExecutableName("LyraServer"); !strings.HasSuffix(filepath.ToSlash(got), want) {
		t.Errorf("executable = %q, want the Shipping-suffixed binary %q", got, want)
	}
}

// TestResolveServerExecutablePathMissingBinary pins that a config pointing at
// a nonexistent executable is never produced silently.
func TestResolveServerExecutablePathMissingBinary(t *testing.T) {
	d := NewDeployer(DeployOptions{
		ServerBuildDir: t.TempDir(),
		ProjectName:    "Lyra",
		ServerTarget:   "LyraServer",
	}, aws.Config{}, runner.NewRunner(false, true))

	_, err := d.resolveServerExecutablePath()
	if err == nil {
		t.Fatal("resolveServerExecutablePath() error = nil, want missing-binary error")
	}
	if !strings.Contains(err.Error(), "server executable not found") {
		t.Errorf("error = %v, want a clear missing-executable error", err)
	}
}

// TestWrapperConfigDirScoped pins #720: the runtime directory is scoped per
// profile and project so deployments stop sharing one config path.
func TestWrapperConfigDirScoped(t *testing.T) {
	setTestHome(t, t.TempDir())
	tests := []struct {
		name        string
		profile     string
		projectName string
		want        string
	}{
		{"default profile scopes by project", "", "Lyra", filepath.Join("anywhere", "Lyra")},
		{"named profile nests under profile", "staging", "Lyra", filepath.Join("anywhere", "staging", "Lyra")},
		{"different projects never collide", "", "OtherGame", filepath.Join("anywhere", "OtherGame")},
		{"empty scope falls back to default", "", "", filepath.Join("anywhere", "default")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state.SetProfile(tt.profile)
			t.Cleanup(func() { state.SetProfile("") })

			d := NewDeployer(DeployOptions{ProjectName: tt.projectName},
				aws.Config{}, runner.NewRunner(false, true))
			got, err := d.wrapperConfigDir()
			if err != nil {
				t.Fatalf("wrapperConfigDir() error = %v", err)
			}
			if !strings.HasSuffix(got, tt.want) {
				t.Errorf("wrapperConfigDir() = %q, want suffix %q", got, tt.want)
			}
		})
	}
}

// TestWrapperConfigDirSanitizesScope pins that profile and project names
// cannot escape the cache directory.
func TestWrapperConfigDirSanitizesScope(t *testing.T) {
	setTestHome(t, t.TempDir())
	state.SetProfile("")
	t.Cleanup(func() { state.SetProfile("") })

	d := NewDeployer(DeployOptions{ProjectName: "../etc"}, aws.Config{}, runner.NewRunner(false, true))
	got, err := d.wrapperConfigDir()
	if err != nil {
		t.Fatalf("wrapperConfigDir() error = %v", err)
	}
	if strings.Contains(got, "..") {
		t.Errorf("wrapperConfigDir() = %q, want sanitized path segment", got)
	}
	if !strings.HasSuffix(got, filepath.Join("anywhere", "---etc")) {
		t.Errorf("wrapperConfigDir() = %q, want sanitized ---etc segment", got)
	}
}

// TestWrapperCleanupDir pins the destroy cleanup decision (#720): exactly
// the recorded directory; the legacy shared directory only for deployments
// recorded before per-deployment scoping; nothing otherwise.
func TestWrapperCleanupDir(t *testing.T) {
	tests := []struct {
		name               string
		recorded           string
		legacyDir          string
		legacyConfigExists bool
		want               string
	}{
		{"recorded path wins", "/recorded", "/legacy", true, "/recorded"},
		{"legacy used when unrecorded and present", "", "/legacy", true, "/legacy"},
		{"nothing without record or legacy", "", "/legacy", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wrapperCleanupDir(tt.recorded, tt.legacyDir, tt.legacyConfigExists)
			if got != tt.want {
				t.Errorf("wrapperCleanupDir() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestDestroyCleansOnlyOwnRuntimeDir pins #720: destroying one deployment
// must not remove the runtime directory of another.
func TestDestroyCleansOnlyOwnRuntimeDir(t *testing.T) {
	t.Chdir(t.TempDir())
	home := t.TempDir()
	setTestHome(t, home)

	mineDir := filepath.Join(home, "anywhere", "mine")
	otherDir := filepath.Join(home, "anywhere", "other")
	for _, dir := range []string{mineDir, otherDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("creating %s: %v", dir, err)
		}
		if err := os.WriteFile(filepath.Join(dir, wrapperConfigFileName), []byte("cfg"), 0644); err != nil {
			t.Fatalf("writing config: %v", err)
		}
	}

	if err := state.UpdateAnywhere(&state.AnywhereState{
		FleetID: "fleet-1", WrapperConfigDir: mineDir,
	}); err != nil {
		t.Fatalf("UpdateAnywhere: %v", err)
	}

	a := newRegressionDeployer(t, &fakeGameLift{}, DeployOptions{})
	if err := a.Destroy(context.Background()); err != nil {
		t.Fatalf("Destroy() error = %v", err)
	}
	if _, err := os.Stat(mineDir); !os.IsNotExist(err) {
		t.Error("own runtime directory was not removed")
	}
	if _, err := os.Stat(otherDir); err != nil {
		t.Errorf("another deployment's runtime directory must survive: %v", err)
	}
}

// TestDestroyLegacyFallback pins backward compatibility (#720): deployments
// recorded before per-deployment scoping have no path in state; their
// destroy still cleans the legacy shared directory.
func TestDestroyLegacyFallback(t *testing.T) {
	t.Chdir(t.TempDir())
	home := t.TempDir()
	setTestHome(t, home)

	legacyDir := filepath.Join(home, ".cache", "ludus", "anywhere")
	if err := os.MkdirAll(legacyDir, 0755); err != nil {
		t.Fatalf("creating legacy dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, wrapperConfigFileName), []byte("cfg"), 0644); err != nil {
		t.Fatalf("writing legacy config: %v", err)
	}

	if err := state.UpdateAnywhere(&state.AnywhereState{FleetID: "fleet-1"}); err != nil {
		t.Fatalf("UpdateAnywhere: %v", err)
	}

	a := newRegressionDeployer(t, &fakeGameLift{}, DeployOptions{})
	if err := a.Destroy(context.Background()); err != nil {
		t.Fatalf("Destroy() error = %v", err)
	}
	if _, err := os.Stat(legacyDir); !os.IsNotExist(err) {
		t.Error("legacy runtime directory was not removed")
	}
}

// TestDestroyWithoutLegacyLeavesNothing pins that a deployment with no
// recorded directory cleans nothing when no legacy config exists.
func TestDestroyWithoutLegacyLeavesNothing(t *testing.T) {
	t.Chdir(t.TempDir())
	setTestHome(t, t.TempDir())

	if err := state.UpdateAnywhere(&state.AnywhereState{FleetID: "fleet-1"}); err != nil {
		t.Fatalf("UpdateAnywhere: %v", err)
	}

	a := newRegressionDeployer(t, &fakeGameLift{}, DeployOptions{})
	if err := a.Destroy(context.Background()); err != nil {
		t.Fatalf("Destroy() error = %v", err)
	}
}

// TestLaunchServerWritesScopedConfig pins that the wrapper config lands in
// the deployment's scoped runtime directory and the directory is returned
// for the caller to persist (#720).
func TestLaunchServerWritesScopedConfig(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	d := NewDeployer(DeployOptions{
		ProjectName: "Lyra",
		ServerMap:   "L_Expanse",
		ServerPort:  7777,
		AWSProfile:  "default",
	}, aws.Config{}, runner.NewRunner(false, true)) // dry-run: no process

	_, configDir, err := d.LaunchServer(context.Background(), "/usr/local/bin/wrapper",
		"/builds/Lyra/Binaries/Linux/LyraServer", "arn:fleet", "arn:loc", "10.0.0.1")
	if err != nil {
		t.Fatalf("LaunchServer() error = %v", err)
	}
	wantDir := filepath.Join(home, ".cache", "ludus", "anywhere", "Lyra")
	if configDir != wantDir {
		t.Errorf("configDir = %q, want %q", configDir, wantDir)
	}
	data, err := os.ReadFile(filepath.Join(configDir, wrapperConfigFileName))
	if err != nil {
		t.Fatalf("wrapper config not written to the scoped dir: %v", err)
	}
	if !strings.Contains(string(data), `executable-file-path: "/builds/Lyra/Binaries/Linux/LyraServer"`) {
		t.Errorf("wrapper config missing the executable path:\n%s", data)
	}
}

// TestRollbackPreservesOtherRuntimeDirs pins that a failed deployment attempt
// cleans only its own runtime directory — never another profile's.
func TestRollbackPreservesOtherRuntimeDirs(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	otherDir := filepath.Join(home, ".cache", "ludus", "anywhere", "other")
	if err := os.MkdirAll(otherDir, 0755); err != nil {
		t.Fatalf("creating other dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(otherDir, wrapperConfigFileName), []byte("cfg"), 0644); err != nil {
		t.Fatalf("writing other config: %v", err)
	}

	state.SetProfile("mine")
	t.Cleanup(func() { state.SetProfile("") })
	fake := &fakeGameLift{}
	a := newRegressionDeployer(t, fake, DeployOptions{ProjectName: "Lyra", LocationName: "custom-loc"})
	a.rollbackPartialResources(context.Background(), "fleet-1", "compute-1", true, "")

	if _, err := os.Stat(otherDir); err != nil {
		t.Errorf("rollback must not touch another profile's runtime dir: %v", err)
	}
	if !fake.deletedLocation {
		t.Error("rollback must still delete the location it created")
	}
}

// TestSaveDeployStateRecordsWrapperConfigDir pins #720: the runtime
// directory used by the launch is persisted in the deployment state so
// destroy cleans exactly what deploy created.
func TestSaveDeployStateRecordsWrapperConfigDir(t *testing.T) {
	t.Chdir(t.TempDir())
	a := newRegressionDeployer(t, &fakeGameLift{}, DeployOptions{
		LocationName: "ludus-dev",
		ServerPort:   7777,
	})

	res := &anywhereResources{
		ipAddress:        "10.0.0.1",
		fleetID:          "fleet-1",
		fleetARN:         "arn:fleet",
		locationARN:      "arn:loc",
		computeName:      "compute-1",
		pid:              42,
		wrapperConfigDir: "/home/dev/.cache/ludus/anywhere/Lyra",
	}
	a.saveDeployState(res)

	assertAnywhereState(t, res)
}

// assertAnywhereState reloads state and checks the fields saveDeployState
// must persist, including the wrapper config directory.
func assertAnywhereState(t *testing.T, res *anywhereResources) {
	t.Helper()
	st, err := state.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if st.Anywhere == nil {
		t.Fatal("anywhere state not written")
	}
	want := map[string]struct {
		got  any
		want any
	}{
		"wrapperConfigDir": {st.Anywhere.WrapperConfigDir, res.wrapperConfigDir},
		"fleetId":          {st.Anywhere.FleetID, res.fleetID},
		"pid":              {st.Anywhere.PID, res.pid},
		"locationName":     {st.Anywhere.LocationName, "custom-ludus-dev"},
	}
	for field, tt := range want {
		if tt.got != tt.want {
			t.Errorf("anywhere state %s = %v, want %v", field, tt.got, tt.want)
		}
	}
	if st.Deploy == nil || st.Deploy.TargetName != "anywhere" {
		t.Errorf("deploy state = %+v, want targetName anywhere", st.Deploy)
	}
}
