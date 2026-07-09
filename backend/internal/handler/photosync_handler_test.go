package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	echoMiddleware "github.com/labstack/echo/v4/middleware"
)

// respondError must both send {"error": msg} to the client and surface msg in
// the access log's ${error} field. It previously returned nil to Echo, so 500s
// were logged with "error":"".
func TestRespondErrorPopulatesAccessLog(t *testing.T) {
	e := echo.New()
	var logBuf bytes.Buffer
	e.Use(echoMiddleware.LoggerWithConfig(echoMiddleware.LoggerConfig{
		Format: `{"status":${status},"error":"${error}"}` + "\n",
		Output: &logBuf,
	}))
	e.GET("/boom", func(c echo.Context) error {
		return respondError(c, http.StatusInternalServerError, "failed to fetch photo: device offline")
	})

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if body, want := strings.TrimSpace(rec.Body.String()), `{"error":"failed to fetch photo: device offline"}`; body != want {
		t.Fatalf("body = %q, want %q", body, want)
	}
	if log := logBuf.String(); !strings.Contains(log, `"error":"failed to fetch photo: device offline"`) {
		t.Fatalf("access log missing error message: %s", log)
	}
}
