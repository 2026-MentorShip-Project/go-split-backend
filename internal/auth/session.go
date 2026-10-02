package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go-split-backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// SessionCookieName is the cookie the browser carries after login/join.
const SessionCookieName = "session"

// SessionTTL is how long a fresh session lives before the app rejects it.
const SessionTTL = 30 * 24 * time.Hour

// Subject identifies who a session belongs to.
type Subject struct {
	AccountID int64
	GuestID   int64
}

// IsAccount reports whether the subject is a registered account.
func (s Subject) IsAccount() bool { return s.AccountID != 0 }

// IsGuest reports whether the subject is a session-bound guest.
func (s Subject) IsGuest() bool { return s.GuestID != 0 }

// ErrNoSession is returned when the request has no valid session token.
var ErrNoSession = errors.New("no session")

// IssueSession creates a new session row and writes the cookie on the response.
func IssueSession(ctx context.Context, db database.Store, c *gin.Context, sub Subject) (string, error) {
	token, expires, err := createSession(ctx, db, sub)
	if err != nil {
		return "", err
	}
	setCookie(c, token, expires)
	return token, nil
}

// createSession inserts a session row for sub and returns its token and expiry.
func createSession(ctx context.Context, db database.Store, sub Subject) (string, time.Time, error) {
	token, err := newToken()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("generate session token: %w", err)
	}

	var accountID, guestID any
	switch {
	case sub.IsAccount() && !sub.IsGuest():
		accountID = sub.AccountID
	case sub.IsGuest() && !sub.IsAccount():
		guestID = sub.GuestID
	default:
		return "", time.Time{}, errors.New("session subject must be exactly one of account or guest")
	}

	expires := time.Now().Add(SessionTTL)
	if _, err := db.Exec(ctx,
		`INSERT INTO sessions (token, account_id, guest_id, expires_at) VALUES ($1, $2, $3, $4)`,
		token, accountID, guestID, expires); err != nil {
		return "", time.Time{}, fmt.Errorf("insert session: %w", err)
	}
	return token, expires, nil
}

// sessionToken returns the session cookie, falling back to an
// "Authorization: Bearer" header for non-browser clients such as the MCP server.
func sessionToken(c *gin.Context) string {
	if token, err := c.Cookie(SessionCookieName); err == nil && token != "" {
		return token
	}
	scheme, token, ok := strings.Cut(c.GetHeader("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}

// LookupSession returns the subject for the request's session token, or ErrNoSession.
func LookupSession(ctx context.Context, db database.Store, c *gin.Context) (Subject, error) {
	token := sessionToken(c)
	if token == "" {
		return Subject{}, ErrNoSession
	}
	var sub Subject
	err := db.QueryRow(ctx,
		`SELECT COALESCE(account_id, 0), COALESCE(guest_id, 0)
		   FROM sessions
		  WHERE token = $1 AND expires_at > NOW()`, token).
		Scan(&sub.AccountID, &sub.GuestID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Subject{}, ErrNoSession
	}
	if err != nil {
		return Subject{}, fmt.Errorf("lookup session: %w", err)
	}
	return sub, nil
}

// RevokeSession deletes the request's session and clears the cookie.
func RevokeSession(ctx context.Context, db database.Store, c *gin.Context) error {
	token := sessionToken(c)
	if token == "" {
		return nil
	}
	if _, err := db.Exec(ctx, `DELETE FROM sessions WHERE token = $1`, token); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	clearCookie(c)
	return nil
}

func newToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

func setCookie(c *gin.Context, token string, expires time.Time) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}
