// Package goroutines holds goroutine/channel fixtures. Blocking functions
// are lowered to async JS functions; from JavaScript they return Promises.
package goroutines

func Worker(ch chan int) {
	ch <- 42
}

func Example() int {
	ch := make(chan int)

	go Worker(ch)

	return <-ch
}

// Unbuffered handoff in both directions plus close/range.
func Pipeline() []int {
	src := make(chan int)
	dst := make(chan int)
	go func() {
		for i := 1; i <= 4; i++ {
			src <- i
		}
		close(src)
	}()
	go func() {
		for v := range src {
			dst <- v * v
		}
		close(dst)
	}()
	var out []int
	for v := range dst {
		out = append(out, v)
	}
	return out
}

func Buffered() []any {
	ch := make(chan string, 2)
	ch <- "a"
	ch <- "b"
	n := len(ch)
	close(ch)
	x, ok1 := <-ch
	y, ok2 := <-ch
	z, ok3 := <-ch
	return []any{n, cap(ch), x, ok1, y, ok2, z, ok3}
}

func Select() []string {
	a := make(chan string)
	b := make(chan string)
	quit := make(chan bool)
	go func() {
		a <- "from a"
		b <- "from b"
		close(quit)
	}()
	var got []string
	for {
		select {
		case s := <-a:
			got = append(got, s)
		case s := <-b:
			got = append(got, s)
		case <-quit:
			return got
		}
	}
}

func SelectDefault() []bool {
	ch := make(chan int, 1)
	sent := false
	select {
	case ch <- 1:
		sent = true
	default:
	}
	full := false
	select {
	case ch <- 2:
	default:
		full = true
	}
	return []bool{sent, full}
}

// WaitGroup-style fan-in using only channels.
func FanIn() int {
	results := make(chan int)
	for i := 1; i <= 10; i++ {
		go func() { results <- i }()
	}
	total := 0
	for range 10 {
		total += <-results
	}
	return total
}

type Job interface {
	Run(out chan<- string)
}

type Echo struct{ Msg string }

func (e Echo) Run(out chan<- string) { out <- e.Msg }

// Blocking reached through interface dispatch.
func InterfaceBlocking() string {
	var j Job = Echo{"hi"}
	out := make(chan string, 1)
	j.Run(out)
	return <-out
}

func DeferInGoroutine() []int {
	done := make(chan []int)
	go func() {
		var log []int
		defer func() { done <- append(log, 2) }()
		log = append(log, 1)
	}()
	return <-done
}

func PanicAcrossBlocking() (msg string) {
	defer func() {
		if r := recover(); r != nil {
			msg = r.(string)
		}
	}()
	ch := make(chan int)
	go func() { ch <- 1 }()
	<-ch
	panic("after receive")
}
