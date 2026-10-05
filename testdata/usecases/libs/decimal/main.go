// Command decimal exercises a popular pure-Go library; TestUseCaseLibraries
// compares its output under goesm with native Go.
package main

import (
	"encoding/json"
	"fmt"

	"github.com/shopspring/decimal"
)

func main() {
	a := decimal.RequireFromString("0.1")
	b := decimal.NewFromFloat(0.2)
	fmt.Println(a.Add(b), a.Add(b).Equal(decimal.RequireFromString("0.3")))
	price := decimal.RequireFromString("19.99")
	qty := decimal.NewFromInt(3)
	tax := decimal.RequireFromString("0.0825")
	sub := price.Mul(qty)
	fmt.Println(sub, sub.Mul(tax).Round(2), sub.Add(sub.Mul(tax)).StringFixed(2))
	fmt.Println(decimal.NewFromInt(1).Div(decimal.NewFromInt(3)), decimal.NewFromInt(2).Pow(decimal.NewFromInt(100)))
	fmt.Println(decimal.RequireFromString("123456789012345678901234567890.123").Mul(decimal.RequireFromString("1000")))
	x, _ := decimal.NewFromString("-1.2345")
	fmt.Println(x.Truncate(2), x.RoundBank(3), x.Floor(), x.Ceil(), x.Abs(), x.Neg(), x.Sign(), x.Exponent(), x.Coefficient())
	f, exact := x.Float64()
	fmt.Println(f, exact, x.IntPart())
	bs, _ := json.Marshal(struct{ P decimal.Decimal }{price})
	fmt.Println(string(bs))
	var y struct{ P decimal.Decimal }
	fmt.Println(json.Unmarshal([]byte(`{"P":"3.14159"}`), &y), y.P)
	_, err := decimal.NewFromString("1.2.3")
	fmt.Println(err)
	fmt.Println(decimal.Avg(a, b, price), decimal.Max(a, b), price.Cmp(a))
}
