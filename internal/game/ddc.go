package game

import (
	"fmt"
	"os"

	"github.com/jpvelasco/ludus/internal/ddc"
)

// setupDDC configures DDC for a native (non-container) build.
//
// For "zen" (the default), it is a no-op: UE autolaunches its Zen Store into the
// user's real home directory, which already persists across runs — there is no
// ephemeral container filesystem to redirect, so Ludus leaves it untouched.
// For "local" (legacy), it sets UE-LocalDataCachePath to redirect UE5's
// FileSystem backend to a persistent path. Returns an error if the local DDC
// directory cannot be created (permission denied, disk full, etc.).
func (b *Builder) setupDDC() error {
	switch b.opts.DDCMode {
	case ddc.ModeZen, ddc.ModeNone, "":
		// Zen: UE's own Zen Store persists natively; nothing to redirect.
		return nil
	case ddc.ModeLocal:
		return b.setupLocalDDC()
	default:
		return fmt.Errorf("unsupported DDC mode %q; valid values are %q, %q, or %q", b.opts.DDCMode, ddc.ModeZen, ddc.ModeLocal, ddc.ModeNone)
	}
}

func (b *Builder) setupLocalDDC() error {
	if b.opts.DDCPath == "" {
		return fmt.Errorf("DDC mode is %q but no path configured; set ddc.localPath in ludus.yaml or use --ddc none", ddc.ModeLocal)
	}
	if err := os.MkdirAll(b.opts.DDCPath, 0755); err != nil {
		return fmt.Errorf("creating DDC directory %s: %w", b.opts.DDCPath, err)
	}
	fmt.Printf("  DDC: using persistent cache at %s\n", b.opts.DDCPath)
	b.Runner.Env = append(b.Runner.Env, ddc.EnvOverride(b.opts.DDCPath))
	return nil
}
