// Package reactive is a small reactivity system in the style of
// @vue/reactivity: refs, lazily computed values and effects, with
// dependencies tracked as they are read, glitch-free updates and no
// recomputation downstream of a computed value that did not change. It
// follows the algorithm of Vue 3.5: each dependency is a link in two
// doubly linked lists, the sources of a subscriber and the subscribers of
// a source, and links are reused when a run reads what the run before
// did. Bench drives the same graph as js/impl/reactive.mjs does with
// @vue/reactivity.
package reactive

// A sub's flags.
const (
	flagActive   = 1 << iota // an effect not stopped
	flagRunning              // running its function
	flagTracking             // subscribed to its sources
	flagNotified             // in a batch
	flagDirty                // a source may have changed
	_
	_
	flagEvaluated // a computed value has been computed
)

// A link joins a source to a subscriber that read it.
type link struct {
	version int // the source's version when read; -1 while unread in a run
	dep     *dep
	sub     *sub

	prevDep, nextDep *link // the subscriber's sources
	prevSub, nextSub *link // the source's subscribers
	prevActiveLink   *link
}

// A dep is a source: the value of a Ref or a Computed.
type dep struct {
	version    int
	activeLink *link  // the link to the subscriber running, if it read this
	subs       *link  // the last subscriber
	computed   *sub   // the Computed this is the value of
	refresh    func() // brings that Computed up to date
}

// A sub is a subscriber: a Computed or an Effect.
type sub struct {
	deps, depsTail *link
	flags          uint8
	next           *sub // in a batch
	dep            *dep // a Computed's value
	effect         *Effect
	globalVersion  int // a Computed's globalVersion when last checked
}

var (
	activeSub       *sub
	globalVersion   int // counts every change to a Ref
	batchDepth      int
	batchedSubs     *sub
	batchedComputed *sub
)

// track records that the running subscriber reads d.
func (d *dep) track() *link {
	s := activeSub
	if s == nil || s == d.computed {
		return nil
	}
	l := d.activeLink
	if l == nil || l.sub != s {
		l = &link{version: d.version, dep: d, sub: s}
		d.activeLink = l
		if s.deps == nil {
			s.deps, s.depsTail = l, l
		} else {
			l.prevDep = s.depsTail
			s.depsTail.nextDep = l
			s.depsTail = l
		}
		addSub(l)
	} else if l.version == -1 {
		// Read again in this run: move the link to the end.
		l.version = d.version
		if next := l.nextDep; next != nil {
			next.prevDep = l.prevDep
			if l.prevDep != nil {
				l.prevDep.nextDep = next
			}
			l.prevDep = s.depsTail
			l.nextDep = nil
			s.depsTail.nextDep = l
			s.depsTail = l
			if s.deps == l {
				s.deps = next
			}
		}
	}
	return l
}

func (d *dep) trigger() {
	d.version++
	globalVersion++
	d.notify()
}

func (d *dep) notify() {
	batchDepth++
	for l := d.subs; l != nil; l = l.prevSub {
		if l.sub.notify() {
			l.sub.dep.notify()
		}
	}
	endBatch()
}

// notify marks s as possibly out of date, and reports whether s is a
// Computed whose subscribers must be notified too.
func (s *sub) notify() bool {
	if s.dep != nil {
		s.flags |= flagDirty
		if s.flags&flagNotified == 0 && activeSub != s {
			s.flags |= flagNotified
			s.next = batchedComputed
			batchedComputed = s
			return true
		}
		return false
	}
	if s.flags&flagRunning != 0 {
		return false
	}
	if s.flags&flagNotified == 0 {
		s.flags |= flagNotified
		s.next = batchedSubs
		batchedSubs = s
	}
	return false
}

func endBatch() {
	batchDepth--
	if batchDepth > 0 {
		return
	}
	for e := batchedComputed; e != nil; {
		next := e.next
		e.next = nil
		e.flags &^= flagNotified
		e = next
	}
	batchedComputed = nil
	for batchedSubs != nil {
		e := batchedSubs
		batchedSubs = nil
		for e != nil {
			next := e.next
			e.next = nil
			e.flags &^= flagNotified
			if e.flags&flagActive != 0 && isDirty(e) {
				e.effect.run()
			}
			e = next
		}
	}
}

// prepareDeps marks the sources of s unread before a run.
func prepareDeps(s *sub) {
	for l := s.deps; l != nil; l = l.nextDep {
		l.version = -1
		l.prevActiveLink = l.dep.activeLink
		l.dep.activeLink = l
	}
}

// cleanupDeps drops the sources of s that its run did not read.
func cleanupDeps(s *sub) {
	var head *link
	tail := s.depsTail
	for l := tail; l != nil; {
		prev := l.prevDep
		if l.version == -1 {
			if l == tail {
				tail = prev
			}
			removeSub(l)
			removeDep(l)
		} else {
			head = l
		}
		l.dep.activeLink = l.prevActiveLink
		l.prevActiveLink = nil
		l = prev
	}
	s.deps, s.depsTail = head, tail
}

// isDirty reports whether a source of s changed since s read it.
func isDirty(s *sub) bool {
	for l := s.deps; l != nil; l = l.nextDep {
		if l.dep.version != l.version {
			return true
		}
		if l.dep.refresh != nil {
			l.dep.refresh()
			if l.dep.version != l.version {
				return true
			}
		}
	}
	return false
}

func addSub(l *link) {
	if l.sub.flags&flagTracking == 0 {
		return
	}
	d := l.dep
	if c := d.computed; c != nil && d.subs == nil {
		// A computed value's first subscriber: it subscribes to its sources.
		c.flags |= flagTracking | flagDirty
		for x := c.deps; x != nil; x = x.nextDep {
			addSub(x)
		}
	}
	if tail := d.subs; tail != l {
		l.prevSub = tail
		if tail != nil {
			tail.nextSub = l
		}
	}
	d.subs = l
}

func removeSub(l *link) {
	d, prev, next := l.dep, l.prevSub, l.nextSub
	if prev != nil {
		prev.nextSub = next
		l.prevSub = nil
	}
	if next != nil {
		next.prevSub = prev
		l.nextSub = nil
	}
	if d.subs == l {
		d.subs = prev
		if prev == nil && d.computed != nil {
			// The last subscriber of a computed value is gone.
			d.computed.flags &^= flagTracking
			for x := d.computed.deps; x != nil; x = x.nextDep {
				removeSub(x)
			}
		}
	}
}

func removeDep(l *link) {
	prev, next := l.prevDep, l.nextDep
	if prev != nil {
		prev.nextDep = next
		l.prevDep = nil
	}
	if next != nil {
		next.prevDep = prev
		l.nextDep = nil
	}
}

// Ref is a reactive value.
type Ref[T comparable] struct {
	dep   dep
	value T
}

// NewRef returns a Ref holding v.
func NewRef[T comparable](v T) *Ref[T] { return &Ref[T]{value: v} }

// Get returns the value, and tracks r in the computation running.
func (r *Ref[T]) Get() T {
	r.dep.track()
	return r.value
}

// Set changes the value and updates what depends on it.
func (r *Ref[T]) Set(v T) {
	if v == r.value {
		return
	}
	r.value = v
	r.dep.trigger()
}

// Computed is a value computed from others, recomputed when read after
// one of them changed.
type Computed[T comparable] struct {
	sub   sub
	dep   dep
	fn    func() T
	value T
}

// NewComputed returns a Computed of fn.
func NewComputed[T comparable](fn func() T) *Computed[T] {
	c := &Computed[T]{fn: fn}
	c.sub.flags = flagDirty
	c.sub.globalVersion = globalVersion - 1
	c.sub.dep = &c.dep
	c.dep.computed = &c.sub
	c.dep.refresh = c.refresh
	return c
}

// Get returns the value, recomputed if needed, and tracks c.
func (c *Computed[T]) Get() T {
	l := c.dep.track()
	c.refresh()
	if l != nil {
		l.version = c.dep.version
	}
	return c.value
}

func (c *Computed[T]) refresh() {
	s := &c.sub
	if s.flags&flagTracking != 0 && s.flags&flagDirty == 0 {
		return
	}
	s.flags &^= flagDirty
	if s.globalVersion == globalVersion {
		return // no Ref changed since the last check
	}
	s.globalVersion = globalVersion
	if s.flags&flagEvaluated != 0 && (s.deps == nil || !isDirty(s)) {
		return
	}
	s.flags |= flagRunning
	prev := activeSub
	activeSub = s
	prepareDeps(s)
	v := c.fn()
	if c.dep.version == 0 || v != c.value {
		s.flags |= flagEvaluated
		c.value = v
		c.dep.version++
	}
	activeSub = prev
	cleanupDeps(s)
	s.flags &^= flagRunning
}

// Effect is a function run again whenever what it read changes.
type Effect struct {
	sub sub
	fn  func()
}

// NewEffect runs fn now and again after each change to what it reads.
func NewEffect(fn func()) *Effect {
	e := &Effect{fn: fn}
	e.sub.flags = flagActive | flagTracking
	e.sub.effect = e
	e.run()
	return e
}

func (e *Effect) run() {
	s := &e.sub
	if s.flags&flagActive == 0 {
		e.fn()
		return
	}
	s.flags |= flagRunning
	prepareDeps(s)
	prev := activeSub
	activeSub = s
	e.fn()
	cleanupDeps(s)
	activeSub = prev
	s.flags &^= flagRunning
}

// Stop ends e: it no longer runs.
func (e *Effect) Stop() {
	s := &e.sub
	if s.flags&flagActive == 0 {
		return
	}
	for l := s.deps; l != nil; l = l.nextDep {
		removeSub(l)
	}
	s.deps, s.depsTail = nil, nil
	s.flags &^= flagActive
}

// Bench builds width chains of depth computed values over width refs, a
// diamond and an effect over all of them, then sets the refs rounds
// times. It returns how often the effect ran and the last value it saw.
func Bench(width, depth, rounds int) []int {
	refs := make([]*Ref[int], width)
	ends := make([]*Computed[int], width)
	for i := range refs {
		r := NewRef(i)
		refs[i] = r
		var prev interface{ Get() int } = r
		for j := range depth {
			p := prev
			k := j
			prev = NewComputed(func() int { return p.Get() + k })
		}
		ends[i] = prev.(*Computed[int])
	}
	// A diamond whose two sides change together, and a value that often
	// stays the same.
	left := NewComputed(func() int { return refs[0].Get() * 2 })
	right := NewComputed(func() int { return refs[0].Get() + 1 })
	parity := NewComputed(func() int { return (left.Get() + right.Get()) % 2 })
	total := NewComputed(func() int {
		s := 0
		for _, c := range ends {
			s += c.Get()
		}
		return s
	})
	runs, last := 0, 0
	NewEffect(func() {
		runs++
		last = total.Get()*10 + parity.Get()
	})
	for r := range rounds {
		refs[(r*7)%width].Set(r * 3)
	}
	return []int{runs, last}
}
