package utility

import (
	"strings"
)

// Theme options, as @theme's keywords set them.
const (
	optInline = 1 << iota
	optReference
	optDefault
	optStatic
	optUsed
)

type themeEntry struct {
	key, value string
	opts       int
}

// A theme is the design tokens of @theme blocks, in their order.
type theme struct {
	entries []*themeEntry
	index   map[string]*themeEntry
}

// parseTheme reads the @theme blocks of a stylesheet. Other rules, and the
// @keyframes inside @theme, are skipped.
func parseTheme(css string) *theme {
	t := &theme{index: map[string]*themeEntry{}}
	p := cssScanner{s: css}
	for {
		p.space()
		if p.done() {
			break
		}
		if !strings.HasPrefix(p.s[p.i:], "@theme") {
			p.skipStatement()
			continue
		}
		p.i += len("@theme")
		open := strings.IndexByte(p.s[p.i:], '{')
		if open < 0 {
			break
		}
		opts := 0
		for _, w := range strings.Fields(p.s[p.i : p.i+open]) {
			switch w {
			case "inline":
				opts |= optInline
			case "reference":
				opts |= optReference
			case "default":
				opts |= optDefault
			case "static":
				opts |= optStatic
			}
		}
		p.i += open + 1
		for {
			p.space()
			if p.done() {
				break
			}
			if p.s[p.i] == '}' {
				p.i++
				break
			}
			if p.s[p.i] != '-' {
				p.skipStatement()
				continue
			}
			colon := strings.IndexByte(p.s[p.i:], ':')
			if colon < 0 {
				p.i = len(p.s)
				break
			}
			key := strings.TrimSpace(p.s[p.i : p.i+colon])
			p.i += colon + 1
			start := p.i
			p.valueEnd()
			value := strings.TrimSpace(p.s[start:p.i])
			if p.i < len(p.s) && p.s[p.i] == ';' {
				p.i++
			}
			t.add(key, value, opts)
		}
	}
	return t
}

func (t *theme) add(key, value string, opts int) {
	if strings.HasSuffix(key, "-*") {
		ns := key[:len(key)-2]
		kept := t.entries[:0]
		for _, e := range t.entries {
			if ns == "-" || strings.HasPrefix(e.key, ns) && !ignoredKey(e.key, ns) {
				delete(t.index, e.key)
				continue
			}
			kept = append(kept, e)
		}
		t.entries = kept
		return
	}
	if e := t.index[key]; e != nil {
		if opts&optDefault != 0 && e.opts&optDefault == 0 {
			return
		}
		if value == "initial" {
			t.remove(key)
			return
		}
		e.value, e.opts = value, opts
		return
	}
	if value == "initial" {
		return
	}
	e := &themeEntry{key: key, value: value, opts: opts}
	t.entries = append(t.entries, e)
	t.index[key] = e
}

func (t *theme) remove(key string) {
	for i, e := range t.entries {
		if e.key == key {
			t.entries = append(t.entries[:i], t.entries[i+1:]...)
			break
		}
	}
	delete(t.index, key)
}

// ignoredKeys are keys under a namespace that belong to another one:
// --font-weight-bold is not a --font.
var ignoredKeys = map[string][]string{
	"--font":        {"--font-weight", "--font-size"},
	"--inset":       {"--inset-shadow", "--inset-ring"},
	"--text":        {"--text-color", "--text-decoration-color", "--text-decoration-thickness", "--text-indent", "--text-shadow", "--text-underline-offset"},
	"--grid-column": {"--grid-column-start", "--grid-column-end"},
	"--grid-row":    {"--grid-row-start", "--grid-row-end"},
}

func ignoredKey(key, namespace string) bool {
	for _, k := range ignoredKeys[namespace] {
		if key == k || strings.HasPrefix(key, k+"-") {
			return true
		}
	}
	return false
}

// resolveKey finds the key for a candidate's value under the first of
// namespaces that has it. A value of "" with none set looks the namespace
// itself up.
func (t *theme) resolveKey(value string, hasValue bool, namespaces []string) *themeEntry {
	for _, ns := range namespaces {
		key := ns
		if hasValue {
			key = ns + "-" + value
		}
		e := t.index[key]
		if e == nil && hasValue && strings.Contains(value, ".") {
			key = ns + "-" + strings.ReplaceAll(value, ".", "_")
			e = t.index[key]
		}
		if e == nil || ignoredKey(key, ns) {
			continue
		}
		return e
	}
	return nil
}

func (e *themeEntry) varRef() string {
	if e.opts&optReference != 0 {
		return "var(" + escapeClass(e.key) + ", " + e.value + ")"
	}
	return "var(" + escapeClass(e.key) + ")"
}

// resolve returns var(--key) for a value under namespaces, or the value
// itself for an inline theme.
func (t *theme) resolve(value string, hasValue bool, namespaces ...string) (string, bool) {
	e := t.resolveKey(value, hasValue, namespaces)
	if e == nil {
		return "", false
	}
	if e.opts&optInline != 0 {
		return e.value, true
	}
	return e.varRef(), true
}

// resolveValue returns the value of a key, never a var().
func (t *theme) resolveValue(value string, hasValue bool, namespaces ...string) (string, bool) {
	e := t.resolveKey(value, hasValue, namespaces)
	if e == nil {
		return "", false
	}
	return e.value, true
}

// resolveWith resolves a value and the nested keys that go with it, like
// --text-sm--line-height for --text-sm.
func (t *theme) resolveWith(value string, namespaces []string, nested []string) (string, []string, bool) {
	e := t.resolveKey(value, true, namespaces)
	if e == nil {
		return "", nil, false
	}
	extra := make([]string, len(nested))
	for i, n := range nested {
		ne := t.index[e.key+n]
		if ne == nil {
			continue
		}
		if ne.opts&optInline != 0 {
			extra[i] = ne.value
		} else {
			extra[i] = ne.varRef()
		}
	}
	if e.opts&optInline != 0 {
		return e.value, extra, true
	}
	return e.varRef(), extra, true
}

func (t *theme) get(keys ...string) (string, bool) {
	for _, k := range keys {
		if e := t.index[k]; e != nil {
			return e.value, true
		}
	}
	return "", false
}

// namespace returns the keys under ns, without the namespace, and their
// values, in theme order.
func (t *theme) namespace(ns string) (keys, values []string) {
	prefix := ns + "-"
	for _, e := range t.entries {
		if strings.HasPrefix(e.key, prefix) {
			keys = append(keys, e.key[len(prefix):])
			values = append(values, e.value)
		}
	}
	return keys, values
}

// A cssScanner walks a stylesheet's top level.
type cssScanner struct {
	s string
	i int
}

func (p *cssScanner) done() bool { return p.i >= len(p.s) }

// space skips whitespace and comments.
func (p *cssScanner) space() {
	for p.i < len(p.s) {
		c := p.s[p.i]
		if c == ' ' || c == '\n' || c == '\t' || c == '\r' || c == '\f' {
			p.i++
			continue
		}
		if strings.HasPrefix(p.s[p.i:], "/*") {
			end := strings.Index(p.s[p.i+2:], "*/")
			if end < 0 {
				p.i = len(p.s)
				return
			}
			p.i += end + 4
			continue
		}
		return
	}
}

// skipStatement skips an at-rule statement, a rule or a declaration, with
// its block.
func (p *cssScanner) skipStatement() {
	depth := 0
	for p.i < len(p.s) {
		c := p.s[p.i]
		switch c {
		case '"', '\'':
			p.skipString(c)
			continue
		case '{':
			depth++
		case '}':
			if depth == 0 {
				return
			}
			depth--
			if depth == 0 {
				p.i++
				return
			}
		case ';':
			if depth == 0 {
				p.i++
				return
			}
		}
		p.i++
	}
}

func (p *cssScanner) skipString(q byte) {
	for p.i++; p.i < len(p.s); p.i++ {
		if p.s[p.i] == '\\' {
			p.i++
		} else if p.s[p.i] == q {
			p.i++
			return
		}
	}
}

// valueEnd moves to the ; or } that ends a declaration's value.
func (p *cssScanner) valueEnd() {
	depth := 0
	for p.i < len(p.s) {
		c := p.s[p.i]
		switch c {
		case '"', '\'':
			p.skipString(c)
			continue
		case '(', '[':
			depth++
		case ')', ']':
			depth--
		case ';', '}':
			if depth <= 0 {
				return
			}
		}
		p.i++
	}
}
