package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Load reads configuration from the given YAML file path, merges with defaults,
// and returns a fully populated Config. If path is empty, it searches for
// ludus.yaml in the current directory.
func Load(path string) (*Config, error) {
	cfg := Defaults()

	v := viper.New()
	v.SetConfigType("yaml")

	if path != "" {
		v.SetConfigFile(path)
	} else {
		v.SetConfigFile("ludus.yaml")
	}

	if err := v.ReadInConfig(); err != nil {
		return handleReadError(cfg, err)
	}

	migrateLyraKey(v)

	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	resolveEnginePath(cfg)
	return cfg, nil
}

// handleReadError returns defaults for missing files, or wraps real errors.
func handleReadError(cfg *Config, err error) (*Config, error) {
	if _, ok := err.(viper.ConfigFileNotFoundError); ok {
		return cfg, nil
	}
	if os.IsNotExist(err) {
		return cfg, nil
	}
	return nil, fmt.Errorf("reading config: %w", err)
}

// migrateLyraKey copies deprecated 'lyra' keys into 'game' namespace.
func migrateLyraKey(v *viper.Viper) {
	if !v.IsSet("lyra") || v.IsSet("game") {
		return
	}
	fmt.Fprintln(os.Stderr, "WARNING: 'lyra:' config key is deprecated, rename to 'game:' in ludus.yaml")
	for _, key := range []string{
		"projectPath", "projectName", "contentSourcePath", "serverTarget",
		"clientTarget", "gameTarget", "platform", "skipCook", "serverMap",
		"contentValidation",
	} {
		if v.IsSet("lyra." + key) {
			v.Set("game."+key, v.Get("lyra."+key))
		}
	}
}

// resolveEnginePath expands a relative engine source path to absolute.
func resolveEnginePath(cfg *Config) {
	if cfg.Engine.SourcePath == "" || filepath.IsAbs(cfg.Engine.SourcePath) {
		return
	}
	if cwd, err := os.Getwd(); err == nil {
		cfg.Engine.SourcePath = filepath.Join(cwd, cfg.Engine.SourcePath)
	}
}
