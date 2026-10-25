package globals

import (
	"fmt"

	"github.com/jpvelasco/ludus/internal/config"
	"github.com/spf13/viper"
)

// ConfigFile is the config file path selected via the root --config flag,
// recorded by the root command after flag parsing and before config load.
// Empty means no explicit selection; ResolveConfigFile derives the default.
var ConfigFile string

// ResolveConfigFile returns the effective config file path — the single
// source of truth shared by the root command, config get/set, setup, and MCP
// config persistence: an explicit --config selection wins, named profiles
// use ludus-<profile>.yaml, and the default profile uses ludus.yaml in the
// current directory.
func ResolveConfigFile() string {
	if ConfigFile != "" {
		return ConfigFile
	}
	if Profile != "" {
		return "ludus-" + Profile + ".yaml"
	}
	return "ludus.yaml"
}

// NewConfigDocument returns an explicit Viper instance bound to path, seeded
// with the file's current contents so settings nobody edits survive the
// rewrite. exists reports whether the file was present and readable.
//
// A missing file returns an empty document with exists=false; the file is
// created by SaveConfigDocument. A file that exists but cannot be parsed
// returns a blank document together with the read error: callers that surface
// the error (config get/set, MCP persistence) treat it as fatal, while setup
// tolerates it after the user confirms an overwrite.
func NewConfigDocument(path string) (v *viper.Viper, exists bool, err error) {
	v = viper.New()
	v.SetConfigType("yaml")
	v.SetConfigFile(path)
	if readErr := v.ReadInConfig(); readErr != nil {
		if config.IsMissingFile(readErr) {
			return v, false, nil
		}
		return v, false, fmt.Errorf("reading %s: %w", path, readErr)
	}
	return v, true, nil
}

// SaveConfigDocument persists the document to path, creating the file when
// it does not exist. Callers apply in-memory changes only after it succeeds.
func SaveConfigDocument(v *viper.Viper, path string) error {
	if err := v.WriteConfigAs(path); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
