package main

import (
	"fmt"
	"math"

	"github.com/goesm-dev/goesm/jscompat"
)

// compat calls package jscompat's engine functions.
func compat() {
	fmt.Println(jscompat.Cos(1) == math.Cos(1), jscompat.Pow(2, 10), jscompat.Atan2(1, 1)*4 == math.Pi)
	fmt.Println(jscompat.Normalize("ｶﾞ①ﬁ", "NFKC"), len(jscompat.Normalize("é", "NFD")))
	fmt.Println(jscompat.Round(-2.5), jscompat.Round(2.5), jscompat.UTF16Len("😀あ"))

	re := jscompat.MustCompileRegExp(`(?<=¥)(\d+)(?!\d*円)`, "")
	fmt.Println(re, re.MatchString("¥1200"), re.MatchString("1200"))
	fmt.Printf("%q\n", re.FindStringSubmatch("値段は¥1200です"))
	fmt.Printf("%q\n", jscompat.MustCompileRegExp(`(a)|(b)`, "").FindStringSubmatch("b"))
	dup := jscompat.MustCompileRegExp(`(\p{L})\1`, "u")
	fmt.Printf("%q %q\n", dup.FindAllString("ああいいう oo", -1), dup.FindAllString("ああいい", 1))
	fmt.Printf("%v %q\n", dup.FindAllString("abc", -1) == nil, jscompat.MustCompileRegExp(`x*`, "").FindAllString("axx", -1))
	fmt.Println(jscompat.MustCompileRegExp(`(?<y>\d{4})-(?<m>\d\d)`, "").ReplaceAllString("2026-10 と 2027-01", "$<m>/$<y>"))
	_, err := jscompat.CompileRegExp(`(`, "")
	fmt.Println(err != nil)
}
