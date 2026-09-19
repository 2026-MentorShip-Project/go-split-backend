package auth

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"go-split-backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

const (
	subjectCtxKey       = "auth.subject"
	eventRoleCtxKey     = "auth.event_role"
	eventMemberIDCtxKey = "auth.event_member_id"
)

// RequireSession returns a middleware that rejects any request without a
// valid session cookie and attaches the resolved Subject to the context.
func RequireSession(db database.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		sub, err := LookupSession(c.Request.Context(), db, c)
		if err != nil {
			if errors.Is(err, ErrNoSession) {
				c.AbortWithStatusJSON(http.StatusUnauthorized, errorResponse{Error: "not signed in"})
				return
			}
			c.AbortWithStatusJSON(http.StatusInternalServerError, errorResponse{Error: "session lookup failed"})
			return
		}
		c.Set(subjectCtxKey, sub)
		c.Next()
	}
}

// CurrentSubject returns the Subject attached by RequireSession, or an empty
// Subject if the middleware did not run.
func CurrentSubject(c *gin.Context) Subject {
	v, ok := c.Get(subjectCtxKey)
	if !ok {
		return Subject{}
	}
	sub, _ := v.(Subject)
	return sub
}

// RequireEventRole returns a middleware that looks up the caller's role on
// the event named by the :id path param and aborts with 403 unless that role
// is in allowed. Must run after RequireSession. Stashes the resolved role
// for downstream handlers to read via EventRole.
func RequireEventRole(db database.Store, allowed ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		eventID, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || eventID <= 0 {
			c.AbortWithStatusJSON(http.StatusBadRequest, errorResponse{Error: "invalid event id"})
			return
		}
		sub := CurrentSubject(c)
		if !sub.IsAccount() && !sub.IsGuest() {
			c.AbortWithStatusJSON(http.StatusUnauthorized, errorResponse{Error: "not signed in"})
			return
		}
		memberID, role, err := lookupEventRole(c.Request.Context(), db, eventID, sub)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				c.AbortWithStatusJSON(http.StatusForbidden, errorResponse{Error: "not a member of this event"})
				return
			}
			c.AbortWithStatusJSON(http.StatusInternalServerError, errorResponse{Error: "role lookup failed"})
			return
		}
		for _, a := range allowed {
			if role == a {
				c.Set(eventRoleCtxKey, role)
				c.Set(eventMemberIDCtxKey, memberID)
				c.Next()
				return
			}
		}
		c.AbortWithStatusJSON(http.StatusForbidden, errorResponse{Error: "insufficient role"})
	}
}

// EventRole returns the role stashed by RequireEventRole, or the empty
// string if the middleware did not run.
func EventRole(c *gin.Context) string {
	v, ok := c.Get(eventRoleCtxKey)
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// EventMemberID returns the caller's event_members.id stashed by
// RequireEventRole, or 0 if the middleware did not run.
func EventMemberID(c *gin.Context) int64 {
	v, ok := c.Get(eventMemberIDCtxKey)
	if !ok {
		return 0
	}
	id, _ := v.(int64)
	return id
}

func lookupEventRole(ctx context.Context, db database.Store, eventID int64, sub Subject) (int64, string, error) {
	var (
		memberID int64
		role     string
	)
	err := db.QueryRow(ctx, `
		SELECT id, role::text
		  FROM event_members
		 WHERE event_id = $1
		   AND (($2 <> 0 AND account_id  = $2)
		     OR ($3 <> 0 AND guest_id = $3))
		 LIMIT 1`,
		eventID, sub.AccountID, sub.GuestID).Scan(&memberID, &role)
	return memberID, role, err
}
