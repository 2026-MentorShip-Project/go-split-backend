package events

import (
	"strings"
	"testing"
)

func TestNewInviteCodeShapeAndAlphabet(t *testing.T) {
	for i := 0; i < 200; i++ {
		code, err := newInviteCode()
		if err != nil {
			t.Fatalf("newInviteCode: %v", err)
		}
		if len(code) != inviteFirstBlock+1+inviteLastBlock {
			t.Fatalf("code %q has wrong length %d", code, len(code))
		}
		if code[inviteFirstBlock] != '-' {
			t.Fatalf("code %q missing dash separator", code)
		}
		for _, r := range code {
			if r == '-' {
				continue
			}
			if !strings.ContainsRune(inviteAlphabet, r) {
				t.Fatalf("code %q contains char %q outside alphabet", code, r)
			}
		}
	}
}

func TestNewInviteCodeDiverges(t *testing.T) {
	// A collision is possible in principle but astronomically unlikely
	// across 32 draws — a real failure here would point at a broken RNG.
	seen := map[string]bool{}
	for i := 0; i < 32; i++ {
		code, err := newInviteCode()
		if err != nil {
			t.Fatalf("newInviteCode: %v", err)
		}
		if seen[code] {
			t.Fatalf("newInviteCode produced duplicate %q within 32 draws", code)
		}
		seen[code] = true
	}
}

func TestIsUniqueViolation(t *testing.T) {
	if !isUniqueViolation(pgErr(`ERROR: dup key (SQLSTATE 23505)`)) {
		t.Fatalf("expected 23505 to be unique violation")
	}
	if isUniqueViolation(pgErr(`ERROR: something else (SQLSTATE 42P01)`)) {
		t.Fatalf("expected 42P01 not to be unique violation")
	}
	if isUniqueViolation(nil) {
		t.Fatalf("nil error must not be a unique violation")
	}
}

type stringError string

func (e stringError) Error() string { return string(e) }
func pgErr(s string) error          { return stringError(s) }
