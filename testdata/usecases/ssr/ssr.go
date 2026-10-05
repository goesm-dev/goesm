// Package ssr renders HTML on the server with html/template, for a JS
// framework's server side to call.
package ssr

import (
	"embed"
	"html/template"
	"strings"
)

//go:embed tmpl/*.html
var files embed.FS

var tmpl = template.Must(template.ParseFS(files, "tmpl/*.html"))

type Item struct {
	Name string
	URL  string
	Done bool
}

type Page struct {
	Title string
	Items []Item
	Note  string
}

func Render(p Page) (string, error) {
	var b strings.Builder
	err := tmpl.ExecuteTemplate(&b, "page", p)
	return b.String(), err
}

func Demo() string {
	s, err := Render(Page{Title: "<Tasks & co>", Note: "a \"quoted\" note", Items: []Item{{"one", "/a?x=1&y=2", true}, {"<b>two</b>", "javascript:alert(1)", false}}})
	if err != nil {
		return err.Error()
	}
	return s
}
