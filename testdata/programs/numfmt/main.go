// Command numfmt formats numbers through fmt and strconv. Integers around
// 2^53, where goesm's fmt switches from uint64 (BigInt) to uint (number)
// digits, and strconv's base 10, which goesm takes from the engine. float64 values in the 'e', 'f' and 'g' formats at the shortest
// and at fixed precisions: goesm takes their digits from the engine's
// Number formatting, which rounds exact ties up where Go rounds them to
// even, so the values include ties at many scales, as well as random bit
// patterns and decimals.
package main

import (
	"fmt"
	"hash/fnv"
	"math"
	"strconv"
)

var precs = []int{-1, 0, 1, 2, 3, 5, 6, 10, 15, 16, 17, 18, 19, 25, 40, 100, 101, 400}

func main() {
	ints := []int64{0, 1, -1, 9, 10, 255, -256, 1<<53 - 1, 1 << 53, 1<<53 + 1, -1 << 53, -1<<53 - 1,
		1<<62 + 12345, math.MaxInt64, math.MinInt64}
	for _, v := range ints {
		fmt.Printf("%d %x %X %o %O %b %#x %#o %08d %+d %.5d %-6d| % d\n", v, v, v, v, v, v, v, v, v, v, v, v, v)
		u := uint64(v)
		fmt.Printf("%d %x %#b %v\n", u, u, u, u)
		fmt.Println(strconv.FormatInt(v, 10), strconv.FormatUint(u, 10), strconv.FormatInt(v, 36),
			string(strconv.AppendInt([]byte("i="), v, 10)), string(strconv.AppendUint(nil, u, 10)),
			strconv.Itoa(int(v>>11)), strconv.Itoa(int(v%1000)))
	}
	for _, i := range []int{0, 99, 100, -99, -100, 1<<53 - 1, -1 << 53, 1 << 60} {
		fmt.Print(strconv.Itoa(i), " ")
	}
	fmt.Println()
	fmt.Println(int32(math.MinInt32), uint32(math.MaxUint32), int8(-128), uint(1<<52+7), uintptr(4096))

	values := []float64{
		0.125, 0.375, 2.5, 0.5, 1.5, 3.5, 9.5, 99.5, 999.5, 0.05, 0.25, 0.75,
		0.005, 1.005, 1.115, 2.675, 1e21, 1e22, 1e23, 9.999999999999999e22,
		123456789.125, 4503599627370495.5, 9007199254740993, 1 << 62,
		5e-324, 1e-320, math.SmallestNonzeroFloat64 * 3, 2.2250738585072014e-308,
		math.MaxFloat64, 1.0 / 3, 2.0 / 3, 0.1, 0.2, 0.3, 100, 1e6, 1e-5, 1e-4,
		123456, 1234567, 12345678.5, 0.000123456, 1e15, 1e16, 1e17, 1.5e300,
		-2.5, -0.125, -1e-7, 7.0e-10, 1.25e-5, 6.103515625e-05,
		math.NaN(), math.Inf(1), math.Inf(-1), 0, math.Copysign(0, -1),
	}
	for _, v := range values {
		line := fmt.Sprintf("%v %g %e %f %.2f %.3e %.4g %8.3f|%-10.1e|%+.0f",
			v, v, v, v, v, v, v, v, v, v)
		for _, f := range []byte{'e', 'f', 'g', 'E', 'G'} {
			for _, p := range precs {
				line += " " + strconv.FormatFloat(v, f, p, 64)
			}
		}
		fmt.Println(line)
	}

	// Ties: k/2^j for small k and j, scaled by powers of ten.
	h := fnv.New64a()
	n := 0
	for j := 1; j <= 12; j++ {
		for k := 1; k < 200; k += 2 {
			for _, s := range []float64{1, 10, 100, 1e-3, 1e5} {
				v := float64(k) / float64(uint(1)<<j) * s
				for _, f := range []byte{'e', 'f', 'g'} {
					for p := 0; p <= 14; p++ {
						h.Write(strconv.AppendFloat(nil, v, f, p, 64))
						n++
					}
				}
			}
		}
	}
	fmt.Println("ties", n, h.Sum64())

	// Random bit patterns and random decimals.
	x := uint64(0x9e3779b97f4a7c15)
	next := func() uint64 {
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
		return x
	}
	h.Reset()
	n = 0
	for range 3000 {
		bits := math.Float64frombits(next())
		dec := float64(next()%1000000) / math.Pow(10, float64(next()%12))
		for _, v := range []float64{bits, dec, -dec} {
			for _, f := range []byte{'e', 'f', 'g'} {
				for _, p := range precs {
					h.Write(strconv.AppendFloat(nil, v, f, p, 64))
					n++
				}
			}
			h.Write(fmt.Appendf(nil, "%v %.2f %.3g %e", v, v, v, v))
		}
	}
	fmt.Println("random", n, h.Sum64())

	// float32 stays on the Go code.
	for _, v := range []float32{0.1, 1.0 / 3, 2.5, 16777217, math.MaxFloat32} {
		fmt.Println(v, strconv.FormatFloat(float64(v), 'g', -1, 32), strconv.FormatFloat(float64(v), 'e', 3, 32))
	}
}
