package game

import (
	"fmt"
	"os"

	"github.com/jpvelasco/ludus/internal/ddc"
)

// setupDDC configures DDC by setting the UE-LocalDataCachePath environment
// variable on the runner. This overrides UE5's default local DDC path without
// modifying any project or engine files. Returns an error if the DDC directory
// cannot be created (permission denied, disk full, etc.).
func (b *Builder) setupDDC() error {
	switch b.opts.DDCMode {
	case ddc.ModeLocal:
		return b.setupLocalDDC()
	case ddc.ModeNone, "":
		return nil
	default:
		return fmt.Errorf("unsupported DDC mode %q; valid values are %q or %q", b.opts.DDCMode, ddc.ModeLocal, ddc.ModeNone)
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
