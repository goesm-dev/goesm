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
