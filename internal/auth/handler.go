package auth

import (
	"errors"
	"net/http"
	"strings"

	"go-split-backend/internal/database"

	"github.com/gin-gonic/gin"
)

// Handler bundles the auth endpoints with the database pool they need.
type Handler struct {
	DB database.Store
	// Google verifies Google ID tokens for POST /auth/google; nil disables it.
	Google GoogleVerifier
}

// New returns a Handler bound to the given pool and Google verifier.
func New(db database.Store, google GoogleVerifier) *Handler {
	return &Handler{DB: db, Google: google}
}

// Register wires every /auth/* endpoint onto the given router.
func (h *Handler) Register(r gin.IRouter) {
	g := r.Group("/auth")
	g.POST("/google", h.PostGoogle)
	h.registerPasswordRoutes(g)
	g.DELETE("/", RequireSession(h.DB), h.DeleteAccount)
	g.POST("/tokens", RequireSession(h.DB), h.PostToken)
	h.registerGuestRoutes(g)
}

type accountResponse struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// DeleteAccount godoc
// @Summary     Delete the current account
// @Description Delete the account belonging to the current account session.
// @Tags        auth
// @Success     204
// @Failure     401  {object} errorResponse
// @Failure     403  {object} errorResponse
// @Router      /auth/ [delete]
func (h *Handler) DeleteAccount(c *gin.Context) {
	accountID, err := accountIDForDeletion(CurrentSubject(c))
	if err != nil {
		respondErr(c, http.StatusForbidden, err.Error())
		return
	}
	if _, err := h.DB.Exec(c.Request.Context(), `DELETE FROM accounts WHERE id = $1`, accountID); err != nil {
		respondErr(c, http.StatusInternalServerError, "delete account")
		return
	}
	c.Status(http.StatusNoContent)
}

func accountIDForDeletion(sub Subject) (int64, error) {
	if !sub.IsAccount() || sub.IsGuest() {
		return 0, errors.New("only account users can delete an account")
	}
	return sub.AccountID, nil
}

type accountRow struct {
	id    int64
	name  string
	email string
}

func isUniqueViolation(err error) bool {
	// pgx wraps a *pgconn.PgError; a Postgres unique_violation is SQLSTATE 23505.
	// Match on the substring to avoid pulling in pgconn just for the code.
	return err != nil && strings.Contains(err.Error(), "SQLSTATE 23505")
}

type errorResponse struct {
	Error string `json:"error"`
}

func respondErr(c *gin.Context, status int, msg string) {
	c.JSON(status, errorResponse{Error: msg})
}
