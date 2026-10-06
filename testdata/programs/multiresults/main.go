// Several results, which goesm returns in result registers from a function
// that does not block and in an array from one that does: every way a
// caller can take them, and the code between a call and the use of its
// results that may call Go code.
package main

import (
	"errors"
	"fmt"
	"math/bits"
	"reflect"
	"strconv"
	"time"
)

var errOdd = errors.New("odd")

func divmod(a, b int) (int, int) { return a / b, a % b }

func three(i int) (int, string, error) {
	if i%2 == 1 {
		return i, "odd", errOdd
	}
	return i, strconv.Itoa(i), nil
}

// clobber calls a function with several results itself.
func clobber() int {
	q, r := divmod(100, 7)
	return q*10 + r
}

// order returns values evaluated in order, the last calling Go code.
func order(x int) (int, int, int) {
	return x, clobber(), x + 1
}

func orderFirst(x *int) (int, int) {
	return bump(x), *x
}

func bump(x *int) int { *x += 10; return *x }

// passes on the results of a call.
func pass(i int) (int, string, error) { return three(i) }

// converts them as it passes them on.
func passConv(i int) (int, any, any) { return three(i) }

func named(i int) (q, r int) {
	q, r = divmod(i, 3)
	if i > 5 {
		return
	}
	return r, q
}

func deferred(i int) (n int, err error) {
	defer func() {
		if r := recover(); r != nil {
			n, err = -1, fmt.Errorf("recovered: %v", r)
		}
	}()
	if i < 0 {
		panic("negative")
	}
	return divmodErr(i, 2)
}

func divmodErr(a, b int) (int, error) {
	if b == 0 {
		return 0, errors.New("division by zero")
	}
	return a / b, nil
}

func unnamedDefer(i int) (int, string) {
	defer func() { _ = clobber() }()
	return i * 2, "d" + strconv.Itoa(i)
}

// blocks: an async function.
func sleepy(i int) (int, string, error) {
	time.Sleep(time.Millisecond)
	q, r := divmod(i, 4)
	return q, strconv.Itoa(r), nil
}

func sleepyPass(i int) (int, string, error) { return sleepy(i) }

func sleepyPassConv(i int) (int, any, error) { return sleepy(i) }

type pair struct{ a, b int }

func (p pair) Swap() (int, int)   { return p.b, p.a }
func (p *pair) Grow() (int, bool) { p.a++; return p.a, p.a > 2 }

type swapper interface{ Swap() (int, int) }

type slowPair struct{ pair }

func (s slowPair) Swap() (int, int) {
	time.Sleep(time.Millisecond)
	return s.b * 10, s.a * 10
}

func seq(yield func(int) bool) {
	for i := 0; i < 5; i++ {
		if !yield(i) {
			return
		}
	}
}

// fromRange returns from a range-over-func body.
func fromRange(stop int) (int, string) {
	for i := range seq {
		if i == stop {
			return i, "stopped at " + strconv.Itoa(i)
		}
	}
	return -1, "none"
}

func fromRangeDefer(stop int) (n int, s string) {
	defer func() { s += "!" }()
	for i := range seq {
		if i == stop {
			return i * 10, "at " + strconv.Itoa(clobber())
		}
	}
	return -1, "none"
}

func takes(a int, b string, err error) string {
	return fmt.Sprint(a, b, err)
}

func variadic(xs ...any) string { return fmt.Sprint(len(xs), xs) }

var pkgA, pkgB = divmod(17, 5)
var pkgV, pkgOk = map[string]int{"x": 9}["x"]

// Struct results and comma-ok values are the caller's own copies: of the
// struct a function returns, and of a map's value.
type seg struct{ start, stop int }

type reader struct{ pos seg }

func (r *reader) position() (int, seg) { return 1, r.pos }

var segs = map[string]seg{"x": {1, 2}}
var pkgSeg, pkgSegOk = segs["x"]

func main() {
	q, r := divmod(17, 5)
	fmt.Println("divmod", q, r)
	var a, b int
	a, b = divmod(23, 4)
	fmt.Println("assign", a, b)
	var c, d = divmod(9, 2)
	fmt.Println("var", c, d)
	_, only := divmod(9, 4)
	fmt.Println("blank", only)

	arr := []int{0, 0, 0}
	m := map[string]int{}
	arr[clobber()%3], m["k"] = divmod(31, 6)
	fmt.Println("targets", arr, m)

	for i := 0; i < 3; i++ {
		n, s, err := three(i)
		fmt.Println("three", n, s, err)
	}
	fmt.Println("order", fmt.Sprint(order(5)))
	x := 1
	f1, f2 := orderFirst(&x)
	fmt.Println("orderFirst", f1, f2)
	fmt.Println("pass", fmt.Sprint(pass(3)), fmt.Sprint(pass(4)))
	fmt.Println("passConv", fmt.Sprint(passConv(3)))
	fmt.Println("named", fmt.Sprint(named(4)), fmt.Sprint(named(8)))
	fmt.Println("deferred", fmt.Sprint(deferred(7)), fmt.Sprint(deferred(-1)))
	fmt.Println("unnamedDefer", fmt.Sprint(unnamedDefer(3)))
	fmt.Println("sleepy", fmt.Sprint(sleepy(11)), fmt.Sprint(sleepyPass(13)), fmt.Sprint(sleepyPassConv(14)))
	sq, ss, serr := sleepy(15)
	fmt.Println("sleepy vars", sq, ss, serr)

	fmt.Println("takes", takes(three(5)), takes(three(6)))
	fmt.Println("variadic", variadic(three(8)))
	fmt.Println("println", len(fmt.Sprintln(divmod(8, 3))))
	println("builtin println", 1)
	sl := append([]int{}, 1)
	fmt.Println("append", sl)
	cx := complex(divmodf(7, 2))
	fmt.Println("complex", cx)

	fns := []func(int) (int, string, error){three, pass, sleepy}
	for i, f := range fns {
		n, s, err := f(i + 20)
		fmt.Println("func value", i, n, s, err)
	}
	var sw swapper = pair{1, 2}
	s1, s2 := sw.Swap()
	fmt.Println("iface", s1, s2)
	sw = slowPair{pair{3, 4}}
	s1, s2 = sw.Swap()
	fmt.Println("iface async", s1, s2)
	p := &pair{1, 5}
	for {
		v, done := p.Grow()
		fmt.Println("grow", v, done)
		if done {
			break
		}
	}
	mv := pair{7, 8}.Swap
	fmt.Println("method value", fmt.Sprint(mv()))
	me := pair.Swap
	fmt.Println("method expr", fmt.Sprint(me(pair{5, 6})))

	fmt.Println("range", fmt.Sprint(fromRange(3)), fmt.Sprint(fromRange(9)))
	fmt.Println("range defer", fmt.Sprint(fromRangeDefer(2)))

	v, ok := m["k"]
	v2, ok2 := m["nope"]
	fmt.Println("comma ok", v, ok, v2, ok2)
	var anyV any = 3
	iv, iok := anyV.(int)
	sv, sok := anyV.(string)
	fmt.Println("assert", iv, iok, sv, sok)
	if n, err := strconv.Atoi("42"); err == nil {
		fmt.Println("atoi", n)
	}
	if _, err := strconv.Atoi("4x2"); err != nil {
		fmt.Println("atoi err", err)
	}
	ch := make(chan int, 1)
	ch <- 4
	close(ch)
	cv, cok := <-ch
	cv2, cok2 := <-ch
	fmt.Println("recv", cv, cok, cv2, cok2)

	hi, lo := bits.Mul64(1<<40, 1<<40)
	sum, carry := bits.Add64(^uint64(0), 1, 0)
	fmt.Println("bits", hi, lo, sum, carry)

	rf := reflect.ValueOf(divmod)
	out := rf.Call([]reflect.Value{reflect.ValueOf(29), reflect.ValueOf(4)})
	fmt.Println("reflect call", out[0].Int(), out[1].Int())
	mk := reflect.MakeFunc(reflect.TypeOf(divmod), func(args []reflect.Value) []reflect.Value {
		return []reflect.Value{reflect.ValueOf(int(args[0].Int() * 2)), reflect.ValueOf(int(args[1].Int() * 3))}
	})
	var made func(int, int) (int, int)
	reflect.ValueOf(&made).Elem().Set(mk)
	m1, m2 := made(4, 5)
	fmt.Println("reflect makefunc", m1, m2)
	mm := reflect.ValueOf(map[string]int{"a": 1})
	fmt.Println("reflect mapindex", mm.MapIndex(reflect.ValueOf("a")), mm.MapIndex(reflect.ValueOf("b")).IsValid())

	fmt.Println("pkg vars", pkgA, pkgB, pkgV, pkgOk)
	rd := &reader{seg{3, 4}}
	_, p1 := rd.position()
	rd.pos.start = 30
	var _, p2 = rd.position()
	p2.stop = 40
	fmt.Println("struct results", p1, p2, rd.pos)
	var sa, saOk = segs["x"]
	sa.start = 10
	sb, _ := segs["x"]
	sb.stop = 20
	var sx seg
	sx, saOk = segs["x"]
	sx.start = 30
	pkgSeg.stop = 50
	fmt.Println("map values", sa, sb, sx, saOk, pkgSeg, pkgSegOk, segs["x"])
	lit := func() (string, int) { return "lit", clobber() }
	ls, li := lit()
	fmt.Println("func lit", ls, li)
}

func divmodf(a, b float64) (float64, float64) { return a / b, a * b }
