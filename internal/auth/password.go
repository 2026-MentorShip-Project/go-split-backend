package auth

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

// registerPasswordRoutes wires email + password sign-in. It exists for easy
// manual testing; the product flow signs accounts in with Google.
func (h *Handler) registerPasswordRoutes(g *gin.RouterGroup) {
	g.POST("/register", h.PostRegister)
	g.POST("/login", h.PostLogin)
}

type registerRequest struct {
	Name     string `json:"name"     binding:"required,min=1,max=64"`
	Email    string `json:"email"    binding:"required,email"`
	Password string `json:"password" binding:"required,min=8,max=72"`
}

// PostRegister godoc
// @Summary     Register an account with a password
// @Description Create an account with name, email, and password, then set a
// @Description session cookie. Intended for testing; the product uses /auth/google.
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       body body     registerRequest true "Account registration"
// @Success     201  {object} accountResponse
// @Failure     400  {object} errorResponse
// @Failure     409  {object} errorResponse
// @Router      /auth/register [post]
func (h *Handler) PostRegister(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondErr(c, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	req.Name = strings.TrimSpace(req.Name)

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		respondErr(c, http.StatusInternalServerError, "hash password")
		return
	}

	ctx := c.Request.Context()
	var id int64
	err = h.DB.QueryRow(ctx,
		`INSERT INTO accounts (name, email, password_hash)
		   VALUES ($1, $2, $3)
		RETURNING id`, req.Name, req.Email, string(hash)).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			respondErr(c, http.StatusConflict, "email already registered")
			return
		}
		respondErr(c, http.StatusInternalServerError, "create account")
		return
	}

	if _, err := IssueSession(ctx, h.DB, c, Subject{AccountID: id}); err != nil {
		respondErr(c, http.StatusInternalServerError, "issue session")
		return
	}
	c.JSON(http.StatusCreated, accountResponse{ID: id, Name: req.Name, Email: req.Email})
}

type loginRequest struct {
	Email    string `json:"email"    binding:"required,email"`
	Password string `json:"password" binding:"required,min=1"`
}

// PostLogin godoc
// @Summary     Log in with a password
// @Description Verify email + password, then set a session cookie. Accounts
// @Description created through Google have no password and are rejected.
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       body body     loginRequest true "Account login"
// @Success     200  {object} accountResponse
// @Failure     400  {object} errorResponse
// @Failure     401  {object} errorResponse
// @Router      /auth/login [post]
func (h *Handler) PostLogin(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondErr(c, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}
	req.Email = strings.TrimSpace(req.Email)

	ctx := c.Request.Context()
	var (
		account accountRow
		hash    string
	)
	err := h.DB.QueryRow(ctx,
		`SELECT id, name, email, COALESCE(password_hash, '') FROM accounts WHERE email = $1`, req.Email).
		Scan(&account.id, &account.name, &account.email, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		respondErr(c, http.StatusUnauthorized, "invalid email or password")
		return
	}
	if err != nil {
		respondErr(c, http.StatusInternalServerError, "load account")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
		respondErr(c, http.StatusUnauthorized, "invalid email or password")
		return
	}

	if _, err := IssueSession(ctx, h.DB, c, Subject{AccountID: account.id}); err != nil {
		respondErr(c, http.StatusInternalServerError, "issue session")
		return
	}
	c.JSON(http.StatusOK, accountResponse{ID: account.id, Name: account.name, Email: account.email})
}
