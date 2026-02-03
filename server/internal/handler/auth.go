package handler

import (
	"net/http"

	"github.com/aitjcize/photoframe-server/server/internal/service"
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
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	token, err := h.authService.Login(req.Username, req.Password, c.Request().UserAgent(), c.RealIP())
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]string{"token": token})
}

func (h *AuthHandler) Register(c echo.Context) error {
	// Check if initialization is allowed
	count, err := h.authService.UserCount()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "database error"})
	}

	// Only allow registration if no users exist
	if count > 0 {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "setup already completed"})
	}

	var req loginRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	if req.Username == "" || req.Password == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "username and password required"})
	}

	if err := h.authService.Register(req.Username, req.Password); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	// Auto-login after register
	token, _ := h.authService.Login(req.Username, req.Password, c.Request().UserAgent(), c.RealIP())

	return c.JSON(http.StatusOK, map[string]string{"message": "user created", "token": token})
}

func (h *AuthHandler) GetStatus(c echo.Context) error {
	count, err := h.authService.UserCount()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "database error"})
	}

	return c.JSON(http.StatusOK, map[string]bool{
		"initialized": count > 0,
	})
}

func (h *AuthHandler) GenerateDeviceToken(c echo.Context) error {
	userID, ok := c.Get("user_id").(uint)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "user id not found in context"})
	}
	username, ok := c.Get("username").(string)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "username not found in context"})
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}
	if req.Name == "" {
		req.Name = "Device Token"
	}

	token, err := h.authService.GenerateDeviceToken(userID, username, req.Name)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to generate token"})
	}

	return c.JSON(http.StatusOK, map[string]string{"token": token})
}

func (h *AuthHandler) ListTokens(c echo.Context) error {
	userID, ok := c.Get("user_id").(uint)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "user id not found in context"})
	}

	tokens, err := h.authService.ListTokens(userID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list tokens"})
	}

	return c.JSON(http.StatusOK, tokens)
}

func (h *AuthHandler) RevokeToken(c echo.Context) error {
	userID, ok := c.Get("user_id").(uint)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "user id not found in context"})
	}

	var req struct {
		ID uint `param:"id"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	if err := h.authService.RevokeToken(userID, req.ID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to revoke token"})
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "token revoked"})
}

func (h *AuthHandler) ListSessions(c echo.Context) error {
	userID, ok := c.Get("user_id").(uint)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "user id not found in context"})
	}

	sessions, err := h.authService.ListSessions(userID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list sessions"})
	}

	return c.JSON(http.StatusOK, sessions)
}

func (h *AuthHandler) RevokeSession(c echo.Context) error {
	userID, ok := c.Get("user_id").(uint)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "user id not found in context"})
	}

	var req struct {
		ID uint `param:"id"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	if err := h.authService.RevokeSession(userID, req.ID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to revoke session"})
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "session revoked"})
}

func (h *AuthHandler) UpdateAccount(c echo.Context) error {
	userID, ok := c.Get("user_id").(uint)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "user id not found in context"})
	}

	var req struct {
		OldPassword string `json:"old_password"`
		NewUsername string `json:"new_username"`
		NewPassword string `json:"new_password"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	if req.OldPassword == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "current password is required"})
	}

	if err := h.authService.UpdateAccount(userID, req.OldPassword, req.NewUsername, req.NewPassword); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "account updated successfully"})
}
