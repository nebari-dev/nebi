package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nebari-dev/nebi/internal/client/api/openapi"
)

// The generated spec must describe every route the API serves, and carry
// the client API version. A route added without annotations, or a spec not
// regenerated with `make swagger`, fails here.
func TestOpenAPISpecCoversEveryRoute(t *testing.T) {
	var spec struct {
		Info struct {
			Version string `json:"version"`
		} `json:"info"`
		BasePath string                                `json:"basePath"`
		Paths    map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal([]byte(openapi.SwaggerInfoclientapi.ReadDoc()), &spec); err != nil {
		t.Fatalf("decoding the generated spec: %v", err)
	}
	if spec.Info.Version != ClientAPIVersion {
		t.Errorf("spec version %q, want ClientAPIVersion %q: change @version in api.go", spec.Info.Version, ClientAPIVersion)
	}
	if spec.BasePath != Prefix {
		t.Errorf("spec base path %q, want %q", spec.BasePath, Prefix)
	}

	h := newHandler(t, testOptions(t))
	r := h.(*router)
	served := map[string]bool{}
	for pattern, methods := range r.routes {
		path := strings.TrimPrefix(pattern, Prefix)
		for method := range methods {
			key := strings.ToLower(method) + " " + path
			served[key] = true
			if _, ok := spec.Paths[path][strings.ToLower(method)]; !ok {
				t.Errorf("the spec lacks %s %s: annotate its handler and run make swagger", method, path)
			}
		}
	}
	for path, ops := range spec.Paths {
		for method := range ops {
			if !served[method+" "+path] {
				t.Errorf("the spec has %s %s, which the API does not serve", strings.ToUpper(method), path)
			}
		}
	}
}
