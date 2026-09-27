//go:build integration

package events

import (
	"fmt"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestEventSchedulingDates(t *testing.T) {
	a := newPRDAPI(t)
	host := a.host(t)

	t.Run("omitted starts_at defaults to today", func(t *testing.T) {
		e := decodePRD[createEventResponse](t, a.call(t, host, "POST", "/events",
			gin.H{"name": "No dates", "template": "自訂"}, 201))
		want := time.Now().In(taipei).Format(dateLayout)
		if e.StartsAt != want {
			t.Fatalf("starts_at = %q, want %q", e.StartsAt, want)
		}
		if e.EndsAt != "" {
			t.Fatalf("ends_at = %q, want empty", e.EndsAt)
		}
	})

	t.Run("dates round-trip through list and detail", func(t *testing.T) {
		e := decodePRD[createEventResponse](t, a.call(t, host, "POST", "/events",
			gin.H{"name": "Camping", "template": "自訂", "starts_at": "2026-10-03", "ends_at": "2026-10-04"}, 201))
		if e.StartsAt != "2026-10-03" || e.EndsAt != "2026-10-04" {
			t.Fatalf("create echoed %q..%q", e.StartsAt, e.EndsAt)
		}

		got := decodePRD[eventDetailResponse](t, a.call(t, host, "GET", fmt.Sprintf("/events/%d", e.ID), nil, 200))
		if got.StartsAt != "2026-10-03" || got.EndsAt != "2026-10-04" {
			t.Fatalf("detail returned %q..%q", got.StartsAt, got.EndsAt)
		}

		list := decodePRD[eventsResponse](t, a.call(t, host, "GET", "/events", nil, 200))
		found := false
		for _, row := range list.Events {
			if row.ID != e.ID {
				continue
			}
			found = true
			if row.StartsAt != "2026-10-03" || row.EndsAt != "2026-10-04" {
				t.Fatalf("list returned %q..%q", row.StartsAt, row.EndsAt)
			}
		}
		if !found {
			t.Fatal("event missing from list")
		}
	})

	t.Run("a name-only patch keeps the dates", func(t *testing.T) {
		e := decodePRD[createEventResponse](t, a.call(t, host, "POST", "/events",
			gin.H{"name": "Keep", "template": "自訂", "starts_at": "2026-11-01"}, 201))
		base := fmt.Sprintf("/events/%d", e.ID)

		a.call(t, host, "PATCH", base, gin.H{"name": "Renamed", "place": "Park"}, 204)

		got := decodePRD[eventDetailResponse](t, a.call(t, host, "GET", base, nil, 200))
		if got.StartsAt != "2026-11-01" {
			t.Fatalf("starts_at = %q after a name-only patch, want 2026-11-01", got.StartsAt)
		}
	})

	t.Run("an empty date clears it, a new one replaces it", func(t *testing.T) {
		e := decodePRD[createEventResponse](t, a.call(t, host, "POST", "/events",
			gin.H{"name": "Edit", "template": "自訂", "starts_at": "2026-11-01", "ends_at": "2026-11-02"}, 201))
		base := fmt.Sprintf("/events/%d", e.ID)

		a.call(t, host, "PATCH", base, gin.H{"name": "Edit", "starts_at": "2026-12-24", "ends_at": ""}, 204)

		got := decodePRD[eventDetailResponse](t, a.call(t, host, "GET", base, nil, 200))
		if got.StartsAt != "2026-12-24" || got.EndsAt != "" {
			t.Fatalf("got %q..%q, want 2026-12-24..empty", got.StartsAt, got.EndsAt)
		}
	})

	t.Run("malformed and reversed dates are refused", func(t *testing.T) {
		for _, body := range []gin.H{
			{"name": "Bad", "template": "自訂", "starts_at": "2026/10/03"},
			{"name": "Bad", "template": "自訂", "starts_at": "2026-10-03T00:00:00Z"},
			{"name": "Bad", "template": "自訂", "starts_at": "2026-10-03", "ends_at": "2026-10-01"},
		} {
			a.call(t, host, "POST", "/events", body, 400)
		}
	})
}
