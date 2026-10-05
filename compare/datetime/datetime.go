// Package datetime does what a page uses luxon for: parse ISO 8601
// timestamps, shift them by calendar days, compare and format them, and lay
// out a month for a calendar. js/impl/datetime.mjs is the same with luxon.
package datetime

import "time"

// Shift parses an RFC 3339 timestamp, adds days calendar days in its own
// offset, and formats the result the same way.
func Shift(iso string, days int) (string, error) {
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return "", err
	}
	return t.AddDate(0, 0, days).Format(time.RFC3339), nil
}

// DaysBetween returns the number of whole days from a to b.
func DaysBetween(a, b string) (int, error) {
	ta, err := time.Parse(time.RFC3339, a)
	if err != nil {
		return 0, err
	}
	tb, err := time.Parse(time.RFC3339, b)
	if err != nil {
		return 0, err
	}
	return int(tb.Sub(ta) / (24 * time.Hour)), nil
}

// Label formats a timestamp for display, such as "Mon, Oct 5 2026 09:30".
func Label(iso string) (string, error) {
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return "", err
	}
	return t.Format("Mon, Jan 2 2006 15:04"), nil
}

// MonthGrid returns the 42 days (as 2006-01-02) of a calendar page for the
// month, six weeks starting on the Monday on or before the 1st.
func MonthGrid(year, month int) []string {
	first := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	back := (int(first.Weekday()) + 6) % 7
	start := first.AddDate(0, 0, -back)
	days := make([]string, 42)
	for i := range days {
		days[i] = start.AddDate(0, 0, i).Format(time.DateOnly)
	}
	return days
}
