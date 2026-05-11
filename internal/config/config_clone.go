package config

import (
	"maps"
	"slices"
)

// Clone returns a deep copy of the Config, ensuring nested maps, slices, and
// pointers are independently owned by the copy.
func (c *Config) Clone() Config {
	cp := *c

	cp.AWS.Tags = maps.Clone(c.AWS.Tags)
	cp.CI.RunnerLabels = slices.Clone(c.CI.RunnerLabels)

	if c.Game.ContentValidation != nil {
		cv := *c.Game.ContentValidation
		cv.PluginContentDirs = slices.Clone(c.Game.ContentValidation.PluginContentDirs)
		cp.Game.ContentValidation = &cv
	}

	return cp
}
