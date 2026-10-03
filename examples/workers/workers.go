// Package workers uses goroutines, channels and sync. Functions that may
// block are async in JavaScript and return Promises; the others stay
// synchronous.
package workers

import "sync"

// Square never blocks: a plain synchronous function in JS.
func Square(x int) int { return x * x }

// SumSquares computes 1² + ... + n² on a pool of worker goroutines fed
// through a channel.
func SumSquares(n, workers int) int {
	jobs := make(chan int)
	results := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				results <- Square(j)
			}
		}()
	}
	go func() {
		for i := 1; i <= n; i++ {
			jobs <- i
		}
		close(jobs)
	}()
	go func() {
		wg.Wait()
		close(results)
	}()
	sum := 0
	for r := range results {
		sum += r
	}
	return sum
}

// Count increments a shared counter from many goroutines under a mutex.
func Count(goroutines, perGoroutine int) int {
	var mu sync.Mutex
	var wg sync.WaitGroup
	n := 0
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				mu.Lock()
				n++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return n
}
