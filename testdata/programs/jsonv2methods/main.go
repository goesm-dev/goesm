// Command jsonv2methods has a type with json v2's own methods
// (MarshalJSONTo, UnmarshalJSONFrom), which only json v2 calls: goesm
// keeps json v2 for json.Marshal and json.Unmarshal in such a program
// instead of encoding/json's legacy engine.
package main

import (
	"encoding/json"
	"encoding/json/jsontext"
	"fmt"
	"strings"
)

// Upper is written in upper case and read in any case.
type Upper string

func (u Upper) MarshalJSONTo(enc *jsontext.Encoder) error {
	return enc.WriteToken(jsontext.String(strings.ToUpper(string(u))))
}

func (u *Upper) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	tok, err := dec.ReadToken()
	if err != nil {
		return err
	}
	if tok.Kind() != '"' {
		return fmt.Errorf("Upper: not a string")
	}
	*u = Upper(strings.ToUpper(tok.String()))
	return nil
}

type Doc struct {
	Name Upper   `json:"name"`
	Tags []Upper `json:"tags,omitempty"`
	N    int     `json:"n"`
}

func main() {
	b, err := json.Marshal(Doc{Name: "go", Tags: []Upper{"a", "b"}, N: 1})
	fmt.Printf("%s %v\n", b, err)
	var d Doc
	err = json.Unmarshal([]byte(`{"name":"esm","tags":["x"],"n":2}`), &d)
	fmt.Printf("%+v %v\n", d, err)
	err = json.Unmarshal([]byte(`{"name":1,"n":"x"}`), &d)
	fmt.Printf("%+v %v\n", d, err)
	err = json.Unmarshal([]byte(`{"name":`), &d)
	fmt.Printf("%v\n", err)
}
