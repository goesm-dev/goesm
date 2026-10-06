// Command regexpjs matches constant patterns, all of which goesm
// translates to RegExps (internal/lower/regexpjs.go), against inputs with
// ASCII, other Unicode and invalid UTF-8, through every method of
// *regexp.Regexp that a RegExp serves.
package main

import (
	"fmt"
	"regexp"
	"strings"
)

var patterns = []*regexp.Regexp{
	regexp.MustCompile(`^(\d{4})-(\d{1,2})-(\d{1,2})$`),
	regexp.MustCompile(`(\d{4})/(\d\d)/(\d\d)`),
	regexp.MustCompile(`^(?:(\d+)h)?(?:(\d+)m)?(?:(\d+)s)?$`),
	regexp.MustCompile(`\d+`),
	regexp.MustCompile(`[a-z]+`),
	regexp.MustCompile(`\w+`),
	regexp.MustCompile(`\s+`),
	regexp.MustCompile(`\S+`),
	regexp.MustCompile(`\D`),
	regexp.MustCompile(`\W`),
	regexp.MustCompile(`.`),
	regexp.MustCompile(`(?s).`),
	regexp.MustCompile(`[^a]`),
	regexp.MustCompile(`[^\n]`),
	regexp.MustCompile(`(?i)k`),
	regexp.MustCompile(`(?i)straße`),
	regexp.MustCompile(`(?i)ǅ`),
	regexp.MustCompile(`(?i)[k-m]+`),
	regexp.MustCompile(`\bfoo\b`),
	regexp.MustCompile(`\Bo\B`),
	regexp.MustCompile(`^`),
	regexp.MustCompile(`$`),
	regexp.MustCompile(`^$`),
	regexp.MustCompile(`(?m)^\w`),
	regexp.MustCompile(`(?m)\w$`),
	regexp.MustCompile(`(?m)^$`),
	regexp.MustCompile(`a|ab`),
	regexp.MustCompile(`ab|a`),
	regexp.MustCompile(`(a)?b`),
	regexp.MustCompile(`(a)|(b)`),
	regexp.MustCompile(`x*`),
	regexp.MustCompile(`x+?`),
	regexp.MustCompile(`x*?`),
	regexp.MustCompile(`a{2,}`),
	regexp.MustCompile(`a{2,3}?`),
	regexp.MustCompile(`(?U)a+`),
	regexp.MustCompile(`日本`),
	regexp.MustCompile(`[日本語]+`),
	regexp.MustCompile(`\x{1F600}`),
	regexp.MustCompile(`[\x{1F600}-\x{1F64F}]`),
	regexp.MustCompile(`\x{FFFD}`),
	regexp.MustCompile(`[^\x00-\x7F]`),
	regexp.MustCompile(`\pL+`),
	regexp.MustCompile(`\p{Greek}+`),
	regexp.MustCompile(`\PN`),
	regexp.MustCompile(`[[:alpha:]]+`),
	regexp.MustCompile(`[[:^space:]]`),
	regexp.MustCompile(`(?P<year>\d{4})-(?P<month>\d\d)`),
	regexp.MustCompile(`^(?P<key>[a-z]+)=(?P<val>[0-9]*)$`),
	regexp.MustCompile(`\$\{([a-z]+)\}`),
	regexp.MustCompile(`\Qa.b\E`),
	regexp.MustCompile(`^[\t ]*#`),
	regexp.MustCompile(`\.`),
	regexp.MustCompile(`[.]`),
	regexp.MustCompile(`\x00`),
	regexp.MustCompile(`\n`),
	regexp.MustCompile(`\r\n`),
	regexp.MustCompile(`^\pL\pM*$`),
	regexp.MustCompile(`é`),
	regexp.MustCompile(`(?i)é`),
	regexp.MustCompile(`ab(cd)?ef`),
	regexp.MustCompile(`(?:foo|bar|baz)`),
	regexp.MustCompile(`fo(?:o|x)`),
	regexp.MustCompile(`\A\d`),
	regexp.MustCompile(`\d\z`),
}

var inputs = []string{
	"",
	"a",
	"ab",
	"abc",
	"b",
	"aab",
	"xxxy",
	"xy",
	"aaaa",
	"foo bar",
	"foobar",
	"2026-10-05",
	"1999-1-2",
	"on 2026/10/05 and 2027/01/02",
	"1h30m",
	"45s",
	"2h",
	"k K K",
	"STRASSE straße STRAßE",
	"ǆ Ǆ ǅ",
	"日本語のテキスト 日本",
	"smile 😀 😃!",
	"café CAFÉ",
	"a\xffb",
	"\xe2\x82",
	"\xed\xa0\x80x",
	"\xf4\x90\x80\x80",
	"�",
	"line1\x0aline2\x0a\x0aline4",
	"a\x0d\x0ab",
	" \x09#comment",
	"x y",
	"１２3",
	"${name} and ${other}",
	"key=123",
	"a.b axb",
	"Ω ωmega Ελλάδα",
	"\x00\x01",
	"é",
	"foo foo",
	"zoo boo",
}

func main() {
	for _, re := range patterns {
		prefix, complete := re.LiteralPrefix()
		fmt.Printf("== %q subexp=%d names=%q prefix=%q %v\n", re.String(), re.NumSubexp(), re.SubexpNames(), prefix, complete)
		for _, s := range inputs {
			b := []byte(s)
			fmt.Printf("%q: %v %v %q %v\n", s, re.MatchString(s), re.Match(b), re.FindString(s), re.FindStringIndex(s))
			fmt.Printf("  sub %q %v\n", re.FindStringSubmatch(s), re.FindSubmatchIndex(b))
			fmt.Printf("  all %q %v\n", re.FindAllString(s, -1), re.FindAllIndex(b, 3))
			fmt.Printf("  allsub %v\n", re.FindAllStringSubmatchIndex(s, -1))
			fmt.Printf("  repl %q %q\n", re.ReplaceAllString(s, "<$0|${1}>"), re.ReplaceAll(b, []byte("[$1]")))
			fmt.Printf("  func %q lit %q\n", re.ReplaceAllStringFunc(s, strings.ToUpper), re.ReplaceAllLiteralString(s, "$"))
			fmt.Printf("  split %q %q\n", re.Split(s, -1), re.Split(s, 2))
		}
		if i := re.SubexpIndex("year"); i >= 0 {
			fmt.Println("year at", i)
		}
	}
	fmt.Println(regexp.MustCompile(`^\d+$`).MatchString("12345"), regexp.MustCompile(`^(\w+)@(\w+)\.com$`).ReplaceAllString("bob@example.com", "$2:$1"))
	ok, err := regexp.MatchString(`^[a-z]+\[[0-9]+\]$`, "adam[23]")
	fmt.Println(ok, err)
	ok, err = regexp.Match(`^\pL+$`, []byte("Ελλάδα"))
	fmt.Println(ok, err)
	re, err := regexp.Compile(`(?i)go+gle`)
	fmt.Println(re.FindAllString("Google GOOOGLE gogle ggle", -1), err)
	src := []byte("a1b22c333")
	re = regexp.MustCompile(`[0-9]+`)
	fmt.Printf("%s\n", re.ReplaceAllFunc(src, func(m []byte) []byte { return []byte(fmt.Sprint(len(m))) }))
	tmpl := regexp.MustCompile(`^(?P<key>\w+):\s*(?P<value>\w+)$`)
	var out []byte
	for _, m := range tmpl.FindAllStringSubmatchIndex("option: value", -1) {
		out = tmpl.ExpandString(out, "$key=$value", "option: value", m)
	}
	fmt.Println(string(out))
	for i, m := range re.FindAllString("1 22 333 4444", 2) {
		fmt.Println(i, m)
	}
	fmt.Println(re.Copy().String(), regexp.QuoteMeta("a.b*c"))
}
