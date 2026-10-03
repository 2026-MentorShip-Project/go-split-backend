package auth

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type tokenResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// PostToken godoc
// @Summary     Issue an API token
// @Description Creates a new session for the caller and returns its token in
// @Description the body instead of a cookie, for clients that authenticate
// @Description with "Authorization: Bearer <token>". Revoke it with
// @Description POST /auth/logout using the same header.
// @Tags        auth
// @Produce     json
// @Success     201  {object} tokenResponse
// @Failure     401  {object} errorResponse
// @Router      /auth/tokens [post]
func (h *Handler) PostToken(c *gin.Context) {
	token, expires, err := createSession(c.Request.Context(), h.DB, CurrentSubject(c))
	if err != nil {
		respondErr(c, http.StatusInternalServerError, "issue token")
		return
	}
	c.JSON(http.StatusCreated, tokenResponse{Token: token, ExpiresAt: expires})
}
