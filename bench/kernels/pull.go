//go:build go1.23

package kernels

import "iter"

func count(n int) iter.Seq[int] {
	return func(yield func(int) bool) {
		for i := 0; i < n; i++ {
			if !yield(i) {
				return
			}
		}
	}
}

// Pull steps through a sequence of n ints with iter.Pull, which native Go
// runs as a coroutine.
func Pull(n int) int {
	next, stop := iter.Pull(count(n))
	defer stop()
	s := 0
	for {
		v, ok := next()
		if !ok {
			return s
		}
		s += v & 0xff
	}
}
