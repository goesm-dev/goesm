// Command fmtverbs formats values of many kinds with fmt's verbs, flags,
// widths and the Stringer, error, Formatter and GoStringer interfaces.
package main

import (
	"errors"
	"fmt"
	"math"
	"os"
	"reflect"
	"strconv"
)

type point struct {
	X, Y int
}

type named struct {
	Name  string
	Inner *point
	List  []int
	M     map[string]int
	Any   any
	f     float64
}

type celsius float64

func (c celsius) String() string { return strconv.FormatFloat(float64(c), 'f', 1, 64) + "°C" }

type myErr struct{ code int }

func (e *myErr) Error() string { return fmt.Sprintf("my error %d", e.code) }

type gostr struct{}

func (gostr) GoString() string { return "gostr{}!" }

type custom int

func (c custom) Format(f fmt.State, verb rune) {
	w, wok := f.Width()
	p, pok := f.Precision()
	fmt.Fprintf(f, "custom(%c,%d,%v,%d,%v,%v)", verb, w, wok, p, pok, f.Flag('+'))
}

type color int

const (
	red color = iota
	green
)

func (c color) String() string { return [...]string{"red", "green"}[c] }

func main() {
	p := point{1, -2}
	n := named{Name: "n", List: []int{1, 2}, M: map[string]int{"b": 2, "a": 1, "c": 3}, Any: 1.5, f: 2}
	fmt.Println(p, &p, n.List, n.M)
	fmt.Printf("%v|%+v|%#v\n", p, p, p)
	fmt.Printf("%v\n%+v\n", n, n)
	fmt.Printf("%#v\n", n.List)
	fmt.Printf("%#v %#v %#v\n", n.M, []string{"x"}, map[int]bool{2: true, 1: false})
	fmt.Printf("%T %T %T %T %T %T\n", p, &p, n.List, n.M, n.Any, celsius(1))
	fmt.Printf("%d|%5d|%-5d|%05d|%+d|%x|%X|%o|%O|%b|%#x|%#o|%c|%U|%#U|%q\n", 42, 42, 42, 42, 42, 255, 255, 8, 8, 5, 255, 8, 'G', 0x1F600, 'x', 'q')
	fmt.Printf("%f|%.2f|%8.3f|%-8.2f|%e|%E|%g|%G|%.3g|%v|%v|%v\n", math.Pi, math.Pi, math.Pi, math.Pi, 123456.789, 1e-7, 1e21, 1e-5, 2.0/3, 1e6, 1e21, float32(0.1))
	fmt.Printf("%v %v %v %.1f %5.1f|\n", math.Inf(1), math.Inf(-1), math.NaN(), math.Inf(1), -0.0)
	fmt.Printf("%s|%10s|%-10s|%.2s|%q|%x|% x|%X|%#q\n", "héllo", "hi", "hi", "héllo", "a\"b\n", "hi", "hi", []byte("hi"), "back`tick")
	fmt.Printf("%v %d %s %x\n", []byte("ab"), []byte("ab"), []byte("ab"), []byte{1, 171})
	fmt.Printf("%t %v %08.3f %+.2e\n", true, false, -3.14159, 12345.678)
	fmt.Printf("%v %v\n", complex(1, -2), complex64(complex(0.5, 0.25)))
	fmt.Printf("%v %s %d\n", celsius(21.5), celsius(-3), celsius(4))
	fmt.Printf("%v %v %s\n", red, []color{green, red}, map[color]int{green: 1})
	var e error = &myErr{7}
	fmt.Println(e, []error{e, nil})
	w := fmt.Errorf("wrapped: %w", e)
	var target *myErr
	fmt.Println(w, errors.As(w, &target), target.code, errors.Unwrap(w) == e)
	fmt.Printf("%v %#v\n", gostr{}, gostr{})
	fmt.Printf("%v|%+8.3x|%d\n", custom(1), custom(2), custom(3))
	fmt.Printf("%d %s\n", "str", 5)
	fmt.Printf("%d\n")
	fmt.Printf("%d %d\n", 1, 2, 3)
	fmt.Printf("%!\n", 1)
	fmt.Printf("%[2]d %[1]d %*d|%-*d|%.*f\n", 1, 2, 4, 7, 3, 8, 2, math.E)
	var np *point
	var ni any
	var ns []int
	var nm map[string]int
	fmt.Println(np, ni, ns, nm, e == nil)
	fmt.Printf("%v %+v %#v %#v %#v\n", np, ni, ns, nm, ni)
	fmt.Print("a", "b", 1, 2, "c", 3.5, nil, "\n")
	fmt.Println(fmt.Sprint(1, 2), fmt.Sprintln("x", 1), fmt.Sprintf("%6.2f%%", 99.5))
	fmt.Println([3]int{1, 2, 3}, [2][2]string{{"a", "b"}, {"c", "d"}}, struct {
		A int
		B string
	}{1, "x"})
	fmt.Printf("%v %d\n", []any{1, "a", nil, 2.5, []int{1}}, []int8{-1, 2})
	fmt.Printf("%x %X %x\n", -255, "Hello", 3.5)
	fmt.Printf("%v %v %v\n", int64(math.MaxInt64), uint64(math.MaxUint64), int64(math.MinInt64))
	fmt.Printf("%d %x %b\n", uint64(1<<63), int64(-1), int64(5))
	fmt.Fprintln(os.Stderr, "to stderr")
	var s string
	var i, j int
	k, err := fmt.Sscanf("abc 12 34", "%s %d %d", &s, &i, &j)
	fmt.Println(k, err, s, i, j)
	k, err = fmt.Sscan("7 8.5 word", &i, new(float64), &s)
	fmt.Println(k, err, i, s)
	sprintfMatrix()
	loweredSprintf()
}

type plainInt int

// sprintfMatrix prints Sprintf of many formats and values: the cases fmt
// formats by concatenation under goesm and the ones next to them that it
// leaves to fmt's own code.
func sprintfMatrix() {
	formats := []string{
		"%d", "%5d", "%-5d|", "%05d", "%-05d|", "%x", "%X", "%08x", "%v", "%s", "%10s|", "%-10s|", "%t", "%6t|",
		"%f", "%.2f", "%.0f", "%8.3f", "%08.3f", "%-8.2f|", "%e", "%.3e", "%E", "%g", "%.4g", "%G", "%F", "%9.2e",
		"%%", "%5.1f%%", "a%db", "%", "%z", "%+d", "% d", "%#x", "%q", "%c", "%U", "%o", "%b", "%.3d", "%.2s",
		"%05s", "%*d", "%[1]d", "%v %v", "", "%!", "x%", "%.f", "%3.d|", "%010.4f", "%-010d|", "%0-10d|",
	}
	values := []any{
		0, 42, -42, int8(-128), int16(300), int32(-7), int64(math.MinInt64), uint8(255), uint16(65535),
		uint32(4294967295), uint64(math.MaxUint64), uint(1 << 60), uintptr(10), 1 << 53, -1 << 60,
		0.0, math.Copysign(0, -1), 0.125, 2.675, -1.5, 1e21, 1e20, 1e-7, 123456789.0, 1.0 / 3, 100000.0, 1e6,
		math.NaN(), math.Inf(1), math.Inf(-1), float32(0.1), float32(16777216), 5e-324, math.MaxFloat64,
		"", "héllo", "日本", "a%b", true, false, plainInt(3), celsius(21.5), nil, errors.New("boom"), []int{1},
	}
	for _, f := range formats {
		line := f + " =>"
		for _, v := range values {
			line += " " + fmt.Sprintf(f, v)
		}
		fmt.Println(line)
	}
	fmt.Println(fmt.Sprintf("%d:%s:%.2f:%x|%v", 7, "item", 7.0/3, 7*31, false))
	fmt.Println(fmt.Sprintf("%d %d", 1), fmt.Sprintf("%d", 1, 2), fmt.Sprintf("no verbs"), fmt.Sprintf("%s-%s", "a", "b"))
}

// loweredSprintf calls Sprintf with constant formats on variables of
// predeclared types, which goesm lowers to string concatenation, at the
// values where JS formatting differs from Go's.
func loweredSprintf() {
	for _, i := range []int{0, 1, -1, 255, -255, 1<<31 - 1, -1 << 31, 1<<53 - 1, -(1<<53 - 1), 1 << 53, 1<<53 + 2, -1 << 60} {
		fmt.Println(fmt.Sprintf("%d|%v|%x|%X", i, i, i, i), fmt.Sprintf("[%d]", i))
	}
	for _, u := range []uint{0, 7, 1<<53 - 1, 1 << 53, 1 << 63} {
		fmt.Println(fmt.Sprintf("%d %x %X", u, u, u), fmt.Sprintf("%v", uintptr(u)))
	}
	for _, i := range []int64{0, -1, math.MinInt64, math.MaxInt64} {
		u := uint64(i)
		fmt.Println(fmt.Sprintf("%d %x %X %v", i, i, i, i), fmt.Sprintf("%d %x %X %v", u, u, u, u))
	}
	i8, i16, i32 := int8(-128), int16(-300), int32(math.MinInt32)
	u8, u16, u32 := uint8(255), uint16(65535), uint32(math.MaxUint32)
	r, b := rune('世'), byte('A')
	fmt.Println(fmt.Sprintf("%d %x %d %X %d %x|%v %x %v %X %d %x|%d %v %x", i8, i8, i16, i16, i32, i32, u8, u8, u16, u16, u32, u32, r, b, b))
	for _, f := range []float64{0, math.Copysign(0, -1), 0.125, 0.375, 2.675, -1.5, 1.005, 1e20, 1e21, -1e21, 1e-7, 123456789, 1.0 / 3, 5e-324, math.MaxFloat64, math.NaN(), math.Inf(1), math.Inf(-1)} {
		fmt.Println(fmt.Sprintf("%v|%f|%F|%.0f|%.2f|%.1f", f, f, f, f, f, f), fmt.Sprintf("%.20f", f))
	}
	for _, s := range []string{"", "héllo", "日本", "a%b", "\xff\x00"} {
		t := s == ""
		fmt.Println(fmt.Sprintf("<%s|%v|%t|%v>", s, s, t, !t))
	}
	n := 0
	next := func() int { n++; return n }
	fmt.Println(fmt.Sprintf("%d %d %d", next(), next()*10, next()), fmt.Sprintf("%d%d", n, n), fmt.Sprintf("%v%v", true, n))
	const c, h, neg = 12345678901234, 0xbeef, -42
	fmt.Println(fmt.Sprintf("%d %x %X %v %d %x %s %t %v", c, h, h, 'a', neg, neg, "lit%", false, 1.5))
	fmt.Println(fmt.Sprintf("100%%"), fmt.Sprintf("%d%%", n), fmt.Sprintf("%.2f%%", 99.5))

	// Errors and Stringers through Sprintf and Errorf.
	var me error = &myErr{8}
	var nilErr *myErr
	var nilColor *color
	fmt.Println(fmt.Sprintf("a %v b %s c %8v d %-8s|", me, me, red, green))
	fmt.Println(fmt.Sprintf("%v %s %x %q %d", nilErr, nilColor, me, red, red))
	fmt.Println(fmt.Sprintf("%w %v %.3s %05s", me, panicky{}, me, red))
	fmt.Println(fmt.Sprintf("%v %s", reflect.ValueOf(red), reflect.ValueOf(me)))
	fmt.Println(fmt.Sprintf("%v %s", custom(5), custom(6)))
	var nilIface error
	fmt.Println(fmt.Sprintf("%v %s", nilIface, nilIface))
	sentinel := errors.New("sentinel")
	e1 := fmt.Errorf("line %d: %w", 3, sentinel)
	e2 := fmt.Errorf("ctx: %v", sentinel)
	e3 := fmt.Errorf("%w and %w", sentinel, me)
	e4 := fmt.Errorf("%w", "not an error")
	e5 := fmt.Errorf("deep: %w", e1)
	fmt.Println(e1, errors.Is(e1, sentinel), errors.Unwrap(e1) == sentinel)
	fmt.Println(e2, errors.Is(e2, sentinel), errors.Unwrap(e2) == nil)
	fmt.Println(e3, errors.Is(e3, sentinel), errors.Is(e3, me))
	fmt.Println(e4, errors.Unwrap(e4) == nil)
	fmt.Println(e5, errors.Is(e5, sentinel), fmt.Sprintf("%T %T %T", e1, e2, e3))
	fmt.Println(fmt.Errorf("%w: %v", nilErr, red), fmt.Errorf("%s", panicky{}))
}

type panicky struct{}

func (panicky) Error() string { panic("boom") }
