// String constants with non-ASCII text, which goesm writes as text.
package main

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const greeting = "こんにちは、世界"

var messages = map[string]string{
	"save":  "保存しました。",
	"fail":  "読み込みに失敗しました。",
	"emoji": "絵文字 😀 と結合文字 é",
	"mixed": "ASCII と日本語",
}

func label(n int) string {
	if n > 1 {
		return "複数の項目"
	}
	return "1 つの項目"
}

func main() {
	fmt.Println(greeting, len(greeting), utf8.RuneCountInString(greeting))
	for _, k := range []string{"save", "fail", "emoji", "mixed"} {
		m := messages[k]
		fmt.Printf("%s %q %d %d\n", m, m, len(m), utf8.RuneCountInString(m))
	}
	fmt.Println(label(1), label(2), strings.Contains(label(2), "項目"), greeting[:3] == "こ")
	fmt.Println([]byte("日本")[0], strings.Index("ドキュメント", "メ"))
	fmt.Printf("%s:%d\n", "件数", 3)
}
