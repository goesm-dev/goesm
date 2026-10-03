// Package conformance holds cases found by running the Go repository's own
// tests (GOROOT/test) through goesm.
package conformance

// ---- recursive types (issue17039, typeparam/issue47901) ----

type S []S

type Chan[T any] chan Chan[T]

type F func(F) int

func RecursiveTypes() []int {
	s := S{S{}, S{S{}, S{}}}
	c := make(Chan[int], 1)
	c <- c
	var f F = func(g F) int { return 7 }
	return []int{len(s), len(s[1]), len(<-c), cap(c), f(f)}
}

// ---- new(expr) (Go 1.26) ----

type point struct{ X, Y int }

func NewExpr() []int {
	p := new(42)
	q := new(point{1, 2})
	r := new(len("abc") * 2)
	*p++
	return []int{*p, q.X + q.Y, *r}
}
