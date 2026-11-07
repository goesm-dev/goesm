// Command jsonerrassert asserts the concrete types of the errors of
// decoding JSON, without fmt or reflect: goesm's runtime decodes these calls
// and must return errors of encoding/json's own types.
package main

import (
	"encoding/json"
	"errors"
	"os"
	"strconv"
)

func main() {
	var i int
	err := json.Unmarshal([]byte(`"x"`), &i)
	_, ok := err.(*json.UnmarshalTypeError)
	os.Stdout.WriteString("type error: " + strconv.FormatBool(ok) + "\n")

	var s struct{ A int }
	err = json.Unmarshal([]byte(`{"A":`), &s)
	var syn *json.SyntaxError
	if errors.As(err, &syn) {
		os.Stdout.WriteString("syntax error at " + strconv.FormatInt(syn.Offset, 10) + "\n")
	} else {
		os.Stdout.WriteString("syntax error: false\n")
	}
}
