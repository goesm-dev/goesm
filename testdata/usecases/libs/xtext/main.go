// Command xtext exercises a popular pure-Go library; TestUseCaseLibraries
// compares its output under goesm with native Go.
package main

import (
	"fmt"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"golang.org/x/text/unicode/norm"
	"golang.org/x/text/width"
)

func main() {
	s := "é" // e + combining acute
	fmt.Printf("%q %q %v %v\n", norm.NFC.String(s), norm.NFD.String("é"), norm.NFC.IsNormalString(s), len(norm.NFKC.String("ﬁ①")))
	fmt.Println(norm.NFKC.String("ｱｲｳ１２３"), width.Narrow.String("ＡＢＣ"), width.Widen.String("abc"))
	for _, t := range []string{"en-US", "ja-JP", "zh-Hant-TW", "sr-Latn", "und", "xx-!!"} {
		tag, err := language.Parse(t)
		b, conf := tag.Base()
		r, _ := tag.Region()
		fmt.Println(t, tag, b, conf, r, err)
	}
	m := language.NewMatcher([]language.Tag{language.English, language.Japanese, language.MustParse("fr-CA")})
	tags, q, _ := language.ParseAcceptLanguage("fr-FR,fr;q=0.9,ja;q=0.8")
	tag, idx, conf := m.Match(tags...)
	fmt.Println(tags, q, tag, idx, conf)
	fmt.Println(cases.Title(language.English).String("hello wORLD"), cases.Upper(language.Turkish).String("istanbul"), cases.Fold().String("Straße"))
}
