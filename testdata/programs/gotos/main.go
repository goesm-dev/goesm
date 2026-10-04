// goto: forward and backward jumps, jumps out of nested blocks and loops,
// and unlabelled break and continue across a goto state machine.
package main

import "fmt"

func countdown(n int) {
L:
	if n > 0 {
		fmt.Print(n, " ")
		n--
		goto L
	}
	fmt.Println("liftoff")
}

// Like regexp's backtracker: a forward goto past a label that a later
// backward goto targets, inside a loop whose body continues.
func backtrack(jobs []int) int {
	visits := 0
	for len(jobs) > 0 {
		pc := jobs[len(jobs)-1]
		jobs = jobs[:len(jobs)-1]
		goto Skip
	CheckAndLoop:
		visits++
		if pc > 10 {
			continue
		}
	Skip:
		switch {
		case pc%3 == 0:
			pc += 4
			goto CheckAndLoop
		case pc%3 == 1:
			pc += 2
			if pc > 6 {
				break
			}
			goto CheckAndLoop
		default:
			jobs = append(jobs, pc+5)
		}
		visits += 100
	}
	return visits
}

// A declaration after the label is made again on every jump back; one
// before it keeps its value.
func decls() {
	total := 0
	i := 0
again:
	sq := i * i
	total += sq
	i++
	if i < 4 {
		goto again
	}
	fmt.Println("sum of squares", total, sq)
}

func nestedLoops() {
	n := 0
outer:
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			n++
			if j == 1 {
				continue outer
			}
		}
	}
	k := 0
retry:
	k++
	for i := range 5 {
		if i == 2 {
			break
		}
		if i == 1 && k < 3 {
			goto retry
		}
	}
	fmt.Println("nested", n, k)
}

func loopBody() {
	for i := 0; i < 5; i++ {
		x := i
	redo:
		if x%2 == 1 {
			x++
			goto redo
		}
		if i == 3 {
			continue
		}
		if i == 4 {
			break
		}
		fmt.Print(x, " ")
	}
	fmt.Println()
}

func closures() []func() int {
	var fs []func() int
	for i := range 3 {
		v := i * 10
	again:
		if v%20 != 0 {
			v++
			goto again
		}
		fs = append(fs, func() int { return v })
	}
	return fs
}

func addr() {
	i := 0
L:
	p := &i
	*p++
	if i < 3 {
		goto L
	}
	fmt.Println("addr", i)
}

func main() {
	countdown(5)
	fmt.Println("backtrack", backtrack([]int{0, 1, 2, 5}))
	decls()
	nestedLoops()
	loopBody()
	for _, f := range closures() {
		fmt.Print(f(), " ")
	}
	fmt.Println()
	addr()
}
