// Package markdown renders CommonMark to HTML with goldmark, as a
// documentation site renders its pages. js/impl/markdown-it.mjs does the
// same with markdown-it (VitePress's renderer) and js/impl/remark.mjs with
// remark and rehype (Astro's).
package markdown

import (
	"bytes"

	"github.com/yuin/goldmark"
)

var md = goldmark.New()

// Render returns the HTML of the CommonMark document src.
func Render(src string) (string, error) {
	var b bytes.Buffer
	if err := md.Convert([]byte(src), &b); err != nil {
		return "", err
	}
	return b.String(), nil
}
