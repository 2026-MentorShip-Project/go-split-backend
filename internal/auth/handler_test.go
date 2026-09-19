package auth

import (
	"errors"
	"testing"
)

func TestIsUniqueViolation(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"pgx unique violation", errors.New(`ERROR: duplicate key value violates unique constraint "accounts_email_key" (SQLSTATE 23505)`), true},
		{"other pg error", errors.New(`ERROR: relation "x" does not exist (SQLSTATE 42P01)`), false},
		{"nil error", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isUniqueViolation(tc.err); got != tc.want {
				t.Fatalf("isUniqueViolation(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestSubjectExactlyOne(t *testing.T) {
	tests := []struct {
		name    string
		sub     Subject
		isHost  bool
		isGuest bool
	}{
		{"account only", Subject{AccountID: 1}, true, false},
		{"guest only", Subject{GuestID: 2}, false, true},
		{"empty", Subject{}, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.sub.IsAccount() != tc.isHost || tc.sub.IsGuest() != tc.isGuest {
				t.Fatalf("Subject{%+v} IsAccount=%v IsGuest=%v; want %v %v",
					tc.sub, tc.sub.IsAccount(), tc.sub.IsGuest(), tc.isHost, tc.isGuest)
			}
		})
	}
}

func TestAccountIDForDeletion(t *testing.T) {
	if got, err := accountIDForDeletion(Subject{AccountID: 42}); err != nil || got != 42 {
		t.Fatalf("accountIDForDeletion(account) = %d, %v; want 42, nil", got, err)
	}
	if _, err := accountIDForDeletion(Subject{GuestID: 7}); err == nil {
		t.Fatal("accountIDForDeletion(guest) returned nil error")
	}
}

func TestPhoneValidation(t *testing.T) {
	for _, s := range []string{"", "abc", "09-123", "１２３"} {
		if validPhone(s) {
			t.Fatalf("accepted %q", s)
		}
	}
	if !validPhone("0912345678") {
		t.Fatal("valid phone rejected")
	}
}
