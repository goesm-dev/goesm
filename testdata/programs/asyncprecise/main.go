// Command asyncprecise checks that one blocking function value or one type
// with a blocking method keeps the code that cannot reach it synchronous:
// crypto/rand.Read and json.Encoder.Encode next to an io.Reader whose Read
// waits, calls of other func(string) string values next to a blocking one,
// and os.Getenv (whose environment copy is a sync.OnceFunc) next to a
// blocking func().
package main

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

// chanReader's Read waits for the next chunk.
type chanReader struct{ c chan []byte }

func (r chanReader) Read(p []byte) (int, error) {
	b, ok := <-r.c
	if !ok {
		return 0, io.EOF
	}
	return copy(p, b), nil
}

func drain(c chan []byte) string {
	b, err := io.ReadAll(chanReader{c})
	if err != nil {
		return err.Error()
	}
	return string(b)
}

// source's reader is a field: its known types include chanReader.
type source struct{ r io.Reader }

func (s *source) next() string {
	b := make([]byte, 16)
	n, _ := s.r.Read(b)
	return string(b[:n])
}

// handler's run holds a blocking function, stored in an element of a
// slice of pointers, whose composite literal elides &handler.
type handler struct{ run func() string }

var handlers = []*handler{{run: func() string {
	c := make(chan string, 1)
	c <- "handled"
	return <-c
}}}

func handle() string { return handlers[0].run() }

func randomLen() int {
	b := make([]byte, 8)
	rand.Read(b)
	return len(b)
}

func quote(s string) string {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(s); err != nil {
		return err.Error()
	}
	return strings.TrimSpace(buf.String())
}

// slowUpper blocks; only shout calls it.
var slowUpper func(string) string = func(s string) string {
	c := make(chan string, 1)
	c <- strings.ToUpper(s)
	return <-c
}

func shout(s string) string { return slowUpper(s) }

// applyAll calls function values it cannot know.
func applyAll(fs []func(string) string, s string) string {
	for _, f := range fs {
		s = f(s)
	}
	return s
}

// onEvent is some func() that blocks.
var onEvent func() = func() {
	c := make(chan int, 1)
	c <- 1
	<-c
}

func env() string { return os.Getenv("GOESM_ASYNCPRECISE_UNSET") }

var setupOnce = sync.OnceFunc(func() { fmt.Println("setup") })

func setup() { setupOnce() }

var valueOnce = sync.OnceValue(func() int { return len(os.Args) })

func once() int { return valueOnce() }

var pool = sync.Pool{New: func() any { return new(bytes.Buffer) }}

func pooled(s string) string {
	b := pool.Get().(*bytes.Buffer)
	defer pool.Put(b)
	b.Reset()
	b.WriteString(s)
	return b.String()
}

func main() {
	c := make(chan []byte)
	go func() {
		c <- []byte("chunk ")
		c <- []byte("chunk")
		close(c)
	}()
	fmt.Println(drain(c))
	c2 := make(chan []byte, 1)
	c2 <- []byte("field")
	fmt.Println((&source{r: chanReader{c2}}).next())
	fmt.Println(handle())
	fmt.Println(randomLen())
	fmt.Println(quote(`a "b"`))
	fmt.Println(shout("hi"))
	fmt.Println(applyAll([]func(string) string{strings.TrimSpace, strings.ToLower}, "  MiXed "))
	onEvent()
	fmt.Printf("%q\n", env())
	setup()
	setup()
	fmt.Println(once() == once())
	fmt.Println(pooled("pooled"))
}
