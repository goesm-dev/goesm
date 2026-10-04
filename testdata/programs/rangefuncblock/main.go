// Range-over-func loops whose bodies block: channel operations, select,
// sleeping, and calls of function values that may block.
package main

import (
	"fmt"
	"iter"
	"maps"
	"slices"
	"sync"
	"time"
)

func count(n int) iter.Seq[int] {
	return func(yield func(int) bool) {
		for i := range n {
			if !yield(i) {
				fmt.Println("stopped at", i)
				return
			}
		}
	}
}

func pairs(m map[string]int) iter.Seq2[string, int] {
	return func(yield func(string, int) bool) {
		for _, k := range slices.Sorted(maps.Keys(m)) {
			if !yield(k, m[k]) {
				return
			}
		}
	}
}

func sum(ch chan int, deliver func(int)) int {
	total := 0
	for v := range count(5) {
		ch <- v
		total += <-ch
		deliver(v)
	}
	return total
}

func find(seq iter.Seq[int], want int) (int, bool) {
	for v := range seq {
		time.Sleep(time.Millisecond)
		if v == want {
			return v * 100, true
		}
	}
	return 0, false
}

func main() {
	ch := make(chan int)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for v := range ch {
			ch <- v * v
		}
	}()
	var got []int
	fmt.Println("sum of squares", sum(ch, func(v int) { got = append(got, v) }), got)
	close(ch)
	wg.Wait()

	fmt.Println(find(count(10), 3))
	fmt.Println(find(count(2), 3))

	results := make(chan string, 10)
	for k, v := range pairs(map[string]int{"a": 1, "b": 2, "c": 3}) {
		select {
		case results <- fmt.Sprint(k, "=", v):
		default:
		}
		if k == "b" {
			break
		}
	}
	close(results)
	for r := range results {
		fmt.Println(r)
	}

	done := make(chan struct{})
	timer := time.After(20 * time.Millisecond)
	go func() {
		time.Sleep(time.Millisecond)
		close(done)
	}()
outer:
	for i := range count(3) {
		for j := range count(3) {
			if i == 1 && j == 1 {
				<-done
				continue outer
			}
			fmt.Println(i, j)
		}
	}
	<-timer
	fmt.Println("done")
}
