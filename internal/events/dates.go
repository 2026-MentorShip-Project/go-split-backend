package events

import (
	"fmt"
	"strings"
	"time"
)

const dateLayout = "2006-01-02"

// taipei fixes the offset rather than loading a zone, so a container without
// tzdata still dates events correctly. Taiwan observes no daylight saving.
var taipei = time.FixedZone("UTC+8", 8*60*60)

// parseDate reads a wire date. An empty string is absent, not an error.
func parseDate(value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	d, err := time.Parse(dateLayout, value)
	if err != nil {
		return nil, fmt.Errorf("date must be YYYY-MM-DD")
	}
	return &d, nil
}

func formatDate(d *time.Time) string {
	if d == nil {
		return ""
	}
	return d.Format(dateLayout)
}

func today() time.Time {
	now := time.Now().In(taipei)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

// nullableDate keeps an absent date NULL rather than storing a zero day.
func nullableDate(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// trimmed reads an optional wire string; a nil pointer means the field was
// absent, which parseDate treats the same as empty.
func trimmed(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
