package oci

import (
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2"
)

// bundleConfig holds user-configurable publish bundle filters parsed from
// pixi.toml's [tool.nebi.bundle] section. Both include and exclude are
// glob patterns (doublestar syntax). Absent sections yield a zero value,
// which means "all files are candidates" with no filtering.
type bundleConfig struct {
	Include []string
	Exclude []string
}

// pixiBundleDoc matches only the bundle config fragment we care about.
type pixiBundleDoc struct {
	Tool struct {
		Nebi struct {
			Bundle struct {
				Include []string `toml:"include"`
				Exclude []string `toml:"exclude"`
			} `toml:"bundle"`
		} `toml:"nebi"`
	} `toml:"tool"`
}

// loadBundleConfig parses [tool.nebi.bundle] out of a pixi.toml at path.
// Missing file or missing section both return a zero value with nil error.
// Unknown keys under [tool.nebi.bundle] are silently ignored (tolerated
// for forward compat).
func loadBundleConfig(pixiTomlPath string) (bundleConfig, error) {
	data, err := os.ReadFile(pixiTomlPath)
	if err != nil {
		if os.IsNotExist(err) {
			return bundleConfig{}, nil
		}
		return bundleConfig{}, fmt.Errorf("read %s: %w", pixiTomlPath, err)
	}
	var doc pixiBundleDoc
	if err := toml.Unmarshal(data, &doc); err != nil {
		return bundleConfig{}, fmt.Errorf("parse %s: %w", pixiTomlPath, err)
	}
	return bundleConfig{
		Include: doc.Tool.Nebi.Bundle.Include,
		Exclude: doc.Tool.Nebi.Bundle.Exclude,
	}, nil
}
