//go:build goesm

// goesm's patch of package sync/atomic's Value: the gc implementation reads
// the interface's type and data words through unsafe.Pointer, which goesm
// has no address space for. Goroutines never run in parallel under goesm, so
// the plain interface field is already atomic.
package atomic

// sameType reports whether the non-nil interfaces x and y hold values of the
// same dynamic type.
func sameType(x, y any) bool

// Load returns the value set by the most recent Store.
// It returns nil if there has been no call to Store for this Value.
func (v *Value) Load() (val any) {
	return v.v
}

// Store sets the value of the [Value] v to val.
// All calls to Store for a given Value must use values of the same concrete type.
// Store of an inconsistent type panics, as does Store(nil).
func (v *Value) Store(val any) {
	if val == nil {
		panic("sync/atomic: store of nil value into Value")
	}
	if v.v != nil && !sameType(v.v, val) {
		panic("sync/atomic: store of inconsistently typed value into Value")
	}
	v.v = val
}

// Swap stores new into Value and returns the previous value. It returns nil if
// the Value is empty.
//
// All calls to Swap for a given Value must use values of the same concrete
// type. Swap of an inconsistent type panics, as does Swap(nil).
func (v *Value) Swap(new any) (old any) {
	if new == nil {
		panic("sync/atomic: swap of nil value into Value")
	}
	if v.v != nil && !sameType(v.v, new) {
		panic("sync/atomic: swap of inconsistently typed value into Value")
	}
	old, v.v = v.v, new
	return old
}

// CompareAndSwap executes the compare-and-swap operation for the [Value].
//
// All calls to CompareAndSwap for a given Value must use values of the same
// concrete type. CompareAndSwap of an inconsistent type panics, as does
// CompareAndSwap(old, nil).
func (v *Value) CompareAndSwap(old, new any) (swapped bool) {
	if new == nil {
		panic("sync/atomic: compare and swap of nil value into Value")
	}
	if old != nil && !sameType(old, new) {
		panic("sync/atomic: compare and swap of inconsistently typed values")
	}
	if v.v == nil {
		if old != nil {
			return false
		}
		v.v = new
		return true
	}
	if !sameType(v.v, new) {
		panic("sync/atomic: compare and swap of inconsistently typed value into Value")
	}
	if v.v != old {
		return false
	}
	v.v = new
	return true
}

func runtime_procPin() int { return 0 }
func runtime_procUnpin()   {}
