// Command jsoncodec encodes and decodes JSON with encoding/json: struct tags,
// embedded structs, maps, interfaces, custom marshalers, streams and errors.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type Address struct {
	Street string `json:"street"`
	City   string `json:"city,omitempty"`
}

type Status int

func (s Status) MarshalJSON() ([]byte, error) { return json.Marshal(fmt.Sprintf("status-%d", int(s))) }

func (s *Status) UnmarshalJSON(b []byte) error {
	var str string
	if err := json.Unmarshal(b, &str); err != nil {
		return err
	}
	_, err := fmt.Sscanf(str, "status-%d", (*int)(s))
	return err
}

type Level int

func (l Level) MarshalText() ([]byte, error) { return []byte(fmt.Sprintf("L%d", int(l))), nil }

func (l *Level) UnmarshalText(b []byte) error {
	_, err := fmt.Sscanf(string(b), "L%d", (*int)(l))
	return err
}

type Embedded struct {
	Level int `json:"level"`
}

type User struct {
	ID      int64              `json:"id"`
	Big     uint64             `json:"big,string"`
	Name    string             `json:"name"`
	Email   string             `json:"email,omitempty"`
	Tags    []string           `json:"tags"`
	Address *Address           `json:"address,omitempty"`
	Meta    map[string]any     `json:"meta"`
	Scores  map[string]float64 `json:"scores"`
	Status  Status             `json:"status"`
	Levels  map[Level]bool     `json:"levels"`
	Raw     json.RawMessage    `json:"raw"`
	Skip    string             `json:"-"`
	Bytes   []byte             `json:"bytes"`
	Arr     [2]int             `json:"arr"`
	private int
	Embedded
}

func main() {
	u := User{ID: 9007199254740993, Big: 1 << 63, Name: "Ann <&>", Tags: []string{"a", "b"}, Address: &Address{Street: "Main"},
		Meta: map[string]any{"k": 1, "z": []any{true, nil, "s"}}, Scores: map[string]float64{"x": 1.5, "y": 1e21},
		Status: 7, Levels: map[Level]bool{2: true, 1: false}, Raw: json.RawMessage(`{"pre":1}`), Skip: "no",
		Bytes: []byte("hi!"), Arr: [2]int{1, 2}, private: 1, Embedded: Embedded{3}}
	b, err := json.Marshal(u)
	fmt.Println(string(b), err)
	b, _ = json.MarshalIndent(map[string]Status{"s": 7, "a": 1}, "", "  ")
	fmt.Println(string(b))

	var u2 User
	err = json.Unmarshal(b2(), &u2)
	fmt.Println(err, u2.ID, u2.Big, u2.Name, u2.Tags, *u2.Address, u2.Meta["n"], u2.Meta["arr"], u2.Level, u2.Status, u2.Levels, string(u2.Raw), string(u2.Bytes), u2.Arr)

	var anyv any
	json.Unmarshal([]byte(`{"a":[1,2,{"b":null}],"c":"d","e":1e400}`), &anyv)
	fmt.Println(anyv)
	err = json.Unmarshal([]byte(`{"a":[1,2,{"b":null}],"c":"d"}`), &anyv)
	fmt.Println(anyv, err)

	dec := json.NewDecoder(strings.NewReader(`{"x":1} {"x":2} [`))
	for {
		var v struct{ X int }
		if err := dec.Decode(&v); err != nil {
			fmt.Println("end:", err)
			break
		}
		fmt.Println(v.X)
	}
	dec = json.NewDecoder(strings.NewReader(`{"n": 12345678901234567890}`))
	dec.UseNumber()
	var nm map[string]any
	fmt.Println(dec.Decode(&nm), nm["n"])
	dec = json.NewDecoder(strings.NewReader(`{"x":1,"y":2}`))
	dec.DisallowUnknownFields()
	var onlyX struct{ X int }
	fmt.Println(dec.Decode(&onlyX))

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", " ")
	enc.Encode([]any{1, "a<b>", 2.5, nil, map[string]int{}})
	enc = json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	enc.Encode("a<b>")

	fmt.Println(json.Unmarshal([]byte(`{"id":"x"}`), &u2))
	fmt.Println(json.Unmarshal([]byte(`{"id":1`), &u2))
	fmt.Println(json.Unmarshal([]byte(`[1,2,3]`), &u2))
	_, err = json.Marshal(map[string]any{"f": func() {}})
	fmt.Println(err)
	fmt.Println(json.Valid([]byte(`{"a":1}`)), json.Valid([]byte(`{a:1}`)))
	var buf bytes.Buffer
	json.Indent(&buf, []byte(`{"a":[1,2],"b":{}}`), ">", "\t")
	fmt.Println(buf.String())
	buf.Reset()
	json.Compact(&buf, []byte("{ \"a\" : [ 1 , 2 ] }"))
	fmt.Println(buf.String())
	var nums []float64
	fmt.Println(json.Unmarshal([]byte(`[0.1, 1e-7, 123456789012, -0]`), &nums), nums)
	out, _ := json.Marshal(nums)
	fmt.Println(string(out))
	var ptrs struct {
		P *int
		Q **string
		I interface{ M() }
	}
	fmt.Println(json.Unmarshal([]byte(`{"P":5,"Q":"s"}`), &ptrs), *ptrs.P, **ptrs.Q)
	stringResults()

	// Non-ASCII field names: Ünit and Δx are exported, ätsch is not.
	type intl struct {
		Ünit  string
		Δx    int
		ätsch int
	}
	ib, err := json.Marshal(intl{"m", 2, 3})
	fmt.Println(string(ib), err)
	var iv intl
	fmt.Println(json.Unmarshal([]byte(`{"Ünit":"s","Δx":5,"ätsch":6}`), &iv), iv)
}

func b2() []byte {
	return []byte(`{"id":9007199254740993,"big":"18446744073709551615","name":"Bob","tags":["x"],"address":{"street":"S","city":"C"},
	"meta":{"n":1.5,"arr":[1,"two"]},"level":5,"unknown":1,"status":"status-4","levels":{"L3":true},"raw":[1, 2],"bytes":"aGkh","arr":[5,6]}`)
}

// stringResults encodes into []byte locals that are only converted to
// strings or measured, and decodes from string variables: goesm keeps
// those bytes as strings.
func stringResults() {
	enc := func(v any) string {
		b, err := json.Marshal(v)
		return fmt.Sprint(len(b), " ", string(b), " ", err)
	}
	fmt.Println(enc(map[string]any{"k": "héllo\xff<&>", "n": 1.5}))
	fmt.Println(enc(Status(3)), enc(func() {}), enc(nil), enc([]int{1, 2}))
	b, _ := json.Marshal(struct {
		A string
		B []byte
	}{"日本", []byte{0, 255}})
	s := string(b)
	fmt.Println(s, len(b))
	for _, in := range []string{`{"ID":7,"Name":"é\u00e9"}`, `{"ID":"x"}`, "{\"Name\":\"\xff\"}", `[`} {
		var u struct {
			ID   int
			Name string
		}
		err := json.Unmarshal([]byte(in), &u)
		fmt.Printf("%d %q %v\n", u.ID, u.Name, err)
	}
}
