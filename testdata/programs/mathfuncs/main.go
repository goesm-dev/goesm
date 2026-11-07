// Command mathfuncs checks the math functions goesm implements with JS
// builtins on special values: signed zeros, infinities and NaN. It also
// checks the functions whose bodies goesm patches or replaces (Mod, Max,
// Min, Frexp, Ldexp, Modf, the trigonometric functions) bit for bit on
// values across the whole exponent range, denormals included.
package main

import (
	"fmt"
	"math"
)

func main() {
	vals := []float64{0, math.Copysign(0, -1), 1.5, -1.5, 2.5, -2.5, 1e300, -7.0000001, math.Inf(1), math.Inf(-1), math.NaN(), 0.49999999999999994}
	for _, v := range vals {
		bits := math.Float64bits(math.Abs(v))
		if math.IsNaN(v) {
			bits = 0 // JavaScriptCore (Bun) does not keep NaN payloads
		}
		fmt.Println(v, math.Floor(v), math.Ceil(v), math.Trunc(v), math.Round(v), math.RoundToEven(v), math.Sqrt(v), math.Abs(v), math.Signbit(v),
			math.Copysign(3, v), math.Signbit(math.Copysign(0, v)), math.Max(v, 0), math.Min(v, 0), bits)
	}
	sweep(vals)
	fmt.Println(math.Inf(0), math.Inf(-3), math.IsNaN(math.NaN()), math.Sqrt(2), math.Sqrt(1e-310), math.Hypot(3, 4), math.Mod(7, -3), math.Pow(2, 0.5))
}

// sweep prints, for each function, a digest of the bits of its results on
// the special values and on values of every magnitude.
func sweep(special []float64) {
	xs := append([]float64{math.SmallestNonzeroFloat64, -math.SmallestNonzeroFloat64, 2.2250738585072014e-308, 1 << 29, 1<<29 - 0.5, math.Pi, -math.Pi / 2, 1e22, math.MaxFloat64}, special...)
	seed := uint32(12345)
	next := func() uint32 {
		seed ^= seed << 13
		seed ^= seed >> 17
		seed ^= seed << 5
		return seed
	}
	for range 3000 {
		m := float64(next()) / (1 << 32)
		e := int(next()%2200) - 1100
		x := math.Ldexp(0.5+m/2, e)
		if next()%2 == 0 {
			x = -x
		}
		xs = append(xs, x, float64(int32(next()))/64)
	}
	digests := map[string]uint64{}
	names := []string{}
	put := func(name string, vs ...float64) {
		h, ok := digests[name]
		if !ok {
			names = append(names, name)
			h = 14695981039346656037
		}
		for _, v := range vs {
			b := math.Float64bits(v)
			if math.IsNaN(v) {
				b = 1 // JavaScriptCore does not keep NaN payloads
			}
			h = (h ^ b) * 1099511628211
		}
		digests[name] = h
	}
	for i, x := range xs {
		y := xs[(i*7+3)%len(xs)]
		put("Mod", math.Mod(x, y), math.Mod(y, x), math.Mod(x, 3), math.Mod(x, math.Inf(-1)))
		put("Max", math.Max(x, y), math.Min(x, y), math.Max(x, math.NaN()), math.Min(math.Inf(-1), math.NaN()))
		f, e := math.Frexp(x)
		put("Frexp", f, float64(e))
		put("Ldexp", math.Ldexp(x, (i*37)%2400-1200), math.Ldexp(f, e), math.Ldexp(x, -1074), math.Ldexp(x, 1023), math.Ldexp(x, -1100))
		ip, fp := math.Modf(x)
		put("Modf", ip, fp)
		s, c := math.Sincos(x)
		put("Trig", math.Sin(x), math.Cos(x), math.Tan(x), s, c)
		// Exp and Log are left out: gc uses assembly for them on amd64,
		// whose results differ from the Go bodies' in the last bit.
	}
	for _, n := range names {
		fmt.Printf("%s %016x\n", n, digests[n])
	}
}
