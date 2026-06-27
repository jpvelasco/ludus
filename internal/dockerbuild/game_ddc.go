package dockerbuild

import (
	"fmt"
	"os"

	"github.com/jpvelasco/ludus/internal/ddc"
)

// ddcArgs returns the extra container args (volume mounts and env vars) for the
// configured DDC mode. It also creates the host cache directory if needed.
//
// Zen (the default, UE's default local backend since 5.4) mounts only the
// ZenStore data path, where the cook DDC is written. Legacy local mounts only
// the FileSystem cache at /ddc via UE-LocalDataCachePath.
func (b *DockerGameBuilder) ddcArgs() ([]string, error) {
	switch b.opts.DDCMode {
	case ddc.ModeZen:
		return b.zenDDCArgs()
	case ddc.ModeLocal:
		return b.localDDCArgs()
	case ddc.ModeNone:
		fmt.Println("DDC: disabled")
		return nil, nil
	case "":
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported DDC mode %q; valid values are %q, %q, or %q", b.opts.DDCMode, ddc.ModeZen, ddc.ModeLocal, ddc.ModeNone)
	}
}

// zenDDCArgs mounts the host ZenStore directory at the container's Zen data path
// so the cook DDC persists across --rm runs.
func (b *DockerGameBuilder) zenDDCArgs() ([]string, error) {
	if b.opts.DDCZenPath == "" {
		return nil, fmt.Errorf("DDC mode is %q but no zen path configured; set ddc.zenPath in ludus.yaml or use --ddc none", ddc.ModeZen)
	}
	if err := os.MkdirAll(b.opts.DDCZenPath, 0755); err != nil { //nolint:gosec // user-configured path
		return nil, fmt.Errorf("creating DDC zen directory: %w", err)
	}
	fmt.Printf("DDC: zen (ZenStore at %s)\n", b.opts.DDCZenPath)
	return []string{
		"-v", fmt.Sprintf("%s:%s", b.opts.DDCZenPath, ddc.ZenContainerPath),
	}, nil
}

// localDDCArgs mounts the legacy FileSystem DDC at /ddc and redirects UE's
// local backend there via UE-LocalDataCachePath.
func (b *DockerGameBuilder) localDDCArgs() ([]string, error) {
	if b.opts.DDCPath == "" {
		return nil, fmt.Errorf("DDC mode is %q but no path configured; set ddc.localPath in ludus.yaml or use --ddc none", ddc.ModeLocal)
	}
	if err := os.MkdirAll(b.opts.DDCPath, 0755); err != nil {
		return nil, fmt.Errorf("creating DDC directory: %w", err)
	}
	fmt.Printf("DDC: local (persistent at %s)\n", b.opts.DDCPath)
	return []string{
		"-v", fmt.Sprintf("%s:/ddc", b.opts.DDCPath),
		"-e", ddc.EnvOverride("/ddc"),
	}, nil
}
