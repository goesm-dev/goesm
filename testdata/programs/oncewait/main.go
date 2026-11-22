// Command oncewait calls sync.Once.Do, OnceFunc, OnceValue and OnceValues
// from several goroutines while the first call's function is blocked: the
// others wait for it to return, as in Go, and the function runs once. A
// function that panics while others wait makes them return (Do) or panic
// with the same value (OnceFunc).
package main

import (
	"errors"
	"fmt"
	"slices"
	"sync"
)

const n = 3

// all runs f in n goroutines after sending on release and returns what
// each returned, in order.
func all(f func(i int) string, release chan bool) []string {
	out := make([]string, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i] = f(i)
		}()
	}
	release <- true
	wg.Wait()
	return out
}

func catch(f func()) (r any) {
	defer func() { r = recover() }()
	f()
	return nil
}

func main() {
	// Once.Do with a function that blocks.
	{
		var once sync.Once
		release := make(chan bool)
		calls, value := 0, 0
		out := all(func(int) string {
			once.Do(func() {
				calls++
				<-release
				value = 42
			})
			return fmt.Sprint("value ", value)
		}, release)
		fmt.Println(out, "calls", calls)
		// Later calls return at once, with a function that cannot block.
		once.Do(func() { calls++ })
		fmt.Println("calls", calls)
	}

	// Once.Do whose function panics while the others wait.
	{
		var once sync.Once
		release := make(chan bool)
		calls := 0
		out := all(func(int) string {
			return fmt.Sprint(catch(func() {
				once.Do(func() {
					calls++
					<-release
					panic("boom")
				})
			}))
		}, release)
		slices.Sort(out) // which goroutine ran the function
		fmt.Println(out, "calls", calls)
	}

	// OnceFunc, OnceValue and OnceValues.
	{
		release := make(chan bool)
		calls := 0
		f := sync.OnceFunc(func() { calls++; <-release })
		out := all(func(int) string { f(); return "done" }, release)
		fmt.Println("OnceFunc", out, "calls", calls)

		release = make(chan bool)
		calls = 0
		v := sync.OnceValue(func() int { calls++; <-release; return 7 })
		out = all(func(int) string { return fmt.Sprint(v()) }, release)
		fmt.Println("OnceValue", out, "calls", calls)

		release = make(chan bool)
		calls = 0
		vs := sync.OnceValues(func() (string, error) { calls++; <-release; return "x", errors.New("e") })
		out = all(func(int) string { s, err := vs(); return s + " " + err.Error() }, release)
		fmt.Println("OnceValues", out, "calls", calls)

		release = make(chan bool)
		calls = 0
		p := sync.OnceFunc(func() { calls++; <-release; panic("once") })
		out = all(func(int) string { return fmt.Sprint(catch(p)) }, release)
		fmt.Println("OnceFunc panic", out, "calls", calls)
	}
}
