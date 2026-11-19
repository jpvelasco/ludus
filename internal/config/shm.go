package config

import (
	"fmt"
	"regexp"
)

// shmSizePattern mirrors the size syntax docker and podman accept for
// --shm-size (e.g. 2g, 1024m, 2GiB, 536870912).
var shmSizePattern = regexp.MustCompile(`^\d+(\.\d+)? ?[kKmMgGtTpP]?[iI]?[bB]?$`)

func validateShmSize(value string) error {
	if value != "" && !shmSizePattern.MatchString(value) {
		return fmt.Errorf("engine.shmSize %q is not a valid size; use a number with an optional unit such as 2g or 1024m", value)
	}
	return nil
}
