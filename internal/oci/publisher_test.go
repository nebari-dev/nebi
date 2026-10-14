package oci

import "testing"

func TestResolveConfig_CoreLayerCapDefaultAndExplicitDisable(t *testing.T) {
	cfg := resolveConfig(nil)
	if cfg.maxCoreLayerBytes != DefaultMaxCoreLayerBytes {
		t.Fatalf("default core cap: got %d want %d", cfg.maxCoreLayerBytes, DefaultMaxCoreLayerBytes)
	}

	cfg = resolveConfig([]PublishOption{WithMaxCoreLayerBytes(0)})
	if cfg.maxCoreLayerBytes != 0 {
		t.Fatalf("explicit disabled core cap: got %d want 0", cfg.maxCoreLayerBytes)
	}
}
