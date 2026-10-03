// Package complexnum holds complex64/complex128 fixtures.
package complexnum

import (
	"math"
	"math/cmplx"
	"strconv"
)

func parts(cs ...complex128) []float64 {
	var out []float64
	for _, c := range cs {
		out = append(out, real(c), imag(c))
	}
	return out
}

func Arith() []float64 {
	a := complex(1.5, -2)
	b := 3 + 4i
	c := a * b
	c += 1
	c++
	d := a / b
	return parts(a+b, a-b, c, d, -a, complex(real(b), imag(a)))
}

func DivSpecial() []string {
	var zero complex128
	one := complex(1, 0)
	inf := complex(math.Inf(1), 0)
	f := func(c complex128) string { return strconv.FormatComplex(c, 'g', -1, 128) }
	return []string{f(one / zero), f(inf / one), f(one / inf), f(zero / zero)}
}

func Complex64() []float64 {
	var x complex64 = complex(0.1, 0.2)
	y := x * x
	z := complex128(y)
	w := complex64(complex(1.0/3, 2.0/3))
	return []float64{float64(real(y)), float64(imag(y)), real(z), float64(real(w)), float64(imag(w))}
}

func Compare() []bool {
	a, b := 1+2i, complex(1, 2)
	m := map[complex128]int{a: 1}
	m[b]++
	var i any = a
	var j any = b
	nan := complex(math.NaN(), 0)
	return []bool{a == b, a != b, m[1+2i] == 2, len(m) == 1, i == j, nan == nan}
}

func Format() []string {
	c := complex(1.5, -0.25)
	var c64 complex64 = 2 + 3i
	return []string{
		strconv.FormatComplex(c, 'g', -1, 128),
		strconv.FormatComplex(complex128(c64), 'f', 2, 64),
		strconv.FormatComplex(c, 'e', 3, 128),
		strconv.FormatFloat(cmplx.Abs(3+4i), 'g', -1, 64),
		strconv.FormatComplex(cmplx.Sqrt(-4), 'f', 4, 128),
		strconv.FormatComplex(cmplx.Exp(1i*math.Pi), 'g', 6, 128),
	}
}

func generic[T complex64 | complex128](a, b T) T { return a*b - a/b }

func Generic() []float64 {
	r := generic(1+1i, 2-1i)
	r64 := generic[complex64](1+1i, 2-1i)
	return parts(r, complex128(r64))
}

func ZeroValues() []float64 {
	var c complex128
	var arr [2]complex64
	s := struct{ C complex128 }{}
	return parts(c, complex128(arr[1]), s.C)
}
