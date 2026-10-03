// Package textstats uses the standard library (strings, strconv, sort,
// unicode, errors) compiled from its Go source.
package textstats

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// TopWords returns the n most frequent words of text as "word:count", most
// frequent first, ties in alphabetical order.
func TopWords(text string, n int) []string {
	counts := map[string]int{}
	for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && r != '\''
	}) {
		counts[w]++
	}
	words := make([]string, 0, len(counts))
	for w := range counts {
		words = append(words, w)
	}
	sort.Slice(words, func(i, j int) bool {
		if counts[words[i]] != counts[words[j]] {
			return counts[words[i]] > counts[words[j]]
		}
		return words[i] < words[j]
	})
	if len(words) > n {
		words = words[:n]
	}
	out := make([]string, len(words))
	for i, w := range words {
		out[i] = w + ":" + strconv.Itoa(counts[w])
	}
	return out
}

// Slug turns a title into a URL path segment. Letters and digits of any
// script are kept.
func Slug(title string) string {
	fields := strings.FieldsFunc(strings.ToLower(title), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	return strings.Join(fields, "-")
}

// ErrEmpty is returned by SumCSV for empty input.
var ErrEmpty = errors.New("empty input")

// SumCSV adds up comma-separated integers.
func SumCSV(s string) (int, error) {
	if strings.TrimSpace(s) == "" {
		return 0, ErrEmpty
	}
	sum := 0
	for _, f := range strings.Split(s, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(f))
		if err != nil {
			return 0, errors.Join(errors.New("bad number"), err)
		}
		sum += n
	}
	return sum, nil
}

// IsEmpty reports whether err is ErrEmpty (errors.Is works across wrapping).
func IsEmpty(err error) bool { return errors.Is(err, ErrEmpty) }
