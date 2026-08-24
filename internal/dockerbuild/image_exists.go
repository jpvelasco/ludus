package dockerbuild

import (
	"context"

	"github.com/jpvelasco/ludus/internal/runner"
)

// ImageExists reports whether the given image reference resolves locally via
// the given container CLI. Used to guard container-stage cache hits: a
// recorded success is only valid if the engine image it needs still exists.
func ImageExists(r *runner.Runner, ctx context.Context, cli, image string) bool {
	return r.RunQuiet(ctx, cli, "image", "inspect", image) == nil
}
