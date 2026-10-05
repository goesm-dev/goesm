// Package render renders the product list page of ../../render with
// functions that write HTML to a strings.Builder, one per component, the
// way templ-generated code or a hand-written Go renderer does, instead of
// with html/template, which interprets the template through reflect at run
// time. js/impl/render.mjs renders the same page with React.
package render

import (
	"html"
	"strconv"
	"strings"
)

// Item is one product of the list.
type Item struct {
	ID      int      `json:"id"`
	Name    string   `json:"name"`
	Price   int      `json:"price"`
	Tags    []string `json:"tags"`
	SoldOut bool     `json:"soldOut"`
}

// Render renders the page for items.
func Render(title string, items []Item) string {
	var b strings.Builder
	page(&b, title, items)
	return b.String()
}

func page(b *strings.Builder, title string, items []Item) {
	b.WriteString(`<main class="page"><h1>`)
	b.WriteString(html.EscapeString(title))
	b.WriteString(`</h1><p class="count">`)
	b.WriteString(strconv.Itoa(len(items)))
	b.WriteString(` items</p><ul class="items">`)
	for i := range items {
		product(b, &items[i])
	}
	b.WriteString(`</ul></main>`)
}

func product(b *strings.Builder, it *Item) {
	b.WriteString(`<li class="item`)
	if it.SoldOut {
		b.WriteString(` sold-out`)
	}
	b.WriteString(`" data-id="`)
	b.WriteString(strconv.Itoa(it.ID))
	b.WriteString(`"><h2>`)
	b.WriteString(html.EscapeString(it.Name))
	b.WriteString(`</h2><span class="price">`)
	b.WriteString(yen(it.Price))
	b.WriteString(`</span>`)
	if len(it.Tags) > 0 {
		b.WriteString(`<ul class="tags">`)
		for _, t := range it.Tags {
			b.WriteString(`<li>`)
			b.WriteString(html.EscapeString(t))
			b.WriteString(`</li>`)
		}
		b.WriteString(`</ul>`)
	}
	if it.SoldOut {
		b.WriteString(`<em>Sold out</em>`)
	}
	b.WriteString(`</li>`)
}

// yen formats a price with thousands separators.
func yen(n int) string {
	s := strconv.Itoa(n)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return "¥" + b.String()
}
