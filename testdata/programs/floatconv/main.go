// FormatFloat, AppendFloat and ParseFloat with bitSize 64 and a constant
// format, which goesm lowers to the engine's number conversions.
package main

import (
	"fmt"
	"math"
	"math/rand/v2"
	"strconv"
)

func main() {
	r := rand.New(rand.NewPCG(1, 2))
	vals := []float64{0, math.Copysign(0, -1), 1, -1, 0.1, 0.5, 1.5, 2.5, 1e21, 1e-7, 123456789, 5e-324, math.MaxFloat64,
		math.SmallestNonzeroFloat64 * 3, 2.2250738585072014e-308, 0.000001, 1e20, 9.999999999999999e22, math.Inf(1), math.Inf(-1), math.NaN()}
	for range 300 {
		vals = append(vals, math.Float64frombits(r.Uint64()), r.Float64()*math.Pow(10, float64(r.IntN(40)-20)), float64(r.IntN(100000))/8)
	}
	precs := []int{-1, 0, 1, 2, 3, 5, 10, 17, 20}
	var buf []byte
	for _, v := range vals {
		for _, p := range precs {
			fmt.Println(strconv.FormatFloat(v, 'e', p, 64), strconv.FormatFloat(v, 'E', p, 64), strconv.FormatFloat(v, 'f', p, 64),
				strconv.FormatFloat(v, 'g', p, 64), strconv.FormatFloat(v, 'G', p, 64))
		}
		buf = strconv.AppendFloat(buf[:0], v, 'g', -1, 64)
		f, err := strconv.ParseFloat(string(buf), 64)
		fmt.Println(string(buf), f == v || f != f, err)
	}
	for _, s := range []string{"", "1", "-0", "+1.5", ".5", "5.", "1e", "1e+", "1_000.5", "1__0", "_1", "0x1p-2", "0x1.8p1", "0X_1p0", "0x1p", "0x.8p1",
		"inf", "+Inf", "-infinity", "nan", "NaN", "infx", "1e309", "-1e309", "1e-400", "4.9e-324", "2.4703282292062328e-324", "2.4703282292062327e-324",
		"0x1p-1074", "0x1p-1075", "0x1.0000000000001p-1075", "0x1fffffffffffffp971", "0x1fffffffffffffp972", "179769313486231580793728971405303415079934132710037826936173778980444968292764750946649017977587207096330286416692887910946555547851940402630657488671505820681908902000708383676273854845817711531764475730270069855571366959622842914819860834936475292719074168444365510704342711559699508093042880177904174497791",
		"1.7976931348623158e308", "1.7976931348623159e308", "12345678901234567890", "0.1e1_0", "1e1_0", "0b101", "0o17", "1.5e-2", " 1", "1 ", "--1", "١"} {
		f, err := strconv.ParseFloat(s, 64)
		fmt.Println(strconv.Quote(s), f, err)
	}
}
