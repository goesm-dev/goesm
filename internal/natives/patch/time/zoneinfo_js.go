//go:build goesm

// goesm's patch of package time's zoneinfo_js.go: the local time zone is
// read from the host's Date directly rather than through syscall/js, so that
// a program that does not use Local does not carry syscall/js.
package time

import "internal/strconv"

// localOffset returns the host's current offset from UTC, in minutes east
// (the negated Date.prototype.getTimezoneOffset).
func localOffset() int

func initLocal() {
	localLoc.name = "Local"

	z := zone{}
	offset := localOffset()
	z.offset = offset * 60
	// According to https://tc39.github.io/ecma262/#sec-timezoneestring,
	// the timezone name from (new Date()).toTimeString() is an implementation-dependent
	// result, and in Google Chrome, it gives the fully expanded name rather than
	// the abbreviation.
	// Hence, we construct the name from the offset.
	z.name = "UTC"
	if offset < 0 {
		z.name += "-"
		offset *= -1
	} else {
		z.name += "+"
	}
	z.name += strconv.Itoa(offset / 60)
	min := offset % 60
	if min != 0 {
		z.name += ":" + strconv.Itoa(min)
	}
	localLoc.zone = []zone{z}
}
