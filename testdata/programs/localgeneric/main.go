package main

import "fmt"

type G[T any] struct{ v T }

func (g *G[T]) M() any {
	type inner struct{ x T }
	return inner{g.v}
}

func F[A comparable, B comparable](a A, b B) (any, any, any) {
	type S struct {
		a A
		b B
	}
	type P[C any] struct {
		c C
		a A
	}
	type L []S
	s := S{a, b}
	m := map[B]S{b: s}
	ps := []P[string]{{"x", a}}
	l := L{s, s}
	fmt.Printf("%T %v %v %v %d\n", s, s, m[b], ps, len(l))
	var z S
	fmt.Println(z == S{}, s == S{a, b})
	return s, P[int]{1, a}, l
}

func Y[Endo ~func(RecFct) RecFct, RecFct ~func(T) R, T, R any](f Endo) RecFct {
	type internal[RecFct ~func(T) R, T, R any] func(internal[RecFct, T, R]) RecFct
	g := func(h internal[RecFct, T, R]) RecFct {
		return func(t T) R {
			return f(h(h))(t)
		}
	}
	return g(g)
}

func main() {
	x1, y1, z1 := F(1, "a")
	x2, y2, _ := F(1, "a")
	x3, _, _ := F("s", 2)
	fmt.Println(x1 == x2, x1 == x3, y1 == y2)
	fmt.Printf("%T %T %T %T\n", x1, x3, y1, z1)
	_, ok := x3.(interface{})
	fmt.Println(ok)
	fmt.Printf("%T %v\n", (&G[int]{5}).M(), (&G[string]{"q"}).M())
	fct := Y(func(r func(int) int) func(int) int {
		return func(n int) int {
			if n <= 0 {
				return 1
			}
			return n * r(n-1)
		}
	})
	fmt.Println(fct(10))
}
