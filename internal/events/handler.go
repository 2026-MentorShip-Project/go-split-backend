package events

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"go-split-backend/internal/auth"
)

// Handler bundles event endpoints with their database pool.
type Handler struct {
	DB *pgxpool.Pool
}

// New returns a Handler bound to the given pool.
func New(db *pgxpool.Pool) *Handler { return &Handler{DB: db} }

// Register wires the /events group with session-required middleware.
func (h *Handler) Register(r gin.IRouter) {
	g := r.Group("/events", auth.RequireSession(h.DB))
	g.GET("", h.GetEvents)
	h.registerCreateRoutes(g)
	h.registerJoinRoutes(g)
}

// eventListItem is one row of the dashboard.
type eventListItem struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	Place       string     `json:"place"`
	StartsAt    *time.Time `json:"starts_at,omitempty"`
	EndsAt      *time.Time `json:"ends_at,omitempty"`
	Template    string     `json:"template"`
	Role        string     `json:"role"`
	MemberCount int        `json:"member_count"`
	Settled     bool       `json:"settled"`
	Archived    bool       `json:"archived"`
}

type eventsResponse struct {
	Events []eventListItem `json:"events"`
}

// GetEvents godoc
// @Summary     List the current user's events
// @Description Returns every event the caller belongs to (as host, co, or
// @Description member), each with role, member count, settled and archived
// @Description flags, and template. Sorted newest event first.
// @Tags        events
// @Produce     json
// @Success     200 {object} eventsResponse
// @Failure     401 {object} errorResponse
// @Router      /events [get]
func (h *Handler) GetEvents(c *gin.Context) {
	sub := auth.CurrentSubject(c)
	ctx := c.Request.Context()

	rows, err := queryEventsForSubject(ctx, h.DB, sub)
	if err != nil {
		respondErr(c, http.StatusInternalServerError, "list events")
		return
	}
	c.JSON(http.StatusOK, eventsResponse{Events: rows})
}

// queryEventsForSubject returns every event the subject participates in,
// with the subject's role in each event and the event's member count.
func queryEventsForSubject(ctx context.Context, db *pgxpool.Pool, sub auth.Subject) ([]eventListItem, error) {
	// A host's role in an event they own is always 'host'; membership rows
	// exist for guests and for hosts joining someone else's event. The
	// UNION covers both cases and deduplicates a host who also appears in
	// event_members for their own event (should not happen, but harmless).
	const q = `
		SELECT e.id, e.name, e.place, e.starts_at, e.ends_at, e.template,
		       'host'::text AS role,
		       (SELECT COUNT(*) FROM event_members WHERE event_id = e.id) AS member_count,
		       e.settled, e.archived, e.created_at
		  FROM events e
		 WHERE $1 <> 0 AND e.host_id = $1

		UNION

		SELECT e.id, e.name, e.place, e.starts_at, e.ends_at, e.template,
		       em.role::text AS role,
		       (SELECT COUNT(*) FROM event_members WHERE event_id = e.id) AS member_count,
		       e.settled, e.archived, e.created_at
		  FROM events e
		  JOIN event_members em ON em.event_id = e.id
		 WHERE ($1 <> 0 AND em.host_id  = $1)
		    OR ($2 <> 0 AND em.guest_id = $2)

		 ORDER BY 11 DESC`

	rows, err := db.Query(ctx, q, sub.HostID, sub.GuestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []eventListItem{}
	for rows.Next() {
		var (
			it        eventListItem
			createdAt time.Time
		)
		if err := rows.Scan(&it.ID, &it.Name, &it.Place, &it.StartsAt, &it.EndsAt, &it.Template,
			&it.Role, &it.MemberCount, &it.Settled, &it.Archived, &createdAt); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

type errorResponse struct {
	Error string `json:"error"`
}

func respondErr(c *gin.Context, status int, msg string) {
	c.JSON(status, errorResponse{Error: msg})
}
