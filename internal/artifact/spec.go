package artifact

// Spec is the environment specification shown on a version: the channels,
// platforms and requested packages from pixi.toml.
type Spec struct {
	Channels  []string           `json:"channels"`
	Platforms []string           `json:"platforms"`
	Requested []RequestedPackage `json:"requested"`
}

// SpecFromManifest parses pixi.toml content and returns its Spec.
func SpecFromManifest(content []byte) (*Spec, error) {
	m, err := ParseManifest(content)
	if err != nil {
		return nil, err
	}
	return m.Spec(), nil
}
