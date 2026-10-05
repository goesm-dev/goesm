// Command bigfields exercises int64 and uint64 arithmetic on struct fields
// and through pointers, which stay BigInts, with operators nested so that
// the wrapping of inner results is folded into the outer ones.
package main

import (
	"fmt"
	"math"
)

type xorshift struct{ s uint64 }

func (x *xorshift) next() uint64 {
	x.s ^= x.s << 13
	x.s ^= x.s >> 7
	x.s ^= x.s << 17
	return x.s
}

type mix struct {
	u uint64
	i int64
}

//go:noinline
func id[T any](x T) T { return x }

func main() {
	r := &xorshift{88172645463325252}
	var acc uint64
	for i := 0; i < 1000; i++ {
		acc += r.next() >> 40
	}
	fmt.Println(r.s, acc)

	m := &mix{u: math.MaxUint64 - 5, i: math.MinInt64 + 9}
	k := id(uint64(0x9e3779b97f4a7c15))
	for i := 0; i < 5; i++ {
		m.u = (m.u*k + m.u<<3) ^ (m.u >> 11) | (m.u &^ k)
		m.i = (m.i*-7 - m.i<<5) ^ (m.i>>3) & ^(m.i+1)
		fmt.Println(m.u, m.i)
	}
	m.u = m.u<<63 + m.u<<62 + m.u*m.u*m.u - (m.u &^ (m.u << 1))
	m.i = -(m.i<<62 ^ m.i<<61) + m.i*m.i*m.i
	fmt.Println(m.u, m.i)

	p := &m.u
	*p = (*p ^ *p<<1) + (*p | ^*p) + (*p & (*p << 2))
	q := &m.i
	*q = (*q ^ *q<<1) + (*q | ^*q) + (*q &^ (*q << 2))
	fmt.Println(m.u, m.i, *p&1, *q|1)

	vals := []mix{{1, -1}, {math.MaxUint64, math.MaxInt64}, {1 << 63, math.MinInt64}}
	for j := range vals {
		v := &vals[j]
		v.u = (v.u << 1) ^ (v.u + v.u) ^ (v.u * 3) | (v.u-1)&(v.u<<2)
		v.i = (v.i << 1) ^ (v.i + v.i) ^ (v.i * 3) | (v.i-1)&(v.i<<2)
		fmt.Println(v.u, v.i, v.u/id(uint64(7)), v.i/id(int64(-7)), v.u%id(uint64(1000)), v.i%id(int64(1000)))
	}
}
