package lockblock

import "sync"

type queue struct {
	mu    sync.Mutex
	items []int
	ready chan int
}

// Push holds q.mu while sending: every Lock of the field waits.
func (q *queue) Push(v int) {
	q.mu.Lock()
	q.items = append(q.items, v)
	q.ready <- v
	q.mu.Unlock()
}

// Pop releases q.mu before it blocks: no warning.
func (q *queue) Pop() int {
	v := <-q.ready
	q.mu.Lock()
	q.items = q.items[1:]
	q.mu.Unlock()
	return v
}

func wait(ch chan int) int { return <-ch }

// Wait holds a read lock (released by defer) across a blocking call, on a
// mutex that callers may lock under other names: goesm warns.
func Wait(mu *sync.RWMutex, ch chan int) int {
	mu.RLock()
	defer mu.RUnlock()
	return wait(ch)
}

// Poll's select has a default case: no warning.
func Poll(mu *sync.Mutex, ch chan int) int {
	mu.Lock()
	defer mu.Unlock()
	select {
	case v := <-ch:
		return v
	default:
		return 0
	}
}

var global sync.Mutex

func lockOf() *sync.Mutex { return &global }

// Unnamed locks the mutex returned by a call across a receive.
func Unnamed(ch chan int) int {
	lockOf().Lock()
	v := <-ch
	lockOf().Unlock()
	return v
}
