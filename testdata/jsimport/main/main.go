// Command main calls TypeScript and JavaScript functions imported with
// //goesm:import. TestJSImport compares its output with want.txt.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"sort"
	"sync"
	"syscall/js"
)

type Item struct {
	Name  string   `json:"name"`
	Price int      `json:"price"`
	Tags  []string `json:"tags"`
}

type Inner struct{ Name string }

type Outer struct {
	*Inner
	Extra int
}

type hidden struct{ Secret string }

type Outer2 struct {
	hidden
	N int
}

//goesm:import "./lib.ts" stringify
func stringify(v any) string

//goesm:import "./lib.ts" keys
func ownKeys(m map[string]int) string

//goesm:import "node:path" basename
func basename(p string) string

//goesm:import "./lib.ts" later
func laterNotAwaited(ms int, v string) string

//goesm:import "./lib.ts" rejectLater
func rejectNotAwaited() error

//goesm:import "./lib.ts" throwValue
func throwValue(v any) error

//goesm:import "./lib.ts" greet
func greet(name string) string

//goesm:import "./lib.ts" sum
func sum(xs []float64) float64

//goesm:import "./lib.ts" describe
func describe(item *Item) string

//goesm:import "./lib.ts" makeItems
func makeItems(n int) []Item

//goesm:import "./lib.ts" parsePrice
func parsePrice(s string) (float64, error)

//goesm:import "./lib.ts" mustBePositive
func mustBePositive(n int) error

//goesm:import "./lib.ts" later await
func later(ms int, v string) string

//goesm:import "./lib.ts" rejectLater await
func rejectLater() (string, error)

//goesm:import "./lib.ts" mapStrings
func mapStrings(xs []string, f func(s string, i int) string) []string

//goesm:import "./lib.ts" join
func join(sep string, parts ...any) string

//goesm:import "./lib.ts" bytes
func bytes(n int) []byte

//goesm:import "./lib.ts" byteSum
func byteSum(b []byte) int

//goesm:import "./lib.ts" big
func big(x int64) int64

//goesm:import "./lib.ts" divmod
func divmod(a, b int) (int, int)

//goesm:import "./lib.ts" counts
func counts(words []string) map[string]int

//goesm:import "./lib.ts" decode
func decode(s string) any

//goesm:import "./lib.ts"
func shout(s string) string

//goesm:import "./plain.mjs" twice
func twice(f func() int) int

//goesm:import "./lib.ts" Counter
var Counter js.Value

//goesm:import "./lib.ts" VERSION
var version string

//goesm:import "./lib.ts" *
var lib js.Value

func main() {
	fmt.Println(greet("世界"))
	fmt.Println(sum([]float64{1, 2, 3.5}))
	fmt.Println(describe(&Item{Name: "ペン", Price: 120, Tags: []string{"文具", "青"}}))
	for _, it := range makeItems(3) {
		fmt.Printf("%+v\n", it)
	}

	if p, err := parsePrice("42.5"); err == nil {
		fmt.Println("price", p)
	}
	_, err := parsePrice("abc")
	fmt.Println(err)
	var jerr js.Error
	if errors.As(err, &jerr) {
		fmt.Println("js.Error name:", jerr.Get("name").String())
	}
	fmt.Println(mustBePositive(1), mustBePositive(-1))

	fmt.Println(later(5, "done"))
	_, err = rejectLater()
	fmt.Println(err)

	fmt.Println(mapStrings([]string{"a", "b"}, func(s string, i int) string {
		return fmt.Sprint(s, i)
	}))
	fmt.Println(join("-", "x", 1, true, 2.5))
	b := bytes(4)
	fmt.Println(b, byteSum(b))
	fmt.Println(big(1<<40), big(1<<62))
	q, r := divmod(17, 5)
	fmt.Println(q, r)
	m := counts([]string{"go", "js", "go"})
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Println(k, m[k])
	}
	fmt.Printf("%v\n", decode(`{"a":[1,"x",null],"b":true}`))
	fmt.Println(shout("hello"))
	n := 0
	fmt.Println(twice(func() int { n++; return n * 10 }))

	c := Counter.New()
	c.Call("add", 2)
	fmt.Println("counter", c.Call("add", 3).Int())
	fmt.Println("version", version)
	fmt.Println("namespace", lib.Get("VERSION").String())

	// Values are shaped as encoding/json shapes them.
	for _, v := range []any{Outer{&Inner{"x"}, 1}, Outer{nil, 2}, Outer2{hidden{"s"}, 3}, map[string]any{"__proto__": map[string]int{"a": 1}}} {
		j, _ := json.Marshal(v)
		s := stringify(v)
		fmt.Println(s, s == string(j))
	}
	fmt.Println(ownKeys(map[string]int{"__proto__": 1, "a": 2}))

	fmt.Println(basename("/a/b/c.txt"))

	syscallJS()

	// A Promise returned to a function declared without await panics.
	for _, f := range []func(){
		func() { laterNotAwaited(1, "x") },
		func() { rejectNotAwaited() },
	} {
		func() {
			defer func() { fmt.Println("recovered:", recover()) }()
			f()
		}()
	}

	// A thrown string or null becomes an Error with it as the message.
	for _, v := range []any{"plain string", nil, 42} {
		err := throwValue(v)
		var jerr js.Error
		errors.As(err, &jerr)
		fmt.Println(err, jerr.Get("cause"))
	}

	// runtime.Goexit in a callback ends the goroutine, not as a panic.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer fmt.Println("goroutine ended by Goexit")
		twice(func() int { runtime.Goexit(); return 0 })
		fmt.Println("not reached")
	}()
	wg.Wait()

	defer func() {
		fmt.Println("recovered:", recover())
	}()
	parsePriceOrPanic("x")
}

//goesm:import "./lib.ts" parsePrice
func parsePriceOrPanic(s string) float64
