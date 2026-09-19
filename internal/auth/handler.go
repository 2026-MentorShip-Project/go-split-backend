package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// Handler bundles the auth endpoints with the database pool they need.
type Handler struct {
	DB *pgxpool.Pool
	// Google verifies Google ID tokens for POST /auth/google; nil disables it.
	Google GoogleVerifier
}

// New returns a Handler bound to the given pool and Google verifier.
func New(db *pgxpool.Pool, google GoogleVerifier) *Handler {
	return &Handler{DB: db, Google: google}
}

// Register wires every /auth/* endpoint onto the given router.
func (h *Handler) Register(r gin.IRouter) {
	g := r.Group("/auth")
	g.POST("/register", h.PostRegister)
	g.POST("/login", h.PostLogin)
	g.POST("/google", h.PostGoogle)
	g.DELETE("/", RequireSession(h.DB), h.DeleteAccount)
	h.registerGuestRoutes(g)
}

type registerRequest struct {
	Name     string `json:"name"     binding:"required,min=1,max=64"`
	Email    string `json:"email"    binding:"required,email"`
	Password string `json:"password" binding:"required,min=8,max=128"`
}

type accountResponse struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// PostRegister godoc
// @Summary     Register an account
// @Description Create an account with name, email, and password. Sets a
// @Description session cookie on success. Accounts can create events;
// @Description co-organizers and participants join through /auth/join.
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
// @Summary     Log in an account
// @Description Verify the account's email + password, then set a session cookie.
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
	account, err := loadAccountByEmail(ctx, h.DB, req.Email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondErr(c, http.StatusUnauthorized, "invalid email or password")
			return
		}
		respondErr(c, http.StatusInternalServerError, "load account")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(account.passwordHash), []byte(req.Password)); err != nil {
		respondErr(c, http.StatusUnauthorized, "invalid email or password")
		return
	}

	if _, err := IssueSession(ctx, h.DB, c, Subject{AccountID: account.id}); err != nil {
		respondErr(c, http.StatusInternalServerError, "issue session")
		return
	}
	c.JSON(http.StatusOK, accountResponse{ID: account.id, Name: account.name, Email: account.email})
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
	id           int64
	name         string
	email        string
	passwordHash string
}

func loadAccountByEmail(ctx context.Context, db *pgxpool.Pool, email string) (accountRow, error) {
	var h accountRow
	err := db.QueryRow(ctx,
		`SELECT id, name, email, COALESCE(password_hash, '') FROM accounts WHERE email = $1`, email).
		Scan(&h.id, &h.name, &h.email, &h.passwordHash)
	return h, err
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
