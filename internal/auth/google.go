package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	log "github.com/sirupsen/logrus"
)

// GoogleTokenInfoURL is Google's ID-token introspection endpoint.
const GoogleTokenInfoURL = "https://oauth2.googleapis.com/tokeninfo"

// GoogleIdentity is what a verified Google ID token says about the user.
type GoogleIdentity struct {
	Sub   string
	Email string
	Name  string
}

// ErrInvalidGoogleToken means Google rejected the token or its claims do not
// match this application.
var ErrInvalidGoogleToken = errors.New("invalid google token")

// GoogleVerifier checks a Google ID token and returns the identity it carries.
type GoogleVerifier interface {
	Verify(ctx context.Context, idToken string) (GoogleIdentity, error)
}

type googleTokenInfoVerifier struct {
	clientID string
	endpoint string
	client   *http.Client
}

// NewGoogleVerifier returns a GoogleVerifier that asks Google's tokeninfo
// endpoint to validate tokens issued for clientID. It returns nil when
// clientID is empty so callers can treat Google sign-in as disabled.
func NewGoogleVerifier(clientID string) GoogleVerifier {
	return newGoogleVerifier(clientID, GoogleTokenInfoURL)
}

func newGoogleVerifier(clientID, endpoint string) GoogleVerifier {
	if clientID == "" {
		return nil
	}
	return &googleTokenInfoVerifier{
		clientID: clientID,
		endpoint: endpoint,
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

// tokenInfo mirrors the tokeninfo response; Google returns every claim as a
// string.
type tokenInfo struct {
	Iss           string `json:"iss"`
	Aud           string `json:"aud"`
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified string `json:"email_verified"`
	Name          string `json:"name"`
	Exp           string `json:"exp"`
	Error         string `json:"error_description"`
}

func (v *googleTokenInfoVerifier) Verify(ctx context.Context, idToken string) (GoogleIdentity, error) {
	endpoint := v.endpoint + "?" + url.Values{"id_token": {idToken}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return GoogleIdentity{}, fmt.Errorf("build tokeninfo request: %w", err)
	}

	resp, err := v.client.Do(req)
	if err != nil {
		// net/http wraps transport failures with the request URL, which
		// contains the ID token. Keep the cause without logging that URL.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return GoogleIdentity{}, fmt.Errorf("call tokeninfo: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusBadRequest {
		return GoogleIdentity{}, fmt.Errorf("tokeninfo returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return GoogleIdentity{}, fmt.Errorf("read tokeninfo response: %w", err)
	}
	var info tokenInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return GoogleIdentity{}, fmt.Errorf("decode tokeninfo response (status=%d, content-type=%q): %w", resp.StatusCode, resp.Header.Get("Content-Type"), err)
	}
	switch {
	case resp.StatusCode == http.StatusBadRequest:
		return GoogleIdentity{}, fmt.Errorf("%w: %s", ErrInvalidGoogleToken, info.Error)
	case resp.StatusCode != http.StatusOK:
		return GoogleIdentity{}, fmt.Errorf("tokeninfo returned %d", resp.StatusCode)
	}
	return v.checkClaims(info)
}

func (v *googleTokenInfoVerifier) checkClaims(info tokenInfo) (GoogleIdentity, error) {
	if info.Aud != v.clientID {
		return GoogleIdentity{}, fmt.Errorf("%w: audience mismatch", ErrInvalidGoogleToken)
	}
	if info.Iss != "accounts.google.com" && info.Iss != "https://accounts.google.com" {
		return GoogleIdentity{}, fmt.Errorf("%w: unexpected issuer", ErrInvalidGoogleToken)
	}
	exp, err := strconv.ParseInt(info.Exp, 10, 64)
	if err != nil || time.Now().Unix() >= exp {
		return GoogleIdentity{}, fmt.Errorf("%w: expired", ErrInvalidGoogleToken)
	}
	if info.Sub == "" || info.Email == "" || info.EmailVerified != "true" {
		return GoogleIdentity{}, fmt.Errorf("%w: email not verified", ErrInvalidGoogleToken)
	}
	return GoogleIdentity{Sub: info.Sub, Email: info.Email, Name: info.Name}, nil
}

type googleLoginRequest struct {
	IDToken string `json:"id_token" binding:"required,min=1"`
}

// PostGoogle godoc
// @Summary     Sign in an account with a Google ID token
// @Description Validate the ID token the frontend obtained from Google
// @Description Sign-In, then create or link the account for that Google
// @Description identity and set a session cookie. Returns 503 when the server
// @Description has no GOOGLE_CLIENT_ID configured.
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       body body     googleLoginRequest true "Google ID token"
// @Success     200  {object} accountResponse
// @Failure     400  {object} errorResponse
// @Failure     401  {object} errorResponse
// @Failure     503  {object} errorResponse
// @Router      /auth/google [post]
func (h *Handler) PostGoogle(c *gin.Context) {
	if h.Google == nil {
		respondErr(c, http.StatusServiceUnavailable, "google sign-in not configured")
		return
	}
	var req googleLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondErr(c, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}

	ctx := c.Request.Context()
	identity, err := h.Google.Verify(ctx, req.IDToken)
	if err != nil {
		if errors.Is(err, ErrInvalidGoogleToken) {
			log.WithContext(ctx).WithError(err).Info("reject google token")
			respondErr(c, http.StatusUnauthorized, "invalid google token")
			return
		}
		log.WithContext(ctx).WithError(err).Warn("verify google token upstream failure")
		respondErr(c, http.StatusBadGateway, "verify google token")
		return
	}

	account, err := upsertGoogleAccount(ctx, h.DB, identity)
	if err != nil {
		log.WithContext(ctx).WithError(err).WithField("google_sub", identity.Sub).Error("upsert google account")
		respondErr(c, http.StatusInternalServerError, "create account")
		return
	}
	if _, err := IssueSession(ctx, h.DB, c, Subject{AccountID: account.id}); err != nil {
		log.WithContext(ctx).WithError(err).WithField("account_id", account.id).Error("issue google session")
		respondErr(c, http.StatusInternalServerError, "issue session")
		return
	}
	c.JSON(http.StatusOK, accountResponse{ID: account.id, Name: account.name, Email: account.email})
}

// upsertGoogleAccount finds the account by Google subject, otherwise links the
// Google identity to an existing account with the same email, otherwise creates
// a password-less account.
func upsertGoogleAccount(ctx context.Context, db *pgxpool.Pool, id GoogleIdentity) (accountRow, error) {
	var h accountRow
	err := db.QueryRow(ctx,
		`SELECT id, name, email FROM accounts WHERE google_sub = $1`, id.Sub).
		Scan(&h.id, &h.name, &h.email)
	if err == nil {
		return h, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return accountRow{}, err
	}

	err = db.QueryRow(ctx,
		`UPDATE accounts SET google_sub = $1 WHERE email = $2 AND google_sub IS NULL
		 RETURNING id, name, email`, id.Sub, id.Email).
		Scan(&h.id, &h.name, &h.email)
	if err == nil {
		return h, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return accountRow{}, err
	}

	name := id.Name
	if name == "" {
		name, _, _ = strings.Cut(id.Email, "@")
	}
	err = db.QueryRow(ctx,
		`INSERT INTO accounts (name, email, google_sub) VALUES ($1, $2, $3)
		 RETURNING id, name, email`, name, id.Email, id.Sub).
		Scan(&h.id, &h.name, &h.email)
	return h, err
}
