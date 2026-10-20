// Print, Println and Printf, and Fprint* to os.Stdout and os.Stderr, which
// goesm writes without os.File when the program never changes those.
package main

import (
	"errors"
	"fmt"
	"os"
)

type name string

type point struct{ x, y int }

func (p point) String() string { return fmt.Sprintf("(%d,%d)", p.x, p.y) }

func pair() (int, string) { return 7, "seven" }

func main() {
	var e error = errors.New("boom")
	var nilErr error
	var a any = 3.5
	fmt.Println()
	fmt.Println("n =", 42, -1.5, true, 1e21, uint8(200), int64(1)<<60, name("named"))
	fmt.Println(e, nilErr, a, nil, point{1, 2}, []int{1, 2}, map[string]int{"a": 1})
	fmt.Println(pair())
	xs := []any{"x", 1, 2.5}
	fmt.Println(xs...)
	fmt.Print("a", "b", 1, 2, "c", 3, name("d"), 4, 5.5, true, "\n")
	fmt.Print(a, 1, e, "\n")
	fmt.Print()
	fmt.Printf("%d|%5s|%-4d|%.2f|%x|%v|%q\n", 12, "ab", 3, 3.14159, 255, e, "q")
	fmt.Printf("%v %d\n", point{3, 4}, int64(9007199254740993))
	fmt.Printf("no args\n")
	f := "%s=%d\n"
	fmt.Printf(f, "dyn", 5)
	n, err := fmt.Println("count me")
	fmt.Println(n, err)
	n, err = fmt.Printf("%s\n", "printf count")
	fmt.Println(n, err)
	fmt.Fprintln(os.Stdout, "to stdout", 1)
	fmt.Fprint(os.Stdout, "x", 1, 2, "\n")
	fmt.Fprintf(os.Stdout, "%d-%d\n", 1, 2)
	fmt.Fprintln(os.Stderr, "to stderr", 2)
	fmt.Fprintf(os.Stderr, "%s\n", "err printf")
	n, err = fmt.Fprint(os.Stderr, "err print\n")
	fmt.Println(n, err)
	for i := range 3 {
		defer fmt.Println("deferred", i)
	}
	i := 0
	defer fmt.Printf("i at defer: %d\n", i)
	i = 10
	fmt.Println("日本語", "テキスト")
	os.Stdout.WriteString("direct write\n")
	fmt.Println("after direct")
}
