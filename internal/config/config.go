package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
	"go.yaml.in/yaml/v3"
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

	// Viper lowercases map keys on unmarshal, which would silently rename an
	// operator's aws.tags key "ManagedBy" to "managedby" and then collide with
	// the derived ManagedBy default (IAM rejects case-only-duplicate tag keys,
	// #775). Re-read just that one map from the raw YAML so the operator's
	// spelling is preserved.
	if raw, err := readRawTags(v.ConfigFileUsed()); err == nil && raw != nil {
		cfg.AWS.Tags = raw
	}

	resolveEnginePath(cfg)
	return cfg, nil
}

// readRawTags returns the aws.tags map with its original key casing, read
// directly from the YAML file rather than through Viper's lowercased map. A
// nil map means the file or the tags block is absent and the caller should
// keep the Viper-unmarshalled value.
func readRawTags(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc struct {
		AWS struct {
			Tags map[string]string `yaml:"tags"`
		} `yaml:"aws"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if doc.AWS.Tags == nil {
		return nil, nil
	}
	return doc.AWS.Tags, nil
}

// IsMissingFile reports whether err means the config file is absent. Shared
// by Load and the CLI-side config document helpers so both treat the same
// errors as "no config yet" rather than a read failure.
func IsMissingFile(err error) bool {
	if _, ok := err.(viper.ConfigFileNotFoundError); ok {
		return true
	}
	return os.IsNotExist(err)
}

// handleReadError returns defaults for missing files, or wraps real errors.
func handleReadError(cfg *Config, err error) (*Config, error) {
	if IsMissingFile(err) {
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
