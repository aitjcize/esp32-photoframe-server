package handler

import (
	"net/http"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/service"
	"github.com/labstack/echo/v4"
)

type AuthHandler struct {
	authService *service.AuthService
}

func NewAuthHandler(authService *service.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *AuthHandler) Login(c echo.Context) error {
	var req loginRequest
	if err := c.Bind(&req); err != nil {
		return respondError(c, http.StatusBadRequest, "invalid request")
	}

	token, err := h.authService.Login(req.Username, req.Password, c.Request().UserAgent(), c.RealIP())
	if err != nil {
		return respondError(c, http.StatusUnauthorized, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]string{"token": token})
}

func (h *AuthHandler) Register(c echo.Context) error {
	// Check if initialization is allowed
	count, err := h.authService.UserCount()
	if err != nil {
		return respondError(c, http.StatusInternalServerError, "database error")
	}

	// Only allow registration if no users exist
	if count > 0 {
		return respondError(c, http.StatusForbidden, "setup already completed")
	}

	var req loginRequest
	if err := c.Bind(&req); err != nil {
		return respondError(c, http.StatusBadRequest, "invalid request")
	}

	if req.Username == "" || req.Password == "" {
		return respondError(c, http.StatusBadRequest, "username and password required")
	}

	if err := h.authService.Register(req.Username, req.Password); err != nil {
		return respondError(c, http.StatusInternalServerError, err.Error())
	}

	// Auto-login after register
	token, _ := h.authService.Login(req.Username, req.Password, c.Request().UserAgent(), c.RealIP())

	return c.JSON(http.StatusOK, map[string]string{"message": "user created", "token": token})
}

func (h *AuthHandler) GetStatus(c echo.Context) error {
	count, err := h.authService.UserCount()
	if err != nil {
		return respondError(c, http.StatusInternalServerError, "database error")
	}

	return c.JSON(http.StatusOK, map[string]bool{
		"initialized": count > 0,
	})
}

func (h *AuthHandler) GenerateDeviceToken(c echo.Context) error {
	userID, ok := c.Get("user_id").(uint)
	if !ok {
		return respondError(c, http.StatusUnauthorized, "user id not found in context")
	}
	username, ok := c.Get("username").(string)
	if !ok {
		return respondError(c, http.StatusUnauthorized, "username not found in context")
	}

	var req struct {
		Name     string `json:"name"`
		DeviceID *uint  `json:"device_id"`
	}
	if err := c.Bind(&req); err != nil {
		return respondError(c, http.StatusBadRequest, "invalid request")
	}
	if req.Name == "" {
		req.Name = "Device Token"
	}

	token, err := h.authService.GenerateDeviceToken(userID, username, req.Name, req.DeviceID)
	if err != nil {
		return respondError(c, http.StatusInternalServerError, "failed to generate token")
	}

	return c.JSON(http.StatusOK, map[string]string{"token": token})
}

func (h *AuthHandler) UpdateTokenDevice(c echo.Context) error {
	userID, ok := c.Get("user_id").(uint)
	if !ok {
		return respondError(c, http.StatusUnauthorized, "user id not found in context")
	}

	var req struct {
		ID       uint  `param:"id"`
		DeviceID *uint `json:"device_id"`
	}
	if err := c.Bind(&req); err != nil {
		return respondError(c, http.StatusBadRequest, "invalid request")
	}

	if err := h.authService.UpdateTokenDevice(userID, req.ID, req.DeviceID); err != nil {
		return respondError(c, http.StatusInternalServerError, "failed to update token")
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "token updated"})
}

func (h *AuthHandler) ListTokens(c echo.Context) error {
	userID, ok := c.Get("user_id").(uint)
	if !ok {
		return respondError(c, http.StatusUnauthorized, "user id not found in context")
	}

	tokens, err := h.authService.ListTokens(userID)
	if err != nil {
		return respondError(c, http.StatusInternalServerError, "failed to list tokens")
	}

	return c.JSON(http.StatusOK, tokens)
}

func (h *AuthHandler) RevokeToken(c echo.Context) error {
	userID, ok := c.Get("user_id").(uint)
	if !ok {
		return respondError(c, http.StatusUnauthorized, "user id not found in context")
	}

	var req struct {
		ID uint `param:"id"`
	}
	if err := c.Bind(&req); err != nil {
		return respondError(c, http.StatusBadRequest, "invalid request")
	}

	if err := h.authService.RevokeToken(userID, req.ID); err != nil {
		return respondError(c, http.StatusInternalServerError, "failed to revoke token")
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "token revoked"})
}

func (h *AuthHandler) ListSessions(c echo.Context) error {
	userID, ok := c.Get("user_id").(uint)
	if !ok {
		return respondError(c, http.StatusUnauthorized, "user id not found in context")
	}

	sessions, err := h.authService.ListSessions(userID)
	if err != nil {
		return respondError(c, http.StatusInternalServerError, "failed to list sessions")
	}

	return c.JSON(http.StatusOK, sessions)
}

func (h *AuthHandler) RevokeSession(c echo.Context) error {
	userID, ok := c.Get("user_id").(uint)
	if !ok {
		return respondError(c, http.StatusUnauthorized, "user id not found in context")
	}

	var req struct {
		ID uint `param:"id"`
	}
	if err := c.Bind(&req); err != nil {
		return respondError(c, http.StatusBadRequest, "invalid request")
	}

	if err := h.authService.RevokeSession(userID, req.ID); err != nil {
		return respondError(c, http.StatusInternalServerError, "failed to revoke session")
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "session revoked"})
}

func (h *AuthHandler) UpdateAccount(c echo.Context) error {
	userID, ok := c.Get("user_id").(uint)
	if !ok {
		return respondError(c, http.StatusUnauthorized, "user id not found in context")
	}

	var req struct {
		OldPassword string `json:"old_password"`
		NewUsername string `json:"new_username"`
		NewPassword string `json:"new_password"`
	}
	if err := c.Bind(&req); err != nil {
		return respondError(c, http.StatusBadRequest, "invalid request")
	}

	if req.OldPassword == "" {
		return respondError(c, http.StatusBadRequest, "current password is required")
	}

	if err := h.authService.UpdateAccount(userID, req.OldPassword, req.NewUsername, req.NewPassword); err != nil {
		return respondError(c, http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "account updated successfully"})
}
