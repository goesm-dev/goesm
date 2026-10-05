// Package reactive is a small reactivity system in the style of
// @vue/reactivity: refs, lazily computed values and effects, with
// dependencies tracked as they are read, glitch-free updates and no
// recomputation downstream of a computed value that did not change.
// Bench drives the same graph as js/impl/reactive.mjs does with
// @vue/reactivity.
package reactive

// A source is a node others depend on: a Ref or a Computed.
type source interface {
	version() int
	refresh() // brings a computed value up to date
	subscribe(s subscriber)
	unsubscribe(s subscriber)
}

// A subscriber is a node that depends on sources: a Computed or an Effect.
type subscriber interface {
	notify() // a source may have changed
}

type dep struct {
	src     source
	version int
}

// tracker collects the sources a computation reads.
type tracker struct {
	deps  []dep
	spare []dep // the buffer of the run before last, reused
}

var active *tracker

func track(s source) {
	if active != nil {
		active.deps = append(active.deps, dep{s, s.version()})
	}
}

// subs is the set of subscribers of a source, in subscription order.
type subs struct {
	list []subscriber
}

func (s *subs) subscribe(x subscriber) {
	for _, y := range s.list {
		if y == x {
			return
		}
	}
	s.list = append(s.list, x)
}

func (s *subs) unsubscribe(x subscriber) {
	for i, y := range s.list {
		if y == x {
			s.list = append(s.list[:i], s.list[i+1:]...)
			return
		}
	}
}

func (s *subs) notifyAll() {
	for _, x := range s.list {
		x.notify()
	}
}

// Ref is a reactive value.
type Ref[T comparable] struct {
	subs
	value T
	ver   int
}

// NewRef returns a Ref holding v.
func NewRef[T comparable](v T) *Ref[T] { return &Ref[T]{value: v} }

func (r *Ref[T]) version() int { return r.ver }
func (r *Ref[T]) refresh()     {}

// Get returns the value, and tracks r in the computation running.
func (r *Ref[T]) Get() T {
	track(r)
	return r.value
}

// Set changes the value and updates what depends on it.
func (r *Ref[T]) Set(v T) {
	if v == r.value {
		return
	}
	r.value = v
	r.ver++
	batchDepth++
	r.notifyAll()
	endBatch()
}

// Computed is a value computed from others, recomputed when read after
// one of them changed.
type Computed[T comparable] struct {
	subs
	tracker
	fn    func() T
	value T
	ver   int
	dirty bool // a source may have changed
	init  bool
}

// NewComputed returns a Computed of fn.
func NewComputed[T comparable](fn func() T) *Computed[T] {
	return &Computed[T]{fn: fn, dirty: true}
}

func (c *Computed[T]) version() int { c.refresh(); return c.ver }

func (c *Computed[T]) notify() {
	if c.dirty {
		return
	}
	c.dirty = true
	c.notifyAll()
}

func (c *Computed[T]) refresh() {
	if !c.dirty {
		return
	}
	c.dirty = false
	if c.init && !changed(c.deps) {
		return
	}
	v := run(&c.tracker, c, c.fn)
	if !c.init || v != c.value {
		c.value = v
		c.ver++
		c.init = true
	}
}

// Get returns the value, recomputed if needed, and tracks c.
func (c *Computed[T]) Get() T {
	c.refresh()
	track(c)
	return c.value
}

// changed reports whether one of deps has a newer version.
func changed(deps []dep) bool {
	for _, d := range deps {
		if d.src.version() != d.version {
			return true
		}
	}
	return false
}

// run calls fn with t tracking what it reads, and moves sub's
// subscriptions to the sources read. As in Vue, a run that reads the
// same sources as the one before keeps its subscriptions.
func run[T any](t *tracker, sub subscriber, fn func() T) T {
	old := t.deps
	t.deps = t.spare[:0]
	prev := active
	active = t
	v := fn()
	active = prev
	if !sameSources(old, t.deps) {
		for _, d := range old {
			d.src.unsubscribe(sub)
		}
		for _, d := range t.deps {
			d.src.subscribe(sub)
		}
	}
	t.spare = old
	return v
}

// sameSources reports whether a and b list the same sources in order.
func sameSources(a, b []dep) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].src != b[i].src {
			return false
		}
	}
	return true
}

// Effect is a function run again whenever what it read changes.
type Effect struct {
	tracker
	fn     func()
	queued bool
}

var (
	batchDepth int
	queue      []*Effect
)

// NewEffect runs fn now and again after each change to what it reads.
func NewEffect(fn func()) *Effect {
	e := &Effect{fn: fn}
	e.run()
	return e
}

func (e *Effect) run() {
	run(&e.tracker, e, func() struct{} { e.fn(); return struct{}{} })
}

func (e *Effect) notify() {
	if !e.queued {
		e.queued = true
		queue = append(queue, e)
	}
}

func endBatch() {
	batchDepth--
	if batchDepth > 0 {
		return
	}
	for len(queue) > 0 {
		q := queue
		queue = nil
		for _, e := range q {
			e.queued = false
			if changed(e.deps) {
				e.run()
			}
		}
	}
}

// Stop ends e: it no longer runs.
func (e *Effect) Stop() {
	for _, d := range e.deps {
		d.src.unsubscribe(e)
	}
	e.deps, e.spare = nil, nil
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
