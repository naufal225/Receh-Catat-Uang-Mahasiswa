package domain

import "testing"

func TestNextOccurrence(t *testing.T) {
	cases := []struct {
		from   string
		f      Frequency
		anchor int
		want   string
	}{
		{"2026-01-15", FreqDaily, 15, "2026-01-16"},
		{"2026-12-31", FreqDaily, 31, "2027-01-01"},
		{"2026-01-15", FreqWeekly, 15, "2026-01-22"},
		{"2026-01-15", FreqMonthly, 15, "2026-02-15"},
		{"2026-01-31", FreqMonthly, 31, "2026-02-28"},
		{"2026-02-28", FreqMonthly, 31, "2026-03-31"},
		{"2028-01-31", FreqMonthly, 31, "2028-02-29"},
		{"2026-12-10", FreqMonthly, 10, "2027-01-10"},
		{"2028-02-29", FreqYearly, 29, "2029-02-28"},
		{"2029-02-28", FreqYearly, 29, "2030-02-28"},
	}
	for _, c := range cases {
		got, err := NextOccurrence(c.from, c.f, c.anchor)
		if err != nil || got != c.want {
			t.Errorf("%s %s anchor %d: got %s (%v), want %s", c.from, c.f, c.anchor, got, err, c.want)
		}
	}
}
