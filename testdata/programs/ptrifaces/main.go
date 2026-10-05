// Pointers to structs stored in interfaces again and again, and slices
// whose elements move inside their own backing array.
package main

import (
	"errors"
	"fmt"
)

type node struct{ n int }

func (x *node) String() string { return fmt.Sprint("node ", x.n) }

type nodePtr *node

type namer interface{ String() string }

type myErr struct{ msg string }

func (e *myErr) Error() string { return e.msg }

func remove(s []namer, x namer) []namer {
	for i, y := range s {
		if y == x {
			return append(s[:i], s[i+1:]...)
		}
	}
	return s
}

func main() {
	a, b := &node{1}, &node{1}
	var x, y any = a, a
	var z any = b
	fmt.Println(x == y, x == z, x.(*node) == a)
	var w any = nodePtr(a)
	fmt.Println(w == x, w.(nodePtr) == nodePtr(a))
	m := map[any]int{}
	m[a]++
	m[a]++
	m[b]++
	fmt.Println(len(m), m[a], m[b])

	var subs []namer
	nodes := []*node{{1}, {2}, {3}, {4}}
	for _, n := range nodes {
		subs = append(subs, n)
		subs = append(subs, n) // twice
	}
	subs = remove(subs, nodes[1])
	subs = remove(subs, nodes[0])
	fmt.Println(len(subs), subs)

	e := &myErr{"bad"}
	var err1, err2 error = e, e
	fmt.Println(err1 == err2, errors.Is(fmt.Errorf("wrap: %w", err1), err2))

	ints := []int{1, 2, 3, 4, 5, 6}
	ints = append(ints[:1], ints[2:]...)
	fmt.Println(ints)
	ints = append(ints[:1], ints[:3]...)
	fmt.Println(ints)
	strs := []string{"a", "b", "c", "d"}
	fmt.Println(append(strs[:2], strs[1:]...)[:4], strs)

	var nilNode *node
	var n1, n2 any = nilNode, nilNode
	fmt.Println(n1 == n2, n1 != nil)
}
