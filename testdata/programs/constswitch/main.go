// Command constswitch covers switches whose cases are all constants, which
// goesm lowers to a JS switch on the tag: several values per case, default
// before other cases, fallthrough into and out of default, break, a labeled
// break and continue of an enclosing loop, and tags of named, float, byte
// and boolean types.
package main

import "fmt"

type color string

const (
	red   color = "red"
	green color = "green"
)

func kind(s string) string {
	switch s {
	case "a", "e", "i":
		return "vowel"
	default:
		return "other"
	case "y":
		fmt.Print("(y) ")
		fallthrough
	case "w":
		return "semi"
	}
}

func steps(n int) (out []int) {
	switch n {
	case 1:
		out = append(out, 1)
		fallthrough
	default:
		out = append(out, 0)
		fallthrough
	case 2:
		out = append(out, 2)
		if n > 5 {
			break
		}
		out = append(out, 3)
	case 3:
		out = append(out, 4)
	}
	return out
}

func main() {
	for _, s := range []string{"a", "i", "y", "w", "z", ""} {
		fmt.Println(s, kind(s))
	}
	for _, n := range []int{1, 2, 3, 4, 9} {
		fmt.Println(n, steps(n))
	}
	for _, c := range []color{red, green, "blue"} {
		switch c {
		case red:
			fmt.Println("stop")
		case green:
			fmt.Println("go")
		default:
			fmt.Println("?", c)
		}
	}
	for _, f := range []float64{0, -0.0, 0.5, 1} {
		switch f {
		case 0:
			fmt.Println(f, "zero")
		case 0.5, 1:
			fmt.Println(f, "half or one")
		}
	}
	for _, b := range []byte("a\x00\xff") {
		switch b {
		case 'a':
			fmt.Println("a")
		case 0:
			fmt.Println("nul")
		case 255:
			fmt.Println("ff")
		}
	}
	for _, t := range []bool{true, false} {
		switch t {
		case true:
			fmt.Println("yes")
		case false:
			fmt.Println("no")
		}
	}
loop:
	for i := 0; ; i++ {
		switch i {
		case 0, 2:
			continue loop
		case 5:
			break loop
		}
		fmt.Println("i", i)
	}
}
