package utility

import (
	"strings"
)

var statics map[string][]node

// staticUtilities returns the static utilities of tables.go, read once.
func staticUtilities() map[string][]node {
	if statics == nil {
		statics = map[string][]node{}
		for _, line := range strings.Split(staticTable, "\n") {
			name, body, _ := strings.Cut(line, "|")
			statics[name] = parseStaticNodes(body)
		}
	}
	return statics
}

func parseStaticNodes(s string) []node {
	var nodes []node
	for _, item := range segment(s, ';') {
		switch {
		case item == "":
		case item[0] == '@':
			f := segment(item[1:], ',')
			nodes = append(nodes, node{kind: 'p', property: newProperty(f[0], f[1], f[2])})
		case item[0] == '&':
			open := strings.IndexByte(item, '{')
			nodes = append(nodes, node{kind: 'r', prop: item[1:open], nodes: parseStaticNodes(item[open+1 : len(item)-1])})
		default:
			k, v, _ := strings.Cut(item, ":")
			nodes = append(nodes, decl(k, v))
		}
	}
	return nodes
}

func props(ps ...*property) []node {
	nodes := make([]node, len(ps))
	for i, p := range ps {
		nodes[i] = node{kind: 'p', property: p}
	}
	return nodes
}

func rule(selector string, nodes ...node) node { return node{kind: 'r', prop: selector, nodes: nodes} }

func join(parts ...[]node) []node {
	var out []node
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// declIf is a declaration that is left out when value is empty, as
// Tailwind leaves out declarations without a value.
func declIf(prop, value string) []node {
	if value == "" {
		return nil
	}
	return []node{decl(prop, value)}
}

// A functionalDesc describes a utility that takes a value, as Tailwind's
// functionalUtility.
type functionalDesc struct {
	negative, fractions bool
	themeKeys           []string
	defaultValue        *string // the value without one, or nil to resolve themeKeys
	noDefault           bool    // no rule without a value
	bare                func(v *utilityValue) (string, bool)
	negativeBare        func(v *utilityValue) (string, bool)
	statics             map[string][]node
	handle              func(value, dataType string) []node
}

func (b *builder) add(root string, fn func(c *candidate) []node) {
	b.utilities[root] = append(b.utilities[root], utility{fn: fn})
}

func (b *builder) functional(root string, d functionalDesc) {
	handler := func(negative bool) func(c *candidate) []node {
		return func(c *candidate) []node {
			var value, dataType string
			ok := false
			switch {
			case c.value == nil:
				if c.modifier != nil || d.noDefault {
					return nil
				}
				if d.defaultValue != nil {
					value, ok = *d.defaultValue, true
				} else {
					value, ok = b.theme.resolve("", false, d.themeKeys...)
				}
			case c.value.arbitrary:
				if c.modifier != nil {
					return nil
				}
				value, ok, dataType = c.value.value, true, c.value.dataType
			default:
				key := c.value.value
				if c.value.fraction != "" {
					key = c.value.fraction
				}
				value, ok = b.theme.resolve(key, true, d.themeKeys...)
				if !ok && d.fractions && c.value.fraction != "" {
					f := segment(c.value.fraction, '/')
					if !isPositiveInteger(f[0]) || !isPositiveInteger(f[1]) {
						return nil
					}
					value, ok = "calc("+f[0]+" / "+f[1]+" * 100%)", true
				}
				if !ok && negative && d.negativeBare != nil {
					value, ok = d.negativeBare(c.value)
					if !strings.Contains(value, "/") && c.modifier != nil {
						return nil
					}
					if ok {
						return d.handle(value, "")
					}
				}
				if !ok && d.bare != nil {
					value, ok = d.bare(c.value)
					if !strings.Contains(value, "/") && c.modifier != nil {
						return nil
					}
				}
				if !ok && !negative && d.statics != nil && c.modifier == nil {
					if s, has := d.statics[c.value.value]; has {
						return s
					}
				}
			}
			if !ok {
				return nil
			}
			if negative {
				value = spaceMathOperators("calc(" + value + " * -1)")
			}
			return d.handle(value, dataType)
		}
	}
	if d.negative {
		b.add("-"+root, handler(true))
	}
	b.add(root, handler(false))
}

func bareInteger(v *utilityValue) (string, bool) { return v.value, isPositiveInteger(v.value) }

func barePixels(v *utilityValue) (string, bool) {
	return v.value + "px", isPositiveInteger(v.value)
}

// spacing registers a utility whose bare values are multiples of the
// theme's --spacing.
func (b *builder) spacing(root string, keys []string, handle func(string) []node, negative, fractions bool, statics map[string][]node) {
	b.functional(root, functionalDesc{
		negative: negative, fractions: fractions, themeKeys: keys, noDefault: true, statics: statics,
		bare: func(v *utilityValue) (string, bool) {
			if _, ok := b.theme.resolve("", false, "--spacing"); !ok || !isValidSpacingMultiplier(v.value) {
				return "", false
			}
			return "--spacing(" + v.value + ")", true
		},
		negativeBare: func(v *utilityValue) (string, bool) {
			if _, ok := b.theme.resolve("", false, "--spacing"); !ok || !isValidSpacingMultiplier(v.value) {
				return "", false
			}
			return "--spacing(-" + v.value + ")", true
		},
		handle: func(value, _ string) []node { return handle(value) },
	})
}

// withAlpha mixes a color with transparent for an opacity.
func withAlpha(value, alpha string) string {
	if n, ok := jsNumber(alpha); ok {
		alpha = jsString(n*100) + "%"
	}
	if alpha == "100%" {
		return value
	}
	return "color-mix(in oklab, " + value + " " + alpha + ", transparent)"
}

func replaceAlpha(value, alpha string) string {
	if n, ok := jsNumber(alpha); ok {
		alpha = jsString(n*100) + "%"
	}
	return "oklab(from " + value + " l a b / " + alpha + ")"
}

// asColor applies a candidate's opacity modifier to a color.
func (b *builder) asColor(value string, m *modifier) (string, bool) {
	if m == nil {
		return value, true
	}
	if m.arbitrary {
		return withAlpha(value, m.value), true
	}
	if alpha, ok := b.theme.resolve(m.value, true, "--opacity"); ok {
		return withAlpha(value, alpha), true
	}
	if !isValidOpacityValue(m.value) {
		return "", false
	}
	return withAlpha(value, m.value+"%"), true
}

func (b *builder) themeColor(c *candidate, keys ...string) (string, bool) {
	var value string
	ok := true
	switch c.value.value {
	case "inherit":
		value = "inherit"
	case "transparent":
		value = "transparent"
	case "current":
		value = "currentcolor"
	default:
		value, ok = b.theme.resolve(c.value.value, true, keys...)
	}
	if !ok {
		return "", false
	}
	return b.asColor(value, c.modifier)
}

// color registers a utility for a color, with an opacity modifier.
func (b *builder) color(root string, keys []string, handle func(string) []node) {
	b.add(root, func(c *candidate) []node {
		if c.value == nil {
			return nil
		}
		var v string
		var ok bool
		if c.value.arbitrary {
			v, ok = b.asColor(c.value.value, c.modifier)
		} else {
			v, ok = b.themeColor(c, keys...)
		}
		if !ok {
			return nil
		}
		return handle(v)
	})
}

func str(s string) *string { return &s }

func sides(pairs ...string) [][2]string {
	out := make([][2]string, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		out = append(out, [2]string{pairs[i], pairs[i+1]})
	}
	return out
}

func one(prop string) func(string) []node {
	return func(v string) []node { return []node{decl(prop, v)} }
}

const (
	filterValue         = "var(--tw-blur,) var(--tw-brightness,) var(--tw-contrast,) var(--tw-grayscale,) var(--tw-hue-rotate,) var(--tw-invert,) var(--tw-saturate,) var(--tw-sepia,) var(--tw-drop-shadow,)"
	backdropFilterValue = "var(--tw-backdrop-blur,) var(--tw-backdrop-brightness,) var(--tw-backdrop-contrast,) var(--tw-backdrop-grayscale,) var(--tw-backdrop-hue-rotate,) var(--tw-backdrop-invert,) var(--tw-backdrop-opacity,) var(--tw-backdrop-saturate,) var(--tw-backdrop-sepia,)"
	boxShadowValue      = "var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)"
	nullShadow          = "0 0 #0000"
	transitionDefault   = "color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events"
	ringOffsetShadow    = "var(--tw-ring-inset,) 0 0 0 var(--tw-ring-offset-width) var(--tw-ring-offset-color)"
)

func filterProperties() []node {
	return props(newProperty("--tw-blur", "", ""), newProperty("--tw-brightness", "", ""), newProperty("--tw-contrast", "", ""),
		newProperty("--tw-grayscale", "", ""), newProperty("--tw-hue-rotate", "", ""), newProperty("--tw-invert", "", ""),
		newProperty("--tw-opacity", "", ""), newProperty("--tw-saturate", "", ""), newProperty("--tw-sepia", "", ""),
		newProperty("--tw-drop-shadow", "", ""), newProperty("--tw-drop-shadow-color", "", ""),
		newProperty("--tw-drop-shadow-alpha", "100%", "<percentage>"), newProperty("--tw-drop-shadow-size", "", ""))
}

func backdropFilterProperties() []node {
	return props(newProperty("--tw-backdrop-blur", "", ""), newProperty("--tw-backdrop-brightness", "", ""),
		newProperty("--tw-backdrop-contrast", "", ""), newProperty("--tw-backdrop-grayscale", "", ""),
		newProperty("--tw-backdrop-hue-rotate", "", ""), newProperty("--tw-backdrop-invert", "", ""),
		newProperty("--tw-backdrop-opacity", "", ""), newProperty("--tw-backdrop-saturate", "", ""),
		newProperty("--tw-backdrop-sepia", "", ""))
}

func boxShadowProperties() []node {
	return props(newProperty("--tw-shadow", nullShadow, ""), newProperty("--tw-shadow-color", "", ""),
		newProperty("--tw-shadow-alpha", "100%", "<percentage>"), newProperty("--tw-inset-shadow", nullShadow, ""),
		newProperty("--tw-inset-shadow-color", "", ""), newProperty("--tw-inset-shadow-alpha", "100%", "<percentage>"),
		newProperty("--tw-ring-color", "", ""), newProperty("--tw-ring-shadow", nullShadow, ""),
		newProperty("--tw-inset-ring-color", "", ""), newProperty("--tw-inset-ring-shadow", nullShadow, ""),
		newProperty("--tw-ring-inset", "", ""), newProperty("--tw-ring-offset-width", "0px", "<length>"),
		newProperty("--tw-ring-offset-color", "#fff", ""), newProperty("--tw-ring-offset-shadow", nullShadow, ""))
}

func translateProperties() []node {
	return props(newProperty("--tw-translate-x", "0", ""), newProperty("--tw-translate-y", "0", ""), newProperty("--tw-translate-z", "0", ""))
}

func (b *builder) registerUtilities() map[string][]utility {
	b.utilities = map[string][]utility{}

	for _, s := range sides("inset", "inset", "inset-x", "inset-inline", "inset-y", "inset-block", "inset-s", "inset-inline-start",
		"inset-e", "inset-inline-end", "inset-bs", "inset-block-start", "inset-be", "inset-block-end",
		"top", "top", "right", "right", "bottom", "bottom", "left", "left") {
		b.spacing(s[0], []string{"--inset", "--spacing"}, one(s[1]), true, true, nil)
	}
	b.functional("z", functionalDesc{negative: true, bare: bareInteger, themeKeys: []string{"--z-index"},
		handle: func(v, _ string) []node { return []node{decl("z-index", v)} }, statics: map[string][]node{"auto": {decl("z-index", "auto")}}})
	b.functional("order", functionalDesc{negative: true, bare: bareInteger, themeKeys: []string{"--order"},
		handle:  func(v, _ string) []node { return []node{decl("order", v)} },
		statics: map[string][]node{"first": {decl("order", "-9999")}, "last": {decl("order", "9999")}}})
	for _, axis := range sides("col", "grid-column", "row", "grid-row") {
		prop := axis[1]
		b.functional(axis[0], functionalDesc{negative: true, bare: bareInteger, themeKeys: []string{"--" + prop},
			handle: func(v, _ string) []node { return []node{decl(prop, v)} }, statics: map[string][]node{"auto": {decl(prop, "auto")}}})
		b.functional(axis[0]+"-span", functionalDesc{bare: bareInteger,
			handle:  func(v, _ string) []node { return []node{decl(prop, "span "+v+" / span "+v)} },
			statics: map[string][]node{"full": {decl(prop, "1 / -1")}}})
		for _, edge := range []string{"start", "end"} {
			p := prop + "-" + edge
			b.functional(axis[0]+"-"+edge, functionalDesc{negative: true, bare: bareInteger, themeKeys: []string{"--" + p},
				handle: func(v, _ string) []node { return []node{decl(p, v)} }, statics: map[string][]node{"auto": {decl(p, "auto")}}})
		}
	}
	for _, s := range sides("m", "margin", "mx", "margin-inline", "my", "margin-block", "ms", "margin-inline-start",
		"me", "margin-inline-end", "mbs", "margin-block-start", "mbe", "margin-block-end",
		"mt", "margin-top", "mr", "margin-right", "mb", "margin-bottom", "ml", "margin-left") {
		b.spacing(s[0], []string{"--margin", "--spacing"}, one(s[1]), true, false, nil)
	}
	b.functional("line-clamp", functionalDesc{themeKeys: []string{"--line-clamp"}, bare: bareInteger,
		handle: func(v, _ string) []node {
			return []node{decl("overflow", "hidden"), decl("display", "-webkit-box"), decl("-webkit-box-orient", "vertical"), decl("-webkit-line-clamp", v)}
		},
		statics: map[string][]node{"none": {decl("overflow", "visible"), decl("display", "block"), decl("-webkit-box-orient", "horizontal"), decl("-webkit-line-clamp", "unset")}}})
	b.functional("aspect", functionalDesc{themeKeys: []string{"--aspect"},
		bare: func(v *utilityValue) (string, bool) {
			if v.fraction == "" {
				return "", false
			}
			f := segment(v.fraction, '/')
			return v.fraction, isValidSpacingMultiplier(f[0]) && isValidSpacingMultiplier(f[1])
		},
		handle:  func(v, _ string) []node { return []node{decl("aspect-ratio", v)} },
		statics: map[string][]node{"auto": {decl("aspect-ratio", "auto")}, "square": {decl("aspect-ratio", "1 / 1")}}})
	b.spacing("size", []string{"--size", "--spacing"}, func(v string) []node {
		return []node{decl("--tw-sort", "size"), decl("width", v), decl("height", v)}
	}, false, true, nil)
	for _, s := range [][3]string{
		{"w", "width", "--width --spacing --container"}, {"min-w", "min-width", "--min-width --spacing --container"},
		{"max-w", "max-width", "--max-width --spacing --container"}, {"h", "height", "--height --spacing"},
		{"min-h", "min-height", "--min-height --height --spacing"}, {"max-h", "max-height", "--max-height --height --spacing"},
		{"inline", "inline-size", "--spacing --container"}, {"min-inline", "min-inline-size", "--spacing --container"},
		{"max-inline", "max-inline-size", "--spacing --container"}, {"block", "block-size", "--spacing"},
		{"min-block", "min-block-size", "--spacing"}, {"max-block", "max-block-size", "--spacing"},
	} {
		b.spacing(s[0], strings.Fields(s[2]), one(s[1]), false, true, nil)
	}

	b.add("flex", func(c *candidate) []node {
		switch {
		case c.value == nil:
			return nil
		case c.value.arbitrary:
			if c.modifier != nil {
				return nil
			}
			return []node{decl("flex", c.value.value)}
		case c.value.fraction != "":
			f := segment(c.value.fraction, '/')
			if !isPositiveInteger(f[0]) || !isPositiveInteger(f[1]) {
				return nil
			}
			return []node{decl("flex", "calc("+c.value.fraction+" * 100%)")}
		case isPositiveInteger(c.value.value):
			if c.modifier != nil {
				return nil
			}
			return []node{decl("flex", c.value.value)}
		}
		return nil
	})
	b.functional("shrink", functionalDesc{defaultValue: str("1"), bare: bareInteger, handle: func(v, _ string) []node { return []node{decl("flex-shrink", v)} }})
	b.functional("grow", functionalDesc{defaultValue: str("1"), bare: bareInteger, handle: func(v, _ string) []node { return []node{decl("flex-grow", v)} }})
	b.spacing("basis", []string{"--flex-basis", "--spacing", "--container"}, one("flex-basis"), false, true, nil)

	for _, s := range sides("grid-cols", "grid-template-columns", "grid-rows", "grid-template-rows") {
		prop := s[1]
		b.functional(s[0], functionalDesc{themeKeys: []string{"--" + prop},
			bare: func(v *utilityValue) (string, bool) {
				return "repeat(" + v.value + ", minmax(0, 1fr))", isStrictPositiveInteger(v.value)
			},
			handle:  func(v, _ string) []node { return []node{decl(prop, v)} },
			statics: map[string][]node{"none": {decl(prop, "none")}, "subgrid": {decl(prop, "subgrid")}}})
	}
	b.spacing("gap", []string{"--gap", "--spacing"}, one("gap"), false, false, nil)
	b.spacing("gap-x", []string{"--gap", "--spacing"}, one("column-gap"), false, false, nil)
	b.spacing("gap-y", []string{"--gap", "--spacing"}, one("row-gap"), false, false, nil)
	for _, s := range [][4]string{{"space-x", "row-gap", "inline", "x"}, {"space-y", "column-gap", "block", "y"}} {
		sortBy, dir, axis := s[1], s[2], s[3]
		b.spacing(s[0], []string{"--space", "--spacing"}, func(v string) []node {
			zero := v == "--spacing(0)" || v == "--spacing(-0)"
			if n, unit, ok := dimension(v); ok && n == 0 && (unit == "" || isLength(v)) {
				zero = true
			}
			start, end := "calc("+v+" * var(--tw-space-"+axis+"-reverse))", "calc("+v+" * calc(1 - var(--tw-space-"+axis+"-reverse)))"
			if zero {
				start, end = "0", "0"
			}
			return []node{
				{kind: 'p', property: newProperty("--tw-space-"+axis+"-reverse", "0", "")},
				rule(":where(& > :not(:last-child))", decl("--tw-sort", sortBy), decl("--tw-space-"+axis+"-reverse", "0"),
					decl("margin-"+dir+"-start", start), decl("margin-"+dir+"-end", end)),
			}
		}, true, false, nil)
	}
	b.color("accent", []string{"--accent-color", "--color"}, one("accent-color"))
	b.color("caret", []string{"--caret-color", "--color"}, one("caret-color"))
	b.color("divide", []string{"--divide-color", "--border-color", "--color"}, func(v string) []node {
		return []node{rule(":where(& > :not(:last-child))", decl("--tw-sort", "divide-color"), decl("border-color", v))}
	})
	b.color("placeholder", []string{"--placeholder-color", "--color"}, func(v string) []node {
		return []node{rule("&::placeholder", decl("--tw-sort", "placeholder-color"), decl("color", v))}
	})
	b.color("fill", []string{"--fill", "--color"}, one("fill"))

	for _, r := range [][]string{
		{"rounded", "border-radius"}, {"rounded-s", "border-start-start-radius", "border-end-start-radius"},
		{"rounded-e", "border-start-end-radius", "border-end-end-radius"}, {"rounded-t", "border-top-left-radius", "border-top-right-radius"},
		{"rounded-r", "border-top-right-radius", "border-bottom-right-radius"}, {"rounded-b", "border-bottom-right-radius", "border-bottom-left-radius"},
		{"rounded-l", "border-top-left-radius", "border-bottom-left-radius"}, {"rounded-ss", "border-start-start-radius"},
		{"rounded-se", "border-start-end-radius"}, {"rounded-ee", "border-end-end-radius"}, {"rounded-es", "border-end-start-radius"},
		{"rounded-tl", "border-top-left-radius"}, {"rounded-tr", "border-top-right-radius"},
		{"rounded-br", "border-bottom-right-radius"}, {"rounded-bl", "border-bottom-left-radius"},
	} {
		properties := r[1:]
		all := func(v string) []node {
			out := make([]node, len(properties))
			for i, p := range properties {
				out[i] = decl(p, v)
			}
			return out
		}
		b.functional(r[0], functionalDesc{themeKeys: []string{"--radius"}, handle: func(v, _ string) []node { return all(v) },
			statics: map[string][]node{"none": all("0"), "full": all("calc(infinity * 1px)")}})
	}

	borderProperties := func() []node { return props(newProperty("--tw-border-style", "solid", "")) }
	for _, s := range sides("border", "border", "border-x", "border-inline", "border-y", "border-block",
		"border-s", "border-inline-start", "border-e", "border-inline-end", "border-bs", "border-block-start",
		"border-be", "border-block-end", "border-t", "border-top", "border-r", "border-right",
		"border-b", "border-bottom", "border-l", "border-left") {
		prefix := s[1]
		width := func(v string) []node {
			return join(borderProperties(), []node{decl(prefix+"-style", "var(--tw-border-style)"), decl(prefix+"-width", v)})
		}
		b.add(s[0], func(c *candidate) []node {
			if c.value == nil {
				if c.modifier != nil {
					return nil
				}
				v, ok := b.theme.get("--default-border-width")
				if !ok {
					v = "1px"
				}
				return width(v)
			}
			if c.value.arbitrary {
				v := c.value.value
				t := c.value.dataType
				if t == "" {
					t = inferDataType(v, "color", "line-width", "length")
				}
				if t == "line-width" || t == "length" {
					if c.modifier != nil {
						return nil
					}
					return width(v)
				}
				v, ok := b.asColor(v, c.modifier)
				if !ok {
					return nil
				}
				return []node{decl(prefix+"-color", v)}
			}
			if v, ok := b.themeColor(c, "--border-color", "--color"); ok {
				return []node{decl(prefix+"-color", v)}
			}
			if c.modifier != nil {
				return nil
			}
			if v, ok := b.theme.resolve(c.value.value, true, "--border-width"); ok {
				return width(v)
			}
			if isPositiveInteger(c.value.value) {
				return width(c.value.value + "px")
			}
			return nil
		})
	}

	b.add("bg", func(c *candidate) []node {
		if c.value == nil {
			return nil
		}
		if c.value.arbitrary {
			v := c.value.value
			t := c.value.dataType
			if t == "" {
				t = inferDataType(v, "image", "color", "percentage", "position", "bg-size", "length", "url")
			}
			switch t {
			case "percentage", "position":
				if c.modifier != nil {
					return nil
				}
				return []node{decl("background-position", v)}
			case "bg-size", "length", "size":
				if c.modifier != nil {
					return nil
				}
				return []node{decl("background-size", v)}
			case "image", "url":
				if c.modifier != nil {
					return nil
				}
				return []node{decl("background-image", v)}
			}
			v, ok := b.asColor(v, c.modifier)
			if !ok {
				return nil
			}
			return []node{decl("background-color", v)}
		}
		if v, ok := b.themeColor(c, "--background-color", "--color"); ok {
			return []node{decl("background-color", v)}
		}
		if c.modifier != nil {
			return nil
		}
		if v, ok := b.theme.resolve(c.value.value, true, "--background-image"); ok {
			return []node{decl("background-image", v)}
		}
		return nil
	})

	for _, s := range sides("p", "padding", "px", "padding-inline", "py", "padding-block", "ps", "padding-inline-start",
		"pe", "padding-inline-end", "pbs", "padding-block-start", "pbe", "padding-block-end",
		"pt", "padding-top", "pr", "padding-right", "pb", "padding-bottom", "pl", "padding-left") {
		b.spacing(s[0], []string{"--padding", "--spacing"}, one(s[1]), false, false, nil)
	}
	b.spacing("indent", []string{"--text-indent", "--spacing"}, one("text-indent"), true, false, nil)

	fontWeight := func(v string) []node {
		return join(props(newProperty("--tw-font-weight", "", "")), []node{decl("--tw-font-weight", v), decl("font-weight", v)})
	}
	b.add("font", func(c *candidate) []node {
		if c.value == nil || c.modifier != nil {
			return nil
		}
		if c.value.arbitrary {
			v := c.value.value
			t := c.value.dataType
			if t == "" {
				t = inferDataType(v, "number", "generic-name", "family-name")
			}
			if t == "generic-name" || t == "family-name" {
				return []node{decl("font-family", v)}
			}
			return fontWeight(v)
		}
		if v, extra, ok := b.theme.resolveWith(c.value.value, []string{"--font"}, []string{"--font-feature-settings", "--font-variation-settings"}); ok {
			return join([]node{decl("font-family", v)}, declIf("font-feature-settings", extra[0]), declIf("font-variation-settings", extra[1]))
		}
		if v, ok := b.theme.resolve(c.value.value, true, "--font-weight"); ok {
			return fontWeight(v)
		}
		return nil
	})

	lineHeightModifier := func(c *candidate) (string, bool) {
		m := c.modifier
		if m.arbitrary {
			return m.value, true
		}
		if v, ok := b.theme.resolve(m.value, true, "--leading"); ok {
			return v, true
		}
		if isValidSpacingMultiplier(m.value) {
			if _, ok := b.theme.resolve("", false, "--spacing"); !ok {
				return "", false
			}
			return "--spacing(" + m.value + ")", true
		}
		if m.value == "none" {
			return "1", true
		}
		return "", false
	}
	b.add("text", func(c *candidate) []node {
		if c.value == nil {
			return nil
		}
		if c.value.arbitrary {
			v := c.value.value
			t := c.value.dataType
			if t == "" {
				t = inferDataType(v, "color", "length", "percentage", "absolute-size", "relative-size")
			}
			switch t {
			case "size", "length", "percentage", "absolute-size", "relative-size":
				if c.modifier != nil {
					lh, ok := lineHeightModifier(c)
					if !ok {
						return nil
					}
					return []node{decl("font-size", v), decl("line-height", lh)}
				}
				return []node{decl("font-size", v)}
			}
			v, ok := b.asColor(v, c.modifier)
			if !ok {
				return nil
			}
			return []node{decl("color", v)}
		}
		if v, ok := b.themeColor(c, "--text-color", "--color"); ok {
			return []node{decl("color", v)}
		}
		size, extra, ok := b.theme.resolveWith(c.value.value, []string{"--text"}, []string{"--line-height", "--letter-spacing", "--font-weight"})
		if !ok {
			return nil
		}
		if c.modifier != nil {
			lh, ok := lineHeightModifier(c)
			if !ok {
				return nil
			}
			return []node{decl("font-size", size), decl("line-height", lh)}
		}
		out := []node{decl("font-size", size)}
		if extra[0] != "" {
			out = append(out, decl("line-height", "var(--tw-leading, "+extra[0]+")"))
		}
		if extra[1] != "" {
			out = append(out, decl("letter-spacing", "var(--tw-tracking, "+extra[1]+")"))
		}
		if extra[2] != "" {
			out = append(out, decl("font-weight", "var(--tw-font-weight, "+extra[2]+")"))
		}
		return out
	})

	leading := func(v string) []node {
		return join(props(newProperty("--tw-leading", "", "")), []node{decl("--tw-leading", v), decl("line-height", v)})
	}
	b.spacing("leading", []string{"--leading", "--spacing"}, leading, false, false, map[string][]node{"none": leading("1")})
	b.functional("tracking", functionalDesc{negative: true, themeKeys: []string{"--tracking"}, handle: func(v, _ string) []node {
		return join(props(newProperty("--tw-tracking", "", "")), []node{decl("--tw-tracking", v), decl("letter-spacing", v)})
	}})
	b.functional("opacity", functionalDesc{themeKeys: []string{"--opacity"},
		bare:   func(v *utilityValue) (string, bool) { return v.value + "%", isValidOpacityValue(v.value) },
		handle: func(v, _ string) []node { return []node{decl("opacity", v)} }})
	b.functional("underline-offset", functionalDesc{negative: true, themeKeys: []string{"--text-underline-offset"}, bare: barePixels,
		handle:  func(v, _ string) []node { return []node{decl("text-underline-offset", v)} },
		statics: map[string][]node{"auto": {decl("text-underline-offset", "auto")}}})

	outlineWidth := func(v string) []node {
		return join(props(newProperty("--tw-outline-style", "solid", "")), []node{decl("outline-style", "var(--tw-outline-style)"), decl("outline-width", v)})
	}
	b.add("outline", func(c *candidate) []node {
		if c.value == nil {
			if c.modifier != nil {
				return nil
			}
			v, ok := b.theme.get("--default-outline-width")
			if !ok {
				v = "1px"
			}
			return outlineWidth(v)
		}
		if c.value.arbitrary {
			v := c.value.value
			t := c.value.dataType
			if t == "" {
				t = inferDataType(v, "color", "length", "number", "percentage")
			}
			if t == "length" || t == "number" || t == "percentage" {
				if c.modifier != nil {
					return nil
				}
				return outlineWidth(v)
			}
			v, ok := b.asColor(v, c.modifier)
			if !ok {
				return nil
			}
			return []node{decl("outline-color", v)}
		}
		if v, ok := b.themeColor(c, "--outline-color", "--color"); ok {
			return []node{decl("outline-color", v)}
		}
		if c.modifier != nil {
			return nil
		}
		if v, ok := b.theme.resolve(c.value.value, true, "--outline-width"); ok {
			return outlineWidth(v)
		}
		if isPositiveInteger(c.value.value) {
			return outlineWidth(c.value.value + "px")
		}
		return nil
	})
	b.functional("outline-offset", functionalDesc{negative: true, themeKeys: []string{"--outline-offset"}, bare: barePixels,
		handle: func(v, _ string) []node { return []node{decl("outline-offset", v)} }})

	b.registerShadows()

	filter := func(v string) []node {
		return join(filterProperties(), []node{decl("--tw-blur", v), decl("filter", filterValue)})
	}
	b.functional("blur", functionalDesc{themeKeys: []string{"--blur"}, handle: func(v, _ string) []node { return filter("blur(" + v + ")") },
		statics: map[string][]node{"none": filter(" ")}})
	backdrop := func(v string) []node {
		return join(backdropFilterProperties(), []node{decl("--tw-backdrop-blur", v), decl("-webkit-backdrop-filter", backdropFilterValue), decl("backdrop-filter", backdropFilterValue)})
	}
	b.functional("backdrop-blur", functionalDesc{themeKeys: []string{"--backdrop-blur", "--blur"}, handle: func(v, _ string) []node { return backdrop("blur(" + v + ")") },
		statics: map[string][]node{"none": backdrop(" ")}})

	timing := "var(--tw-ease, ease)"
	if v, ok := b.theme.resolve("", false, "--default-transition-timing-function"); ok {
		timing = "var(--tw-ease, " + v + ")"
	}
	duration := "var(--tw-duration, 0s)"
	if v, ok := b.theme.resolve("", false, "--default-transition-duration"); ok {
		duration = "var(--tw-duration, " + v + ")"
	}
	transition := func(v string) []node {
		return []node{decl("transition-property", v), decl("transition-timing-function", timing), decl("transition-duration", duration)}
	}
	b.functional("transition", functionalDesc{defaultValue: str(transitionDefault), themeKeys: []string{"--transition-property"},
		handle: func(v, _ string) []node { return transition(v) },
		statics: map[string][]node{
			"none":      {decl("transition-property", "none")},
			"all":       transition("all"),
			"colors":    transition("color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to"),
			"opacity":   transition("opacity"),
			"shadow":    transition("box-shadow"),
			"transform": transition("transform, translate, scale, rotate"),
		}})
	b.functional("delay", functionalDesc{themeKeys: []string{"--transition-delay"},
		bare:   func(v *utilityValue) (string, bool) { return v.value + "ms", isPositiveInteger(v.value) },
		handle: func(v, _ string) []node { return []node{decl("transition-delay", v)} }})
	b.add("duration", func(c *candidate) []node {
		if c.modifier != nil || c.value == nil {
			return nil
		}
		var v string
		if c.value.arbitrary {
			v = c.value.value
		} else {
			key := c.value.value
			if c.value.fraction != "" {
				key = c.value.fraction
			}
			var ok bool
			if v, ok = b.theme.resolve(key, true, "--transition-duration"); !ok {
				if !isPositiveInteger(c.value.value) {
					return nil
				}
				v = c.value.value + "ms"
			}
		}
		return join(props(newProperty("--tw-duration", "", "")), []node{decl("--tw-duration", v), decl("transition-duration", v)})
	})
	ease := func(v string) []node {
		return join(props(newProperty("--tw-ease", "", "")), []node{decl("--tw-ease", v), decl("transition-timing-function", v)})
	}
	b.functional("ease", functionalDesc{themeKeys: []string{"--ease"}, handle: func(v, _ string) []node { return ease(v) },
		statics: map[string][]node{
			"initial": join(props(newProperty("--tw-ease", "", "")), []node{decl("--tw-ease", "initial")}),
			"linear":  ease("linear"),
		}})

	b.spacing("translate", []string{"--translate", "--spacing"}, func(v string) []node {
		return join(translateProperties(), []node{decl("--tw-translate-x", v), decl("--tw-translate-y", v), decl("translate", "var(--tw-translate-x) var(--tw-translate-y)")})
	}, true, true, nil)
	for _, axis := range []string{"x", "y"} {
		prop := "--tw-translate-" + axis
		b.spacing("translate-"+axis, []string{"--translate", "--spacing"}, func(v string) []node {
			return join(translateProperties(), []node{decl(prop, v), decl("translate", "var(--tw-translate-x) var(--tw-translate-y)")})
		}, true, true, nil)
	}
	return b.utilities
}

// registerShadows registers shadow, ring and ring-offset.
func (b *builder) registerShadows() {
	b.add("shadow", func(c *candidate) []node {
		alpha := ""
		if m := c.modifier; m != nil {
			if m.arbitrary {
				alpha = m.value
			} else if isValidOpacityValue(m.value) {
				alpha = m.value + "%"
			}
		}
		sized := func(v string) []node {
			return join(boxShadowProperties(), declIf("--tw-shadow-alpha", alpha),
				alphaShadow("--tw-shadow", v, alpha, func(color string) string { return "var(--tw-shadow-color, " + color + ")" }),
				[]node{decl("box-shadow", boxShadowValue)})
		}
		colored := func(v string) []node {
			return join(boxShadowProperties(), []node{decl("--tw-shadow-color", withAlpha(v, "var(--tw-shadow-alpha)"))})
		}
		if c.value == nil {
			v, ok := b.theme.get("--shadow")
			if !ok {
				return nil
			}
			return sized(v)
		}
		if c.value.arbitrary {
			v := c.value.value
			t := c.value.dataType
			if t == "" {
				t = inferDataType(v, "color")
			}
			if t == "color" {
				v, ok := b.asColor(v, c.modifier)
				if !ok {
					return nil
				}
				return colored(v)
			}
			return sized(v)
		}
		switch c.value.value {
		case "none":
			if c.modifier != nil {
				return nil
			}
			return join(boxShadowProperties(), []node{decl("--tw-shadow", nullShadow), decl("box-shadow", boxShadowValue)})
		case "inherit":
			if c.modifier != nil {
				return nil
			}
			return join(boxShadowProperties(), []node{decl("--tw-shadow-color", "inherit")})
		}
		if v, ok := b.theme.get("--shadow-" + c.value.value); ok {
			return sized(v)
		}
		if v, ok := b.themeColor(c, "--box-shadow-color", "--color"); ok {
			return colored(v)
		}
		return nil
	})

	ringColor, ok := b.theme.get("--default-ring-color")
	if !ok {
		ringColor = "currentcolor"
	}
	ring := func(v string) []node {
		return join(boxShadowProperties(), []node{
			decl("--tw-ring-shadow", "var(--tw-ring-inset,) 0 0 0 calc("+v+" + var(--tw-ring-offset-width)) var(--tw-ring-color, "+ringColor+")"),
			decl("box-shadow", boxShadowValue),
		})
	}
	b.add("ring", func(c *candidate) []node {
		if c.value == nil {
			if c.modifier != nil {
				return nil
			}
			v, ok := b.theme.get("--default-ring-width")
			if !ok {
				v = "1px"
			}
			return ring(v)
		}
		if c.value.arbitrary {
			v := c.value.value
			t := c.value.dataType
			if t == "" {
				t = inferDataType(v, "color", "length")
			}
			if t == "length" {
				if c.modifier != nil {
					return nil
				}
				return ring(v)
			}
			v, ok := b.asColor(v, c.modifier)
			if !ok {
				return nil
			}
			return []node{decl("--tw-ring-color", v)}
		}
		if v, ok := b.themeColor(c, "--ring-color", "--color"); ok {
			return []node{decl("--tw-ring-color", v)}
		}
		if c.modifier != nil {
			return nil
		}
		v, ok := b.theme.resolve(c.value.value, true, "--ring-width")
		if !ok && isPositiveInteger(c.value.value) {
			v, ok = c.value.value+"px", true
		}
		if !ok {
			return nil
		}
		return ring(v)
	})

	offset := func(v string) []node {
		return []node{decl("--tw-ring-offset-width", v), decl("--tw-ring-offset-shadow", ringOffsetShadow)}
	}
	b.add("ring-offset", func(c *candidate) []node {
		if c.value == nil {
			return nil
		}
		if c.value.arbitrary {
			v := c.value.value
			t := c.value.dataType
			if t == "" {
				t = inferDataType(v, "color", "length")
			}
			if t == "length" {
				if c.modifier != nil {
					return nil
				}
				return offset(v)
			}
			v, ok := b.asColor(v, c.modifier)
			if !ok {
				return nil
			}
			return []node{decl("--tw-ring-offset-color", v)}
		}
		if v, ok := b.theme.resolve(c.value.value, true, "--ring-offset-width"); ok {
			if c.modifier != nil {
				return nil
			}
			return offset(v)
		}
		if isPositiveInteger(c.value.value) {
			if c.modifier != nil {
				return nil
			}
			return offset(c.value.value + "px")
		}
		if v, ok := b.themeColor(c, "--ring-offset-color", "--color"); ok {
			return []node{decl("--tw-ring-offset-color", v)}
		}
		return nil
	})
}

// alphaShadow sets a shadow property to value with its colors wrapped by
// inject, and given alpha.
func alphaShadow(prop, value, alpha string, inject func(string) string) []node {
	fallback := false
	replaced := replaceShadowColors(value, func(color string) string {
		if alpha == "" {
			return inject(color)
		}
		if strings.HasPrefix(color, "current") {
			return inject(withAlpha(color, alpha))
		}
		if strings.HasPrefix(color, "var(") || strings.HasPrefix(alpha, "var(") {
			fallback = true
		}
		return inject(replaceAlpha(color, alpha))
	})
	if fallback {
		return []node{decl(prop, replaceShadowColors(value, inject)), rule("@supports (color: lab(from red l a b))", decl(prop, replaced))}
	}
	return []node{decl(prop, replaced)}
}

// replaceShadowColors replaces the color of each shadow in a list.
func replaceShadowColors(input string, replace func(string) string) string {
	shadows := segment(input, ',')
	for i, shadow := range shadows {
		shadow = strings.TrimSpace(shadow)
		ast := parseValue(shadow)
		var unknown *value
		unknowns, lengths := 0, 0
		replaced := false
		for j, n := range ast {
			switch n.kind {
			case 'w':
				lower := strings.ToLower(n.text)
				if lower == "inset" || lower == "inherit" || lower == "initial" || lower == "revert" || lower == "unset" {
					continue
				}
				if isLengthWord(lower) {
					lengths++
					continue
				}
				if n.text[0] == '#' || isNamedColor(n.text) {
					ast[j] = &value{kind: 'w', text: replace(n.text)}
					replaced = true
				} else {
					unknown = n
					unknowns++
				}
			case 'f':
				fn := strings.ToLower(n.text)
				if isShadowColorFunction(fn) {
					ast[j] = &value{kind: 'w', text: replace(valueString([]*value{n}))}
					replaced = true
				} else if fn == "calc" || fn == "clamp" || fn == "max" || fn == "min" || fn == "--spacing" {
					lengths++
				} else {
					unknown = n
					unknowns++
				}
			}
			if replaced {
				break
			}
		}
		switch {
		case replaced:
			shadows[i] = valueString(ast)
		case lengths < 2:
			shadows[i] = shadow
		case unknowns == 0:
			shadows[i] = shadow + " " + replace("currentcolor")
		case unknowns == 1:
			for j, n := range ast {
				if n == unknown {
					ast[j] = &value{kind: 'w', text: replace(valueString([]*value{n}))}
				}
			}
			shadows[i] = valueString(ast)
		default:
			shadows[i] = shadow
		}
	}
	return strings.Join(shadows, ", ")
}

func isLengthWord(s string) bool {
	i := 0
	if i < len(s) && s[i] == '-' {
		i++
	}
	if i < len(s) && isDigit(s[i]) {
		return true
	}
	return i+1 < len(s) && s[i] == '.' && isDigit(s[i+1])
}

func isShadowColorFunction(fn string) bool {
	switch fn {
	case "color", "color-mix", "contrast-color", "device-cmyk", "hsl", "hsla", "hwb", "lab", "lch", "light-dark", "oklab", "oklch", "rgb", "rgba", "--alpha":
		return true
	}
	return false
}
