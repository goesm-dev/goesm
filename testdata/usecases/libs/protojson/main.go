// Command protojson decodes protobuf JSON with strings that do
// and do not need escaping, in ASCII and in other scripts, and reports the
// errors of invalid input. goesm patches the decoder's scan for the end of
// a string (internal/natives/patch/google.golang.org/protobuf).
package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"
)

func main() {
	inputs := []string{
		`["plain", "with \"quotes\" and \\ backslash", "tab\tescapedé", "日本語の文字列", "emoji 🎉 and 🎉"]`,
		`{"a": "� literal replacement", "b": "\u0000 nul", "c": ["nested", {"d": "x"}]}`,
		"[\"raw \xef\xbf\xbd replacement\"]",
		"[\"invalid \xff byte\"]",
		"[\"control \x01 char\"]",
		`["unterminated`,
		"[\"\xe3\x81\x82\xe3\x81\"]",
	}
	var long strings.Builder
	long.WriteString("[")
	for i := range 2000 {
		if i > 0 {
			long.WriteString(",")
		}
		fmt.Fprintf(&long, `"item %d テキスト"`, i)
	}
	long.WriteString("]")
	inputs = append(inputs, long.String())
	for _, in := range inputs {
		var v structpb.Value
		if err := protojson.Unmarshal([]byte(in), &v); err != nil {
			// protobuf's errors also put a space or a no-break space
			// after "proto:" by the hash of the binary.
			fmt.Println("error:", strings.ReplaceAll(err.Error(), "\u00a0", " "))
			continue
		}
		// protojson.Marshal's spacing depends on a hash of the binary
		// (internal/detrand), so the decoded value is printed with
		// encoding/json.
		out, err := json.Marshal(v.AsInterface())
		if err != nil {
			fmt.Println("marshal error:", err)
			continue
		}
		if len(out) > 200 {
			fmt.Println(len(out), string(out[:100]), string(out[len(out)-100:]))
			continue
		}
		fmt.Println(string(out))
	}
}
