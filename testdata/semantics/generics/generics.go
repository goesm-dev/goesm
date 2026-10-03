// Package generics holds generic fixtures. Go 1.27 generic methods are
// included: goesm accepts them because the Go frontend does.
package generics

// itoa avoids importing strconv: the PoC cannot compile the standard library
// packages that depend on internal/abi and unsafe yet (see ARCHITECTURE.md).
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	if neg {
		return "-" + string(b)
	}
	return string(b)
}

func First[T any](values []T) T {
	return values[0]
}

func FirstInt() int       { return First([]int{7, 8}) }
func FirstString() string { return First([]string{"a", "b"}) }

type Number interface {
	~int | ~int32 | ~float64
}

func SumOf[T Number](xs ...T) T {
	var total T // zero value of a type parameter
	for _, x := range xs {
		total += x
	}
	return total
}

type MyInt int

func Sums() []float64 {
	return []float64{float64(SumOf(1, 2, 3)), SumOf(1.5, 2.5), float64(SumOf[MyInt](4, 5))}
}

func Map[T, U any](xs []T, f func(T) U) []U {
	out := make([]U, 0, len(xs))
	for _, x := range xs {
		out = append(out, f(x))
	}
	return out
}

func MapItoa() []string {
	return Map([]int{1, 2, 3}, itoa)
}

type Pair[K comparable, V any] struct {
	Key K
	Val V
}

type Stack[T any] struct {
	items []T
}

func (s *Stack[T]) Push(v T) { s.items = append(s.items, v) }

func (s *Stack[T]) Pop() (T, bool) {
	var zero T
	if len(s.items) == 0 {
		return zero, false
	}
	v := s.items[len(s.items)-1]
	s.items = s.items[:len(s.items)-1]
	return v, true
}

func StackOps() []any {
	var s Stack[Pair[string, int]]
	s.Push(Pair[string, int]{"a", 1})
	s.Push(Pair[string, int]{"b", 2})
	top, ok := s.Pop()
	n := s.Len()
	_, _ = s.Pop()
	_, ok2 := s.Pop()
	return []any{top.Key, top.Val, ok, n, ok2}
}

// Value semantics survive type erasure: T may be a struct.
func CopyThroughGeneric() []int {
	type box struct{ n int }
	b := box{1}
	c := First([]box{b})
	c.n = 2
	return []int{b.n, c.n}
}

func Keys[K comparable, V any](m map[K]V) int { return len(m) }

func Equal[T comparable](a, b T) bool { return a == b }

func Comparisons() []bool {
	type P struct{ X, Y int }
	return []bool{Equal(1, 1), Equal("a", "b"), Equal(P{1, 2}, P{1, 2}), Equal[any](1, int64(1))}
}

// Type identity of instantiated types is kept at run time.
func Identity() []bool {
	var a any = Pair[string, int]{"x", 1}
	_, ok1 := a.(Pair[string, int])
	_, ok2 := a.(Pair[string, string])
	return []bool{ok1, ok2}
}

// ---- Go 1.27 generic methods ----

type List struct{ xs []int }

func (l List) MapTo[U any](f func(int) U) []U {
	out := []U{}
	for _, x := range l.xs {
		out = append(out, f(x))
	}
	return out
}

func GenericMethod() []string {
	l := List{[]int{1, 2, 3}}
	return l.MapTo(func(i int) string { return itoa(i * 10) })
}

func (s *Stack[T]) Fold[A any](init A, f func(A, T) A) A {
	acc := init
	for _, x := range s.items {
		acc = f(acc, x)
	}
	return acc
}

func GenericMethodOnGenericType() string {
	s := &Stack[int]{}
	s.Push(1)
	s.Push(2)
	s.Push(3)
	return s.Fold("", func(acc string, x int) string { return acc + itoa(x) })
}

// ---- methods of generic types reached through interfaces ----

func (s Stack[T]) Len() int { return len(s.items) }

func (s *Stack[T]) Top() T {
	var zero T
	if len(s.items) == 0 {
		return zero
	}
	return s.items[len(s.items)-1]
}

type Lener interface{ Len() int }

type Topper[T any] interface{ Top() T }

func GenericMethodViaInterface() []any {
	s := &Stack[string]{}
	s.Push("a")
	s.Push("b")
	var l Lener = *s
	var t Topper[string] = s
	var e Topper[float64] = &Stack[float64]{}
	return []any{l.Len(), t.Top(), e.Top()}
}

// ---- constraint methods called on type-parameter values ----

type Celsius float64

func (c Celsius) String() string { return "C" + itoa(int(c)) }

type Name struct{ s string }

func (n *Name) String() string { return "N:" + n.s }

type Stringer interface{ String() string }

func Str[T Stringer](xs ...T) string {
	out := ""
	for _, x := range xs {
		out += x.String() + ";"
	}
	return out
}

type Labeled[T Stringer] struct{ v T }

func (l Labeled[T]) Label() string { return "<" + l.v.String() + ">" }

func ConstraintMethodCall() []string {
	return []string{
		Str(Celsius(21), Celsius(-3)),
		Str(&Name{"x"}, &Name{"y"}),
		Labeled[Celsius]{37}.Label(),
		Labeled[*Name]{&Name{"z"}}.Label(),
	}
}

// ---- operators on type-parameter operands follow the type argument ----

type Num interface {
	Integer | ~float32 | ~float64
}

type Integer interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64
}

func Div[T Num](a, b T) T     { return a / b }
func Add[T Num](a, b T) T     { return a + b }
func Mul[T Num](a, b T) T     { return a * b }
func Neg[T Num](a T) T        { return -a }
func Rem[T Integer](a, b T) T { return a % b }
func Shl[T Integer](a T, n int) T {
	a <<= n
	return a
}
func Not[T Integer](a T) T { return ^a }
func Inc[T Num](a T) T {
	a++
	return a
}
func ToInt[T Num](x T) int         { return int(x) }
func FromFloat[T Num](f float64) T { return T(f) }
func Concat[T ~string](a, b T) T   { return a + b }

type Small int8

func TypeParamArith() []any {
	return []any{
		Div(7, 2), Div(7.0, 2), Div[int32](-7, 2),
		Add[int8](100, 100), Add[Small](100, 100), Mul[int8](16, 16), Add[int32](2147483647, 1),
		Add[uint8](200, 100), Neg[uint16](1), Rem[int](-7, 3),
		Shl[uint8](0x81, 1), Shl[int16](1, 15), Not[uint32](0), Not[int8](5),
		Inc[int8](127), Inc[float32](0.5),
		ToInt(2.75), ToInt(-2.75), ToInt[int8](-5),
		FromFloat[int](2.75), FromFloat[int8](-2.75), float64(FromFloat[float32](0.1)),
		Concat("go", "esm"),
	}
}
