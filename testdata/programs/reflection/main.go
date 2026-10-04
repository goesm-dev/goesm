// Command reflection exercises package reflect: types, kinds, struct
// fields and tags, methods, setting through pointers, calls, maps, slices,
// conversions and DeepEqual.
package main

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

type Base struct {
	ID int `json:"id" db:"pk"`
}

func (b Base) Describe() string { return fmt.Sprintf("base %d", b.ID) }

type Item struct {
	Base
	Name    string            `json:"name,omitempty"`
	Price   float64           `json:"price"`
	Tags    []string          `json:"tags"`
	Attrs   map[string]string `json:"-"`
	Next    *Item
	private bool
}

func (i *Item) Rename(s string) { i.Name = s }
func (i Item) Total(n int, extra ...float64) float64 {
	t := i.Price * float64(n)
	for _, e := range extra {
		t += e
	}
	return t
}

type Shape interface {
	Area() float64
}

type Sq struct{ S float64 }

func (s Sq) Area() float64 { return s.S * s.S }

type MyInt int

func main() {
	it := Item{Base: Base{7}, Name: "pen", Price: 1.5, Tags: []string{"a"}}
	t := reflect.TypeOf(it)
	fmt.Println(t, t.Name(), t.Kind(), t.PkgPath(), t.NumField(), t.NumMethod(), t.Size() > 0)
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		fmt.Println(i, f.Name, f.Type, f.Tag.Get("json"), f.Anonymous, f.IsExported(), f.Index)
	}
	if f, ok := t.FieldByName("ID"); ok {
		v, ok := f.Tag.Lookup("db")
		fmt.Println("ID", f.Index, v, ok)
	}
	for i := 0; i < t.NumMethod(); i++ {
		m := t.Method(i)
		fmt.Println("method", m.Name, m.Type)
	}
	pt := reflect.TypeOf(&it)
	fmt.Println(pt, pt.Kind(), pt.Elem() == t, pt.NumMethod())
	for i := 0; i < pt.NumMethod(); i++ {
		fmt.Println("ptr method", pt.Method(i).Name)
	}

	v := reflect.ValueOf(&it).Elem()
	v.FieldByName("Name").SetString("pencil")
	v.Field(2).SetFloat(2.25)
	v.FieldByName("ID").SetInt(99)
	v.FieldByName("Tags").Set(reflect.Append(v.FieldByName("Tags"), reflect.ValueOf("b")))
	fmt.Println(it.Name, it.Price, it.ID, it.Tags, v.FieldByName("private").CanSet(), v.Field(0).CanSet())
	res := reflect.ValueOf(it).MethodByName("Total").Call([]reflect.Value{reflect.ValueOf(2), reflect.ValueOf(0.5), reflect.ValueOf(0.25)})
	fmt.Println(res[0].Float(), res[0].Kind())
	reflect.ValueOf(&it).MethodByName("Rename").Call([]reflect.Value{reflect.ValueOf("marker")})
	fmt.Println(it.Name, reflect.ValueOf(it).MethodByName("Describe").Call(nil)[0])

	m := map[string]int{"b": 2, "a": 1}
	mv := reflect.ValueOf(m)
	keys := mv.MapKeys()
	sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
	for _, k := range keys {
		fmt.Println(k, mv.MapIndex(k))
	}
	mv.SetMapIndex(reflect.ValueOf("c"), reflect.ValueOf(3))
	mv.SetMapIndex(reflect.ValueOf("a"), reflect.Value{})
	fmt.Println(len(m), m["c"], mv.Len(), mv.MapIndex(reflect.ValueOf("zz")).IsValid())
	iter := mv.MapRange()
	sum := 0
	for iter.Next() {
		sum += int(iter.Value().Int())
	}
	fmt.Println("sum", sum)

	s := reflect.MakeSlice(reflect.TypeOf([]int{}), 2, 5)
	s.Index(1).SetInt(5)
	s = reflect.Append(s, reflect.ValueOf(9))
	fmt.Println(s.Interface(), s.Len(), s.Cap(), s.Slice(1, 3).Interface())
	arr := [3]string{"x", "y", "z"}
	av := reflect.ValueOf(&arr).Elem()
	av.Index(0).SetString("w")
	fmt.Println(arr, av.Len(), av.Type())

	var sh Shape = Sq{3}
	sv := reflect.ValueOf(sh)
	fmt.Println(sv.Type(), sv.Kind(), sv.Method(0).Call(nil)[0].Float())
	st := reflect.TypeOf((*Shape)(nil)).Elem()
	fmt.Println(st, st.Kind(), reflect.TypeOf(Sq{}).Implements(st), reflect.TypeOf(1).Implements(st))

	mi := reflect.ValueOf(MyInt(5))
	fmt.Println(mi.Type(), mi.Kind(), mi.Int(), mi.Convert(reflect.TypeOf(0.0)).Float(), mi.CanConvert(reflect.TypeOf("")))
	fmt.Println(reflect.ValueOf(65).Convert(reflect.TypeOf("")).String(), reflect.ValueOf("hi").Convert(reflect.TypeOf([]byte{})).Bytes())
	fmt.Println(reflect.ValueOf(int8(-3)).Convert(reflect.TypeOf(uint8(0))).Uint(), reflect.ValueOf(uint64(1<<63)).Uint(), reflect.ValueOf(int64(-1<<62)).Int())

	fn := reflect.ValueOf(strings.Repeat)
	fmt.Println(fn.Type(), fn.Call([]reflect.Value{reflect.ValueOf("ab"), reflect.ValueOf(3)})[0])
	swap := func(in []reflect.Value) []reflect.Value { return []reflect.Value{in[1], in[0]} }
	var intSwap func(int, int) (int, int)
	reflect.ValueOf(&intSwap).Elem().Set(reflect.MakeFunc(reflect.TypeOf(intSwap), swap))
	fmt.Println(intSwap(1, 2))

	fmt.Println(reflect.DeepEqual(map[string][]int{"a": {1}}, map[string][]int{"a": {1}}), reflect.DeepEqual([]int{1}, []int{2}), reflect.DeepEqual(it, it), reflect.DeepEqual(nil, nil), reflect.DeepEqual([]int(nil), []int{}))
	z := reflect.Zero(reflect.TypeOf(it))
	fmt.Println(z.FieldByName("Name").String() == "", z.IsZero(), reflect.ValueOf(it).IsZero())
	np := reflect.New(reflect.TypeOf(0))
	np.Elem().SetInt(42)
	fmt.Println(*(np.Interface().(*int)), np.Kind(), np.Elem().CanAddr())
	var nilp *Item
	fmt.Println(reflect.ValueOf(nilp).IsNil(), reflect.ValueOf(nilp).Elem().IsValid(), reflect.Indirect(reflect.ValueOf(&it)).Type())
	fmt.Println(reflect.TypeOf([]map[string]*Item{}), reflect.TypeOf(func(int, ...string) error { return nil }), reflect.TypeOf(make(chan<- int)), reflect.TypeOf([2]bool{}))
	fmt.Println(reflect.PointerTo(t), reflect.SliceOf(t), reflect.MapOf(reflect.TypeOf(""), t), reflect.TypeFor[error]())
	ch := reflect.MakeChan(reflect.TypeOf(make(chan int)), 1)
	fmt.Println(ch.TrySend(reflect.ValueOf(3)), ch.Len())
	x, ok := ch.TryRecv()
	fmt.Println(x, ok)
	fields := reflect.VisibleFields(t)
	var names []string
	for _, f := range fields {
		names = append(names, f.Name)
	}
	fmt.Println(names)
	var anyv any = &it
	fmt.Println(reflect.ValueOf(anyv).Elem().FieldByName("Price").Interface(), reflect.TypeOf(anyv).Elem().Name())
}
