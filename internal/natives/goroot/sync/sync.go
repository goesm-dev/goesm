//go:build goesm

// Package sync is goesm's replacement for the standard library's sync
// package. Goroutines share one JavaScript thread and switch only where they
// block, so the primitives need no atomics: a contended Mutex, RWMutex,
// WaitGroup or Cond waits on a channel (and is therefore lowered to an async
// function), and the uncontended paths are plain field updates.
package sync

// A Locker represents an object that can be locked and unlocked.
type Locker interface {
	Lock()
	Unlock()
}

type noCopy struct{}

func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}

// waitList is a broadcast point: waiters block until the next wake.
type waitList struct {
	ch chan struct{}
}

func (w *waitList) wait() {
	if w.ch == nil {
		w.ch = make(chan struct{})
	}
	ch := w.ch
	<-ch
}

func (w *waitList) wake() {
	if w.ch != nil {
		close(w.ch)
		w.ch = nil
	}
}

// A Mutex is a mutual exclusion lock.
type Mutex struct {
	locked  bool
	waiters waitList
}

// Lock locks m, waiting until it is available.
func (m *Mutex) Lock() {
	for m.locked {
		m.waiters.wait()
	}
	m.locked = true
}

// TryLock tries to lock m and reports whether it succeeded.
func (m *Mutex) TryLock() bool {
	if m.locked {
		return false
	}
	m.locked = true
	return true
}

// Unlock unlocks m.
func (m *Mutex) Unlock() {
	if !m.locked {
		fatal("sync: unlock of unlocked mutex")
	}
	m.locked = false
	m.waiters.wake()
}

// A RWMutex is a reader/writer mutual exclusion lock. A blocked Lock call
// excludes new readers, as in Go.
type RWMutex struct {
	readers        int
	writer         bool
	writersWaiting int
	waiters        waitList
}

// Lock locks rw for writing.
func (rw *RWMutex) Lock() {
	rw.writersWaiting++
	for rw.writer || rw.readers > 0 {
		rw.waiters.wait()
	}
	rw.writersWaiting--
	rw.writer = true
}

// TryLock tries to lock rw for writing and reports whether it succeeded.
func (rw *RWMutex) TryLock() bool {
	if rw.writer || rw.readers > 0 {
		return false
	}
	rw.writer = true
	return true
}

// Unlock unlocks rw for writing.
func (rw *RWMutex) Unlock() {
	if !rw.writer {
		fatal("sync: Unlock of unlocked RWMutex")
	}
	rw.writer = false
	rw.waiters.wake()
}

// RLock locks rw for reading.
func (rw *RWMutex) RLock() {
	for rw.writer || rw.writersWaiting > 0 {
		rw.waiters.wait()
	}
	rw.readers++
}

// TryRLock tries to lock rw for reading and reports whether it succeeded.
func (rw *RWMutex) TryRLock() bool {
	if rw.writer || rw.writersWaiting > 0 {
		return false
	}
	rw.readers++
	return true
}

// RUnlock undoes a single RLock call.
func (rw *RWMutex) RUnlock() {
	if rw.readers <= 0 {
		fatal("sync: RUnlock of unlocked RWMutex")
	}
	rw.readers--
	if rw.readers == 0 {
		rw.waiters.wake()
	}
}

// RLocker returns a Locker interface that implements Lock and Unlock by
// calling rw.RLock and rw.RUnlock.
func (rw *RWMutex) RLocker() Locker { return (*rlocker)(rw) }

type rlocker RWMutex

func (r *rlocker) Lock()   { (*RWMutex)(r).RLock() }
func (r *rlocker) Unlock() { (*RWMutex)(r).RUnlock() }

// A WaitGroup waits for a collection of goroutines to finish.
type WaitGroup struct {
	noCopy  noCopy
	n       int
	waiters waitList
}

// Add adds delta, which may be negative, to the WaitGroup counter.
func (wg *WaitGroup) Add(delta int) {
	wg.n += delta
	if wg.n < 0 {
		panic("sync: negative WaitGroup counter")
	}
	if wg.n == 0 {
		wg.waiters.wake()
	}
}

// Done decrements the WaitGroup counter by one.
func (wg *WaitGroup) Done() { wg.Add(-1) }

// Wait blocks until the WaitGroup counter is zero.
func (wg *WaitGroup) Wait() {
	for wg.n > 0 {
		wg.waiters.wait()
	}
}

// Go calls f in a new goroutine and adds that task to the WaitGroup.
func (wg *WaitGroup) Go(f func()) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		f()
	}()
}

// Once is an object that will perform exactly one action.
//
// Under goesm, a second Do while the first call's f is blocked (which can
// only happen if f blocks, since goroutines switch only where they block)
// panics instead of waiting for f to return.
type Once struct {
	noCopy  noCopy
	done    bool
	running bool
}

// Do calls the function f if and only if Do is being called for the first
// time for this instance of Once.
func (o *Once) Do(f func()) {
	if o.done {
		return
	}
	if o.running {
		fatal("sync: Once.Do called while its function is running (unsupported under goesm)")
	}
	o.running = true
	defer func() {
		o.running = false
		o.done = true
	}()
	f()
}

// OnceFunc returns a function that invokes f only once.
func OnceFunc(f func()) func() {
	var (
		once  Once
		valid bool
		p     any
	)
	g := func() {
		defer func() {
			p = recover()
			if !valid {
				panic(p)
			}
		}()
		f()
		f = nil
		valid = true
	}
	return func() {
		once.Do(g)
		if !valid {
			panic(p)
		}
	}
}

// OnceValue returns a function that invokes f only once and returns the
// value returned by f.
func OnceValue[T any](f func() T) func() T {
	var (
		once   Once
		valid  bool
		p      any
		result T
	)
	g := func() {
		defer func() {
			p = recover()
			if !valid {
				panic(p)
			}
		}()
		result = f()
		f = nil
		valid = true
	}
	return func() T {
		once.Do(g)
		if !valid {
			panic(p)
		}
		return result
	}
}

// OnceValues returns a function that invokes f only once and returns the
// values returned by f.
func OnceValues[T1, T2 any](f func() (T1, T2)) func() (T1, T2) {
	var (
		once  Once
		valid bool
		p     any
		r1    T1
		r2    T2
	)
	g := func() {
		defer func() {
			p = recover()
			if !valid {
				panic(p)
			}
		}()
		r1, r2 = f()
		f = nil
		valid = true
	}
	return func() (T1, T2) {
		once.Do(g)
		if !valid {
			panic(p)
		}
		return r1, r2
	}
}

// Cond implements a condition variable.
type Cond struct {
	noCopy  noCopy
	L       Locker
	waiters []chan struct{}
}

// NewCond returns a new Cond with Locker l.
func NewCond(l Locker) *Cond { return &Cond{L: l} }

// Wait atomically unlocks c.L and suspends the calling goroutine.
func (c *Cond) Wait() {
	ch := make(chan struct{})
	c.waiters = append(c.waiters, ch)
	c.L.Unlock()
	<-ch
	c.L.Lock()
}

// Signal wakes one goroutine waiting on c, if there is any.
func (c *Cond) Signal() {
	if len(c.waiters) > 0 {
		close(c.waiters[0])
		c.waiters = c.waiters[1:]
	}
}

// Broadcast wakes all goroutines waiting on c.
func (c *Cond) Broadcast() {
	for _, ch := range c.waiters {
		close(ch)
	}
	c.waiters = nil
}

// A Pool is a set of temporary objects that may be individually saved and
// retrieved.
type Pool struct {
	noCopy noCopy
	items  []any
	New    func() any
}

// Put adds x to the pool.
func (p *Pool) Put(x any) {
	if x != nil {
		p.items = append(p.items, x)
	}
}

// Get selects an arbitrary item from the Pool, removes it from the Pool,
// and returns it to the caller.
func (p *Pool) Get() any {
	if n := len(p.items); n > 0 {
		x := p.items[n-1]
		p.items[n-1] = nil
		p.items = p.items[:n-1]
		return x
	}
	if p.New != nil {
		return p.New()
	}
	return nil
}

// Map is like a Go map[any]any but is safe for concurrent use.
type Map struct {
	noCopy noCopy
	m      map[any]any
}

// Load returns the value stored in the map for a key.
func (m *Map) Load(key any) (value any, ok bool) {
	value, ok = m.m[key]
	return
}

// Store sets the value for a key.
func (m *Map) Store(key, value any) { m.Swap(key, value) }

// Clear deletes all the entries.
func (m *Map) Clear() { clear(m.m) }

// LoadOrStore returns the existing value for the key if present. Otherwise,
// it stores and returns the given value.
func (m *Map) LoadOrStore(key, value any) (actual any, loaded bool) {
	if v, ok := m.m[key]; ok {
		return v, true
	}
	m.Swap(key, value)
	return value, false
}

// LoadAndDelete deletes the value for a key, returning the previous value
// if any.
func (m *Map) LoadAndDelete(key any) (value any, loaded bool) {
	value, loaded = m.m[key]
	delete(m.m, key)
	return
}

// Delete deletes the value for a key.
func (m *Map) Delete(key any) { delete(m.m, key) }

// Swap swaps the value for a key and returns the previous value if any.
func (m *Map) Swap(key, value any) (previous any, loaded bool) {
	if m.m == nil {
		m.m = map[any]any{}
	}
	previous, loaded = m.m[key]
	m.m[key] = value
	return
}

// CompareAndSwap swaps the old and new values for key if the value stored
// in the map is equal to old.
func (m *Map) CompareAndSwap(key, old, new any) (swapped bool) {
	if v, ok := m.m[key]; ok && v == old {
		m.m[key] = new
		return true
	}
	return false
}

// CompareAndDelete deletes the entry for key if its value is equal to old.
func (m *Map) CompareAndDelete(key, old any) (deleted bool) {
	if v, ok := m.m[key]; ok && v == old {
		delete(m.m, key)
		return true
	}
	return false
}

// Range calls f sequentially for each key and value present in the map.
func (m *Map) Range(f func(key, value any) bool) {
	for k, v := range m.m {
		if !f(k, v) {
			break
		}
	}
}

func fatal(msg string) { panic(msg) }
