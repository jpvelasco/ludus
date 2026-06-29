package dockerbuild

import (
	"fmt"
	"path/filepath"

	"github.com/jpvelasco/ludus/internal/config"
)

// generateBuildScript creates the shell script that runs inside the container.
func (b *DockerGameBuilder) generateBuildScript(serverBuild bool) string {
	script := "#!/bin/bash\nset -e\n\n"
	if serverBuild {
		script += b.serverBuildScript()
	} else {
		script += b.clientBuildScript()
	}
	return script
}

// scriptPreamble returns a standalone root-level setup script that installs
// runtime deps, creates a non-root build user, and re-execs /build.sh as that
// user. UE 5.7+ refuses to run UnrealEditor-Cmd as root on x86_64.
func (b *DockerGameBuilder) scriptPreamble() string {
	script := "#!/bin/bash\nset -e\n\n"

	// Install runtime libraries if missing. Older engine images may not include
	// them. Uses the centralized package list from deps.go.
	script += RuntimeDepsInstallScript()
	script += "\n"

	// Create non-root build user if not already in the image.
	// UE 5.7+ checks geteuid() == 0 in UnixPlatformMemory.cpp and aborts.
	script += `# Create non-root build user (UE 5.7+ refuses root on x86_64)
if ! id ue >/dev/null 2>&1; then
    useradd -m -s /bin/bash ue
    chown -R ue:ue /engine /output /ddc 2>/dev/null || true
    chown -R ue:ue /project 2>/dev/null || true
else
    # User exists (new engine image) but mounted volumes need ownership
    chown ue:ue /output /ddc 2>/dev/null || true
    # /project is the user's source tree; UAT (as ue) edits subdirs in place
    # (e.g. sed -i on Config/DefaultEngine.ini), so it must be recursive — a
    # non-recursive chown leaves root-owned subdirs and the build fails with
    # "sed: couldn't open temporary file .../Config/...". Unlike /engine this
    # is small, so recursive chown is safe (no large overlayfs copy-up).
    chown -R ue:ue /project 2>/dev/null || true
    # Engine root may be root-owned in images where user ue pre-exists; UAT (as ue)
    # mkdir's build outputs (Intermediate/, Saved/, DerivedDataCache/) directly under
    # /engine and /engine/Engine. Chown ONLY these parent dirs (non-recursive) so the
    # mkdir succeeds — a recursive chown would force an expensive overlayfs copy-up of
    # the whole multi-GB engine tree and can exhaust disk.
    chown ue:ue /engine /engine/Engine 2>/dev/null || true
    # Safety net for engine images built before the Dockerfile ownership fix.
    # New images handle this at build time; these are no-ops on current images.
    find /engine/Engine/Plugins -path '*/Build/Scripts/obj' -type d -exec chown -R ue:ue {} + 2>/dev/null || true
    chown ue:ue /engine/Engine/Binaries/Linux/*.sym 2>/dev/null || true
fi
`

	// When a ZenStore mount is configured, Docker auto-creates the bind-mount
	// parent chain (/home/ue/.config/Epic/UnrealEngine/Common/Zen/Data) owned by
	// root, and the host DDC dir backing the mount is itself root-owned. UAT runs
	// as ue and needs to (a) write the Zen Data store, (b) create the sibling
	// Zen/Install/zenserver dir, and (c) mkdir /home/ue/.config/Unreal Engine.
	// Chowning only the top two levels (the original #340 fix) left the deeper
	// Zen paths root-owned, so zenserver failed its readiness check, the DDC
	// backend graph had no writable node, and the cook crashed (errno=13).
	// Recursively chown the whole .config tree — it's small (no large overlayfs
	// copy-up risk, unlike /engine) — so every Zen path is writable by ue.
	if b.opts.DDCZenPath != "" {
		script += `# Fix ownership of the Docker-created ZenStore mount tree so UAT (as ue) can
# write the Zen Data store, create Zen/Install, and mkdir its config dir (#340).
chown -R ue:ue /home/ue/.config 2>/dev/null || true
`
	}

	script += `# Re-exec the build as the ue user, preserving container env vars (-p).
# Override HOME because su -p keeps HOME=/root from the container's root user,
# and .NET SDK / UE tools write to $HOME/.dotnet, $HOME/.local, etc.
exec su -p ue -c "export HOME=/home/ue && cd /engine && bash /build.sh"
`
	return script
}

// envArgs returns extra container -e flags for environment variables that must
// survive the preamble's su user switch (container-level env vars persist).
func (b *DockerGameBuilder) envArgs() []string {
	var args []string
	v := b.opts.EngineVersion
	if v == "" || v == "5.6" {
		args = append(args, "-e", "NuGetAuditLevel=critical")
	}
	return args
}

// serverBuildScript returns the shell commands for a server build inside Docker.
func (b *DockerGameBuilder) serverBuildScript() string {
	projectPath := b.containerProjectPath()

	if b.opts.CookOnly {
		script := "cd /engine\n\n"
		uePlatform := config.UEPlatformName(b.resolveArch())
		args := fmt.Sprintf(`bash Engine/Build/BatchFiles/RunUAT.sh BuildCookRun \
  -project="%s" \
  -platform=%s \
  -server -noclient \
  -cook -skipbuild \
  -NoCompileEditor -NoP4 \
  -map=MinimalDefaultMap`,
			projectPath, uePlatform)
		return script + args + "\n"
	}

	serverTarget := b.resolveServerTarget()

	// Ensure DefaultServerTarget is set so UAT can resolve the target in projects
	// with multiple *Server.Target.cs files (e.g. Lyra). The prior approach used a
	// conditional sed that silently did nothing when DefaultGameTarget wasn't present.
	// Now we unconditionally append to the [/Script/BuildSettings.BuildSettings]
	// section (creating it if absent) whenever DefaultServerTarget isn't already set.
	script := fmt.Sprintf(`# Ensure DefaultServerTarget in DefaultEngine.ini
INI_PATH="%s/Config/DefaultEngine.ini"
if [ -f "$INI_PATH" ] && ! grep -q "DefaultServerTarget" "$INI_PATH"; then
    if grep -q "\[/Script/BuildSettings.BuildSettings\]" "$INI_PATH"; then
        sed -i "s|\[/Script/BuildSettings.BuildSettings\]|[/Script/BuildSettings.BuildSettings]\nDefaultServerTarget=%s|" "$INI_PATH"
    else
        printf "\n[/Script/BuildSettings.BuildSettings]\nDefaultServerTarget=%s\n" >> "$INI_PATH"
    fi
    echo "Set DefaultServerTarget=%s in $INI_PATH"
fi

`, filepath.Dir(projectPath), serverTarget, serverTarget, serverTarget)

	script += "cd /engine\n\n"

	uePlatform := config.UEPlatformName(b.resolveArch())
	serverPlatform := config.UEServerPlatformName(b.resolveArch())
	// -pak -iostore produce a self-contained server (pak + IoStore) so the
	// deployed binary loads cooked data from disk rather than relying on Zen
	// loose-file streaming, which a deployed server cannot reach. (#406)
	args := fmt.Sprintf(`bash Engine/Build/BatchFiles/RunUAT.sh BuildCookRun \
  -project="%s" \
  -platform=%s \
  -serverplatform=%s \
  -server -noclient \
  -servertargetname=%s \
  -build -stage -package -pak -iostore -archive \
  -archivedirectory="/output"`,
		projectPath, uePlatform, serverPlatform, serverTarget)

	if !b.opts.SkipCook {
		args += " \\\n  -cook"
	} else {
		args += " \\\n  -skipcook"
	}

	if b.opts.ServerMap != "" {
		args += fmt.Sprintf(` \
  -map="%s"`, b.opts.ServerMap)
	}

	return script + args + "\n"
}

// clientBuildScript returns the shell commands for a client build inside Docker.
func (b *DockerGameBuilder) clientBuildScript() string {
	projectPath := b.containerProjectPath()

	platform := b.resolveClientPlatform()
	clientTarget := b.opts.ClientTarget
	if clientTarget == "" {
		clientTarget = b.resolveProjectName() + "Game"
	}

	script := "cd /engine\n\n"

	args := fmt.Sprintf(`bash Engine/Build/BatchFiles/RunUAT.sh BuildCookRun \
  -project="%s" \
  -platform=%s \
  -build -stage -package -archive \
  -archivedirectory="/output"`,
		projectPath, platform)

	if !b.opts.SkipCook {
		args += " \\\n  -cook"
	} else {
		args += " \\\n  -skipcook"
	}

	_ = clientTarget // target name is implicit in the project for client builds
	return script + args + "\n"
}
