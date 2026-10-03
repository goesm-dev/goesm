// Package basics holds semantic fixtures. Every exported function without
// parameters is a golden case: its result under native Go must equal its
// result in the ES module built by goesm.
package basics

import (
	"example.com/sem/initdeps/constonly"
	"example.com/sem/initdeps/registry"
	_ "example.com/sem/initdeps/sideeffect"
)

// ---- imports used only for initialization ----

func InitOnlyImports() []any {
	return []any{constonly.K, registry.Inited}
}

// ---- literal evaluation order ----

type Pair struct {
	A, B int
}

func KeyedLiteralOrder() []any {
	var order []int
	next := func(v int) int {
		order = append(order, v)
		return v
	}
	p := Pair{B: next(1), A: next(2)}
	arr := [3]int{2: next(3), 0: next(4)}
	s := []int{1: next(5), 0: next(6)}
	return []any{p, arr, s, order}
}

// ---- float32 constants ----

const third float32 = 1.0 / 3

func widen(x float32) float64 { return float64(x) }

func narrow() float32 { return 0.2 }

func Float32Const() []float64 {
	var f float32 = 0.1
	g := float32(1.0 / 3)
	return []float64{float64(f), float64(g), float64(f + g), float64(third), widen(0.3), float64(narrow())}
}

// ---- struct + method ----

type User struct {
	Name string
	Age  int
}

func (u User) Adult() bool {
	return u.Age >= 18
}

func (u *User) Birthday() {
	u.Age++
}

func NewUser(name string, age int) User { return User{Name: name, Age: age} }

func StructMethod() []bool {
	return []bool{User{Name: "a", Age: 20}.Adult(), User{Name: "b", Age: 3}.Adult()}
}

// Value semantics: assignment copies, pointer methods mutate in place.
func StructCopy() []int {
	u := User{Name: "x", Age: 17}
	v := u
	v.Age = 30
	u.Birthday()
	p := &u
	p.Birthday()
	w := *p
	w.Age = 100
	return []int{u.Age, v.Age, w.Age, p.Age}
}

type Point struct{ X, Y int }

type Rect struct {
	Min, Max Point
}

func (r Rect) Area() int { return (r.Max.X - r.Min.X) * (r.Max.Y - r.Min.Y) }

func NestedStructs() []int {
	r := Rect{Max: Point{3, 4}}
	q := r
	q.Max.X = 10
	pm := &r.Max
	pm.Y = 5
	return []int{r.Area(), q.Area(), r.Max.Y}
}

func StructEquality() []bool {
	a := Point{1, 2}
	b := Point{1, 2}
	c := Point{2, 1}
	return []bool{a == b, a == c, Rect{Min: a} == Rect{Min: b}}
}

// Embedding and promotion.
type Animal struct{ Sound string }

func (a Animal) Speak() string { return a.Sound + "!" }

type Dog struct {
	Animal
	Name string
}

func Embedded() string {
	d := Dog{Animal{"woof"}, "rex"}
	return d.Name + ":" + d.Speak() + ":" + d.Sound
}

// ---- slice + range ----

func Sum(values []int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}

func SliceRange() int { return Sum([]int{1, 2, 3, 4}) }

// Slices share backing arrays exactly like Go.
func SliceAliasing() []int {
	a := make([]int, 3, 10)
	b := append(a, 4)
	c := append(a, 5) // overwrites b[3]: same backing array
	a[0] = 9
	d := a[1:2]
	d = append(d, 7) // writes a[2]
	return []int{len(a), cap(a), b[3], c[3], b[0], a[2], len(d), cap(d)}
}

func SliceOfStructs() []User {
	us := []User{{"a", 1}, {"b", 2}}
	for _, u := range us {
		u.Age = 99 // copy; no effect
	}
	for i := range us {
		us[i].Age *= 10
	}
	return us
}

func NilSlice() []any {
	var s []int
	var t []int
	t = append(t, 1)
	return []any{s == nil, len(s), t == nil, len(t), s}
}

func Copy() []int {
	s := []int{1, 2, 3, 4, 5}
	n := copy(s[1:], s) // overlapping
	return append(s, n)
}

func Arrays() []int {
	a := [3]int{1, 2, 3}
	b := a
	b[0] = 100
	p := &a
	p[1] = 20
	s := a[:]
	s[2] = 30
	return []int{a[0], a[1], a[2], b[0], len(a)}
}

// ---- map ----

func Map() []any {
	m := map[string]int{
		"a": 1,
	}

	m["b"] = 2

	value, ok := m["a"]
	missing, ok2 := m["zzz"]
	delete(m, "a")
	_, ok3 := m["a"]
	return []any{value, ok, missing, ok2, ok3, len(m), m["b"]}
}

func MapStructKeys() []int {
	m := map[Point]int{}
	m[Point{1, 2}] = 10
	m[Point{1, 2}] += 5
	m[Point{2, 1}] = 1
	return []int{m[Point{1, 2}], m[Point{2, 1}], len(m)}
}

func MapInterfaceKeys() []any {
	m := map[any]string{}
	m[1] = "int"
	m[int64(1)] = "int64"
	m["1"] = "string"
	m[Point{1, 1}] = "point"
	return []any{len(m), m[1], m[int64(1)], m[Point{1, 1}]}
}

func NilMap() []any {
	var m map[string]int
	v, ok := m["x"]
	r := []any{v, ok, len(m), m == nil}
	defer func() {
		_ = recover()
	}()
	return r
}

func MapOfSlices() map[string][]int {
	m := map[string][]int{}
	m["x"] = append(m["x"], 1)
	m["x"] = append(m["x"], 2)
	m["y"] = nil
	return m
}

func MapRangeSum() int {
	m := map[string]int{"a": 1, "b": 2, "c": 3}
	total := 0
	for k, v := range m {
		total += v * len(k)
	}
	return total
}

// ---- pointer ----

func Set(v *int) {
	*v = 10
}

func PointerExample() int {
	value := 1
	Set(&value)
	return value
}

func PointerIdentity() []bool {
	x, y := 1, 1
	p, q := &x, &x
	r := &y
	s := []int{1, 2}
	u := User{}
	return []bool{p == q, p == r, &s[0] == &s[0], &s[0] == &s[1], &u.Age == &u.Age, &u == &u}
}

func PointerToField() []int {
	u := User{Age: 1}
	p := &u.Age
	*p = 5
	s := []int{1, 2, 3}
	q := &s[1]
	*q = 20
	pp := &p
	**pp = 7
	return []int{u.Age, s[1]}
}

func NewBuiltin() []int {
	p := new(int)
	*p = 3
	u := new(User)
	u.Age = 4
	return []int{*p, u.Age}
}

// ---- defer ----

func DeferExample() []int {
	result := []int{}

	defer func() {
		result = append(result, 3)
	}()

	result = append(result, 1)
	result = append(result, 2)

	return result
}

func DeferNamed() (result []int) {
	defer func() {
		result = append(result, 3)
	}()
	result = append(result, 1)
	return append(result, 2)
}

func DeferOrder() (out string) {
	for i := 0; i < 3; i++ {
		defer func() { out += string(rune('a' + i)) }()
	}
	return "x"
}

func DeferArgsEvaluatedEarly() (out []int) {
	x := 1
	defer func(v int) { out = append(out, v, x) }(x)
	x = 2
	return nil
}

// ---- interface ----

type Stringer interface {
	String() string
}

type Named struct {
	Name string
}

func (u Named) String() string {
	return u.Name
}

func Format(v Stringer) string {
	return v.String()
}

type Celsius float64

func (c Celsius) String() string { return "C" }

type Counter int

func (c *Counter) Inc()           { *c++ }
func (c *Counter) String() string { return "counter" }

func Interface() []string {
	var c Counter
	c.Inc()
	c.Inc()
	return []string{Format(Named{"gopher"}), Format(Celsius(1)), Format(&c), string(rune('0' + int(c)))}
}

func InterfaceNil() []bool {
	var s Stringer
	var p *Counter
	var t Stringer = p
	return []bool{s == nil, t == nil, t != nil}
}

func TypeSwitch(v any) string {
	switch x := v.(type) {
	case nil:
		return "nil"
	case int:
		return "int"
	case Celsius:
		return "celsius"
	case Stringer:
		return "stringer:" + x.String()
	case []int:
		return "slice"
	default:
		return "other"
	}
}

func TypeSwitches() []string {
	var c Counter
	return []string{TypeSwitch(nil), TypeSwitch(1), TypeSwitch(Celsius(2)), TypeSwitch(Named{"n"}), TypeSwitch(&c), TypeSwitch([]int{}), TypeSwitch("s"), TypeSwitch(int32(1))}
}

func TypeAssert() []any {
	var v any = Named{"x"}
	n, ok := v.(Named)
	_, ok2 := v.(*Named)
	s, ok3 := v.(Stringer)
	return []any{n.Name, ok, ok2, s.String(), ok3}
}

func InterfaceEquality() []bool {
	var a, b any = 1, 1
	var c any = int64(1)
	var d any = Point{1, 2}
	var e any = Point{1, 2}
	return []bool{a == b, a == c, d == e, a == any(1)}
}

// ---- closures, loops, control flow ----

func Closures() []int {
	var fs []func() int
	for i := 0; i < 3; i++ { // Go 1.22 per-iteration variables
		fs = append(fs, func() int { return i * i })
	}
	counter := 0
	inc := func() { counter++ }
	inc()
	inc()
	return []int{fs[0](), fs[1](), fs[2](), counter}
}

func LoopVarAddress() []int {
	var ps []*int
	for i := 0; i < 3; i++ {
		ps = append(ps, &i)
	}
	return []int{*ps[0], *ps[1], *ps[2]}
}

func Labels() []int {
	var out []int
outer:
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			if j == 2 {
				continue outer
			}
			if i == 3 {
				break outer
			}
			out = append(out, i*10+j)
		}
	}
	return out
}

func Switch(x int) string {
	switch {
	case x < 0:
		return "neg"
	case x == 0:
		return "zero"
	}
	s := ""
	switch x {
	case 1, 2:
		s += "small"
		fallthrough
	case 3:
		s += "+three"
	case 4:
		s += "four"
		break
	default:
		s += "big"
	}
	return s
}

func Switches() []string {
	return []string{Switch(-1), Switch(0), Switch(1), Switch(3), Switch(4), Switch(9)}
}

func RangeInt() int {
	t := 0
	for i := range 5 {
		t += i
	}
	return t
}

// ---- integers, strings ----

func Int32Overflow() []int32 {
	var x int32 = 2147483647
	x++
	var y int32 = -7
	return []int32{x, y / 2, y % 2, x * 3}
}

func Uint8Wrap() []uint8 {
	var b uint8 = 250
	b += 10
	c := uint8(300 % 256)
	return []uint8{b, c, ^b, b << 3}
}

func IntDivision() []int {
	return []int{7 / 2, -7 / 2, 7 % -3, -7 % 3}
}

func Strings() []any {
	s := "héllo, 世界"
	var runes []rune
	var idx []int
	for i, r := range s {
		idx = append(idx, i)
		runes = append(runes, r)
	}
	b := []byte(s)
	return []any{len(s), s[1], idx, runes, string(runes[1]), len(b), string(b[:2]), s[7:], len([]rune(s))}
}

func StringConcat() string {
	s := ""
	for i := 0; i < 3; i++ {
		s += string(rune('a'+i)) + "-"
	}
	return s + "日本"
}

func Floats() []float64 {
	var f32 float32 = 0.1
	a, b := 3.9, -3.9
	x, y := 0.1, 0.2
	return []float64{float64(f32), x + y, float64(int(a)), float64(int(b))}
}

const Pi = 3.14159

func Consts() []any {
	const big = 1 << 40
	type Weekday int
	const (
		Sunday Weekday = iota
		Monday
		Tuesday
	)
	return []any{big >> 38, int(Tuesday), Pi * 2}
}

func MinMax() []int { return []int{min(3, 1, 2), max(3, 1, 2)} }

// Range over function iterators (Go 1.23).
func Count(n int) func(func(int) bool) {
	return func(yield func(int) bool) {
		for i := 0; i < n; i++ {
			if !yield(i) {
				return
			}
		}
	}
}

func RangeFunc() []int {
	var out []int
	for i := range Count(10) {
		if i == 1 {
			continue
		}
		if i == 4 {
			break
		}
		out = append(out, i)
	}
	return out
}

func FindFirst() int {
	for i := range Count(10) {
		if i*i > 20 {
			return i
		}
	}
	return -1
}

type Shape interface {
	Area() int
	Perimeter() int
}

type Square struct{ S int }

func (s Square) Area() int      { return s.S * s.S }
func (s Square) Perimeter() int { return 4 * s.S }

func MethodValues() []int {
	sq := Square{3}
	f := sq.Area
	sq.S = 10 // f captured a copy of sq
	g := Square.Perimeter
	var sh Shape = sq
	h := sh.Area
	return []int{f(), g(Square{2}), h()}
}

var counter = initCounter()

func initCounter() int { return len(table) * 2 }

var table = map[string]int{"a": 1, "b": 2}

func PackageVars() []int { return []int{counter, len(table)} }

func init() {
	table["c"] = 3
}
