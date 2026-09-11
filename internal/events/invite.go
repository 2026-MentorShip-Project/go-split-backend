package events

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// inviteAlphabet omits characters that are easy to confuse when typed: 0/O,
// 1/I, L/l. Case is uppercase-only in the visible code (as in the "4KQ2-8P"
// example from the prototype).
const inviteAlphabet = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"

// inviteCodeFormat is length of each block; the visible code is
// blockLen-first + "-" + blockLen-last (e.g. "4KQ2-8P").
const (
	inviteFirstBlock = 4
	inviteLastBlock  = 2
)

// inviteIssueRetries is how many collisions we tolerate before giving up.
// At ~28 trillion combinations for XXXX-XX the odds of hitting even one
// collision in a reasonable dataset are already tiny.
const inviteIssueRetries = 8

// newInviteCode returns a random invite code in the "XXXX-XX" shape. The
// caller is responsible for uniqueness; use issueInviteCode instead when
// the code needs to be inserted into the invitations table.
func newInviteCode() (string, error) {
	first, err := randomBlock(inviteFirstBlock)
	if err != nil {
		return "", err
	}
	last, err := randomBlock(inviteLastBlock)
	if err != nil {
		return "", err
	}
	return first + "-" + last, nil
}

func randomBlock(n int) (string, error) {
	max := big.NewInt(int64(len(inviteAlphabet)))
	var b strings.Builder
	b.Grow(n)
	for i := 0; i < n; i++ {
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("random invite char: %w", err)
		}
		b.WriteByte(inviteAlphabet[idx.Int64()])
	}
	return b.String(), nil
}

// inviteExecer is any pgx handle that can execute the INSERT. Both
// pgxpool.Pool and pgx.Tx satisfy it, so an event-creation transaction can
// pass the tx to keep everything atomic.
type inviteExecer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconnCommandTag, error)
}

// pgconnCommandTag is the return of pgx Exec; aliased so callers do not need
// to import pgconn directly.
type pgconnCommandTag = pgconn.CommandTag

// issueInviteCode inserts a fresh code for the event, retrying on unique
// collisions. Returns the code that was actually persisted.
func issueInviteCode(ctx context.Context, ex inviteExecer, eventID int64) (string, error) {
	for i := 0; i < inviteIssueRetries; i++ {
		code, err := newInviteCode()
		if err != nil {
			return "", err
		}
		_, err = ex.Exec(ctx,
			`INSERT INTO invitations (code, event_id) VALUES ($1, $2)`, code, eventID)
		if err == nil {
			return code, nil
		}
		if !isUniqueViolation(err) {
			return "", fmt.Errorf("insert invitation: %w", err)
		}
	}
	return "", errors.New("could not allocate a unique invite code")
}

// isUniqueViolation mirrors auth.isUniqueViolation but is duplicated here to
// keep the two packages independent for now. The check on SQLSTATE 23505
// avoids pulling in pgconn just for the constant.
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "SQLSTATE 23505")
}
