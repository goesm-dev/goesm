// Package jsexport is called from JavaScript through the JS calling ABI
// (test/js/poc.test.mjs): its exported functions take and return plain
// JavaScript values.
package jsexport

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
)

// Weekday is the Japanese name of the day of the week of a date.
func Weekday(year, month, day int) string {
	names := [...]string{"日曜日", "月曜日", "火曜日", "水曜日", "木曜日", "金曜日", "土曜日"}
	return names[time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC).Weekday()]
}

// Count is the number of runes and of bytes of s.
func Count(s string) (runes, bytes int) {
	return len([]rune(s)), len(s)
}

var ErrEmpty = errors.New("empty input")

// Sum adds comma-separated integers.
func Sum(s string) (int, error) {
	if strings.TrimSpace(s) == "" {
		return 0, ErrEmpty
	}
	total := 0
	for _, f := range strings.Split(s, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(f))
		if err != nil {
			return 0, err
		}
		total += n
	}
	return total, nil
}

// IsEmpty reports whether err is ErrEmpty.
func IsEmpty(err error) bool { return errors.Is(err, ErrEmpty) }

// Check fails for a negative n.
func Check(n int) error {
	if n < 0 {
		return errors.New("negative: " + strconv.Itoa(n))
	}
	return nil
}

type Item struct {
	Name     string `json:"name"`
	Price    int    `json:"price"`
	Quantity int    `json:"quantity,omitempty"`
	Tags     []string
	secret   int
}

type Order struct {
	ID    int64
	Items []Item
	Note  *string
}

// Total is the price of an order.
func Total(o Order) int {
	t := 0
	for _, it := range o.Items {
		q := it.Quantity
		if q == 0 {
			q = 1
		}
		t += it.Price * q
	}
	return t
}

// Cheapest returns the cheapest item, with its name in upper case.
func Cheapest(items []Item) Item {
	best := items[0]
	for _, it := range items[1:] {
		if it.Price < best.Price {
			best = it
		}
	}
	best.Name = strings.ToUpper(best.Name)
	return best
}

// Cart is used from JavaScript through a pointer: its type has methods.
type Cart struct {
	owner string
	items []Item
}

func NewCart(owner string) *Cart { return &Cart{owner: owner} }

func (c *Cart) Add(items ...Item) { c.items = append(c.items, items...) }

func (c *Cart) Len() int { return len(c.items) }

func (c Cart) Owner() string { return c.owner }

func (c *Cart) Items() []Item { return c.items }

// Join joins its arguments.
func Join(sep string, parts ...string) string { return strings.Join(parts, sep) }

// MapStrings applies f to each string.
func MapStrings(xs []string, f func(string) string) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = f(x)
	}
	return out
}

// Exclaim is passed to JavaScript as a function.
func Exclaim() func(string) string {
	return func(s string) string { return s + "!" }
}

// Bytes returns b reversed.
func Bytes(b []byte) []byte {
	out := make([]byte, len(b))
	for i, c := range b {
		out[len(b)-1-i] = c
	}
	return out
}

func Square64(x int64) int64 { return x * x }

func Counts(words []string) map[string]int {
	m := map[string]int{}
	for _, w := range words {
		m[w]++
	}
	return m
}

func Lengths(words []string) map[int][]string {
	m := map[int][]string{}
	for _, w := range words {
		m[len(w)] = append(m[len(w)], w)
	}
	return m
}

// Decode decodes JSON into an any.
func Decode(s string) (any, error) {
	var v any
	err := json.Unmarshal([]byte(s), &v)
	return v, err
}

// Describe reports the dynamic type of v.
func Describe(v any) string {
	switch v := v.(type) {
	case nil:
		return "nil"
	case string:
		return "string " + v
	case float64:
		return "float64 " + strconv.FormatFloat(v, 'g', -1, 64)
	case bool:
		return "bool"
	case []any:
		return "array of " + strconv.Itoa(len(v))
	case map[string]any:
		return "object of " + strconv.Itoa(len(v))
	case *Cart:
		return "cart of " + v.owner
	}
	return "other"
}

// Later waits, so it is a Promise in JavaScript.
func Later(s string) string {
	time.Sleep(time.Millisecond)
	return "later " + s
}

// NewBuilder returns a handle of a type of another package: JavaScript
// calls its methods as on the entry package's types.
func NewBuilder(s string) *strings.Builder {
	b := &strings.Builder{}
	b.WriteString(s)
	return b
}

// Node embeds itself; JavaScript sees the fields of the outer Node only.
type Node struct {
	*Node
	Value int
}

func MakeNode(v int) Node { return Node{&Node{nil, v + 1}, v} }

// Depth is the length of the chain of embedded Nodes.
func Depth(n Node) int {
	d := 0
	for p := n.Node; p != nil; p = p.Node {
		d++
	}
	return d
}

// Inc increments the int p points to: JavaScript passes the int.
func Inc(p *int) int {
	*p++
	return *p
}

// Parser returns a function that JavaScript calls like an exported one.
func Parser() func(string) (int, error) {
	return func(s string) (int, error) { return strconv.Atoi(s) }
}

func Grouper() func([]string) map[int][]string { return Lengths }
