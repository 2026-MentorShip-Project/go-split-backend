package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SessionCookieName is the cookie the browser carries after login/join.
const SessionCookieName = "session"

// SessionTTL is how long a fresh session lives before the app rejects it.
const SessionTTL = 30 * 24 * time.Hour

// Subject identifies who a session belongs to.
type Subject struct {
	HostID  int64
	GuestID int64
}

// IsHost reports whether the subject is a host account.
func (s Subject) IsHost() bool { return s.HostID != 0 }

// IsGuest reports whether the subject is a session-bound guest.
func (s Subject) IsGuest() bool { return s.GuestID != 0 }

// ErrNoSession is returned when the request has no valid session cookie.
var ErrNoSession = errors.New("no session")

// IssueSession creates a new session row and writes the cookie on the response.
func IssueSession(ctx context.Context, db *pgxpool.Pool, c *gin.Context, sub Subject) (string, error) {
	token, err := newToken()
	if err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}

	var hostID, guestID any
	switch {
	case sub.IsHost() && !sub.IsGuest():
		hostID = sub.HostID
	case sub.IsGuest() && !sub.IsHost():
		guestID = sub.GuestID
	default:
		return "", errors.New("session subject must be exactly one of host or guest")
	}

	expires := time.Now().Add(SessionTTL)
	if _, err := db.Exec(ctx,
		`INSERT INTO sessions (token, host_id, guest_id, expires_at) VALUES ($1, $2, $3, $4)`,
		token, hostID, guestID, expires); err != nil {
		return "", fmt.Errorf("insert session: %w", err)
	}

	setCookie(c, token, expires)
	return token, nil
}

// LookupSession returns the subject for the request's cookie, or ErrNoSession.
func LookupSession(ctx context.Context, db *pgxpool.Pool, c *gin.Context) (Subject, error) {
	token, err := c.Cookie(SessionCookieName)
	if err != nil || token == "" {
		return Subject{}, ErrNoSession
	}
	var sub Subject
	err = db.QueryRow(ctx,
		`SELECT COALESCE(host_id, 0), COALESCE(guest_id, 0)
		   FROM sessions
		  WHERE token = $1 AND expires_at > NOW()`, token).
		Scan(&sub.HostID, &sub.GuestID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Subject{}, ErrNoSession
	}
	if err != nil {
		return Subject{}, fmt.Errorf("lookup session: %w", err)
	}
	return sub, nil
}

// RevokeSession deletes the request's session and clears the cookie.
func RevokeSession(ctx context.Context, db *pgxpool.Pool, c *gin.Context) error {
	token, err := c.Cookie(SessionCookieName)
	if err != nil || token == "" {
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
		SameSite: http.SameSiteNoneMode,
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
		SameSite: http.SameSiteNoneMode,
	})
}
