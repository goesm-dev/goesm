// iter.Pull whose next and stop are only called, as most programs use them:
// those calls are the only ones that switch coroutines.
package main

import (
	"fmt"
	"iter"
	"maps"
	"slices"
	"sort"
	"strings"
)

func count(n int) iter.Seq[int] {
	return func(yield func(int) bool) {
		defer fmt.Println("count done", n)
		for i := 0; i < n; i++ {
			if !yield(i) {
				return
			}
		}
	}
}

func sum(n int) int {
	next, stop := iter.Pull(count(n))
	defer stop()
	s := 0
	for {
		v, ok := next()
		if !ok {
			return s
		}
		s += v
	}
}

func firstTwo(seq iter.Seq[int]) (int, int) {
	next, stop := iter.Pull(seq)
	defer stop()
	a, _ := next()
	b, _ := next()
	return a, b
}

func closure(n int) []int {
	next, stop := iter.Pull(count(n))
	get := func() (int, bool) { return next() }
	var out []int
	for v, ok := get(); ok; v, ok = get() {
		out = append(out, v)
	}
	stop()
	return out
}

func pairs() []string {
	next, stop := iter.Pull2(maps.All(map[string]int{"a": 1}))
	defer stop()
	var out []string
	for {
		k, v, ok := next()
		if !ok {
			return out
		}
		out = append(out, fmt.Sprint(k, v))
	}
}

func main() {
	fmt.Println(sum(5))
	fmt.Println(firstTwo(count(10)))
	fmt.Println(closure(3))
	fmt.Println(pairs())
	// Calls of func(rune) bool and func() values, which are not iter.Pull's.
	// (A call of a func(int) bool value, as sort.Search's, may be of the
	// yield function of iter.Pull[int].)
	fmt.Println(strings.ToUpper("héllo"), strings.IndexFunc("ab1", func(r rune) bool { return r == '1' }))
	fmt.Println(sort.SearchInts([]int{1, 3, 5}, 3), slices.IndexFunc([]int{1, 2}, func(v int) bool { return v == 2 }))
}
