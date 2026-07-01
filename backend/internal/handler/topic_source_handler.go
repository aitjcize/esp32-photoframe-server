package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// TopicSourceBackend is the surface the topic-source handler needs. All three
// topic services (Unsplash, Pexels) satisfy it.
type TopicSourceBackend interface {
	SetSyncTopics(topics []string) error
	TestConnection() error
}

// TopicSourceHandler serves the topic-management endpoint shared by the
// search-topic sources. Listing topics reuses the generic GET /albums?source=,
// and sync/count/clear reuse PhotoSyncHandler.
type TopicSourceHandler struct {
	svc TopicSourceBackend
}

func NewTopicSourceHandler(svc TopicSourceBackend) *TopicSourceHandler {
	return &TopicSourceHandler{svc: svc}
}

type setSyncTopicsRequest struct {
	Topics []string `json:"topics"`
}

// SetSyncTopics replaces the source's synced topic set.
func (h *TopicSourceHandler) SetSyncTopics(c echo.Context) error {
	var req setSyncTopicsRequest
	if err := c.Bind(&req); err != nil {
		return respondError(c, http.StatusBadRequest, err.Error())
	}
	if err := h.svc.SetSyncTopics(req.Topics); err != nil {
		return respondError(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// TestConnection validates the source's API key (a trivial search). Returns 400
// with the error message if the key is missing or rejected.
func (h *TopicSourceHandler) TestConnection(c echo.Context) error {
	if err := h.svc.TestConnection(); err != nil {
		return respondError(c, http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
