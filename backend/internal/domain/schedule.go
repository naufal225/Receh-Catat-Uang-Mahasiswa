package domain

import "time"

const DateLayout = "2006-01-02"

// ParseDate parses a YYYY-MM-DD string as a pure calendar date.
func ParseDate(s string) (time.Time, error) {
	return time.Parse(DateLayout, s)
}

// NextOccurrence computes the date after `from` for a schedule. Monthly and
// yearly rules clamp to the end of short months: a rule anchored on the 31st
// runs on 28 Feb, then returns to 31 Mar.
func NextOccurrence(from string, f Frequency, anchorDay int) (string, error) {
	d, err := ParseDate(from)
	if err != nil {
		return "", err
	}
	var next time.Time
	switch f {
	case FreqDaily:
		next = d.AddDate(0, 0, 1)
	case FreqWeekly:
		next = d.AddDate(0, 0, 7)
	case FreqMonthly:
		next = clampedDate(d.Year(), d.Month()+1, anchorDay)
	case FreqYearly:
		next = clampedDate(d.Year()+1, d.Month(), anchorDay)
	default:
		next = clampedDate(d.Year(), d.Month()+1, anchorDay)
	}
	return next.Format(DateLayout), nil
}

func clampedDate(year int, month time.Month, day int) time.Time {
	first := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, 1, -1).Day()
	if day > last {
		day = last
	}
	return first.AddDate(0, 0, day-1)
}
