// Package localtime checks the local time zone against Go's own, run with
// TZ=America/New_York (TestGoldenLocalTime): times on either side of a
// daylight saving transition have that side's offset. Zone names are not
// compared: goesm names a zone by its offset, as UTC-5.
package localtime

import "time"

func offsets(ts ...time.Time) []int {
	var out []int
	for _, t := range ts {
		_, off := t.Zone()
		out = append(out, off)
	}
	return out
}

// Offsets of instants in winter, summer and around the 2026 transitions.
func Offsets() []int {
	var ts []time.Time
	for _, sec := range []int64{1767225600, 1782864000, 1773558000, 1773558001, 1773557999, 1793512800, 1793516400, 0, -1000000000, 4102444800} {
		ts = append(ts, time.Unix(sec, 0).Local())
	}
	return offsets(ts...)
}

// Dates are local wall times turned into instants, on both sides of the
// transitions and in the hours they skip or repeat.
func Dates() []int64 {
	var out []int64
	for _, d := range [][3]int{{1, 15, 12}, {7, 15, 12}, {3, 8, 1}, {3, 8, 2}, {3, 8, 3}, {11, 1, 0}, {11, 1, 1}, {11, 1, 2}} {
		out = append(out, time.Date(2026, time.Month(d[0]), d[1], d[2], 30, 0, 0, time.Local).Unix())
	}
	return out
}

// Clocks are the local wall clocks of instants across a transition.
func Clocks() []int {
	var out []int
	for sec := int64(1773550000); sec < 1773570000; sec += 3333 {
		h, m, s := time.Unix(sec, 0).Local().Clock()
		out = append(out, h*10000+m*100+s)
	}
	return out
}

func DST() []bool {
	return []bool{time.Unix(1767225600, 0).Local().IsDST(), time.Unix(1782864000, 0).Local().IsDST()}
}

// Bounds are the transitions around a summer instant.
func Bounds() []int64 {
	start, end := time.Unix(1782864000, 0).Local().ZoneBounds()
	return []int64{start.Unix(), end.Unix()}
}
