// Package timeonly uses the six time functions of a calendar page: the bundle
// must not keep the rest of package time.
package timeonly

import "time"

// Weekday returns the weekday (0 = Sunday) of a civil date.
func Weekday(y, m, d int) int {
	return int(time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC).Weekday())
}

// DaysFromCivil returns the Unix day number of a civil date.
func DaysFromCivil(y, m, d int) int64 {
	return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC).Unix() / 86400
}

// CivilFromDays returns the civil date of a Unix day number.
func CivilFromDays(days int64) (int, int, int) {
	y, m, d := time.Unix(days*86400, 0).UTC().Date()
	return y, int(m), d
}
