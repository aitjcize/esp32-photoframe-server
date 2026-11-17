package handler

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/service"
)

// inUseCode marks a 409 for removing something a frame is bound to (a
// connection, an album, a topic). The body names the frames so the dashboard
// can show them: {"error", "code": "in_use", "devices": [{id, name}]}.
const inUseCode = "in_use"

// respondInUse answers an InUseError, or returns false for any other error.
func respondInUse(c echo.Context, err error) (bool, error) {
	var inUse *service.InUseError
	if !errors.As(err, &inUse) {
		return false, nil
	}
	devices := make([]map[string]any, 0, len(inUse.Devices))
	for i, d := range inUse.Devices {
		devices = append(devices, map[string]any{"id": d.ID, "name": inUse.DeviceNames()[i]})
	}
	return true, c.JSON(http.StatusConflict, map[string]any{
		"error": inUse.Error(), "code": inUseCode, "devices": devices,
	})
}
