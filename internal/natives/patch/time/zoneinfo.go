//go:build goesm

// goesm's patch of package time's zoneinfo.go: the local time zone follows
// the host's own rules (Date.prototype.getTimezoneOffset at each instant)
// rather than the one offset Go's zoneinfo_js.go reads at startup, so a
// time on the other side of a daylight saving transition has that side's
// offset, as a JavaScript Date has.
package time

// lookup is Go's, with local times looked up on the host (lookupLocal).
func (l *Location) lookup(sec int64) (name string, offset int, start, end int64, isDST bool) {
	l = l.get()

	if l == &localLoc {
		return lookupLocal(sec)
	}

	if len(l.zone) == 0 {
		name = "UTC"
		offset = 0
		start = alpha
		end = omega
		isDST = false
		return
	}

	if zone := l.cacheZone; zone != nil && l.cacheStart <= sec && sec < l.cacheEnd {
		name = zone.name
		offset = zone.offset
		start = l.cacheStart
		end = l.cacheEnd
		isDST = zone.isDST
		return
	}

	if len(l.tx) == 0 || sec < l.tx[0].when {
		zone := &l.zone[l.lookupFirstZone()]
		name = zone.name
		offset = zone.offset
		start = alpha
		if len(l.tx) > 0 {
			end = l.tx[0].when
		} else {
			end = omega
		}
		isDST = zone.isDST
		return
	}

	// Binary search for entry with largest time <= sec.
	// Not using sort.Search to avoid dependencies.
	tx := l.tx
	end = omega
	lo := 0
	hi := len(tx)
	for hi-lo > 1 {
		m := int(uint(lo+hi) >> 1)
		lim := tx[m].when
		if sec < lim {
			end = lim
			hi = m
		} else {
			lo = m
		}
	}
	zone := &l.zone[tx[lo].index]
	name = zone.name
	offset = zone.offset
	start = tx[lo].when
	// end = maintained during the search
	isDST = zone.isDST

	// If we're at the end of the known zone transitions,
	// try the extend string.
	if lo == len(tx)-1 && l.extend != "" {
		if ename, eoffset, estart, eend, eisDST, ok := tzset(l.extend, start, sec); ok {
			return ename, eoffset, estart, eend, eisDST
		}
	}

	return
}

// localOffsetAt returns the host's offset from UTC at the Unix time sec, in
// seconds east (the negated Date.prototype.getTimezoneOffset).
func localOffsetAt(sec int64) int

// localSpan is the span of time around the latest local lookup that has one
// offset, so that formatting the times of one period asks the host once.
var localSpan struct {
	start, end int64
	offset     int
	isDST      bool
	ok         bool
}

// lookupLocal looks sec up in the host's time zone. The zone's bounds are
// found by stepping a week at a time up to a year either way and bisecting
// the step where the offset changes; with no change within that year the
// bound is a year away, which ZoneBounds then reports.
func lookupLocal(sec int64) (name string, offset int, start, end int64, isDST bool) {
	s := &localSpan
	if !s.ok || sec < s.start || sec >= s.end {
		const week, steps = 7 * secondsPerDay, 53
		off := localOffsetAt(sec)
		s.start = sec - steps*week
		for i, hi := 1, sec; i <= steps; i++ {
			lo := sec - int64(i)*week
			if localOffsetAt(lo) != off {
				for hi-lo > 1 {
					if m := lo + (hi-lo)/2; localOffsetAt(m) == off {
						hi = m
					} else {
						lo = m
					}
				}
				s.start = hi
				break
			}
			hi = lo
		}
		s.end = sec + steps*week
		for i, lo := 1, sec; i <= steps; i++ {
			hi := sec + int64(i)*week
			if localOffsetAt(hi) != off {
				for hi-lo > 1 {
					if m := lo + (hi-lo)/2; localOffsetAt(m) == off {
						lo = m
					} else {
						hi = m
					}
				}
				s.end = hi
				break
			}
			lo = hi
		}
		// Daylight saving time is the larger offset of the year.
		const half = 182 * secondsPerDay
		s.offset = off
		s.isDST = off > localOffsetAt(sec-half) || off > localOffsetAt(sec+half)
		s.ok = true
	}
	return offsetName(s.offset / 60), s.offset, s.start, s.end, s.isDST
}
