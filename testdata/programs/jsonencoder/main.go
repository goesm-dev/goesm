// json.Encoder's Encode, with and without HTML escaping and indentation,
// for values the runtime encodes and values json v2 encodes.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

type item struct {
	Name  string            `json:"name"`
	Note  string            `json:"note,omitempty"`
	Tags  []string          `json:"tags"`
	Attrs map[string]string `json:"attrs,omitempty"`
	Count int64             `json:"count"`
	Ratio float64           `json:"ratio"`
	Next  *item             `json:"next,omitempty"`
	Flag  *bool             `json:"flag,omitempty"`
}

type stamp int

func (s stamp) MarshalJSON() ([]byte, error) { return []byte(fmt.Sprintf(`"<%d>"`, int(s))), nil }

type failing struct{}

func (failing) Write(p []byte) (int, error) { return 0, errors.New("write failed") }

func main() {
	yes := true
	values := []any{
		item{Name: "a <b> & c", Tags: []string{"<x>", `\u003c`, `\\u003e`, `\\\u0026`, "&amp;"}, Count: 1 << 60, Ratio: 0.1},
		item{Name: "line sep ", Note: "日本語 🙂", Attrs: map[string]string{"z<": "1", "a&": "2"}, Next: &item{Name: "n>"}, Flag: &yes},
		map[string]any{"html": "</script>", "n": 3, "list": []any{1, "<", nil, true}},
		"plain <string>",
		"bad \xff utf8 <",
		[]stamp{1, 2},
		nil,
		[]int(nil),
		struct{ C chan int }{},
	}
	for _, escape := range []bool{true, false} {
		for _, indent := range []string{"", "  "} {
			var b bytes.Buffer
			enc := json.NewEncoder(&b)
			enc.SetEscapeHTML(escape)
			if indent != "" {
				enc.SetIndent(">", indent)
			}
			for _, v := range values {
				if err := enc.Encode(v); err != nil {
					fmt.Fprintf(&b, "error: %v\n", err)
				}
			}
			fmt.Printf("escape=%v indent=%q\n%s", escape, indent, b.String())
		}
	}
	enc := json.NewEncoder(failing{})
	fmt.Println(enc.Encode(1), enc.Encode(2))
	out := json.NewEncoder(os.Stdout)
	out.SetEscapeHTML(false)
	if err := out.Encode(map[string]string{"k": strings.Repeat("<&>", 3)}); err != nil {
		fmt.Println(err)
	}
}
