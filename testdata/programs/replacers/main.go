// strings.Replacer, which goesm runs as one RegExp.
package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	cases := [][]string{{}, {"a", "1"}, {"a", "1", "b", "2"}, {"a", "12", "b", ""}, {"", "X"}, {"", "X", "a", "1"}, {"a", "1", "", "X"},
		{"ab", "Z"}, {"ab", "Z", "a", "Y"}, {"a", "Y", "ab", "Z"}, {"aaa", "3", "aa", "2", "a", "1"}, {"a", "1", "a", "2"},
		{".", "dot", "*", "star", "(", "p", "[", "b", "^", "c", "$", "d", `\`, "e", "-", "f", "]", "g", "/", "h"},
		{"&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;"}, {"日本", "にほん", "本", "ほん"}, {"é", "e"},
		{"", "", "a", ""}, {"aa", "b"}, {"ab", "ba", "ba", "ab"}, {"a", "1", "", "X", "b", "2"}, {"", "X", "ab", "Z", "a", "Y"},
		{"b", "2", "", "X", "a", "1"}, {"", "X", "", "Y"}, {"x", "", "", "-"}, {"\xff", "?"}}
	inputs := []string{"", "a", "aaaa", "abc", "ababab", "baab", `x.y*z(1)[2]^$\-]/`, `<a href="x">&'</a>`, "日本語の本", "été", "aaaaab", "ba", "abba", "bab", "a\xffb"}
	for _, c := range cases {
		r := strings.NewReplacer(c...)
		for _, in := range inputs {
			fmt.Printf("%q ", r.Replace(in))
		}
		n, err := r.WriteString(os.Stdout, "abc")
		fmt.Println("", n, err)
	}
}
