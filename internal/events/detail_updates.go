package events

import (
	"context"
	"encoding/json"
	"errors"
	"go-split-backend/internal/database"
)

var errInvalidDetailID = errors.New("detail ids must be unique and belong to this card")

// Preserve retained identities while deleting omitted lines and applying draft order.
func replaceDetails(ctx context.Context, db database.Store, itemID int64, details []createDetailRequest) error {
	retained := []int64{}
	seen := map[int64]bool{}
	for _, d := range details {
		if d.ID < 0 || (d.ID != 0 && seen[d.ID]) {
			return errInvalidDetailID
		}
		if d.ID == 0 {
			continue
		}
		seen[d.ID] = true
		retained = append(retained, d.ID)
	}
	var owned int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM item_details WHERE item_id=$1 AND id=ANY($2)", itemID, retained).Scan(&owned); err != nil {
		return err
	}
	if owned != len(retained) {
		return errInvalidDetailID
	}
	if _, err := db.Exec(ctx, "DELETE FROM item_details WHERE item_id=$1 AND NOT (id=ANY($2))", itemID, retained); err != nil {
		return err
	}
	// Negative temporary ordinals avoid collisions while swapping existing lines.
	if _, err := db.Exec(ctx, "UPDATE item_details SET ordinal=-ordinal-1 WHERE item_id=$1", itemID); err != nil {
		return err
	}
	for i, d := range details {
		shares := d.CustomShares
		if shares == nil {
			shares = map[string]int64{}
		}
		raw, err := json.Marshal(shares)
		if err != nil {
			return err
		}
		if d.ID == 0 {
			_, err = db.Exec(ctx, `INSERT INTO item_details(item_id,ordinal,name,amount,tag,note,custom_shares,manual_member_ids) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, itemID, i, d.Name, d.Amount, d.Tag, d.Note, raw, d.ManualMemberIDs)
		} else {
			_, err = db.Exec(ctx, `UPDATE item_details SET ordinal=$2,name=$3,amount=$4,tag=$5,note=$6,custom_shares=$7,manual_member_ids=$8 WHERE id=$1`, d.ID, i, d.Name, d.Amount, d.Tag, d.Note, raw, d.ManualMemberIDs)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
