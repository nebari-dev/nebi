package api

import "net/http"

// systemRoutes registers the routes that describe the serving process.
func systemRoutes(r *router, opts *Options) {
	h := systemHandlers{version: opts.Version, folderPicker: opts.FolderPicker != nil}
	r.handle(http.MethodGet, "/version", h.getVersion)
	r.handle(http.MethodGet, "/capabilities", h.getCapabilities)
}

type systemHandlers struct {
	version      VersionInfo
	folderPicker bool
}

// VersionResponse is the body of GET /version.
type VersionResponse struct {
	// NebiVersion is the nebi release of the serving process.
	NebiVersion string `json:"nebi_version" example:"0.15.0"`
	// Commit is the git commit it was built from, when known.
	Commit string `json:"commit,omitempty" example:"3f031d03"`
	// ClientAPIVersion is the version of this API, as major.minor.
	ClientAPIVersion string `json:"client_api_version" example:"1.0"`
	// Mode is always "local".
	Mode string `json:"mode" example:"local"`
}

// getVersion godoc
//
//	@Summary		Get the version of the serving process
//	@Description	Returns the nebi version, the client API version and the runtime mode. It is the only route that needs no session token, and it returns nothing secret.
//	@Tags			system
//	@Produce		json
//	@Success		200	{object}	VersionResponse
//	@Router			/version [get]
func (h systemHandlers) getVersion(w http.ResponseWriter, _ *http.Request) error {
	writeJSON(w, http.StatusOK, VersionResponse{
		NebiVersion:      h.version.NebiVersion,
		Commit:           h.version.Commit,
		ClientAPIVersion: ClientAPIVersion,
		Mode:             Mode,
	})
	return nil
}

// CapabilitiesResponse is the body of GET /capabilities.
type CapabilitiesResponse struct {
	// FolderPicker reports whether POST /desktop/pick-folder can show a
	// native folder dialog. Only the desktop app has one.
	FolderPicker bool `json:"folder_picker" example:"true"`
}

// getCapabilities godoc
//
//	@Summary		Get the capabilities of the serving process
//	@Description	Reports what this process can do beyond the common API, such as showing a native folder dialog, so a client knows which controls to show.
//	@Tags			system
//	@Produce		json
//	@Security		SessionToken
//	@Success		200	{object}	CapabilitiesResponse
//	@Failure		401	{object}	ErrorResponse
//	@Router			/capabilities [get]
func (h systemHandlers) getCapabilities(w http.ResponseWriter, _ *http.Request) error {
	writeJSON(w, http.StatusOK, CapabilitiesResponse{FolderPicker: h.folderPicker})
	return nil
}
