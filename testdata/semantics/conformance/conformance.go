// Package conformance holds cases found by running the Go repository's own
// tests (GOROOT/test) through goesm.
package conformance

// ---- recursive types (issue17039, typeparam/issue47901) ----

type S []S

type Chan[T any] chan Chan[T]

type F func(F) int

func RecursiveTypes() []int {
	s := S{S{}, S{S{}, S{}}}
	c := make(Chan[int], 1)
	c <- c
	var f F = func(g F) int { return 7 }
	return []int{len(s), len(s[1]), len(<-c), cap(c), f(f)}
}

// ---- new(expr) (Go 1.26) ----

type point struct{ X, Y int }

func NewExpr() []int {
	p := new(42)
	q := new(point{1, 2})
	r := new(len("abc") * 2)
	*p++
	return []int{*p, q.X + q.Y, *r}
}

// ---- nil dereferences that must panic (issue15975, method5, nilptr2) ----

type closer interface{ Close() }

type val struct{ n int }

func (v val) Get() int   { return v.n }
func (v *val) PGet() int { return 1 }

func panics(f func()) (p bool) {
	defer func() { p = recover() != nil }()
	f()
	return
}

func NilPanics() []bool {
	var pv *val
	var f func() int
	return []bool{
		// defer x.Close() evaluates x.Close at the defer statement.
		panics(func() {
			var x closer
			defer func() { recover() }()
			func() {
				defer x.Close()
			}()
			panic("not reached")
		}) == false,
		panics(func() { f = pv.Get }),
		!panics(func() { f = pv.PGet }),
		panics(func() { _ = &*pv }),
		panics(func() { var c closer; g := c.Close; _ = g }),
		f != nil,
	}
}

// deferNilInterface reports where the panic of defer x.Close() is raised.
func DeferNilInterface() (where string) {
	defer func() {
		if recover() != nil && where == "" {
			where = "at defer statement"
		}
	}()
	var x closer
	defer x.Close()
	where = "after defer statement"
	return
}

// ---- range assignment order (range.go) ----

func RangeAssignOrder() []int {
	i := 1
	x := []int{0, 0}
	y := []int{10, 20}
	for i, x[i] = range y {
		break
	}
	a := []int{i, x[0], x[1]}
	r := []rune{0, 0}
	i = 1
	for i, r[i] = range "c" {
		break
	}
	a = append(a, i, int(r[0]), int(r[1]))
	z := []int{1, 2, 3}
	ri := rune(1)
	for z[ri], ri = range "\x02" {
		break
	}
	return append(a, z[0], z[1], z[2], int(ri))
}

// DeferValueReceiver: defer p.Get() with a value receiver copies *p at the
// defer statement.
func DeferValueReceiver() (out []int) {
	p := &rec{n: 1, out: &out}
	q := &recInt{n: 5, out: &out}
	func() {
		defer p.Log()
		defer q.Log()
		p.n = 2
		q.n = 6
	}()
	return out
}

type rec struct {
	n   int
	out *[]int
}

func (r rec) Log() { *r.out = append(*r.out, r.n) }

type recInt struct {
	n   int
	out *[]int
}

func (r recInt) Log() { *r.out = append(*r.out, r.n*10) }

type cnt int

var cntLog []int

func (c cnt) Log() { cntLog = append(cntLog, int(c)) }

func DeferValueReceiverScalar() []int {
	cntLog = nil
	p := new(cnt)
	*p = 3
	func() {
		defer p.Log()
		*p = 4
	}()
	var nilp *cnt
	return append(cntLog, len(cntLog), map[bool]int{false: 0, true: 1}[panics(func() { defer nilp.Log() })])
}

// ---- zero-size values share one address (zerosize.go, bug352) ----
//
// The spec leaves this unspecified; gc gives escaping zero-size values the
// address runtime.zerobase, which goesm matches. Locals that gc keeps on the
// stack may compare unequal there, so only escaping values are compared.

var (
	zsX      [0]int
	zsP, zsQ = new([0]int), new([0]int)
	zsArr    [10][0]byte
	zsSlice  = make([]struct{}, 10)
)

func ZeroSize() []bool {
	var np *[0]int
	return []bool{
		zsP == zsQ, &zsX == zsP,
		&zsArr[1] == &zsArr[2], &zsSlice[1] == &zsSlice[2],
		np == nil, np != zsP, zsP != nil,
	}
}

// ---- method expressions on literal and embedding types (method7, method) ----

var meLog string

type meS struct{}

func (meS) m()          { meLog += " m()" }
func (meS) m1(s string) { meLog += " m1(" + s + ")" }

type meT int

func (t meT) m2()  { meLog += " m2()" }
func (t *meT) m3() { *t += 5; meLog += " m3()" }

type meI interface{ m() }

type meOuter struct{ *meInner }
type meInner struct{ s string }

func (i meInner) M() string { return i.s }

type meOuter2 struct{ meInner }

func MethodExprs() string {
	meLog = ""
	meI.m(meS{})
	meS.m1(meS{}, "a")
	f := interface{ m1(string) }.m1
	f(meS{}, "b")
	interface{ m1(string) }.m1(meS{}, "c")
	g := struct{ meT }.m2
	g(struct{ meT }{})
	h := (*struct{ meT }).m3
	sv := &struct{ meT }{1}
	h(sv)
	h(sv)
	meLog += " " + string(rune('0'+sv.meT))
	k := (*struct{ meT }).m2
	k(&struct{ meT }{})
	meLog += " " + (*meOuter).M(&meOuter{&meInner{"hello"}})
	meLog += " " + meOuter.M(meOuter{&meInner{"world"}})
	meLog += " " + meOuter2.M(meOuter2{meInner{"!"}})
	meLog += " " + (*meOuter2).M(&meOuter2{meInner{"?"}})
	return meLog
}

// ---- unnamed struct types with promoted methods in interfaces (method.go) ----

type pmVal interface{ val() int }

type pmS struct{}

func (pmS) val() int { return 1 }

type pmS1 struct{}

func (*pmS1) val() int { return 2 }

type pmT int

func (pmT) val() int { return 7 }

func pmCall(v pmVal) int { return v.val() }

func PromotedIface() []int {
	var zs struct{ pmS }
	var zps struct{ *pmS1 }
	var zt struct{ pmT }
	var zv struct{ pmVal }
	zv.pmVal = zt
	var anys []any = []any{zs, &zs, zps, zt, &zt, zv}
	out := []int{pmCall(zs), pmCall(zps), pmCall(zt), pmCall(&zs), pmCall(&zt), pmCall(zv)}
	for _, a := range anys {
		if v, ok := a.(pmVal); ok {
			out = append(out, v.val())
		} else {
			out = append(out, -1)
		}
	}
	return out
}

// ---- generics (typeparam/interfacearg, typeparam/issue50833) ----

type gpS1 struct{ n int }

func (s *gpS1) M() int { return s.n }

type gpS2 struct{ n int }

func (s gpS2) M() int { return s.n * 10 }

func gpCall[T interface{ M() int }](t T) int {
	f := T.M
	return f(t)
}

type gpS struct{ f int }
type gpPS *gpS

func gpLit[P *gpS]() []P   { return []P{{f: 1}, {f: 2}} }
func gpLitPS[P gpPS]() []P { return []P{{f: 3}} }

func Generics() []int {
	out := []int{gpCall(&gpS1{1}), gpCall(gpS2{2}), gpCall(&gpS2{3})}
	for _, p := range gpLit[*gpS]() {
		out = append(out, p.f)
	}
	return append(out, (*gpLitPS[gpPS]()[0]).f)
}

func gpAdd[S ~string | ~[]byte](buf *[]byte, s S) {
	*buf = append(*buf, s...)
}

// AppendStringOrBytes: typeparam/issue376214.
func AppendStringOrBytes() string {
	var buf []byte
	gpAdd(&buf, "foo")
	gpAdd(&buf, []byte("bar"))
	return string(buf)
}

// ---- review follow-ups ----

type rvI interface{ M() string }
type rvImpl struct{ s string }

func (i rvImpl) M() string { return i.s }

type rvO struct{ rvI }
type rvOP struct{ *rvImpl }

// MethodExprEmbedded: method expressions of methods promoted from an
// embedded interface or pointer.
func MethodExprEmbedded() []string {
	f := rvO.M
	g := (*rvO).M
	h := struct{ rvI }.M
	msg := func(fn func()) (s string) {
		defer func() { s = recover().(error).Error() }()
		fn()
		return
	}
	return []string{
		f(rvO{rvImpl{"a"}}), g(&rvO{rvImpl{"b"}}), h(struct{ rvI }{rvImpl{"c"}}),
		rvOP.M(rvOP{&rvImpl{"d"}}),
		msg(func() { rvOP.M(rvOP{}) }),
	}
}

type rvInner struct{ n int }

var rvLog []int

func (i rvInner) V() { rvLog = append(rvLog, i.n) }

type rvOuterP struct{ *rvInner }

// DeferPromoted: defer o.V() evaluates the promoted receiver at the defer
// statement.
func DeferPromoted() []int {
	rvLog = nil
	o := rvOuterP{&rvInner{1}}
	func() {
		defer o.V()
		o.rvInner.n = 2
	}()
	after := 0
	panicked := panics(func() {
		var z rvOuterP
		defer z.V()
		after = 1
	})
	var c struct{ closer }
	after2 := 0
	panicked2 := panics(func() {
		defer c.Close()
		after2 = 1
	})
	return append(rvLog, after, b2i(panicked), after2, b2i(panicked2))
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

type zsE struct{}

var zsE1, zsE2 = new(zsE), new(zsE)

func zsEq[T comparable](a, b T) bool { return a == b }

// ZeroSizeConsistent: every spelling of == agrees on zero-size pointers.
func ZeroSizeConsistent() []bool {
	type hasP struct{ p *zsE }
	m := map[*zsE]int{zsE1: 1, zsE2: 2}
	var a, b any = zsE1, zsE2
	return []bool{zsE1 == zsE2, a == b, zsEq(zsE1, zsE2), hasP{zsE1} == hasP{zsE2}, len(m) == 1}
}

type gpPt struct{ X, Y int }
type gpPt2 gpPt

func (p gpPt2) Sum() int { return p.X + p.Y }

func gpStructLit[P ~struct{ X, Y int }]() []P { return []P{{1, 2}, {Y: 5}} }

// StructCoreLiteral: composite literals of a type parameter whose core type
// is a struct build values of the type argument.
func StructCoreLiteral() []int {
	a := gpStructLit[gpPt]()
	b := gpStructLit[gpPt2]()
	return []int{a[0].X, a[1].Y, b[0].Sum(), b[1].Sum()}
}
