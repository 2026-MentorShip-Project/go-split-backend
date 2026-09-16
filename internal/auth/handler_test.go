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
		{"pgx unique violation", errors.New(`ERROR: duplicate key value violates unique constraint "hosts_email_key" (SQLSTATE 23505)`), true},
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
		{"host only", Subject{HostID: 1}, true, false},
		{"guest only", Subject{GuestID: 2}, false, true},
		{"empty", Subject{}, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.sub.IsHost() != tc.isHost || tc.sub.IsGuest() != tc.isGuest {
				t.Fatalf("Subject{%+v} IsHost=%v IsGuest=%v; want %v %v",
					tc.sub, tc.sub.IsHost(), tc.sub.IsGuest(), tc.isHost, tc.isGuest)
			}
		})
	}
}

func TestJoinDisplayNameDefaultsWhenOmitted(t *testing.T) {
	if got := joinDisplayName(""); got != "Guest" {
		t.Fatalf("joinDisplayName(\"\") = %q, want %q", got, "Guest")
	}
}
