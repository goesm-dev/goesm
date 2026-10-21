// Command jsonlegacy runs json.Unmarshal and json.Marshal on random JSON
// text and values of types with methods, json tags, embedded fields and
// maps of every key kind, and prints the errors (fields included) and the
// results. goesm decodes and encodes these with encoding/json's legacy
// engine (the patches of v2_decode.go and v2_encode.go), Go with json v2:
// the output shows they agree.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"math/rand"
	"net"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Types for the differential tests of the legacy engine against json v2.
type dInner struct {
	A int     `json:"a"`
	B string  `json:"b,omitempty"`
	C *dInner `json:"c,omitempty"`
	D []int   `json:"d"`
}

type dEmbedded struct {
	E1 int
	E2 string `json:"e2"`
}

type dembedded struct {
	U1 int
	U2 bool
}

type DPtrEmbedded struct {
	P1 int
}

type dTextVal int

func (t dTextVal) MarshalText() ([]byte, error) {
	if t < 0 {
		return nil, errors.New("negative text")
	}
	return []byte("t" + strconv.Itoa(int(t))), nil
}

func (t *dTextVal) UnmarshalText(b []byte) error {
	if len(b) == 0 || b[0] != 't' {
		return errors.New("missing t")
	}
	n, err := strconv.Atoi(string(b[1:]))
	*t = dTextVal(n)
	return err
}

type dJSONVal struct{ X int }

func (j dJSONVal) MarshalJSON() ([]byte, error) {
	switch {
	case j.X < 0:
		return nil, errors.New("negative json")
	case j.X == 7:
		return []byte("{bad"), nil
	case j.X == 8:
		return []byte(" [ 1 , 2 ] "), nil
	case j.X == 9:
		return []byte(`"<&>"`), nil
	case j.X == 10:
		return []byte("\"a\xffb\u2028\""), nil
	case j.X == 11:
		return []byte(`{"b":1,"a":2,"a":3}`), nil
	case j.X == 12:
		return []byte(`"\ud800"`), nil
	}
	return []byte(strconv.Itoa(j.X)), nil
}

func (j *dJSONVal) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '[' {
		return errors.New("no arrays")
	}
	if string(b) == "null" {
		j.X = -100
		return nil
	}
	n, err := strconv.Atoi(string(b))
	if err != nil {
		return &json.UnmarshalTypeError{Value: "custom", Type: reflect.TypeFor[int]()}
	}
	j.X = n
	return nil
}

type dJSONPtr struct{ Y int }

func (j *dJSONPtr) MarshalJSON() ([]byte, error) {
	if j.Y < 0 {
		return nil, errors.New("negative ptr json")
	}
	return []byte(strconv.Itoa(j.Y)), nil
}

type dTextPtr struct{ Z string }

func (t *dTextPtr) MarshalText() ([]byte, error) {
	if t.Z == "bad" {
		return nil, errors.New("bad text")
	}
	return []byte(t.Z), nil
}

func (t *dTextPtr) UnmarshalText(b []byte) error {
	t.Z = string(b)
	return nil
}

type dAll struct {
	I     int                 `json:"i"`
	I8    int8                `json:"i8,omitempty"`
	U16   uint16              `json:"u16"`
	F32   float32             `json:"f32"`
	F64   float64             `json:"f64,omitempty"`
	S     string              `json:"s"`
	Bs    []byte              `json:"bs"`
	B     bool                `json:"b"`
	QI    int                 `json:"qi,string"`
	QB    bool                `json:"qb,string"`
	QS    string              `json:"qs,string"`
	QF    float64             `json:"qf,string,omitempty"`
	QP    *int                `json:"qp,string"`
	N     json.Number         `json:"n"`
	QN    json.Number         `json:"qn,string"`
	Any   any                 `json:"any"`
	Err   error               `json:"err"`
	P     *dInner             `json:"p"`
	In    dInner              `json:"in"`
	Sl    []dInner            `json:"sl"`
	Ar    [2]int              `json:"ar"`
	M     map[string]int      `json:"m"`
	MI    map[int]string      `json:"mi"`
	MU    map[uint8]bool      `json:"mu"`
	MT    map[dTextVal]int    `json:"mt"`
	MP    map[dTextPtr]int    `json:"mp"`
	MF    map[float64]int     `json:"mf"`
	TV    dTextVal            `json:"tv"`
	TVP   *dTextVal           `json:"tvp"`
	TP    dTextPtr            `json:"tp"`
	JV    dJSONVal            `json:"jv"`
	JVP   *dJSONVal           `json:"jvp"`
	JP    dJSONPtr            `json:"jp"`
	JPS   []dJSONPtr          `json:"jps"`
	JVM   map[string]dJSONVal `json:"jvm"`
	Raw   json.RawMessage     `json:"raw"`
	T     time.Time           `json:"t"`
	D     time.Duration       `json:"d"`
	IP    net.IP              `json:"ip"`
	Skip  int                 `json:"-"`
	Dash  int                 `json:"-,"`
	unexp int
	Ifc   fmt.Stringer `json:"ifc"`
	dEmbedded
	dembedded
	*DPtrEmbedded
}

type dFold struct {
	FOO int
	Bar int
	BAR int
}

type dTags struct {
	dFold
	A   int    `json:"bad\\name"`
	B   int    `json:"😀"`
	C   int    `json:"'quoted'"`
	D   int    `json:"x y,omitempty"`
	E   int    `json:"1abc\\x"`
	F   int    `json:",case:strict"`
	G   int    `json:"g,omitEmpty"`
	H   int    `json:"h,string,string"`
	I   dInner `json:",embed"`
	K   int    `json:"\u00e9\xff"`
	L   int    `json:"l" json:"m"`
	M   int    `json:"m,case:ignore"`
	N   string `json:"n,omitzero,string"`
	Foo int
	O   *int   `json:",omitempty,string"`
	P   [2]int `json:"p,omitzero"`
	Q   int    `json:"q,"`
	R   int    `json:",,"`
	S   int    `json:"s,'omitempty'"`
	T   bool   `json:"t,string"`
	U   any    `json:"u,string"`
}

type dFormat struct {
	A int
	J int `json:"j,format:RFC3339"`
}

type dNestFormat struct {
	X dFormat
	Y int
}

type dCycle struct {
	Next *dCycle `json:"next"`
	V    int     `json:"v"`
}

var diffTypes = []reflect.Type{
	reflect.TypeFor[dAll](),
	reflect.TypeFor[*dAll](),
	reflect.TypeFor[dInner](),
	reflect.TypeFor[[]any](),
	reflect.TypeFor[map[string]any](),
	reflect.TypeFor[any](),
	reflect.TypeFor[[]int](),
	reflect.TypeFor[[3]string](),
	reflect.TypeFor[map[int]dInner](),
	reflect.TypeFor[int](),
	reflect.TypeFor[string](),
	reflect.TypeFor[json.Number](),
	reflect.TypeFor[dJSONVal](),
	reflect.TypeFor[dTextVal](),
	reflect.TypeFor[[]dJSONVal](),
	reflect.TypeFor[map[string]*dInner](),
	reflect.TypeFor[error](),
	reflect.TypeFor[time.Time](),
	reflect.TypeFor[dTags](),
	reflect.TypeFor[*dTags](),
	reflect.TypeFor[dFormat](),
	reflect.TypeFor[dNestFormat](),
	reflect.TypeFor[map[string]float32](),
	reflect.TypeFor[struct {
		F float32  `json:",string"`
		G *float64 `json:",string"`
		T time.Time
		M map[float32]any
	}](),
}

// randJSON returns random JSON text, valid most of the time.
func randJSON(r *rand.Rand, depth int) string {
	keys := []string{"i", "I", "i8", "u16", "f32", "f64", "s", "bs", "b", "qi", "qb", "qs", "qf", "qp", "n", "qn", "any", "err", "p", "in", "sl", "ar", "m", "mi", "mu", "mt", "mp", "mf", "tv", "tvp", "tp", "jv", "jvp", "jp", "jps", "jvm", "raw", "t", "d", "ip", "Skip", "-", "unexp", "ifc", "E1", "e2", "U1", "U2", "P1", "a", "b", "c", "d", "A", "x/y", "k~", "1", "-5", "300", "t1", "t-3", "2009-11-10T23:00:00Z", "\u212a", "é", "F", "G", "T", "M", "f", "g", "X", "Y", "bad", "😀", "C", "'quoted'", "x y", "1abc\\x", "h", "H", "I", "j", "J", "K", "é\ufffd", "L", "l", "m", "foo", "FOO", "Foo", "bar", "BAR", "Bar", "n", "O", "o", "p", "q", "Q", "R", "s", "S", "t", "u", "U", "E", "e"}
	scalars := []string{"0", "1", "-1", "1.5", "1e400", "-0", "300", "127", "128", "-129", "65536", "1e2", "12345678901234567890", `""`, `"x"`, `"1"`, `"12"`, `"-7"`, `"1.5"`, `"true"`, `"false"`, `"null"`, `"nul"`, `"\"q\""`, `"\""`, `"t5"`, `"t-1"`, `"5"`, `"aGVsbG8="`, `"!!"`, `"2009-11-10T23:00:00Z"`, `"1s"`, `"127.0.0.1"`, `"\u00e9"`, `"\ud800"`, `"a\u0000b"`, "true", "false", "null", `"bad"`, `"<&>"`, `"\u2028"`, `"1e400"`, `"-1e400"`, `"1e39"`, `"2009-11-10T23:00:00+25:00"`, `"2009-11-10T23:00:00,5Z"`}
	if depth > 3 || r.Intn(3) == 0 {
		return scalars[r.Intn(len(scalars))]
	}
	switch r.Intn(3) {
	case 0:
		n := r.Intn(4)
		var b strings.Builder
		b.WriteByte('[')
		for i := 0; i < n; i++ {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(randJSON(r, depth+1))
		}
		b.WriteByte(']')
		return b.String()
	default:
		n := r.Intn(5)
		var b strings.Builder
		b.WriteByte('{')
		for i := 0; i < n; i++ {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(strconv.Quote(keys[r.Intn(len(keys))]))
			b.WriteByte(':')
			b.WriteString(randJSON(r, depth+1))
		}
		b.WriteByte('}')
		return b.String()
	}
}

// describe renders an error with its fields for comparison.
func describe(err error) string {
	if err == nil {
		return "<nil>"
	}
	s := fmt.Sprintf("%T %v", err, err)
	switch e := err.(type) {
	case *json.UnmarshalTypeError:
		s += fmt.Sprintf(" {%q %v %d %q %q %v}", e.Value, e.Type, e.Offset, e.Struct, e.Field, e.Err)
	case *json.SyntaxError:
		s += fmt.Sprintf(" {%d}", e.Offset)
	case *json.MarshalerError:
		s += fmt.Sprintf(" {%v %q %v}", e.Type, e.Err, e.Unwrap())
	case *json.UnsupportedValueError:
		s += fmt.Sprintf(" {%q}", e.Str)
	case *json.UnsupportedTypeError:
		s += fmt.Sprintf(" {%v}", e.Type)
	}
	return s
}

func randValue(r *rand.Rand, t reflect.Type, depth int) reflect.Value {
	v := reflect.New(t).Elem()
	if depth > 4 {
		return v
	}
	switch t.Kind() {
	case reflect.Bool:
		v.SetBool(r.Intn(2) == 0)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		x := []int64{0, 1, -1, 7, 8, 9, 10, 11, 12, 127, -128, 1 << 40}[r.Intn(12)]
		if !v.OverflowInt(x) {
			v.SetInt(x)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		v.SetUint(uint64(r.Intn(300)))
	case reflect.Float32, reflect.Float64:
		v.SetFloat([]float64{0, 1.5, -0.0, 1e21, 1e-7, 3.4e38, math.NaN(), math.Inf(1), 123456789.125}[r.Intn(9)])
	case reflect.String:
		v.SetString([]string{"", "a", "<&>", "\u2028", "é", "x\xffy", "bad", "t1", "\"q\"", "1.5"}[r.Intn(10)])
	case reflect.Slice:
		if r.Intn(4) == 0 {
			return v
		}
		n := r.Intn(3)
		s := reflect.MakeSlice(t, n, n)
		for i := 0; i < n; i++ {
			s.Index(i).Set(randValue(r, t.Elem(), depth+1))
		}
		v.Set(s)
	case reflect.Array:
		for i := 0; i < t.Len(); i++ {
			v.Index(i).Set(randValue(r, t.Elem(), depth+1))
		}
	case reflect.Map:
		if r.Intn(4) == 0 {
			return v
		}
		m := reflect.MakeMap(t)
		for i := r.Intn(3); i > 0; i-- {
			k := randValue(r, t.Key(), depth+1)
			if t.Key().Kind() == reflect.Interface {
				// Keys of every kind json v2 accepts or rejects, but at
				// most one it rejects: which one a map's iteration meets
				// first varies.
				keys := []any{"a", 1, uint8(2), float32(1.5), dTextVal(3), 2.5, true, nil, dTextVal(-1), &dJSONPtr{}, [1]int{}, math.Inf(1)}
				if m.Len() > 0 {
					keys = keys[:5]
				}
				k = reflect.New(t.Key()).Elem()
				if key := keys[r.Intn(len(keys))]; key != nil {
					k.Set(reflect.ValueOf(key))
				}
			}
			m.SetMapIndex(k, randValue(r, t.Elem(), depth+1))
		}
		v.Set(m)
	case reflect.Pointer:
		if r.Intn(3) == 0 {
			return v
		}
		p := reflect.New(t.Elem())
		p.Elem().Set(randValue(r, t.Elem(), depth+1))
		v.Set(p)
	case reflect.Struct:
		if t == reflect.TypeFor[time.Time]() {
			v.Set(reflect.ValueOf(time.Date(2000+r.Intn(30), 1, 2, 3, 4, 5, r.Intn(3)*1000, time.UTC)))
			return v
		}
		for i := 0; i < t.NumField(); i++ {
			if t.Field(i).IsExported() || t.Field(i).Anonymous && t.Field(i).Type.Kind() != reflect.Pointer {
				f := v.Field(i)
				if f.CanSet() {
					f.Set(randValue(r, t.Field(i).Type, depth+1))
				} else if t.Field(i).Anonymous {
					for j := 0; j < f.NumField(); j++ {
						if f.Field(j).CanSet() {
							f.Field(j).Set(randValue(r, f.Type().Field(j).Type, depth+1))
						}
					}
				}
			}
		}
	case reflect.Interface:
		if t.NumMethod() > 0 {
			if r.Intn(2) == 0 && reflect.TypeFor[time.Duration]().Implements(t) {
				v.Set(reflect.ValueOf(time.Duration(r.Intn(1e9))))
			}
			return v
		}
		switch r.Intn(6) {
		case 0:
			v.Set(reflect.ValueOf(r.Intn(10)))
		case 1:
			v.Set(reflect.ValueOf(dJSONVal{X: r.Intn(14) - 1}))
		case 2:
			v.Set(reflect.ValueOf(&dJSONPtr{Y: r.Intn(3) - 1}))
		case 3:
			v.Set(reflect.ValueOf(map[string]any{"k": []any{1.5, "s", nil}}))
		case 4:
			v.Set(reflect.ValueOf(dJSONPtr{Y: -1}))
		}
	}
	return v
}

// dump renders v deterministically: through pointers, with map entries
// sorted.
func dump(b *strings.Builder, v reflect.Value, depth int) {
	if depth > 20 {
		b.WriteString("…")
		return
	}
	if !v.IsValid() {
		b.WriteString("<invalid>")
		return
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			b.WriteString("nil")
			return
		}
		if v.Kind() == reflect.Interface {
			fmt.Fprintf(b, "(%v)", v.Elem().Type())
		} else {
			b.WriteByte('&')
		}
		dump(b, v.Elem(), depth+1)
	case reflect.Struct:
		if v.Type() == reflect.TypeFor[time.Time]() && v.CanInterface() {
			b.WriteString(v.Interface().(time.Time).String())
			return
		}
		b.WriteByte('{')
		for i := 0; i < v.NumField(); i++ {
			if i > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(v.Type().Field(i).Name + ":")
			dump(b, v.Field(i), depth+1)
		}
		b.WriteByte('}')
	case reflect.Slice, reflect.Array:
		if v.Kind() == reflect.Slice && v.IsNil() {
			b.WriteString("nil")
			return
		}
		b.WriteByte('[')
		for i := 0; i < v.Len(); i++ {
			if i > 0 {
				b.WriteByte(' ')
			}
			dump(b, v.Index(i), depth+1)
		}
		b.WriteByte(']')
	case reflect.Map:
		if v.IsNil() {
			b.WriteString("nilmap")
			return
		}
		var entries []string
		for it := v.MapRange(); it.Next(); {
			var e strings.Builder
			dump(&e, it.Key(), depth+1)
			e.WriteByte(':')
			dump(&e, it.Value(), depth+1)
			entries = append(entries, e.String())
		}
		sort.Strings(entries)
		b.WriteString("map[" + strings.Join(entries, " ") + "]")
	case reflect.String:
		b.WriteString(strconv.Quote(v.String()))
	case reflect.Float32, reflect.Float64:
		b.WriteString(strconv.FormatFloat(v.Float(), 'g', -1, v.Type().Bits()))
	default:
		fmt.Fprint(b, v)
	}
}

func hash(s string) string {
	h := fnv.New64a()
	h.Write([]byte(s))
	return strconv.FormatUint(h.Sum64(), 36)
}

type hidden struct{ H int }

type withHidden struct {
	*hidden
	V int
}

func fixed() {
	var n int
	var nilp *int
	for _, c := range []struct {
		in string
		v  any
	}{
		{"{", nil}, {"1", nil}, {"1", nilp}, {"{", nilp}, {"1", n}, {"[1,", &n}, {" 1 ", &n}, {"1 2", &n}, {"", &n}, {"\xff", &n},
		{`{"H":1,"V":2}`, &withHidden{}}, {`{"V":2}`, &withHidden{}}, {`{"H":null}`, &withHidden{}},
		{`[1, {"a": "\u00e9\ud800"}, null]`, new(json.RawMessage)}, {`{"a":[1,2]}`, &map[string]json.RawMessage{}},
		{`"\u00e9\ud83d\ude00\ud800x"`, new(string)}, {`"\u00e9\ud83d\ude00\ud800x"`, new(any)}, {"\"\xff\xfe\"", new(string)},
		{`12345678901234567890`, new(any)}, {`-0`, new(any)}, {`1e-400`, new(float64)}, {`[1e400, -1e400]`, new([]float32)},
	} {
		err := json.Unmarshal([]byte(c.in), c.v)
		var b strings.Builder
		dump(&b, reflect.ValueOf(c.v), 0)
		fmt.Printf("fixed %q %s %s\n", c.in, describe(err), b.String())
	}
	for _, in := range []string{"", "1", "{", `{"a":1,"a":2}`, "\"\xff\"", "[1,]", "nul", "  {} ", "{}{}"} {
		fmt.Printf("valid %q %v\n", in, json.Valid([]byte(in)))
	}
	for _, v := range []any{
		dInner{A: 1, D: []int{1, 2}},
		map[string]any{"b": []any{}, "a": map[string]int{}},
		withHidden{V: 3},
		withHidden{hidden: &hidden{4}, V: 5},
		json.RawMessage(` { "a" : [ 1 , 2 ] } `),
		json.RawMessage(`{"a":}`),
		json.Number("1e3"),
		[]byte("\xff\x00"),
		"\xff<\u2028>&",
		struct {
			A int `json:"-"`
			B int `json:"-,"`
			C int `json:",omitempty"`
			D int `json:"!#$%&()*+-./:;<=>?@[]^_{|}~ "`
			E int `json:"\u00e9"`
			F int `json:"bad\\name"`
		}{1, 2, 0, 4, 5, 6},
	} {
		out, err := json.Marshal(v)
		fmt.Printf("fixedm %s %s\n", describe(err), out)
		out, err = json.MarshalIndent(v, ">", "  ")
		fmt.Printf("indent %s %s\n", describe(err), out)
	}
}

func main() {
	fixed()
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 3000; i++ {
		in := randJSON(r, 0)
		if r.Intn(10) == 0 && len(in) > 1 {
			j := r.Intn(len(in))
			in = in[:j] + string("{}[],:\"x"[r.Intn(8)]) + in[j+1:]
		}
		typ := diffTypes[r.Intn(len(diffTypes))]
		v := reflect.New(typ)
		if r.Intn(4) == 0 {
			json.Unmarshal([]byte(randJSON(r, 0)), v.Interface())
		}
		err := json.Unmarshal([]byte(in), v.Interface())
		var b strings.Builder
		dump(&b, v.Elem(), 0)
		fmt.Printf("u%d %v %s %s\n", i, typ, describe(err), hash(b.String()))
	}

	types := append(diffTypes, reflect.TypeFor[dCycle](), reflect.TypeFor[map[dTextVal]int](), reflect.TypeFor[map[dTextPtr]int](), reflect.TypeFor[[]dJSONPtr](), reflect.TypeFor[[2]dJSONPtr](), reflect.TypeFor[map[string]dJSONPtr](), reflect.TypeFor[dJSONPtr](), reflect.TypeFor[*dJSONPtr](), reflect.TypeFor[dTextPtr](), reflect.TypeFor[chan int](), reflect.TypeFor[func()](), reflect.TypeFor[complex128](), reflect.TypeFor[map[bool]int](), reflect.TypeFor[map[any]int](), reflect.TypeFor[map[float64]string]())
	for i := 0; i < 2000; i++ {
		typ := types[r.Intn(len(types))]
		v := randValue(r, typ, 0)
		x := v.Interface()
		if typ == reflect.TypeFor[dCycle]() && r.Intn(3) == 0 {
			c := &dCycle{V: 1}
			c.Next = c
			x = c
		}
		if r.Intn(5) == 0 {
			p := reflect.New(typ)
			p.Elem().Set(v)
			x = p.Interface()
		}
		out, err := json.Marshal(x)
		if err != nil && strings.HasSuffix(err.Error(), "Inf") || err != nil && strings.HasSuffix(err.Error(), "NaN") {
			// Which of several unsupported floats a map's iteration
			// meets first varies.
			err = errors.New("unsupported float")
		}
		fmt.Printf("m%d %T %s %s\n", i, x, describe(err), out)
	}
}
