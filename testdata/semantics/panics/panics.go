// Package panics holds panic/recover/defer fixtures.
package panics

func Recover() (out string) {
	defer func() {
		out = "recovered: " + recover().(string)
	}()
	panic("boom")
}

func safeDiv(a, b int) (q int, err string) {
	defer func() {
		if r := recover(); r != nil {
			err = r.(interface{ Error() string }).Error()
		}
	}()
	return a / b, ""
}

func RuntimeErrors() []string {
	_, e1 := safeDiv(1, 0)
	e2 := catch(func() {
		var s []int
		_ = s[3]
	})
	e3 := catch(func() {
		var m map[string]int
		m["x"] = 1
	})
	e4 := catch(func() {
		var p *struct{ X int }
		p.X = 1
	})
	e5 := catch(func() {
		var x any = "str"
		_ = x.(int)
	})
	return []string{e1, e2, e3, e4, e5}
}

func catch(f func()) (msg string) {
	defer func() {
		r := recover()
		if e, ok := r.(interface{ Error() string }); ok {
			msg = e.Error()
		} else if s, ok := r.(string); ok {
			msg = s
		}
	}()
	f()
	return "no panic"
}

func DeferRunsOnPanic() (log []string) {
	defer func() {
		recover()
		log = append(log, "outer")
	}()
	func() {
		defer func() { log = append(log, "inner") }()
		panic("x")
	}()
	log = append(log, "unreachable")
	return
}

func RePanic() string {
	return catch(func() {
		defer func() {
			r := recover()
			panic(r.(string) + " again")
		}()
		panic("first")
	})
}

type MyErr struct{ Code int }

func (e *MyErr) Error() string { return "code " + string(rune('0'+e.Code)) }

func CustomError() []any {
	var err error = &MyErr{7}
	me, ok := err.(*MyErr)
	return []any{err.Error(), ok, me.Code}
}

func RecoverNotPanicking() bool {
	return recover() == nil
}

// MustPositive panics for JS callers to observe.
func MustPositive(x int) int {
	if x < 0 {
		panic("negative")
	}
	return x
}

// Deref dereferences p without recovering, for JS callers to observe.
func Deref(p *int) int {
	return *p
}

// IndexArrayPtr indexes p without recovering, for JS callers to observe.
func IndexArrayPtr(p *[3]int) int {
	return p[1]
}

type Node struct {
	Next *Node
	Val  int
}

type Outer struct{ *Node }

// FieldOf reads a field through a pointer (nil: a Go panic, not a TypeError).
func FieldOf(p *Node) int { return p.Val }

// NextVal reads a field through two pointer hops.
func NextVal(p *Node) int { return p.Next.Val }

// Call calls a function value (nil: a Go panic, not a TypeError).
func Call(f func() int) int { return f() }

func NilSelectorsAndCalls() []string {
	var log []string
	return []string{
		catch(func() { _ = FieldOf(nil) }),
		catch(func() { _ = NextVal(&Node{}) }),
		catch(func() {
			var o Outer
			o.Val = 1 // through the nil embedded *Node
		}),
		catch(func() { _ = Call(nil) }),
		catch(func() {
			var f func(int)
			f(func() int { log = append(log, "args first"); return 1 }())
		}),
		log[0],
	}
}

// NilStoreOrder stores through nil pointers: as in gc, the right-hand side
// is evaluated before the nil dereference panics.
func NilStoreOrder() []string {
	var log []string
	rhs := func(s string) int { log = append(log, s); return 1 }
	var p *Node
	var q *int
	var a *[3]int
	return append([]string{
		catch(func() { p.Val = rhs("field") }),
		catch(func() { *q = rhs("star") }),
		catch(func() { a[1] = rhs("array") }),
	}, log...)
}

// DivOrder divides by variable divisors: both operands are evaluated, left
// to right, before division by zero panics; remainders keep the dividend's
// sign (never -0), and an index computed by a call is evaluated once.
func DivOrder() []string {
	var log []string
	op := func(s string, v int) int { log = append(log, s); return v }
	zero, two := 0, 2
	var nilS []int
	s := []int{10, 20, 30}
	var i32, z32 int32 = -7, 0
	out := []string{
		catch(func() { _ = op("a", 7) / op("b", zero) }),
		catch(func() { _ = op("c", 7) % op("d", zero) }),
		catch(func() { _ = i32 % z32 }),
		catch(func() { _ = s[op("e", 2)%len(nilS)] }),
	}
	for _, v := range []int{-7, -4, 7, 0} {
		out = append(out, itoa(v%two), itoa(v/two), itoa(int(int32(v)%int32(two))), itoa(s[op("i", v&1)+1]))
	}
	return append(out, log...)
}

func itoa(v int) string {
	if v < 0 {
		return "-" + itoa(-v)
	}
	if v < 10 {
		return string(rune('0' + v))
	}
	return itoa(v/10) + string(rune('0'+v%10))
}

// LoopIndices indexes slices and strings in loops whose bounds are known
// (range, counting up to len) next to ones that look alike but may go out
// of range, which must still panic.
func LoopIndices() []string {
	s := []int{1, 2, 3, 4}
	str := "héllo"
	sum, bytes := 0, 0
	for i := range s {
		s[i] *= 2
		for j := i + 1; j < len(s); j++ {
			sum += s[i] * s[j]
		}
	}
	for i := 0; i < len(str); i += 2 {
		bytes += int(str[i])
	}
	type pt struct{ x int }
	ps := []pt{{1}, {2}}
	for i := range ps {
		p := &ps[i]
		p.x += i
	}
	m := map[int]bool{-1: true}
	out := []string{itoa(sum), itoa(bytes), itoa(ps[0].x + ps[1].x)}
	out = append(out,
		catch(func() {
			for i := -1; i < len(s); i++ {
				_ = s[i]
			}
		}),
		catch(func() {
			for i := 0; i < len(s); i++ {
				_ = s[i]
				i += 3
				_ = s[i]
			}
		}),
		catch(func() {
			t := s
			for i := 0; i < len(t); i++ {
				t = t[:1]
				_ = t[i]
			}
		}),
		catch(func() {
			for k := range m {
				_ = s[k]
			}
		}),
		catch(func() {
			for i := range s {
				_ = str[i*3]
			}
		}),
	)
	return out
}

// MadeLenIndices indexes slices in loops bounded by the length they were
// made with (a sieve), next to loops that look alike but may go out of
// range, which must still panic.
func MadeLenIndices() []string { return madeLenIndices(100) }

func madeLenIndices(n int) []string {
	composite := make([]bool, n)
	count := 0
	for i := 2; i < n; i++ {
		if composite[i] {
			continue
		}
		count++
		for j := i * i; j < n; j += i {
			composite[j] = true
		}
	}
	const k = 5
	sq := make([]int, k)
	for i := 0; i < k; i++ {
		sq[i] = i * i
	}
	out := []string{itoa(count), itoa(sq[k-1])}
	return append(out,
		catch(func() {
			m := n
			s := make([]int, m)
			m++
			for i := 0; i < m; i++ {
				s[i] = i
			}
		}),
		catch(func() {
			s := make([]int, k)
			for i := 0; i < k+1; i++ {
				s[i] = i
			}
		}),
		catch(func() {
			s := make([]int, n)
			for i := 0; i < n; i += 0 - 1 {
				s[i] = i
			}
		}),
		catch(func() {
			s := make([]int, n)
			s = s[:1]
			for i := 0; i < n; i++ {
				s[i] = i
			}
		}),
		catch(func() {
			b := make([]bool, 3)
			b[1] = n > 0
			for i := 0; i < 4; i++ {
				_ = b[i]
			}
		}),
	)
}

// LenVarIndices indexes slices in loops bounded by a variable set to their
// length (n := len(s); for i := range n), next to loops that look alike but
// may go out of range, which must still panic.
func LenVarIndices() []string {
	s := []byte("a<b>&c")
	n := len(s)
	esc := 0
	for i := range n {
		if s[i] == '<' || s[i] == '>' || s[i] == '&' {
			esc++
		}
	}
	sum := 0
	for i := 0; i < n; i++ {
		sum += int(s[i])
	}
	for i := range len(s) {
		sum += int(s[i])
	}
	out := []string{itoa(esc), itoa(sum)}
	return append(out,
		catch(func() {
			t := s
			m := len(t)
			m++
			for i := range m {
				_ = t[i]
			}
		}),
		catch(func() {
			t := s
			m := len(t)
			t = t[:2]
			for i := 0; i < m; i++ {
				_ = t[i]
			}
		}),
		catch(func() {
			t := s
			m := len(t)
			for i := range m {
				_ = t[i+1]
			}
		}),
		catch(func() {
			m := len(s)
			u := s[:1]
			for i := range m {
				_ = u[i]
			}
		}),
	)
}

// TableIndices indexes arrays with bytes and masked integers, which cannot
// go out of range of a [256]T, next to tables too short for them, which must
// still panic.
func TableIndices() []string {
	var table [256]int
	for i := range table {
		table[i] = i * 3
	}
	sum := 0
	for _, c := range []byte("héllo\xff") {
		sum += table[c]
	}
	for _, x := range []int{-1, 300, 7, -256} {
		sum += table[x&0xff]
		table[x&255]++
	}
	var t16 [1 << 16]bool
	t16[uint16(65535)] = true
	out := []string{itoa(sum), itoa(table[255]), itoa(table[44])}
	if t16[65535] {
		out = append(out, "t16")
	}
	var short [100]int
	return append(out,
		catch(func() {
			b := byte(200)
			_ = short[b]
		}),
		catch(func() {
			x := 1000
			short[x&0x7f] = 1
		}),
	)
}

// ArrayLoopIndices indexes arrays with the indices of loops over arrays and
// up to constants, next to arrays too short for them, which must still
// panic.
func ArrayLoopIndices() []string {
	var a, b [8]int
	var c [4]int
	for i := range a {
		a[i] = i
		b[i] = a[i] * 2
	}
	p := &b
	sum := 0
	for i := 0; i < 8; i++ {
		sum += p[i]
	}
	for i := range 4 {
		c[i] = a[i] + b[i]
	}
	out := []string{itoa(sum), itoa(c[3])}
	return append(out,
		catch(func() {
			for i := range a {
				c[i] = i
			}
		}),
		catch(func() {
			for i := 0; i < 8; i++ {
				_ = c[i]
			}
		}),
		catch(func() {
			var q *[8]int
			for i := range 8 {
				_ = q[i]
			}
		}),
	)
}
