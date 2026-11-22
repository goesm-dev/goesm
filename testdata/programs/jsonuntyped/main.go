// Decoding into an empty interface, a map[string]any and a []any, which
// goesm does with JSON.parse, from a string's bytes and from a []byte: every kind of value, repeated names, escapes,
// lone surrogates, text in several scripts, invalid UTF-8, numbers out of
// range, deep nesting, white space, trailing data, and values decoded into
// a map or a slice that is not nil.
package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// upper is a map key that UnmarshalText makes upper case.
type upper string

func (u *upper) UnmarshalText(b []byte) error {
	*u = upper(strings.ToUpper(string(b)))
	return nil
}

// counted counts the names of an object in UnmarshalJSON.
type counted struct{ n int }

func (c *counted) UnmarshalJSON(b []byte) error {
	var m map[string]any
	err := json.Unmarshal(b, &m)
	c.n = len(m)
	return err
}

// show prints v with sorted map keys and the types of its values.
func show(v any) string {
	switch v := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = fmt.Sprintf("%q:%s", k, show(v[k]))
		}
		return "{" + strings.Join(parts, ",") + "}"
	case []any:
		parts := make([]string, len(v))
		for i, x := range v {
			parts[i] = show(x)
		}
		return "[" + strings.Join(parts, ",") + "]"
	case string:
		return fmt.Sprintf("%q(%d)", v, len(v))
	case float64:
		return fmt.Sprintf("%v", v)
	case nil:
		return "nil"
	}
	return fmt.Sprintf("%T(%v)", v, v)
}

var inputs = []string{
	`null`, `true`, `false`, `0`, `-0`, `1.5e3`, `-12.25`, `1e-7`, `123456789012345678901234567890`,
	`1e400`, `-1e400`, `"plain"`, `""`, `"tab\tnew\nquote\"back\\slash\/"`, `"é世😀"`,
	`"\ud800"`, `"\ud800\ud800x"`, `"\udc00"`, `"\ud83d"`, "\"\xff\"", "\"caf\xc3\xa9 \xe4\xb8\x96\"",
	"\"\xed\xa0\x80\"", `{}`, `[]`, `{"a":1,"a":2}`, `{"a":{"x":1},"a":{"y":2}}`, `{"a":[1],"a":[2,3]}`,
	`{"__proto__":1,"constructor":2,"toString":"x"}`, `{"10":1,"2":2,"b":3,"a":4,"1":5}`,
	`{"kéy":"v","世界":["a",{"b":null}]}`, ` {"b":1} `, "\t[1,\n2]\r", `[1,2]x`, `{"a":1,}`, `[1,]`,
	`{"a"}`, `"\x"`, `01`, `NaN`, `[1e999]`, "\xef\xbb\xbf{}", `{"a":"  "}`,
	`[[[[[[[[[[1]]]]]]]]]]`,
	// Repeated names whose earlier values Go reports errors for, which
	// JSON.parse drops, and distinct names that are the same Go string.
	`{"a":1e400,"a":1}`, `{"a":-1e400,"a":1}`, `{"a":{"b":[1e999]},"a":1}`, `[{"a":1e400,"a":2}]`,
	`{"a":1e-400,"a":1}`, `{"\ud800":1,"\udc00":2,"\ud800":3}`, `{"\ud800":1,"\udc00":2}`,
	`{"x\udfff":1,"x\ud800":2,"x\udfff":3}`, `{"a":"x:y","a":1}`, `{"\u003a":1,"a":1,"a":2}`,
	`{"a\"":1e400,"a\"":2}`, `{"\u0022":1e400,"\"":1}`, `["\"",{"a":1e400,"a":"\""}]`,
	`{"x":"a\\","x":1e400,"x":"\\"}`, `{"q":"\\\"","q":[1e400],"q":"\"\\"}`,
	`{"a\"b":"c\"","d":["\"\""]}`, `{"a" : 1e400 , "a" :2}`,
}

func main() {
	deep := strings.Repeat("[", 120) + strings.Repeat("]", 120)
	deepObj := strings.Repeat(`{"a":`, 120) + "1" + strings.Repeat("}", 120)
	// Nesting past Go's limit in a value a repeated name replaces.
	deepDup := `{"a":` + strings.Repeat("[", 10000) + strings.Repeat("]", 10000) + `,"a":1}`
	for _, s := range append(inputs, deep, deepObj, deepDup) {
		var v any
		err := json.Unmarshal([]byte(s), &v)
		fmt.Printf("%.40q any: %s %v\n", s, show(v), err)
		// A []byte variable, and a part of a larger one.
		b := []byte(s)
		var m map[string]any
		err = json.Unmarshal(b, &m)
		fmt.Printf("  map: %s %v\n", show(map[string]any(m)), err)
		buf := []byte("[[" + s + "]]")
		var a []any
		err = json.Unmarshal(buf[2:len(buf)-2], &a)
		fmt.Printf("  slice: %s %v\n", show([]any(a)), err)
	}

	// A map that is not nil keeps its entries, and an interface holding a
	// map decodes into it.
	m := map[string]any{"keep": true, "a": "old"}
	err := json.Unmarshal([]byte(`{"a":1,"b":[true]}`), &m)
	fmt.Println("merge:", show(m), err)
	var v any = map[string]any{"keep": true}
	err = json.Unmarshal([]byte(`{"a":1}`), &v)
	fmt.Println("into:", show(v), err)
	a := []any{"x", "y", "z"}
	err = json.Unmarshal([]byte(`[1]`), &a)
	fmt.Println("reuse:", show(a), len(a), cap(a), err)

	// A named empty interface, and maps and slices of it.
	type anything interface{}
	var n anything
	err = json.Unmarshal([]byte(`{"x":[1,"two",null]}`), &n)
	fmt.Printf("named: %T %s %v\n", n, show(n), err)
	var nm map[string]anything
	err = json.Unmarshal([]byte(`{"x":{"y":1}}`), &nm)
	fmt.Printf("named map: %T %T %v\n", nm, nm["x"], err)

	// A map whose key type has an UnmarshalText method and one whose type
	// has an UnmarshalJSON method, which Go calls.
	var um map[upper]any
	err = json.Unmarshal([]byte(`{"ab":1,"cd":[2]}`), &um)
	fmt.Println("text keys:", um[upper("AB")], um[upper("CD")], len(um), err)
	var mj counted
	err = json.Unmarshal([]byte(`{"a":1,"b":2}`), &mj)
	fmt.Println("method:", mj.n, err)

	// Strings decoded from text with many non-ASCII characters, and the
	// decoded value encoded again.
	long := `{"text":"` + strings.Repeat("こんにちは、世界。", 50) + `","n":[` + strings.Repeat("1.25,", 99) + `1]}`
	err = json.Unmarshal([]byte(long), &v)
	b, err2 := json.Marshal(v)
	fmt.Println("long:", len(v.(map[string]any)["text"].(string)), len(v.(map[string]any)["n"].([]any)), err, len(b), err2, string(b) == long)
}
