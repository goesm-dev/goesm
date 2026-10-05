// Package int64s checks exact 64-bit integer semantics: int64 and uint64
// are BigInts in goesm.
package int64s

import (
	"math"
	"math/bits"
	"strconv"
	"sync/atomic"
)

func Wrap() []uint64 {
	var x uint64
	x--
	y := uint64(math.MaxUint64)
	y++
	z := uint64(1) << 63
	return []uint64{x, y, z * 2, z + z - 1, -z}
}

func SignedWrap() []int64 {
	a := int64(math.MaxInt64)
	a++
	b := int64(math.MinInt64)
	c := b / -1
	d := -b
	m := int64(3037000500)
	e := m * m
	return []int64{a, b, c, d, e, b % -1}
}

func Precision() []int64 {
	x := int64(1) << 62
	return []int64{x + 1, x - 1, x*2 - 1, 9007199254740993}
}

func DivRem() []int64 {
	var out []int64
	for _, p := range [][2]int64{{7, 2}, {-7, 2}, {7, -2}, {-7, -2}, {math.MaxInt64, 3}, {math.MinInt64, 7}} {
		out = append(out, p[0]/p[1], p[0]%p[1])
	}
	return out
}

func DivByZero() (r string) {
	defer func() { r = recover().(error).Error() }()
	var z int64
	_ = int64(1) / z
	return "no panic"
}

func Bitwise() []uint64 {
	a := uint64(0xF0F0F0F0F0F0F0F0)
	b := uint64(0x0FF00FF00FF00FF0)
	return []uint64{a & b, a | b, a ^ b, a &^ b, ^a, ^uint64(0)}
}

func SignedBitwise() []int64 {
	a, b := int64(-6), int64(0x7FFF00000000FFFF)
	return []int64{a & b, a | b, a ^ b, a &^ b, ^a, ^b}
}

func Shifts() []uint64 {
	x := uint64(0xDEADBEEFCAFEBABE)
	var out []uint64
	for _, n := range []uint{0, 1, 4, 31, 32, 33, 63, 64, 100} {
		out = append(out, x<<n, x>>n)
	}
	var k int64 = 8
	out = append(out, x<<k, x>>uint64(k))
	return out
}

func SignedShifts() []int64 {
	x := int64(-0x123456789)
	var out []int64
	for _, n := range []int{0, 1, 31, 32, 63, 64, 70} {
		out = append(out, x<<n, x>>n)
	}
	return out
}

func Conversions() []any {
	big := uint64(0xFFFFFFFF_FFFFFFFF)
	neg := int64(-1)
	f, g := 1e19, -2.9
	x1ff, m129, k70000 := uint64(0x1FF), int64(-129), int64(70000)
	one, five, seven, cjk := int64(1), int(-5), uint32(7), int64(0x4e16)
	return []any{
		int64(big), uint32(big), int32(neg), uint8(x1ff), int8(m129),
		uint64(neg), int(one << 40), int64(five), uint64(seven),
		float64(one<<62 + 1), float64(float32(one<<40 + 1)), uint64(f), int64(g), int64(f / 2),
		string(rune(cjk)), uint(one + 4), uint16(k70000),
	}
}

func Fnv64() uint64 {
	h := uint64(14695981039346656037)
	for _, c := range []byte("goesm compiles Go to ES modules") {
		h ^= uint64(c)
		h *= 1099511628211
	}
	return h
}

func XorShift() []uint64 {
	x := uint64(88172645463325252)
	var out []uint64
	for range 5 {
		x ^= x >> 12
		x ^= x << 25
		x ^= x >> 27
		out = append(out, x*2685821657736338717)
	}
	return out
}

type key struct {
	a int64
	b uint64
}

func MapKeys() []any {
	m := map[int64]string{math.MinInt64: "min", 1 << 60: "big", 1: "one"}
	n := map[key]int{{1 << 62, 5}: 1}
	n[key{1 << 62, 5}]++
	var i any = int64(7)
	var j any = 7
	return []any{m[math.MinInt64], m[1<<60], m[(1<<60)+1] == "", n[key{1 << 62, 5}], i == j, i == any(int64(7))}
}

func Sum[T ~int64 | ~uint64 | ~int](xs ...T) T {
	var s T
	for _, x := range xs {
		s += x
	}
	return s * 2 / 2
}

type Duration int64

func Generic() []any {
	return []any{
		Sum[int64](math.MaxInt64, 1),
		Sum[uint64](math.MaxUint64, 2),
		Sum[int](1, 2, 3),
		Sum(Duration(5), 6),
	}
}

func Strconv() []any {
	a, err1 := strconv.ParseInt("-9223372036854775808", 10, 64)
	b, err2 := strconv.ParseUint("18446744073709551615", 10, 64)
	_, err3 := strconv.ParseInt("9223372036854775808", 10, 64)
	return []any{
		a, err1 == nil, b, err2 == nil, err3.Error(),
		strconv.FormatInt(math.MinInt64, 16), strconv.FormatUint(math.MaxUint64, 2),
		strconv.FormatFloat(0.1, 'g', -1, 64), strconv.FormatFloat(1.0/3, 'e', -1, 32),
		strconv.Itoa(1 << 40),
	}
}

func MathBits() []any {
	hi, lo := bits.Mul64(math.MaxUint64, 3)
	q, r := bits.Div64(1, 5, 7)
	s, c := bits.Add64(math.MaxUint64, 1, 0)
	return []any{
		bits.LeadingZeros64(1), bits.TrailingZeros64(1 << 40), bits.OnesCount64(math.MaxUint64),
		bits.Len64(0), bits.RotateLeft64(1, -1), bits.Reverse64(1), bits.ReverseBytes64(0x0102030405060708),
		hi, lo, q, r, s, c, math.Float64bits(-0.0), math.Float64frombits(0x7FF0000000000000) > 0,
	}
}

func Atomics() []int64 {
	var n atomic.Int64
	n.Store(math.MaxInt64)
	n.Add(1)
	var u atomic.Uint64
	u.Add(^uint64(0))
	return []int64{n.Load(), int64(u.Load()), atomic.AddInt64(new(int64), -3)}
}

func RangeAndIndex() []int64 {
	var n int64 = 4
	s := make([]int64, n)
	for i := range n {
		s[i] = i * i
	}
	var j uint64 = 2
	s[j]++
	arr := [3]int64{}
	arr[int64(1)] = 9
	return append(s[int64(1):j+1], arr[1], int64(len(s)))
}

func IncDec() []int64 {
	x := int64(math.MaxInt64)
	x++
	y := int64(math.MinInt64)
	y--
	z := int64(10)
	z += 5
	z <<= 60
	z >>= 2
	z *= 3
	return []int64{x, y, z}
}

// Cases that were known gaps while int64 and uint64 were JS numbers.
func FormerGaps() []any {
	var wrap uint64
	wrap--
	n, err := strconv.ParseInt("-42", 10, 64)
	return []any{wrap, int64(1)<<62 + 1, n, err == nil, strconv.FormatFloat(0.1, 'g', -1, 64)}
}

// The functions below compute in loops, where goesm holds int64 and uint64
// locals as two int32 halves (internal/lower/split64.go).

func SplitHash() []uint64 {
	var out []uint64
	for _, s := range []string{"", "a", "goesm", "\xff\x00\x80"} {
		h := uint64(14695981039346656037)
		for i := 0; i < len(s); i++ {
			h ^= uint64(s[i])
			h *= 1099511628211
		}
		out = append(out, h)
	}
	return out
}

func SplitMix() []uint64 {
	var out []uint64
	x := uint64(0x123456789abcdef0)
	for range 8 {
		x += 0x9e3779b97f4a7c15
		z := x
		z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
		z = (z ^ (z >> 27)) * 0x94d049bb133111eb
		out = append(out, z^(z>>31))
	}
	return out
}

func SplitXorShift() uint64 {
	x := uint64(88172645463325252)
	var sum uint64
	for range 1000 {
		x ^= x >> 12
		x ^= x << 25
		x ^= x >> 27
		sum += x * 2685821657736338717
	}
	return sum
}

func SplitSigned() []int64 {
	var out []int64
	s := int64(-1)
	t := int64(math.MaxInt64)
	for i := range 40 {
		s = s*-7919 + int64(i) - int64(int32(i*3))
		t -= int64(int8(i * 37))
		u := s >> 7
		u ^= t << 3
		out = append(out, s, t, u, s>>63, -s, ^t)
	}
	return out
}

func SplitShifts() []uint64 {
	var out []uint64
	for _, x := range []uint64{1, 0xDEADBEEFCAFEBABE, 1 << 63, math.MaxUint64} {
		a, b := x, x
		var acc uint64
		for range 2 {
			acc ^= a<<0 ^ a<<1 ^ a<<15 ^ a<<31 ^ a<<32 ^ a<<33 ^ a<<47 ^ a<<63 ^ a<<64
			acc += a>>1 + a>>16 + a>>31 + a>>32 + a>>33 + a>>48 + a>>63 + a>>64
			b <<= 1
			b >>= 3
			a = a*3 + 1
		}
		out = append(out, acc, a, b)
	}
	return out
}

func SplitSignedShifts() []int64 {
	var out []int64
	for _, x := range []int64{-1, -0x123456789, math.MinInt64, math.MaxInt64, 7} {
		a := x
		var acc int64
		for range 2 {
			acc ^= a>>1 ^ a>>16 ^ a>>31 ^ a>>32 ^ a>>33 ^ a>>48 ^ a>>63 ^ a>>64 ^ a>>70
			acc += a<<5 - a<<40
			a = a*-3 - 1
		}
		out = append(out, acc, a)
	}
	return out
}

func SplitMul() []uint64 {
	var out []uint64
	vals := []uint64{0, 1, 0xffff, 0x10000, 0xffffffff, 0x100000000, 0xdeadbeefcafebabe, math.MaxUint64}
	for _, x := range vals {
		for _, y := range vals {
			a, b := x, y
			p := a * b
			q := a*0x1b3 + a*0x10001 + a*0xffffffff + a*0x100000001b3 + 0x1234*a + a*0
			p += q
			out = append(out, p)
		}
	}
	return out
}

func SplitCarry() []uint64 {
	var out []uint64
	x := uint64(0xfffffffe)
	y := uint64(0x100000001)
	for range 4 {
		x++
		y--
		x += 0xffffffff
		y -= 0xffffffff
		out = append(out, x, y, x&^y, x|y, x&y)
	}
	return out
}

func SplitConversions() []any {
	var out []any
	vals := []uint64{0, 1, 0x7fffffff, 0x80000000, 0xffffffff, 1 << 52, 1<<53 + 1, 1 << 63, math.MaxUint64}
	for i := range len(vals) {
		v := vals[i]
		for range 1 {
			v ^= 0
		}
		s := int64(v)
		for range 1 {
			s += 0
		}
		out = append(out, int32(v), uint32(v), int16(v), uint16(v), int8(v), uint8(v), float64(v), int32(s), float64(s))
		if v < 1<<53 {
			// int and uint are exact below 2^53 (a documented gap).
			out = append(out, int(v), uint(v), int(s), uint(s), int(-s))
		}
	}
	for _, n := range []int{-1, -5, 1 << 40, -(1 << 40)} {
		var a uint64
		var b int64
		for range 1 {
			a = uint64(n) + uint64(int8(n)) + uint64(uint32(n))
			b = int64(n) - int64(int16(n)) + int64(uint8(n))
		}
		out = append(out, a, b)
	}
	return out
}

func SplitClosure() []uint64 {
	h := uint64(5381)
	add := func(c byte) { h = h*33 + uint64(c) }
	for _, c := range []byte("closure") {
		add(c)
		h ^= h >> 7
	}
	get := func() uint64 { return h }
	return []uint64{h, get()}
}

func splitPass(x uint64) uint64 { return x ^ 0xff }

func SplitMaterialize() []any {
	h := uint64(1)
	var hs []uint64
	for i := range 20 {
		h = h*0x5851f42d4c957f2d + 1442695040888963407
		if i%5 == 0 {
			hs = append(hs, splitPass(h))
		}
	}
	k := uint(3)
	g := h
	for range 3 {
		g <<= k
		g = g + splitPass(g)
	}
	n := int64(10)
	for i := 0; i < 5; i, n = i+1, n*3 {
	}
	m := int64(1)
	for j := 0; j < 5; j++ {
		m = m*7 - 3
	}
	return []any{h, hs, h == hs[3], h > 1<<62, strconv.FormatUint(h, 16), g, n, m}
}

func SplitPost() int64 {
	var x int64 = 3
	for i := 0; i < 10; i, x = i+1, x*x-1 {
	}
	y := int64(2)
	for i := 0; i < 10; y = y*y + int64(i) {
		i++
	}
	return x ^ y
}
