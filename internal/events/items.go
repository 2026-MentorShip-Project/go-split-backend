package events

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"go-split-backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"go-split-backend/internal/auth"
)

func (h *Handler) registerItemRoutes(g *gin.RouterGroup) {
	anyRole := auth.RequireEventRole(h.DB, "host", "co", "member")
	writeRole := auth.RequireEventRole(h.DB, "host", "co")

	g.GET("/:id/items", anyRole, h.GetItems)
	g.GET("/:id/items/:item_id", anyRole, h.GetItem)
	g.POST("/:id/items", writeRole, h.PostItem)
	g.PATCH("/:id/items/:item_id", writeRole, h.PatchItem)
	g.DELETE("/:id/items/:item_id", writeRole, h.DeleteItem)
}

type itemDTO struct {
	ID             int64       `json:"id"`
	PayerMemberID  int64       `json:"payer_member_id"`
	AuthorMemberID int64       `json:"author_member_id"`
	HasReceipt     bool        `json:"has_receipt"`
	CreatedAt      time.Time   `json:"created_at"`
	Total          int64       `json:"total"`
	Details        []detailDTO `json:"details"`
}

type detailDTO struct {
	ID              int64            `json:"id"`
	Ordinal         int              `json:"ordinal"`
	Name            string           `json:"name"`
	Amount          int64            `json:"amount"`
	Tag             *string          `json:"tag"`
	Note            string           `json:"note"`
	CustomShares    map[string]int64 `json:"custom_amounts"`
	ManualMemberIDs []int64          `json:"manual_member_ids"`
}

type itemsResponse struct {
	Items []itemDTO `json:"items"`
}

type createDetailRequest struct {
	ID              int64            `json:"id,omitempty"`
	Name            string           `json:"name"`
	Amount          int64            `json:"amount"`
	Tag             *string          `json:"tag"`
	Note            string           `json:"note"`
	CustomShares    map[string]int64 `json:"custom_amounts"`
	ManualMemberIDs []int64          `json:"manual_member_ids"`
}

// Reject legacy cent-based fields and missing amounts rather than silently save zero.
func (d *createDetailRequest) UnmarshalJSON(data []byte) error {
	type plain createDetailRequest
	var value plain
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&value); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if len(fields["amount"]) == 0 || string(fields["amount"]) == "null" {
		return errors.New("amount is required in whole NT dollars")
	}
	*d = createDetailRequest(value)
	return nil
}

type createItemRequest struct {
	PayerMemberID int64                 `json:"payer_member_id" binding:"required"`
	HasReceipt    bool                  `json:"has_receipt"`
	Details       []createDetailRequest `json:"details"`
}

// PostItem godoc
// @Summary     Create an expense card with inline details
// @Description Host or co-organizer only. The whole card is persisted in
// @Description one transaction: card row, then every detail line with its
// @Description tag, note, amount in whole NT dollars, and optional custom_amounts map.
// @Tags        items
// @Accept      json
// @Produce     json
// @Param       id   path int              true "Event id"
// @Param       body body createItemRequest true "New item"
// @Success     201  {object} itemDTO
// @Failure     400  {object} errorResponse
// @Failure     401  {object} errorResponse
// @Failure     403  {object} errorResponse
// @Router      /events/{id}/items [post]
func (h *Handler) PostItem(c *gin.Context) {
	var req createItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondErr(c, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}
	eventID := eventIDFromPath(c)
	author := auth.EventMemberID(c)

	if err := validatePayer(c.Request.Context(), h.DB, eventID, req.PayerMemberID); err != nil {
		if errors.Is(err, errPayerNotInEvent) {
			respondErr(c, http.StatusBadRequest, "payer is not a member of this event")
			return
		}
		respondErr(c, http.StatusInternalServerError, "verify payer")
		return
	}

	if issues, err := validateDetails(c.Request.Context(), h.DB, eventID, req.Details); err != nil {
		respondErr(c, 500, "validate details")
		return
	} else if len(issues) > 0 {
		c.JSON(422, validationResponse{Error: "invalid details", Details: issues})
		return
	}
	for _, d := range req.Details {
		if d.ID != 0 {
			respondErr(c, 400, "new details must omit id")
			return
		}
	}
	item, err := createItemTx(c.Request.Context(), h.DB, eventID, author, req)
	if err != nil {
		respondErr(c, http.StatusInternalServerError, "create item")
		return
	}
	c.JSON(http.StatusCreated, item)
}

// GetItems godoc
// @Summary     List every item in an event
// @Description Any member may call. Cards ordered newest first, details
// @Description within a card ordered by ordinal.
// @Tags        items
// @Produce     json
// @Param       id path int true "Event id"
// @Success     200 {object} itemsResponse
// @Failure     400 {object} errorResponse
// @Failure     401 {object} errorResponse
// @Failure     403 {object} errorResponse
// @Router      /events/{id}/items [get]
func (h *Handler) GetItems(c *gin.Context) {
	items, err := loadItems(c.Request.Context(), h.DB, eventIDFromPath(c), 0)
	if err != nil {
		respondErr(c, http.StatusInternalServerError, "list items")
		return
	}
	c.JSON(http.StatusOK, itemsResponse{Items: items})
}

// GetItem godoc
// @Summary     Get one item with its details
// @Tags        items
// @Produce     json
// @Param       id      path int true "Event id"
// @Param       item_id path int true "Item id"
// @Success     200     {object} itemDTO
// @Failure     400     {object} errorResponse
// @Failure     401     {object} errorResponse
// @Failure     403     {object} errorResponse
// @Failure     404     {object} errorResponse
// @Router      /events/{id}/items/{item_id} [get]
func (h *Handler) GetItem(c *gin.Context) {
	itemID, ok := itemIDFromPath(c)
	if !ok {
		respondErr(c, http.StatusBadRequest, "invalid item id")
		return
	}
	items, err := loadItems(c.Request.Context(), h.DB, eventIDFromPath(c), itemID)
	if err != nil {
		respondErr(c, http.StatusInternalServerError, "load item")
		return
	}
	if len(items) == 0 {
		respondErr(c, http.StatusNotFound, "item not found")
		return
	}
	c.JSON(http.StatusOK, items[0])
}

var errPayerNotInEvent = errors.New("payer not in event")

func validatePayer(ctx context.Context, db database.Store, eventID, payerID int64) error {
	var found int
	err := db.QueryRow(ctx,
		`SELECT 1 FROM event_members WHERE id = $1 AND event_id = $2`,
		payerID, eventID).Scan(&found)
	if errors.Is(err, pgx.ErrNoRows) {
		return errPayerNotInEvent
	}
	return err
}

func createItemTx(ctx context.Context, db database.Store, eventID, authorID int64, req createItemRequest) (itemDTO, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return itemDTO{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var (
		itemID    int64
		createdAt time.Time
	)
	err = tx.QueryRow(ctx, `
		INSERT INTO items (event_id, payer_member_id, author_member_id, has_receipt)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`,
		eventID, req.PayerMemberID, authorID, req.HasReceipt,
	).Scan(&itemID, &createdAt)
	if err != nil {
		return itemDTO{}, err
	}

	details := make([]detailDTO, 0, len(req.Details))
	var total int64
	for i, d := range req.Details {
		shares := d.CustomShares
		if shares == nil {
			shares = map[string]int64{}
		}
		sharesJSON, err := json.Marshal(shares)
		if err != nil {
			return itemDTO{}, err
		}
		var detailID int64
		err = tx.QueryRow(ctx, `
			INSERT INTO item_details (item_id, ordinal, name, amount, tag, note, custom_shares, manual_member_ids)
			VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8)
			RETURNING id`,
			itemID, i, d.Name, d.Amount, d.Tag, d.Note, sharesJSON, d.ManualMemberIDs,
		).Scan(&detailID)
		if err != nil {
			return itemDTO{}, err
		}
		details = append(details, detailDTO{
			ID:              detailID,
			Ordinal:         i,
			Name:            d.Name,
			Amount:          d.Amount,
			Tag:             d.Tag,
			Note:            d.Note,
			CustomShares:    shares,
			ManualMemberIDs: d.ManualMemberIDs,
		})
		total += d.Amount
	}
	if err := tx.Commit(ctx); err != nil {
		return itemDTO{}, err
	}
	return itemDTO{
		ID:             itemID,
		PayerMemberID:  req.PayerMemberID,
		AuthorMemberID: authorID,
		HasReceipt:     req.HasReceipt,
		CreatedAt:      createdAt,
		Total:          total,
		Details:        details,
	}, nil
}

func loadItems(ctx context.Context, db database.Store, eventID, itemID int64) ([]itemDTO, error) {
	snap, err := loadSnapshot(ctx, db, eventID)
	if err != nil {
		return nil, err
	}
	if snap != nil {
		out := []itemDTO{}
		for _, it := range snap.Event.Items {
			if itemID == 0 || it.ID == itemID {
				out = append(out, it)
			}
		}
		return out, nil
	}

	filter := ""
	args := []any{eventID}
	if itemID > 0 {
		filter = " AND id = $2"
		args = append(args, itemID)
	}
	rows, err := db.Query(ctx,
		`SELECT id, payer_member_id, author_member_id, has_receipt, created_at
		   FROM items
		  WHERE event_id = $1`+filter+`
		  ORDER BY created_at DESC, id DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []itemDTO{}
	ids := []int64{}
	byID := map[int64]*itemDTO{}
	for rows.Next() {
		var it itemDTO
		if err := rows.Scan(&it.ID, &it.PayerMemberID, &it.AuthorMemberID, &it.HasReceipt, &it.CreatedAt); err != nil {
			return nil, err
		}
		it.Details = []detailDTO{}
		out = append(out, it)
		ids = append(ids, it.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		byID[out[i].ID] = &out[i]
	}
	if len(ids) == 0 {
		return out, nil
	}

	dRows, err := db.Query(ctx, `
		SELECT id, item_id, ordinal, name, amount, tag, note, custom_shares::text, manual_member_ids
		  FROM item_details
		 WHERE item_id = ANY($1)
		 ORDER BY item_id, ordinal`, ids)
	if err != nil {
		return nil, err
	}
	defer dRows.Close()
	for dRows.Next() {
		var (
			d          detailDTO
			itemPK     int64
			sharesText string
		)
		if err := dRows.Scan(&d.ID, &itemPK, &d.Ordinal, &d.Name, &d.Amount, &d.Tag, &d.Note, &sharesText, &d.ManualMemberIDs); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(sharesText), &d.CustomShares); err != nil {
			return nil, err
		}
		parent := byID[itemPK]
		parent.Details = append(parent.Details, d)
		parent.Total += d.Amount
	}
	return out, dRows.Err()
}

func itemIDFromPath(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("item_id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

type updateItemRequest struct {
	PayerMemberID *int64                `json:"payer_member_id,omitempty"`
	HasReceipt    *bool                 `json:"has_receipt,omitempty"`
	Details       []createDetailRequest `json:"details,omitempty"`
}

// PatchItem godoc
// @Summary     Update an item
// @Description Host or the co-organizer who authored the item may call.
// @Description Partial update on payer / has_receipt. If details is present
// @Description the whole detail set is saved atomically. Include id to retain an existing detail; omit id for new lines.
// @Tags        items
// @Accept      json
// @Produce     json
// @Param       id      path int                true "Event id"
// @Param       item_id path int                true "Item id"
// @Param       body    body updateItemRequest true "Fields to change"
// @Success     200     {object} itemDTO
// @Failure     400     {object} errorResponse
// @Failure     401     {object} errorResponse
// @Failure     403     {object} errorResponse
// @Failure     404     {object} errorResponse
// @Router      /events/{id}/items/{item_id} [patch]
func (h *Handler) PatchItem(c *gin.Context) {
	itemID, ok := itemIDFromPath(c)
	if !ok {
		respondErr(c, http.StatusBadRequest, "invalid item id")
		return
	}
	var req updateItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondErr(c, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}
	eventID := eventIDFromPath(c)
	if !authorMayMutate(c, h.DB, eventID, itemID) {
		return
	}
	if req.PayerMemberID != nil {
		if err := validatePayer(c.Request.Context(), h.DB, eventID, *req.PayerMemberID); err != nil {
			if errors.Is(err, errPayerNotInEvent) {
				respondErr(c, http.StatusBadRequest, "payer is not a member of this event")
				return
			}
			respondErr(c, http.StatusInternalServerError, "verify payer")
			return
		}
	}

	if req.Details != nil {
		if issues, err := validateDetails(c.Request.Context(), h.DB, eventID, req.Details); err != nil {
			respondErr(c, 500, "validate details")
			return
		} else if len(issues) > 0 {
			c.JSON(422, validationResponse{Error: "invalid details", Details: issues})
			return
		}
	}
	if err := updateItemTx(c.Request.Context(), h.DB, eventID, itemID, req); err != nil {
		if errors.Is(err, errInvalidDetailID) {
			respondErr(c, 400, err.Error())
			return
		}
		respondErr(c, http.StatusInternalServerError, "update item")
		return
	}
	items, err := loadItems(c.Request.Context(), h.DB, eventID, itemID)
	if err != nil || len(items) == 0 {
		respondErr(c, http.StatusInternalServerError, "reload item")
		return
	}
	c.JSON(http.StatusOK, items[0])
}

// DeleteItem godoc
// @Summary     Delete an item
// @Description Host or the co-organizer who authored the item may call.
// @Tags        items
// @Produce     json
// @Param       id      path int true "Event id"
// @Param       item_id path int true "Item id"
// @Success     204     "no content"
// @Failure     400     {object} errorResponse
// @Failure     401     {object} errorResponse
// @Failure     403     {object} errorResponse
// @Failure     404     {object} errorResponse
// @Router      /events/{id}/items/{item_id} [delete]
func (h *Handler) DeleteItem(c *gin.Context) {
	itemID, ok := itemIDFromPath(c)
	if !ok {
		respondErr(c, http.StatusBadRequest, "invalid item id")
		return
	}
	eventID := eventIDFromPath(c)
	if !authorMayMutate(c, h.DB, eventID, itemID) {
		return
	}
	if _, err := h.DB.Exec(c.Request.Context(),
		`DELETE FROM items WHERE id = $1 AND event_id = $2`, itemID, eventID); err != nil {
		respondErr(c, http.StatusInternalServerError, "delete item")
		return
	}
	c.Status(http.StatusNoContent)
}

func authorMayMutate(c *gin.Context, db database.Store, eventID, itemID int64) bool {
	var authorID int64
	err := db.QueryRow(c.Request.Context(),
		`SELECT author_member_id FROM items WHERE id = $1 AND event_id = $2`,
		itemID, eventID).Scan(&authorID)
	if errors.Is(err, pgx.ErrNoRows) {
		respondErr(c, http.StatusNotFound, "item not found")
		return false
	}
	if err != nil {
		respondErr(c, http.StatusInternalServerError, "load item")
		return false
	}
	if auth.EventRole(c) == "host" {
		return true
	}
	if authorID != auth.EventMemberID(c) {
		respondErr(c, http.StatusForbidden, "only the author or host may modify")
		return false
	}
	return true
}

func updateItemTx(ctx context.Context, db database.Store, eventID, itemID int64, req updateItemRequest) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var (
		payerArg   any
		receiptArg any
	)
	if req.PayerMemberID != nil {
		payerArg = *req.PayerMemberID
	}
	if req.HasReceipt != nil {
		receiptArg = *req.HasReceipt
	}
	tag, err := tx.Exec(ctx, `
		UPDATE items
		   SET payer_member_id = COALESCE($1, payer_member_id),
		       has_receipt     = COALESCE($2, has_receipt)
		 WHERE id = $3 AND event_id = $4`,
		payerArg, receiptArg, itemID, eventID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	if req.Details != nil {
		if err := replaceDetails(ctx, tx, itemID, req.Details); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
