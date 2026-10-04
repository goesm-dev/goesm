package main

import (
	"fmt"
	"math/big"
)

func main() {
	f := big.NewInt(1)
	for i := int64(1); i <= 50; i++ {
		f.Mul(f, big.NewInt(i))
	}
	fmt.Println(f)
	q, r := new(big.Int).QuoRem(f, big.NewInt(1234567891011), new(big.Int))
	fmt.Println(q, r)
	x, _ := new(big.Int).SetString("123456789012345678901234567890123456789", 10)
	y, _ := new(big.Int).SetString("987654321098765432109876543210", 10)
	fmt.Println(new(big.Int).Mul(x, y), new(big.Int).Div(x, y), new(big.Int).Mod(x, y))
	fmt.Println(new(big.Int).Exp(big.NewInt(3), big.NewInt(200), nil))
	fmt.Println(new(big.Int).Exp(big.NewInt(3), big.NewInt(200), big.NewInt(1000000007)))
	fmt.Println(new(big.Int).GCD(nil, nil, x, y), x.ProbablyPrime(10), big.NewInt(1000000007).ProbablyPrime(10))
	fmt.Println(new(big.Int).Sqrt(x), x.Text(16), x.BitLen(), new(big.Int).Lsh(x, 100), new(big.Int).Rsh(x, 37))
	fmt.Println(new(big.Int).Neg(x).Int64(), y.Uint64(), x.Cmp(y))
	r1 := big.NewRat(1, 3)
	r1.Add(r1, big.NewRat(2, 7))
	fmt.Println(r1, r1.FloatString(20))
	fl := new(big.Float).SetPrec(200).SetInt64(2)
	fl.Sqrt(fl)
	fmt.Println(fl.Text('g', 50))
	g, _ := new(big.Float).SetString("3.14159265358979323846264338327950288")
	fmt.Println(g.Text('e', 30), g.String())
	fmt.Printf("%x %d %s\n", x, y, new(big.Int).ModInverse(big.NewInt(3), big.NewInt(1000000007)))
	fmt.Println(x.Bytes()[:5], new(big.Int).SetBytes([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9}))
}
