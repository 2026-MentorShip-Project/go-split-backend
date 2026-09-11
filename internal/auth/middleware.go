package auth

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// subjectCtxKey is the gin.Context key under which the current Subject is
// stored. Downstream handlers read it via CurrentSubject.
const subjectCtxKey = "auth.subject"

// RequireSession returns a middleware that rejects any request without a
// valid session cookie and attaches the resolved Subject to the context.
func RequireSession(db *pgxpool.Pool) gin.HandlerFunc {
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
