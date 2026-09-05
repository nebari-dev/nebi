package service

import "github.com/nebari-dev/nebi/internal/oci"

func ociCoreLayerLimit(maxBytes int) int64 {
	if maxBytes <= 0 {
		return -1
	}
	return int64(maxBytes)
}

func mapOCILimitError(err error) error {
	if oci.IsLimitError(err) {
		return &ValidationError{Message: err.Error()}
	}
	return err
}
