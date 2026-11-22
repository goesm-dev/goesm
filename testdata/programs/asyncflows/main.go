// Command asyncflows stores a blocking reader and blocking functions in
// interface and function variables by many paths a program can use, and
// calls them through those variables: each call must wait.
package main

import (
	"fmt"
	"io"
	"strings"
	"sync"
)

// slowReader's Read waits for the next chunk.
type slowReader struct{ c chan string }

func (r slowReader) Read(p []byte) (int, error) {
	s, ok := <-r.c
	if !ok {
		return 0, io.EOF
	}
	return copy(p, s), nil
}

func newSlow(s string) slowReader {
	c := make(chan string, 1)
	c <- s
	close(c)
	return slowReader{c}
}

func read(r io.Reader) string {
	b := make([]byte, 32)
	n, err := r.Read(b)
	return fmt.Sprint(string(b[:n]), " ", err)
}

// A field set by a method called through an interface.
type setter interface{ set(r io.Reader) }

type holder struct{ r io.Reader }

func (h *holder) set(r io.Reader) { h.r = r }

func (h *holder) read() string { return read2(h.r) }

func read2(r io.Reader) string {
	b := make([]byte, 32)
	n, _ := r.Read(b)
	return string(b[:n])
}

// Two struct types with identical fields, converted into each other.
type left struct{ r io.Reader }
type right struct{ r io.Reader }

func viaConversion() string {
	l := left{r: newSlow("conversion")}
	return right(l).r.(slowReader).String() + read2(right(l).r)
}

func (r slowReader) String() string { return "" }

// A named and an unnamed struct type with the same fields.
type named struct{ r io.Reader }

func viaUnnamed() string {
	u := struct{ r io.Reader }{newSlow("unnamed")}
	var n named = u
	return read2(n.r)
}

func twoResults() (io.Reader, error) { return newSlow("two results"), nil }

func id[T any](x T) T { return x }

type wrap struct{ io.Reader }

func deferredResult() (r io.Reader) {
	defer func() { r = newSlow("deferred") }()
	return strings.NewReader("never")
}

var global io.Reader = strings.NewReader("fast")

func setGlobal(r io.Reader) { global = r }

func viaClosure() string {
	r := io.Reader(strings.NewReader("fast"))
	func() { r = newSlow("closure") }()
	return read2(r)
}

func viaLit() string {
	var got io.Reader
	put := func(r io.Reader) { got = r }
	put(newSlow("literal parameter"))
	return read2(got)
}

func viaChannel() string {
	c := make(chan io.Reader, 1)
	c <- newSlow("channel")
	r := <-c
	return read2(r)
}

func viaRange() string {
	out := ""
	for _, r := range []io.Reader{strings.NewReader("fast "), newSlow("range")} {
		out += read2(r)
	}
	return out
}

func viaAssert(x any) string {
	r := x.(io.Reader)
	return read2(r)
}

func viaMap() string {
	m := map[string]io.Reader{"k": newSlow("map")}
	r := m["k"]
	return read2(r)
}

type outer struct{ *holder }

func viaPromoted() string {
	o := outer{&holder{}}
	o.set(newSlow("promoted"))
	return o.read()
}

func viaMethodExpr() string {
	h := &holder{}
	(*holder).set(h, newSlow("method expression"))
	return h.read()
}

func viaGo() string {
	done := make(chan string)
	go func(r io.Reader) { done <- read2(r) }(newSlow("go"))
	return <-done
}

func consume(r io.Reader, done chan string) { done <- read2(r) }

func viaGoDecl() string {
	done := make(chan string)
	go consume(newSlow("go declared"), done)
	return <-done
}

// Function values.

func slowString(s string) string {
	c := make(chan string, 1)
	c <- strings.ToUpper(s)
	return <-c
}

var slowOnce = sync.OnceFunc(func() {
	c := make(chan int, 1)
	c <- 1
	<-c
	fmt.Println("once ran")
})

var slowValue = sync.OnceValue(func() string { return slowString("once value") })

var slowVar = slowString

func viaAppend() string {
	fs := []func(string) string{}
	fs = append(fs, slowVar)
	return fs[0]("append")
}

func viaAny() string {
	var x any = slowVar
	return x.(func(string) string)("any")
}

func makeSlow() func(string) string { return slowVar }

func viaReturned() string {
	g := []func(string) string{makeSlow()}
	return g[0]("returned")
}

func viaFuncChan() string {
	c := make(chan func(string) string, 1)
	c <- slowVar
	return (<-c)("func channel")
}

type fnHolder struct{ f func(string) string }
type fnHolder2 struct{ f func(string) string }

func viaFuncConversion() string {
	h := fnHolder{f: slowVar}
	return fnHolder2(h).f("func conversion")
}

func viaMapFunc() string {
	m := map[string]func(string) string{"k": slowVar}
	return m["k"]("func map")
}

func pick(f, g func(string) string, first bool) func(string) string {
	if first {
		return f
	}
	return g
}

func viaPick() string {
	return pick(strings.ToLower, slowVar, false)("pick")
}

func viaPointer() string {
	f := strings.ToLower
	p := &f
	*p = slowVar
	return f("pointer")
}

func main() {
	var s setter = &holder{}
	s.set(newSlow("interface method"))
	fmt.Println(s.(*holder).read())
	fmt.Println(viaConversion())
	fmt.Println(viaUnnamed())
	r, _ := twoResults()
	fmt.Println(read2(r))
	var g io.Reader = id[io.Reader](newSlow("generic"))
	fmt.Println(read2(g))
	var w io.Reader = wrap{newSlow("embedded")}
	fmt.Println(read2(w))
	var rw io.ReadWriter = struct {
		io.Reader
		io.Writer
	}{newSlow("read writer"), io.Discard}
	var r2 io.Reader = rw
	fmt.Println(read2(r2))
	fmt.Println(read2(deferredResult()))
	setGlobal(newSlow("global"))
	fmt.Println(read2(global))
	fmt.Println(viaClosure())
	fmt.Println(viaLit())
	fmt.Println(viaChannel())
	fmt.Println(viaRange())
	fmt.Println(viaAssert(newSlow("assert")))
	fmt.Println(viaMap())
	fmt.Println(viaPromoted())
	fmt.Println(viaMethodExpr())
	fmt.Println(viaGo())
	fmt.Println(viaGoDecl())
	fmt.Println(read(newSlow("direct")))

	slowOnce()
	slowOnce()
	fmt.Println(slowValue())
	fmt.Println(viaAppend())
	fmt.Println(viaAny())
	fmt.Println(viaReturned())
	fmt.Println(viaFuncChan())
	fmt.Println(viaFuncConversion())
	fmt.Println(viaMapFunc())
	fmt.Println(viaPick())
	fmt.Println(viaPointer())
}
