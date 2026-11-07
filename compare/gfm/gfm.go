// Package gfm renders GitHub Flavored Markdown to HTML with goldmark and
// the extensions of GFM (tables, strikethrough, task lists, autolinks).
// js/impl/marked.mjs does the same with marked.
package gfm

import (
	"bytes"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

// Table cells say their alignment with the align attribute, as marked
// writes them.
var md = goldmark.New(goldmark.WithExtensions(
	extension.Linkify,
	extension.NewTable(extension.WithTableCellAlignMethod(extension.TableCellAlignAttribute)),
	extension.Strikethrough,
	extension.TaskList,
))

// Render returns the HTML of the GFM document src.
func Render(src string) (string, error) {
	var b bytes.Buffer
	if err := md.Convert([]byte(src), &b); err != nil {
		return "", err
	}
	return b.String(), nil
}
