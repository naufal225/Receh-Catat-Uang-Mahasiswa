package domain

import "time"

const DateLayout = "2006-01-02"

// ParseDate parses a YYYY-MM-DD string as a pure calendar date.
func ParseDate(s string) (time.Time, error) {
	return time.Parse(DateLayout, s)
}
