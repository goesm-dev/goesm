package main

import (
	"fmt"
	"slices"
)

func seq(n int) func(func(int) bool) {
	return func(yield func(int) bool) {
		defer fmt.Println("seq done")
		for i := range n {
			if !yield(i) {
				return
			}
		}
	}
}

func f() (r int) {
	for i := range seq(3) {
		defer func() { r += i; fmt.Println("deferred", i) }()
		fmt.Println("body", i)
	}
	fmt.Println("after loop")
	return 100
}

func g() {
	defer func() { fmt.Println("recovered", recover()) }()
	for i := range seq(5) {
		defer fmt.Println("g defer", i)
		if i == 2 {
			panic("stop")
		}
	}
}

func h() (s []string) {
	for v := range slices.Values([]string{"a", "b"}) {
		for j := range seq(2) {
			defer func() { s = append(s, fmt.Sprint(v, j)) }()
			if j == 1 {
				break
			}
		}
	}
	return nil
}

func early() int {
	for i := range seq(10) {
		defer fmt.Println("early defer", i)
		if i == 1 {
			return i
		}
	}
	return -1
}

func main() {
	fmt.Println(f())
	g()
	fmt.Println(h())
	fmt.Println(early())
}

func gotos() {
	n := 0
	for i := range seq(4) {
	again:
		n++
		if n%3 != 0 {
			goto again
		}
		if i == 2 {
			goto out
		}
		fmt.Println("gotos", i, n)
	}
	fmt.Println("not reached")
out:
	fmt.Println("out", n)
	for i := range seq(2) {
		for j := range seq(2) {
			if i == 1 && j == 1 {
				goto done
			}
			fmt.Println("nested", i, j)
		}
	}
done:
	fmt.Println("done")
}

func init() { gotos() }
