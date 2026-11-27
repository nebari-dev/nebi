package api

import (
	"context"
	"net/http"
)

// FolderPicker shows the operating system's folder dialog. The desktop app
// implements it with its native dialog; nebi-web has none, because the user
// brings their own browser. The client UI asks GET /capabilities whether to
// show its "choose folder" button.
type FolderPicker interface {
	// PickFolder shows the dialog and returns the absolute path of the
	// chosen folder, or "" with a nil error when the user cancels.
	PickFolder(ctx context.Context) (string, error)
}

// Window is the desktop app's window.
type Window interface {
	// Show brings the window to the front, unminimizing it if needed. A
	// second desktop app started for the same database calls it, through
	// POST /desktop/show-window, before it exits.
	Show(ctx context.Context) error
}

// desktopRoutes registers the routes only the desktop app serves. Without a
// folder picker or a window they answer 404.
func desktopRoutes(r *router, opts *Options) {
	h := desktopHandlers{picker: opts.FolderPicker, window: opts.Window}
	r.handle(http.MethodPost, "/desktop/pick-folder", h.pickFolder)
	r.handle(http.MethodPost, "/desktop/show-window", h.showWindow)
}

type desktopHandlers struct {
	picker FolderPicker
	window Window
}

// PickFolderResponse is the body of a POST /desktop/pick-folder that chose
// a folder.
type PickFolderResponse struct {
	// Path is the absolute path of the chosen folder.
	Path string `json:"path" example:"/home/me/nebi-projects/data-science"`
}

// pickFolder godoc
//
//	@Summary		Choose a folder with the native dialog
//	@Description	Shows the desktop app's native folder dialog and returns the chosen folder. Answers 204 when the user cancels, and 404 when this process has no folder picker (nebi-web); GET /capabilities tells which.
//	@Tags			desktop
//	@Produce		json
//	@Security		SessionToken
//	@Success		200	{object}	PickFolderResponse
//	@Success		204	"The user cancelled the dialog"
//	@Failure		401	{object}	ErrorResponse
//	@Failure		404	{object}	ErrorResponse	"folder_picker_unavailable"
//	@Router			/desktop/pick-folder [post]
func (h desktopHandlers) pickFolder(w http.ResponseWriter, r *http.Request) error {
	if h.picker == nil {
		return &Error{Status: http.StatusNotFound, Code: CodeFolderPickerUnavailable,
			Message: "this process has no folder picker; only the desktop app has one"}
	}
	path, err := h.picker.PickFolder(r.Context())
	if err != nil {
		return err
	}
	if path == "" {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	writeJSON(w, http.StatusOK, PickFolderResponse{Path: path})
	return nil
}

// showWindow godoc
//
//	@Summary		Bring the desktop window to the front
//	@Description	Shows and raises the desktop app's window. A desktop app that starts while another one owns the same database calls it before it exits. Answers 404 when this process has no window (nebi-web).
//	@Tags			desktop
//	@Security		SessionToken
//	@Success		204	"The window was raised"
//	@Failure		401	{object}	ErrorResponse
//	@Failure		404	{object}	ErrorResponse	"window_unavailable"
//	@Router			/desktop/show-window [post]
func (h desktopHandlers) showWindow(w http.ResponseWriter, r *http.Request) error {
	if h.window == nil {
		return &Error{Status: http.StatusNotFound, Code: CodeWindowUnavailable,
			Message: "this process has no window; only the desktop app has one"}
	}
	if err := h.window.Show(r.Context()); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
