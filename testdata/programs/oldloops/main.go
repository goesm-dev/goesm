//go:build go1.21

// Files whose Go version is before 1.22 share the variables of a loop
// between all its iterations.
package main

import "fmt"

func main() {
	var fs []func() int
	for i := 0; i < 3; i++ {
		fs = append(fs, func() int { return i })
	}
	for _, v := range []int{10, 20, 30} {
		fs = append(fs, func() int { return v })
	}
	var ps []*string
	for k := range map[string]bool{"only": true} {
		ps = append(ps, &k)
	}
	for i, r := range "héllo" {
		fs = append(fs, func() int { return i + int(r) })
	}
	var arr [3]struct{ n int }
	var addrs []*struct{ n int }
	for _, a := range arr {
		addrs = append(addrs, &a)
	}
	for _, f := range fs {
		fmt.Print(f(), " ")
	}
	fmt.Println(*ps[0], addrs[0] == addrs[2])
	ch := make(chan int, 3)
	ch <- 1
	ch <- 2
	ch <- 3
	close(ch)
	var cs []*int
	for c := range ch {
		cs = append(cs, &c)
	}
	fmt.Println(*cs[0], cs[0] == cs[1])
}
