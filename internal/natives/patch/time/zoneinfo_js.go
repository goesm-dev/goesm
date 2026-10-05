//go:build goesm

// goesm's patch of package time's zoneinfo_js.go: the local time zone is
// read from the host's Date directly rather than through syscall/js, so that
// a program that does not use Local does not carry syscall/js. It formats
// the offset with the package's own appendInt rather than the strconv
// package Go's file imports, whose import path differs between Go releases.
package time

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
	z.name += string(appendInt(nil, offset/60, 0))
	min := offset % 60
	if min != 0 {
		z.name += ":" + string(appendInt(nil, min, 0))
	}
	localLoc.zone = []zone{z}
}
