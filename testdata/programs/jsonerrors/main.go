// Command jsonerrors decodes JSON with json.Unmarshal only, which goesm
// does in its runtime without encoding/json, and checks that the errors
// are encoding/json's own: their messages, fields and types, seen through
// reflection since naming the types would bring the package back.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

type Item struct {
	Name  string   `json:"name"`
	Price int      `json:"price"`
	Tags  []string `json:"tags,omitempty"`
	Next  *Item    `json:"next"`
}

type Doc struct {
	Items []Item           `json:"items"`
	Meta  map[string]any   `json:"meta"`
	Count uint8            `json:"count"`
	Ratio float64          `json:"ratio"`
	Seen  map[string]bool  `json:"seen"`
	Deep  map[string][]int `json:"deep"`
}

// describe prints an error as a program that cannot name its type sees it.
func describe(err error) string {
	if err == nil {
		return "<nil>"
	}
	var b strings.Builder
	t := reflect.TypeOf(err)
	fmt.Fprintf(&b, "%T %q", err, err.Error())
	v := reflect.ValueOf(err).Elem()
	for i := 0; i < v.NumField(); i++ {
		f := t.Elem().Field(i)
		fv := v.Field(i)
		switch {
		case !f.IsExported():
			fmt.Fprintf(&b, " %s(%v)", f.Name, f.Type)
		case f.Type.String() == "reflect.Type":
			typ, _ := fv.Interface().(reflect.Type)
			fmt.Fprintf(&b, " %s:%v", f.Name, typ)
		default:
			fmt.Fprintf(&b, " %s:%v(%v)", f.Name, fv.Interface(), f.Type)
		}
	}
	fmt.Fprintf(&b, " unwrap:%v", errors.Unwrap(err))
	return b.String()
}

// show prints d with its pointers followed.
func show(d Doc) string {
	var items []string
	for _, it := range d.Items {
		s := fmt.Sprintf("{%s %d %q", it.Name, it.Price, it.Tags)
		for n := it.Next; n != nil; n = n.Next {
			s += fmt.Sprintf(" -> {%s %d %q}", n.Name, n.Price, n.Tags)
		}
		items = append(items, s+"}")
	}
	return fmt.Sprintf("items=%v nil=%v meta=%v count=%d ratio=%v seen=%v deep=%v", items, d.Items == nil, d.Meta, d.Count, d.Ratio, d.Seen, d.Deep)
}

func main() {
	inputs := []string{
		`{"items":[{"name":"a","price":1},{"name":"b","price":"2"}],"count":3}`,
		`{"items":[{"name":"a","next":{"name":"c","price":1.5}}]}`,
		`{"count":300,"ratio":1e400,"seen":{"x":1},"deep":{"a":[1,"b"]}}`,
		`{"meta":{"a":[1,{"b":null}],"c":true},"items":null}`,
		`{"items":[{"name":"a"}`,
		`{"items":[{"name":"a\x"}]}`,
		`[1,2]`,
		`{"count":-0}`,
		"  ",
		`{"ITEMS":[{"NAME":"k","Price":2}],"count":1}`,
		`{"items":[{"tags":["x","y"]}],"items":[{"tags":["z"]}]}`,
		`{"a/b~c":1,"meta":{"x/y":{"z":1e999}}}`,
	}
	for _, in := range inputs {
		var d Doc
		err := json.Unmarshal([]byte(in), &d)
		fmt.Printf("%s\n  %s\n  %s\n", in, describe(err), show(d))
	}

	var v any
	for _, in := range []string{`{"a":[1,"x",true,null]}`, `[{"b":1}]`, `"s"`, `1e999`, `nul`} {
		err := json.Unmarshal([]byte(in), &v)
		fmt.Printf("%s: %v %v\n", in, v, describe(err))
	}

	var p *Doc
	fmt.Println(describe(json.Unmarshal([]byte(`{}`), p)))
	var n int
	fmt.Println(describe(json.Unmarshal([]byte(`{}`), &n)), n)
}
