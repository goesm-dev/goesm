// Command jsonfast covers the edge cases of encoding/json's one-pass path for
// plain values (escapes, invalid UTF-8, omitempty, nil vs empty, case-insensitive
// keys, duplicates, number ranges, null, unknown fields, trailing data) and the
// values it hands back to the full codec.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
)

type Inner struct {
	A int    `json:"a"`
	B string `json:"b,omitempty"`
}

type Plain struct {
	Name   string            `json:"name"`
	Count  int               `json:"count,omitempty"`
	Ratio  float64           `json:"ratio"`
	Small  int8              `json:"small"`
	U16    uint16            `json:"u16"`
	I64    int64             `json:"i64"`
	U64    uint64            `json:"u64"`
	F32    float32           `json:"f32"`
	OK     bool              `json:"ok"`
	Tags   []string          `json:"tags"`
	Empty  []int             `json:"empty,omitempty"`
	Map    map[string]int    `json:"map"`
	Inner  Inner             `json:"inner"`
	Ptr    *Inner            `json:"ptr"`
	Any    any               `json:"any"`
	Bytes  []byte            `json:"bytes"`
	Arr    [3]uint8          `json:"arr"`
	Nested map[string][]bool `json:"nested,omitempty"`
	Dash   int               `json:"-"`
	NoTag  string
	hidden int
}

type Upper struct {
	Key  string
	Kéy  string
	Kelv string
}

type Dup struct {
	A int `json:"x"`
	B int `json:"x"`
	C int
}

type Named string

type Digits struct {
	A int `json:"1"`
	B int `json:"b"`
}

type WithStr struct {
	N int `json:"n,string"`
}

// Typed values: json.Marshal of a value whose static type goesm knows takes
// an encoder generated for the type.
type Item struct {
	SKU   string   `json:"sku"`
	Price int      `json:"price"`
	Qty   int64    `json:"qty,omitempty"`
	Ratio float64  `json:"ratio"`
	Small uint8    `json:"small"`
	Note  Named    `json:"note,omitempty"`
	Next  *Item    `json:"next,omitempty"`
	Tags  []string `json:"tags"`
	Flags map[Named]bool
	skip  int
}

type Node struct {
	Name string  `json:"name"`
	Kids []*Node `json:"kids"`
}

type Tree struct {
	Name string `json:"name"`
	Kids []Tree `json:"kids"`
}

// direct decodes into pointers of known types, which goesm passes to the
// runtime without boxing them, and arrays the runtime collects in a scratch
// array per slice type: nested arrays of one type, an array after an abort,
// and earlier results after later decodes.
func direct() {
	var t Tree
	err := json.Unmarshal([]byte(`{"name":"r","kids":[{"name":"a","kids":[{"name":"x"},{"name":"y","kids":[]}]},{"name":"b"}]}`), &t)
	fmt.Printf("direct tree: %v %+v\n", err, t)
	var first, second []string
	err = json.Unmarshal([]byte(`["a","b","c"]`), &first)
	err2 := json.Unmarshal([]byte(`["d"]`), &second)
	fmt.Println("direct twice:", err, err2, first, len(first), second)
	var bad []string
	err = json.Unmarshal([]byte(`["a","b",3]`), &bad)
	fmt.Printf("direct abort: %v %q\n", err, bad)
	err = json.Unmarshal([]byte(`["e","f"]`), &first)
	fmt.Println("direct after abort:", err, first, second)
	var grid [][]int
	err = json.Unmarshal([]byte(`[[1,2],[],[3]]`), &grid)
	fmt.Println("direct grid:", err, grid)
	var n *int
	fmt.Println("direct nil:", json.Unmarshal([]byte(`1`), n))
	p := new(int)
	err = json.Unmarshal([]byte(` 42 `), p)
	fmt.Println("direct int:", err, *p)
	m := map[string]int{"keep": 1}
	err = json.Unmarshal([]byte(`{"new":2}`), &m)
	fmt.Println("direct map:", err, m)
	q := Plain{Name: "old", Count: 5}
	err = json.Unmarshal([]byte(`{"name":"new","count":"x"}`), &q)
	fmt.Println("direct partial:", err, q.Name, q.Count)
	b, _ := json.Marshal(Item{Tags: []string{"x", "y", "z"}[1:]})
	fmt.Println("direct subslice:", string(b))
}

func typed() {
	print := func(label string, b []byte, err error) { fmt.Printf("typed %s: %s %v\n", label, b, err) }
	b, err := json.Marshal(Item{SKU: "a", Price: 1, Tags: []string{"x"}})
	print("item", b, err)
	b, err = json.Marshal(&Item{SKU: "<é>&", Qty: 1 << 60, Ratio: 0.1, Note: "n\u2028", Next: &Item{SKU: "next"}, Flags: map[Named]bool{"b": true, "a": false}})
	print("ptr", b, err)
	b, err = json.Marshal([]Item{{Price: 1 << 53}, {Ratio: math.Copysign(0, -1)}})
	print("bignums", b, err)
	b, err = json.Marshal([]Item{{SKU: "bad\xff"}})
	print("badutf8", b, err)
	b, err = json.Marshal(map[string]Item{"2": {}, "1": {}})
	print("numkeys", b, err)
	b, err = json.Marshal(map[string][]float64{"nan": {math.NaN()}})
	print("nan", b, err)
	b, err = json.Marshal([]*Item(nil))
	print("nil", b, err)
	b, err = json.Marshal(map[string]*Item{"z": nil, "y": {}})
	print("nilptr", b, err)
	root := &Node{Name: "root"}
	root.Kids = []*Node{{Name: "a"}, {Name: "b", Kids: []*Node{}}}
	b, err = json.Marshal(root)
	print("tree", b, err)
	cyc := &Node{Name: "cycle"}
	cyc.Kids = []*Node{cyc}
	_, err = json.Marshal(cyc)
	fmt.Println("typed cycle:", err != nil)
	long := make([]Item, 40)
	for i := range long {
		long[i] = Item{SKU: fmt.Sprint("sku-", i), Price: i}
	}
	long[39].SKU = "日本"
	b, err = json.Marshal(long)
	fmt.Println("typed long:", len(b), string(b[len(b)-40:]), err)
	// Texts of 256 bytes and more take the encoder's UTF-8 check.
	for _, sku := range []string{"plain", "<", ">", "&", "bad\xff", "\u2028", "é"} {
		long[39].SKU = sku
		b, err = json.Marshal(long)
		fmt.Printf("typed long %q: %d %s %v\n", sku, len(b), b[bytes.LastIndexByte(b, '{'):], err)
	}
	s, err := json.Marshal(Item{SKU: "s"})
	fmt.Println("typed string:", string(s), len(s), err)
}

func show(label string, v any) {
	b, err := json.Marshal(v)
	fmt.Printf("%s: %s %v\n", label, b, err)
}

func load(label, data string, v any) {
	err := json.Unmarshal([]byte(data), v)
	switch p := v.(type) {
	case *Plain:
		c := *p
		c.Ptr = nil
		fmt.Printf("%s: %v %+v ptr=%v\n", label, err, c, p.Ptr)
	default:
		fmt.Printf("%s: %v %#v\n", label, err, reflect.ValueOf(v).Elem().Interface())
	}
}

func main() {
	p := Plain{Name: "a<b>&\"c\"\\\n\r\t  é😀", Ratio: -0.0, Small: -128, U16: 65535,
		I64: math.MinInt64, U64: math.MaxUint64, F32: 0.1, OK: true, Tags: []string{"x", ""},
		Map: map[string]int{"z": 1, "a": 2, "<": 3}, Inner: Inner{A: 1}, Bytes: []byte{0, 255, 1},
		Arr: [3]uint8{1, 2, 3}, Dash: 9, NoTag: "n", hidden: 4}
	p.Ratio = math.Copysign(0, -1)
	show("plain", p)
	show("zero", Plain{})
	show("emptyslices", Plain{Tags: []string{}, Empty: []int{}, Map: map[string]int{}, Bytes: []byte{}, Nested: map[string][]bool{}})
	show("ptr", &Plain{Ptr: &Inner{B: "x"}, Any: []any{1, "s", nil, 2.5, true, map[string]any{"k": nil}}})
	show("ctl", "a\x00b\x1fc\x7f\b\f")
	show("numkeys", map[string]int{"10": 1, "2": 2, "a": 3, "__proto__": 4})
	show("numkeys2", map[string]any{"b": map[string]int{"z": 1, "é<": 2}, "a": "\u2028x&"})
	show("numtag", Digits{1, 2})
	show("bigints", []any{int64(1) << 60, uint64(1) << 63, int64(-(1 << 53) - 1), 1 << 53})
	show("smallints", []any{int64(-5), uint64(7), int8(-1), uint32(4000000000), 0.5, -1.25e-10})
	show("negzero", map[string]float64{"z": math.Copysign(0, -1)})
	show("unicode", []string{"日本語", "é", "😀", "\u2029"})
	show("badutf8", "a\xffb\xc3")
	show("floats", []float64{0, 1, -1, 0.1, 1e20, 1e21, 1e-6, 1e-7, 123456789, 1.5e300, 5e-324, math.MaxFloat64})
	show("float32s", []float32{0.1, 1e20, 1e21, 1e-7, 3.4e38, 16777216})
	show("nan", math.NaN())
	show("inf", []float64{math.Inf(1)})
	show("named", map[Named]Named{"b": "1", "a": "2"})
	show("intkeys", map[int]string{2: "b", -1: "a", 10: "c"})
	show("withstr", WithStr{5})
	show("nilmap", map[string]int(nil))
	show("nilslice", []int(nil))
	show("nilptr", (*Inner)(nil))
	show("nilany", nil)
	show("upper", Upper{"a", "b", "c"})
	show("dup", Dup{1, 2, 3})
	show("bytesarr", [][]byte{nil, {}, []byte("hello world")})

	var q Plain
	load("full", `{"name":"xA😀\n\/","count":3,"ratio":1.5e2,"small":-128,"u16":65535,"i64":-9223372036854775808,"u64":18446744073709551615,"f32":0.1,"ok":true,"tags":["a","b"],"map":{"k":1},"inner":{"a":1,"b":"y"},"ptr":{"a":2},"any":{"n":[1,null,"s",true]},"bytes":"AP8B","arr":[7,8,9],"nested":{"q":[true,false]},"NoTag":"t","Dash":4,"hidden":5,"extra":{"deep":[1,2]}}`, &q)
	q = Plain{}
	load("caseins", `{"NAME":"up","Count":2,"notag":"lc","INNER":{"A":3}}`, &q)
	q = Plain{}
	load("dupkeys", `{"name":"first","name":"second","count":1,"count":2}`, &q)
	q = Plain{}
	load("nulls", `{"name":null,"tags":null,"map":null,"ptr":null,"any":null,"count":null,"inner":null}`, &q)
	q = Plain{Tags: []string{"keep"}, Map: map[string]int{"old": 1}, Ptr: &Inner{A: 9}}
	load("existing", `{"tags":["new"],"map":{"new":2},"ptr":{"b":"z"}}`, &q)
	q = Plain{}
	load("overflow8", `{"small":128}`, &q)
	q = Plain{}
	load("overflowu", `{"u16":-1}`, &q)
	q = Plain{}
	load("float-int", `{"count":1.5}`, &q)
	q = Plain{}
	load("exp-int", `{"count":1e2}`, &q)
	q = Plain{}
	load("wrongtype", `{"name":5,"count":"x"}`, &q)
	q = Plain{}
	load("trailing", `{"name":"a"} x`, &q)
	q = Plain{}
	load("trailingws", "{\"name\":\"a\"} \n\t", &q)
	q = Plain{}
	load("truncated", `{"name":"a"`, &q)
	q = Plain{}
	load("badescape", `{"name":"\q"}`, &q)
	q = Plain{}
	load("lonesurrogate", `{"name":"\ud800x"}`, &q)
	q = Plain{}
	load("rawctl", "{\"name\":\"a\x01\"}", &q)
	q = Plain{}
	load("rawutf8", "{\"name\":\"\xff\xfe\"}", &q)
	q = Plain{}
	load("leadingzero", `{"count":01}`, &q)
	q = Plain{}
	load("bigexp", `{"ratio":1e400}`, &q)
	q = Plain{}
	load("arrshort", `{"arr":[1]}`, &q)
	q = Plain{}
	load("arrlong", `{"arr":[1,2,3,4,5]}`, &q)
	q = Plain{}
	load("badbase64", `{"bytes":"!!"}`, &q)
	var up Upper
	load("kelvin", `{"key":"1","KÉY":"2","KELV":"3"}`, &up)
	var d Dup
	load("dupfields", `{"x":1,"C":2}`, &d)
	var ws WithStr
	load("withstr", `{"n":"7"}`, &ws)
	var m map[string]float64
	load("map", `{"a":1,"b":-2.5e-3,"c":0}`, &m)
	var mi map[int]string
	load("intmap", `{"1":"a","-2":"b"}`, &mi)
	var s []any
	load("anyslice", `[1, "x", [true, null], {"z": {}}, -0, 1E+2]`, &s)
	var i64 int64
	load("i64", `9223372036854775807`, &i64)
	load("i64over", `9223372036854775808`, &i64)
	var u64 uint64
	load("u64", `18446744073709551615`, &u64)
	var f32 float32
	load("f32", `3.5e38`, &f32)
	var str string
	load("str", `"é\t"`, &str)
	var b bool
	load("bool", ` true `, &b)
	load("empty", ``, &b)
	var arr [2]string
	load("arr", `["a","b","c"]`, &arr)
	var named map[Named]Named
	load("named", `{"x":"y"}`, &named)
	var ip *int
	fmt.Println("newptr", json.Unmarshal([]byte(`5`), &ip), *ip)
	fmt.Println(json.Unmarshal([]byte(`1`), nil))
	var notptr int
	fmt.Println(json.Unmarshal([]byte(`1`), notptr))
	typed()
	direct()
}
