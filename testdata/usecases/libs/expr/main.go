// Command expr exercises a popular pure-Go library; TestUseCaseLibraries
// compares its output under goesm with native Go.
package main

import (
	"fmt"
	"strings"

	"github.com/expr-lang/expr"
)

type Env struct {
	User  User
	Items []Item
	Rate  float64
}
type User struct {
	Name string
	Age  int
}
type Item struct {
	Name  string
	Price float64
	Qty   int
}

func (e Env) Greet(s string) string { return "hi " + strings.ToUpper(s) }

func main() {
	env := Env{User: User{"ann", 31}, Items: []Item{{"a", 1.5, 2}, {"b", 10, 1}}, Rate: 0.1}
	for _, code := range []string{
		`User.Age >= 18 && User.Name startsWith "a"`,
		`sum(map(Items, .Price * .Qty)) * (1 + Rate)`,
		`filter(Items, .Price > 2)[0].Name`,
		`Greet(User.Name) + "!"`,
		`len(Items) == 2 ? "two" : "other"`,
		`{"a": 1, "b": [1,2,3]}.b[1:]`,
		`now().Year() > 2000`,
		`"x" matches "^[a-z]$"`,
		`User.Nope`,
		`1 +`,
	} {
		prog, err := expr.Compile(code, expr.Env(Env{}))
		if err != nil {
			fmt.Println("compile error:", strings.Split(err.Error(), "\n")[0])
			continue
		}
		out, err := expr.Run(prog, env)
		fmt.Printf("%v (%T) %v\n", out, out, err)
	}
	out, err := expr.Eval(`a * b + 1`, map[string]any{"a": 6, "b": 7})
	fmt.Println(out, err)
}
