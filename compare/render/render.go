// Package render renders a product list page to HTML with html/template,
// the way a Go server renders pages. js/impl/render.mjs renders the same
// page with React's renderToString.
package render

import (
	"html/template"
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

type page struct {
	Title string
	Items []Item
	Count int
}

var tmpl = template.Must(template.New("page").Funcs(template.FuncMap{"yen": yen}).Parse(
	`<main class="page"><h1>{{.Title}}</h1><p class="count">{{.Count}} items</p><ul class="items">` +
		`{{range .Items}}<li class="item{{if .SoldOut}} sold-out{{end}}" data-id="{{.ID}}"><h2>{{.Name}}</h2>` +
		`<span class="price">{{yen .Price}}</span>{{if .Tags}}<ul class="tags">{{range .Tags}}<li>{{.}}</li>{{end}}</ul>{{end}}` +
		`{{if .SoldOut}}<em>Sold out</em>{{end}}</li>{{end}}</ul></main>`))

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

// Render renders the page for items.
func Render(title string, items []Item) (string, error) {
	var b strings.Builder
	err := tmpl.Execute(&b, page{Title: title, Items: items, Count: len(items)})
	return b.String(), err
}
