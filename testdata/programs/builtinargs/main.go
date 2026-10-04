// Command builtinargs passes the results of a call to complex, append and
// copy, and converts constants to type parameters.
package main

func f[T ~float64]() T        { return T(0 + 0i) }
func g[T ~int | ~float32]() T { return T(3 + 0i) }

func complexArgs() (float64, float64)       { return 5, 7 }
func appendArgs() ([]string, string)        { return []string{"foo"}, "bar" }
func appendMultiArgs() ([]byte, byte, byte) { return []byte{'a', 'b'}, '1', '2' }
func copyArgs() ([]int, []int)              { return make([]int, 2), []int{1, 2, 3} }

func main() {
	x := f[float64]()
	println(x + 1)
	println(g[int]()+1, g[float32]()/2)
	c := complex(complexArgs())
	println(c)
	s := append(appendArgs())
	println(len(s), s[0], s[1])
	b := append(appendMultiArgs())
	println(string(b))
	println(copy(copyArgs()))
}

func h[T ~int | ~string]() T { return T(65) }

func init() { println(h[string](), h[int]()) }
