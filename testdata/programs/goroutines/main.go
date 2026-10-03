// Command goroutines prints from a pipeline of goroutines; main blocks on
// channels and a WaitGroup.
package main

import (
	"os"
	"strconv"
	"sync"
)

func main() {
	nums := make(chan int)
	squares := make(chan int, 4)
	var wg sync.WaitGroup
	for w := 0; w < 3; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := range nums {
				squares <- n * n
			}
		}()
	}
	go func() {
		for i := 1; i <= 10; i++ {
			nums <- i
		}
		close(nums)
	}()
	go func() {
		wg.Wait()
		close(squares)
	}()
	sum := 0
	for s := range squares {
		sum += s
	}
	var mu sync.Mutex
	mu.Lock()
	sum++
	mu.Unlock()
	os.Stdout.WriteString("sum+1 = " + strconv.Itoa(sum) + "\n")
}
