package events

import (
	"bytes"
	"errors"
	"go-split-backend/internal/database"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// Hold the event lock until validation and mutation commit. Buffer responses so
// clients never observe success before the transaction commits.
func (h *Handler) eventTransaction() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := c.Param("id")
		if raw == "" {
			c.Next()
			return
		}
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			c.AbortWithStatusJSON(400, errorResponse{"invalid event id"})
			return
		}
		ctx := c.Request.Context()
		tx, err := h.DB.Begin(ctx)
		if err != nil {
			c.AbortWithStatusJSON(500, errorResponse{"begin event transaction"})
			return
		}
		defer func() { _ = tx.Rollback(ctx) }()
		lock := " FOR SHARE"
		write := c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead
		if write {
			lock = " FOR UPDATE"
		}
		var settled, archived bool
		err = tx.QueryRow(ctx, "SELECT settled,archived FROM events WHERE id=$1"+lock, id).Scan(&settled, &archived)
		if errors.Is(err, pgx.ErrNoRows) {
			c.AbortWithStatusJSON(404, errorResponse{"event not found"})
			return
		}
		if err != nil {
			c.AbortWithStatusJSON(500, errorResponse{"lock event"})
			return
		}
		if write && (settled || archived) && c.FullPath() != "/events/:id/archive" {
			c.AbortWithStatusJSON(409, errorResponse{"event is read-only"})
			return
		}
		original := c.Writer
		buffer := &bufferedWriter{ResponseWriter: original, status: 200}
		c.Writer = buffer
		defer func() { c.Writer = original }()
		c.Request = c.Request.WithContext(database.WithTransaction(ctx, tx))
		c.Next()
		if buffer.status < 400 {
			if err = tx.Commit(ctx); err != nil {
				c.Writer = original
				c.AbortWithStatusJSON(500, errorResponse{"commit event transaction"})
				return
			}
		}
		original.WriteHeader(buffer.status)
		_, _ = original.Write(buffer.body.Bytes())
	}
}

type bufferedWriter struct {
	gin.ResponseWriter
	body   bytes.Buffer
	status int
}

func (w *bufferedWriter) WriteHeader(code int)              { w.status = code }
func (w *bufferedWriter) WriteHeaderNow()                   {}
func (w *bufferedWriter) Write(b []byte) (int, error)       { return w.body.Write(b) }
func (w *bufferedWriter) WriteString(s string) (int, error) { return w.body.WriteString(s) }
func (w *bufferedWriter) Status() int                       { return w.status }
func (w *bufferedWriter) Size() int                         { return w.body.Len() }
func (w *bufferedWriter) Written() bool                     { return w.body.Len() > 0 }
