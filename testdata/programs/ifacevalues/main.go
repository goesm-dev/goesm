package main

import (
	"io"
	"os"
	"strconv"
)

// Types that reach interface values without an explicit conversion, and
// interface methods called through method values: the calls block.

type I interface{ M() int }

type blocker struct{ ch chan int }

func (b *blocker) M() int { return <-b.ch }

type valueBlocker struct{ ch chan int }

func (b valueBlocker) M() int { return <-b.ch }

func iter(yield func(*blocker) bool) { yield(&blocker{make(chan int, 1)}) }

func main() {
	// range over a function, assigning to an interface variable
	var i I
	for i = range iter {
	}
	i.(*blocker).ch <- 7
	os.Stdout.WriteString("range-over-func: " + strconv.Itoa(i.M()) + "\n")

	// a method value of an interface
	b := valueBlocker{make(chan int, 1)}
	var j I = b
	f := j.M
	b.ch <- 5
	os.Stdout.WriteString("method value: " + strconv.Itoa(f()) + "\n")

	// the same through the standard library: io.PipeWriter.Write blocks
	pr, pw := io.Pipe()
	done := make(chan bool)
	go func() {
		buf := make([]byte, 16)
		n, _ := pr.Read(buf)
		os.Stdout.WriteString("read: " + string(buf[:n]) + "\n")
		pr.Close()
		done <- true
	}()
	var w io.Writer = pw
	write := w.Write
	n, err := write([]byte("hello"))
	<-done
	os.Stdout.WriteString("wrote " + strconv.Itoa(n) + " " + strconv.FormatBool(err == nil) + "\n")
}
