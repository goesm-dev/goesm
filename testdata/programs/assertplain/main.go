// Type assertions to types whose values are not copied, and map reads of
// such values, which goesm lowers without the runtime's general typeIs,
// copy and zero functions: present and missing keys, nil maps, stored nil
// values, keys of every map representation, failed assertions and their
// panics, named types with the same underlying type, and the generic forms.
package main

import (
	"errors"
	"fmt"
	"math"
)

type name string

func (n name) String() string { return "name:" + string(n) }

type count int

type node struct {
	v    int
	next *node
}

type myErr struct{ msg string }

func (e *myErr) Error() string { return e.msg }

type pair [2]int

func try(what string, f func() any) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("%s: panic: %v\n", what, r)
		}
	}()
	fmt.Printf("%s: %v\n", what, f())
}

func assertions(v any) {
	f, okf := v.(float64)
	s, oks := v.(string)
	b, okb := v.(bool)
	i, oki := v.(int)
	i64, oki64 := v.(int64)
	u8, oku8 := v.(uint8)
	n, okn := v.(name)
	c, okc := v.(count)
	p, okp := v.(*node)
	sl, oksl := v.([]any)
	m, okm := v.(map[string]any)
	fn, okfn := v.(func() int)
	ch, okch := v.(chan int)
	e, oke := v.(error)
	st, okst := v.(struct{ A int })
	ar, okar := v.(pair)
	cx, okcx := v.(complex128)
	fmt.Println(f, okf, s, oks, b, okb, i, oki, i64, oki64, u8, oku8, n, okn, c, okc)
	fmt.Println(p == nil, okp, sl == nil, len(sl), oksl, m == nil, len(m), okm, fn == nil, okfn, ch == nil, okch, e, oke, st, okst, ar, okar, cx, okcx)
	if okfn {
		fmt.Println("fn:", fn())
	}
	if okp && p != nil {
		fmt.Println("node:", p.v, p.next == nil)
	}
	if okar {
		ar[0] = 99 // a copy
	}
}

func get[T any](m map[string]T, k string) T { return m[k] }

func as[T any](v any) (T, bool) {
	t, ok := v.(T)
	return t, ok
}

func main() {
	var nilAny any
	vals := []any{nilAny, 1.5, math.NaN(), "x", "", true, 3, int64(-4), uint8(200), name("n"), count(7), "n",
		&node{v: 1}, (*node)(nil), []any{1, "a"}, []any(nil), map[string]any{"a": 1}, map[string]any(nil),
		func() int { return 42 }, make(chan int), &myErr{"bad"}, struct{ A int }{5}, pair{1, 2}, complex(1, 2), errors.New("e")}
	for _, v := range vals {
		fmt.Printf("-- %T\n", v)
		assertions(v)
	}
	arr := pair{1, 2}
	var pv any = arr
	assertions(pv)
	fmt.Println("array after:", pv.(pair), arr)

	// Single-value assertions, failed ones panicking with Go's message.
	var e error = &myErr{"boom"}
	try("float", func() any { return vals[1].(float64) })
	try("float of string", func() any { return vals[3].(float64) })
	try("nil", func() any { return nilAny.(string) })
	try("named", func() any { return vals[9].(name) })
	try("named of string", func() any { return vals[11].(name) })
	try("string of named", func() any { return vals[9].(string) })
	try("count of int", func() any { return vals[6].(count) })
	try("ptr", func() any { return e.(*myErr).msg })
	try("ptr of error", func() any { return vals[24].(*myErr) })
	try("map", func() any { return len(vals[16].(map[string]any)) })
	try("slice of map", func() any { return vals[16].([]any) })
	try("generic", func() any { x, ok := as[string](vals[3]); return fmt.Sprint(x, ok) })
	try("generic miss", func() any { x, ok := as[int](vals[3]); return fmt.Sprint(x, ok) })
	try("generic iface", func() any { x, ok := as[fmt.Stringer](vals[9]); return fmt.Sprint(x, ok) })

	// Map reads: present, missing, a nil map, a stored nil.
	ma := map[string]any{"f": 1.5, "s": "x", "nil": nil, "m": map[string]any{"k": true}}
	fmt.Println(ma["f"], ma["s"], ma["nil"], ma["missing"], ma["m"].(map[string]any)["k"])
	var nm map[string]any
	fmt.Println(nm["x"] == nil, len(nm))
	v, ok := ma["nil"]
	fmt.Println(v, ok)
	v, ok = ma["missing"]
	fmt.Println(v, ok)
	v, ok = nm["x"]
	fmt.Println(v, ok)
	mi := map[string]int{"a": 1}
	mi["a"] += 2
	mi["b"] += 5
	mi["c"]++
	x, okx := mi["z"]
	fmt.Println(mi["a"], mi["b"], mi["c"], mi["zz"], x, okx, len(mi))
	ms := map[name]string{"a": "A"}
	ms["a"] += "!"
	ms["b"] += "?"
	fmt.Println(ms["a"], ms["b"], ms["c"] == "", len(ms))
	mb := map[int]bool{1: true}
	fmt.Println(mb[1], mb[2])
	mf := map[float64]string{math.NaN(): "nan", 1: "one", -0.0: "zero"}
	fmt.Println(mf[math.NaN()] == "", mf[1], mf[0], len(mf))
	mp := map[pair]*node{{1, 2}: {v: 3}, {0, 0}: nil}
	pn, okpn := mp[pair{0, 0}]
	fmt.Println(mp[pair{1, 2}].v, mp[pair{3, 4}] == nil, pn == nil, okpn)
	mk := map[any]int{1: 1, "1": 2, 1.0: 3, pair{1, 1}: 4, nil: 5}
	kk, okk := mk[nil]
	fmt.Println(mk[1], mk["1"], mk[1.0], mk[pair{1, 1}], mk[pair{2, 2}], mk[int64(1)], kk, okk)
	mn := map[*node][]int{}
	k1 := &node{}
	mn[k1] = append(mn[k1], 1)
	mn[k1] = append(mn[k1], 2)
	fmt.Println(mn[k1], mn[&node{}] == nil, len(mn))
	mc := map[string]complex128{"c": 1i}
	mc["d"] += 2
	fmt.Println(mc["c"], mc["d"], mc["e"])
	mst := map[string]struct{ A int }{"a": {1}}
	sv := mst["a"]
	sv.A = 2
	fmt.Println(mst["a"], mst["b"], sv)
	fmt.Println(get(map[string]int{"a": 1}, "a"), get(map[string]int{}, "a"), get(map[string]*node{}, "a") == nil, get(map[string]pair{"p": {5, 6}}, "p"))
	mu := map[string]uint8{"a": 255}
	mu["a"]++
	fmt.Println(mu["a"], mu["b"])
	m64 := map[string]int64{"a": math.MaxInt64}
	m64["b"] -= 1
	fmt.Println(m64["a"], m64["b"], m64["c"])
}
