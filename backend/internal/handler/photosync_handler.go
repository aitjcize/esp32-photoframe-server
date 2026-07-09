package handler

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
)

// respondError writes a JSON {"error": msg} response with the given status.
// It then returns an error carrying msg so the access-log ${error} field
// records it; since the response is already committed, Echo's error handler
// won't write a second body.
func respondError(c echo.Context, status int, msg string) error {
	if err := c.JSON(status, map[string]string{"error": msg}); err != nil {
		return err
	}
	return errors.New(msg)
}

// PhotoSyncBackend is the per-source service surface the generic photo-sync
// endpoints need. Both ImmichService and SynologyService satisfy it.
type PhotoSyncBackend interface {
	ClearAndResync() error
	IsSyncing() bool
	ClearPhotos() error
	GetPhotoCount() (int64, error)
}

// PhotoSyncHandler serves the endpoints that are identical across photo sources
// (sync / sync-status / clear / count), replacing the duplicated per-source
// handler methods.
type PhotoSyncHandler struct {
	svc PhotoSyncBackend
}

func NewPhotoSyncHandler(svc PhotoSyncBackend) *PhotoSyncHandler {
	return &PhotoSyncHandler{svc: svc}
}

// Sync triggers a clear+resync (runs in the background) and returns promptly.
func (h *PhotoSyncHandler) Sync(c echo.Context) error {
	if err := h.svc.ClearAndResync(); err != nil {
		return respondError(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "synced"})
}

// SyncStatus reports whether a sync is currently in progress.
func (h *PhotoSyncHandler) SyncStatus(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]bool{"running": h.svc.IsSyncing()})
}

// Clear removes all of the source's photos from the local DB.
func (h *PhotoSyncHandler) Clear(c echo.Context) error {
	if err := h.svc.ClearPhotos(); err != nil {
		return respondError(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "cleared"})
}

// Count returns the number of locally-stored photos for the source.
func (h *PhotoSyncHandler) Count(c echo.Context) error {
	count, err := h.svc.GetPhotoCount()
	if err != nil {
		return respondError(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"count": count})
}
