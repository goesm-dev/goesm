// Package utility generates utility-first CSS the way a page uses
// Tailwind CSS for: given a theme of design tokens and the class names a
// page uses, it writes the stylesheet with a rule for each class it knows,
// sorted and grouped the same way. js/impl/tailwind.mjs is the same with
// Tailwind CSS, whose output this matches.
//
// It has Tailwind's static utilities and variants, from tables generated
// from Tailwind (tables.go), its common functional utilities (spacing,
// sizing, colors, typography, borders, shadows, filters, transitions;
// utilities.go) and arbitrary values and properties. Group, peer and other
// compound variants are not supported.
package utility

import (
	"slices"
	"strings"
)

// A node is a declaration, a nested rule, or an @property rule for the
// stylesheet's root.
type node struct {
	kind      byte // 'd' declaration, 'r' rule (or at-rule, when it starts with @), 'p' @property
	prop      string
	value     string
	important bool
	nodes     []node    // a rule's
	property  *property // an @property's
}

func decl(prop, value string) node { return node{kind: 'd', prop: prop, value: value} }

// A property is an @property rule.
type property struct {
	name    string
	decls   [][2]string
	initial string
}

func newProperty(name, initial, syntax string) *property {
	p := &property{name: name, initial: "initial"}
	if syntax == "" {
		syntax = "*"
	}
	p.decls = append(p.decls, [2]string{"syntax", `"` + syntax + `"`}, [2]string{"inherits", "false"})
	if initial != "" {
		p.decls = append(p.decls, [2]string{"initial-value", initial})
		p.initial = initial
	}
	return p
}

// Build returns the stylesheet for candidates, the class names a page uses,
// with the design tokens of themeCSS, a stylesheet of @theme blocks.
// Candidates it does not know are left out.
func Build(themeCSS string, candidates []string) string {
	b := newBuilder(parseTheme(themeCSS))
	return b.build(candidates)
}

type builder struct {
	theme     *theme
	utilities map[string][]utility
	variants  map[string]*variant // the theme's breakpoints
	used      map[string]bool
	vars      map[string][]string // the variables of values seen
}

func newBuilder(t *theme) *builder {
	b := &builder{theme: t, used: map[string]bool{}, vars: map[string][]string{}}
	b.utilities = b.registerUtilities()
	b.variants = b.breakpoints()
	return b
}

// A utility compiles a candidate to its nodes, or nil when the candidate
// is not one of its.
type utility struct {
	fn func(c *candidate) []node
}

func (b *builder) hasUtility(root string, static bool) bool {
	if static {
		_, ok := staticUtilities()[root]
		return ok
	}
	return len(b.utilities[root]) > 0
}

// ---- candidates -----------------------------------------------------------

type candidate struct {
	kind      byte // 's' static, 'f' functional, 'a' arbitrary property
	root      string
	value     *utilityValue
	modifier  *modifier
	variants  []*variantUse
	important bool
	raw       string
	property  string // an arbitrary property's
	arbitrary string // an arbitrary property's value
}

type utilityValue struct {
	arbitrary bool
	dataType  string
	value     string
	fraction  string
}

type modifier struct {
	arbitrary bool
	value     string
}

func isNamedValue(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(isAlpha(c) || isDigit(c) || c == '_' || c == '.' || c == '%' || c == '-') {
			return false
		}
	}
	return true
}

func parseModifier(m string) *modifier {
	if len(m) >= 2 && m[0] == '[' && m[len(m)-1] == ']' {
		v := decodeArbitrary(m[1 : len(m)-1])
		if !validArbitrary(v) || strings.TrimSpace(v) == "" {
			return nil
		}
		return &modifier{arbitrary: true, value: v}
	}
	if len(m) >= 2 && m[0] == '(' && m[len(m)-1] == ')' {
		m = m[1 : len(m)-1]
		if !strings.HasPrefix(m, "--") || !validArbitrary(m) {
			return nil
		}
		return &modifier{arbitrary: true, value: decodeArbitrary("var(" + m + ")")}
	}
	if !isNamedValue(m) {
		return nil
	}
	return &modifier{value: m}
}

// findRoots returns the ways input splits into a root that exists and a
// value, the longest root first.
func findRoots(input string, exists func(string) bool) [][2]string {
	var roots [][2]string
	if exists(input) {
		roots = append(roots, [2]string{input, ""})
	}
	for idx := strings.LastIndexByte(input, '-'); idx > 0; idx = strings.LastIndexByte(input[:idx], '-') {
		root := input[:idx]
		if exists(root) {
			value := input[idx+1:]
			if value == "" {
				break
			}
			roots = append(roots, [2]string{root, value})
		}
	}
	return roots
}

func (b *builder) parseCandidate(input string) []*candidate {
	raw := segment(input, ':')
	base := raw[len(raw)-1]
	var variants []*variantUse
	for i := len(raw) - 2; i >= 0; i-- {
		v := b.parseVariant(raw[i])
		if v == nil {
			return nil
		}
		variants = append(variants, v)
	}
	important := false
	if strings.HasSuffix(base, "!") {
		important, base = true, base[:len(base)-1]
	} else if strings.HasPrefix(base, "!") {
		important, base = true, base[1:]
	}
	if base == "" {
		return nil
	}
	var out []*candidate
	if !strings.Contains(base, "[") && b.hasUtility(base, true) {
		out = append(out, &candidate{kind: 's', root: base, variants: variants, important: important, raw: input})
	}
	parts := segment(base, '/')
	if len(parts) > 2 {
		return out
	}
	baseNoMod := parts[0]
	var mod *modifier
	modSegment, hasMod := "", len(parts) == 2
	if hasMod {
		modSegment = parts[1]
		if mod = parseModifier(modSegment); mod == nil {
			return out
		}
	}
	if baseNoMod == "" {
		return out
	}
	if baseNoMod[0] == '[' {
		if baseNoMod[len(baseNoMod)-1] != ']' || len(baseNoMod) < 2 {
			return out
		}
		if c := baseNoMod[1]; c != '-' && !isLower(c) {
			return out
		}
		inner := baseNoMod[1 : len(baseNoMod)-1]
		idx := strings.IndexByte(inner, ':')
		if idx <= 0 || idx == len(inner)-1 {
			return out
		}
		v := decodeArbitrary(inner[idx+1:])
		if !validArbitrary(v) {
			return out
		}
		return append(out, &candidate{kind: 'a', property: inner[:idx], arbitrary: v, modifier: mod, variants: variants, important: important, raw: input})
	}
	var roots [][2]string
	switch baseNoMod[len(baseNoMod)-1] {
	case ']':
		idx := strings.Index(baseNoMod, "-[")
		if idx < 0 || !b.hasUtility(baseNoMod[:idx], false) {
			return out
		}
		roots = [][2]string{{baseNoMod[:idx], baseNoMod[idx+1:]}}
	case ')':
		idx := strings.Index(baseNoMod, "-(")
		if idx < 0 || !b.hasUtility(baseNoMod[:idx], false) {
			return out
		}
		v := baseNoMod[idx+2 : len(baseNoMod)-1]
		dataType := ""
		if p := segment(v, ':'); len(p) == 2 {
			dataType, v = p[0], p[1]
		}
		if !strings.HasPrefix(v, "--") || !validArbitrary(v) {
			return out
		}
		if dataType == "" {
			roots = [][2]string{{baseNoMod[:idx], "[var(" + v + ")]"}}
		} else {
			roots = [][2]string{{baseNoMod[:idx], "[" + dataType + ":var(" + v + ")]"}}
		}
	default:
		roots = findRoots(baseNoMod, func(r string) bool { return b.hasUtility(r, false) })
	}
	for _, r := range roots {
		c := &candidate{kind: 'f', root: r[0], modifier: mod, variants: variants, important: important, raw: input}
		value := r[1]
		if value == "" {
			out = append(out, c)
			continue
		}
		if start := strings.IndexByte(value, '['); start >= 0 {
			if value[len(value)-1] != ']' {
				return out
			}
			av := decodeArbitrary(value[start+1 : len(value)-1])
			if !validArbitrary(av) {
				continue
			}
			hint := ""
			for i := 0; i < len(av); i++ {
				ch := av[i]
				if ch == ':' {
					hint, av = av[:i], av[i+1:]
					if hint == "" {
						hint = "\x00"
					}
					break
				}
				if ch != '-' && !isLower(ch) {
					break
				}
			}
			if strings.TrimSpace(av) == "" || hint == "\x00" {
				continue
			}
			c.value = &utilityValue{arbitrary: true, dataType: hint, value: av}
		} else {
			fraction := ""
			if hasMod && !mod.arbitrary {
				fraction = value + "/" + modSegment
			}
			if !isNamedValue(value) {
				continue
			}
			c.value = &utilityValue{value: value, fraction: fraction}
		}
		out = append(out, c)
	}
	return out
}

// ---- variants -------------------------------------------------------------

// A variant replaces a utility's nodes with its branches: rules (with & for
// the utility's selector) and at-rules nested in each other, around the
// utility's nodes, after the variant's declarations and @property rules.
type variant struct {
	name       string
	order      int
	breakpoint string // a breakpoint variant's width
	branches   [][]wrapper
	decls      []node
	properties []*property
}

type wrapper struct {
	atRule  bool
	prelude string // a rule's selector or an at-rule's prelude
}

// A variantUse is a variant in a candidate, or an arbitrary one.
type variantUse struct {
	v        *variant
	selector string // an arbitrary variant's
}

func (u *variantUse) key() string {
	if u.v != nil {
		return "\x00" + u.v.name
	}
	return u.selector
}

var (
	staticVariantMap map[string]*variant
	breakpointOrder  int
)

// staticVariants returns the static variants of tables.go, read once.
func staticVariants() map[string]*variant {
	if staticVariantMap != nil {
		return staticVariantMap
	}
	vs := map[string]*variant{}
	for _, line := range strings.Split(variantTable, "\n") {
		f := strings.Split(line, "|")
		order := atoi(f[1])
		if f[0] == "@breakpoint" {
			breakpointOrder = order
			continue
		}
		v := &variant{name: f[0], order: order}
		for _, branch := range strings.Split(f[2], "\t") {
			var ws []wrapper
			for _, w := range strings.Split(branch, " => ") {
				ws = append(ws, wrapper{atRule: w[0] == 'A', prelude: w[2:]})
			}
			v.branches = append(v.branches, ws)
		}
		if f[3] != "" {
			for _, d := range strings.Split(f[3], ";") {
				k, val, _ := strings.Cut(d, ":")
				v.decls = append(v.decls, decl(k, val))
			}
		}
		if f[4] != "" {
			for _, p := range strings.Split(f[4], "\t") {
				v.properties = append(v.properties, parsePropertyCSS(p))
			}
		}
		vs[v.name] = v
	}
	staticVariantMap = vs
	return vs
}

// breakpoints returns the variants of the theme's breakpoints.
func (b *builder) breakpoints() map[string]*variant {
	staticVariants()
	vs := map[string]*variant{}
	keys, values := b.theme.namespace("--breakpoint")
	for i, k := range keys {
		vs[k] = &variant{name: k, order: breakpointOrder, breakpoint: values[i], branches: [][]wrapper{{{atRule: true, prelude: "@media (width >= " + values[i] + ")"}}}}
	}
	return vs
}

// parsePropertyCSS reads "@property --name {syntax: ...; inherits: ...;}".
func parsePropertyCSS(s string) *property {
	head, body, _ := strings.Cut(s, "{")
	p := &property{name: strings.TrimSpace(strings.TrimPrefix(head, "@property")), initial: "initial"}
	for _, d := range strings.Split(strings.TrimSuffix(strings.TrimSpace(body), "}"), ";") {
		if d = strings.TrimSpace(d); d == "" {
			continue
		}
		k, v, _ := strings.Cut(d, ":")
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		p.decls = append(p.decls, [2]string{k, v})
		if k == "initial-value" {
			p.initial = v
		}
	}
	return p
}

func atoi(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		n = n*10 + int(s[i]-'0')
	}
	return n
}

func (b *builder) parseVariant(s string) *variantUse {
	if len(s) >= 2 && s[0] == '[' && s[len(s)-1] == ']' {
		if s[1] == '@' && strings.Contains(s, "&") {
			return nil
		}
		sel := decodeArbitrary(s[1 : len(s)-1])
		if !validArbitrary(sel) || strings.TrimSpace(sel) == "" {
			return nil
		}
		if sel[0] == '>' || sel[0] == '+' || sel[0] == '~' {
			return nil // relative selectors only go in compound variants
		}
		if sel[0] != '@' && !strings.Contains(sel, "&") {
			sel = "&:is(" + sel + ")"
		}
		return &variantUse{selector: sel}
	}
	if v := b.variants[s]; v != nil {
		return &variantUse{v: v}
	}
	if v := staticVariants()[s]; v != nil {
		return &variantUse{v: v}
	}
	return nil
}

// compareVariants orders variants as Tailwind does: by their order, the
// breakpoints by width, and arbitrary variants last.
func compareVariants(a, z *variantUse) int {
	switch {
	case a.v == nil && z.v == nil:
		return strings.Compare(a.selector, z.selector)
	case a.v == nil:
		return 1
	case z.v == nil:
		return -1
	}
	if d := a.v.order - z.v.order; d != 0 {
		return d
	}
	if a.v.breakpoint != "" && z.v.breakpoint != "" {
		return compareBreakpoints(a.v.breakpoint, z.v.breakpoint)
	}
	return strings.Compare(a.v.name, z.v.name)
}

func compareBreakpoints(a, z string) int {
	if a == z {
		return 0
	}
	bucket := func(s string) string {
		if i := strings.IndexByte(s, '('); i >= 0 {
			return s[:i]
		}
		var b strings.Builder
		for i := 0; i < len(s); i++ {
			if !isDigit(s[i]) && s[i] != '.' {
				b.WriteByte(s[i])
			}
		}
		return b.String()
	}
	if ab, zb := bucket(a), bucket(z); ab != zb {
		return strings.Compare(ab, zb)
	}
	an, aok := leadingInt(a)
	zn, zok := leadingInt(z)
	if !aok || !zok {
		return strings.Compare(a, z)
	}
	return an - zn
}

func leadingInt(s string) (int, bool) {
	i := 0
	neg := false
	if i < len(s) && (s[i] == '-' || s[i] == '+') {
		neg = s[i] == '-'
		i++
	}
	start := i
	n := 0
	for i < len(s) && isDigit(s[i]) {
		n = n*10 + int(s[i]-'0')
		i++
	}
	if neg {
		n = -n
	}
	return n, i > start
}

// ---- compiling ------------------------------------------------------------

// A tnode is a node of the stylesheet being built: a declaration, a rule,
// an at-rule or an @property rule for the root.
type tnode struct {
	kind      byte   // 'd', 'r', 'a', 'p'
	prop      string // a declaration's property, a rule's selector, an at-rule's prelude
	value     string
	important bool
	nodes     []*tnode
	property  *property
}

// A compiled candidate is the rule of one utility for a candidate, with
// what sorts it.
type compiled struct {
	rule     *tnode
	variants []uint64 // a bit per variant, in variant order
	order    []int
	count    int
	raw      string
}

func (b *builder) compileBase(c *candidate) [][]node {
	if c.kind == 'a' {
		v := c.arbitrary
		if c.modifier != nil {
			var ok bool
			if v, ok = b.asColor(v, c.modifier); !ok {
				return nil
			}
		}
		return [][]node{{decl(c.property, v)}}
	}
	if c.kind == 's' {
		if nodes, ok := staticUtilities()[c.root]; ok {
			return [][]node{nodes}
		}
		return nil
	}
	var out [][]node
	for _, u := range b.utilities[c.root] {
		if nodes := u.fn(c); nodes != nil {
			out = append(out, nodes)
		}
	}
	return out
}

var propertyIndex = func() map[string]int {
	m := map[string]int{}
	for i, p := range strings.Split(propertyOrder, "\n") {
		m[p] = i
	}
	return m
}()

// propertySort returns the order of the properties nodes set, breadth
// first, and how many declarations they have.
func propertySort(nodes []node) ([]int, int) {
	var order []int
	count := 0
	seenSort := false
	queue := append([]node(nil), nodes...)
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		switch n.kind {
		case 'd':
			count++
			if seenSort {
				continue
			}
			if n.prop == "--tw-sort" {
				if i, ok := propertyIndex[n.value]; ok {
					order = appendUnique(order, i)
					seenSort = true
					continue
				}
			}
			if i, ok := propertyIndex[n.prop]; ok {
				order = appendUnique(order, i)
			}
		case 'r':
			queue = append(queue, n.nodes...)
		}
	}
	slices.Sort(order)
	return order, count
}

func appendUnique(s []int, v int) []int {
	if slices.Contains(s, v) {
		return s
	}
	return append(s, v)
}

// tree turns a utility's nodes into the stylesheet's, with --spacing() and
// !important applied.
func (b *builder) tree(nodes []node, important bool) []*tnode {
	out := make([]*tnode, 0, len(nodes))
	for _, n := range nodes {
		switch n.kind {
		case 'd':
			v := n.value
			if strings.Contains(v, "--spacing(") {
				v = b.substituteSpacing(v)
			}
			out = append(out, &tnode{kind: 'd', prop: n.prop, value: v, important: important})
		case 'r':
			kind := byte('r')
			if strings.HasPrefix(n.prop, "@") {
				kind = 'a'
			}
			out = append(out, &tnode{kind: kind, prop: n.prop, nodes: b.tree(n.nodes, important)})
		case 'p':
			out = append(out, &tnode{kind: 'p', property: n.property})
		}
	}
	return out
}

// compile returns a candidate's rules, one per utility that takes it.
func (b *builder) compile(c *candidate) []*compiled {
	var out []*compiled
	for _, nodes := range b.compileBase(c) {
		order, count := propertySort(nodes)
		r := &tnode{kind: 'r', prop: "." + escapeClass(c.raw), nodes: b.tree(nodes, c.important)}
		for _, vu := range c.variants {
			if vu.v == nil {
				kind := byte('r')
				if vu.selector[0] == '@' {
					kind = 'a'
				}
				r.nodes = []*tnode{{kind: kind, prop: vu.selector, nodes: r.nodes}}
				continue
			}
			inner := r.nodes
			if len(vu.v.decls) > 0 || len(vu.v.properties) > 0 {
				inner = nil
				for _, p := range vu.v.properties {
					inner = append(inner, &tnode{kind: 'p', property: p})
				}
				inner = append(inner, b.tree(vu.v.decls, c.important)...)
				inner = append(inner, r.nodes...)
			}
			var branches []*tnode
			for _, ws := range vu.v.branches {
				var top, cur *tnode
				for _, w := range ws {
					n := &tnode{kind: 'r', prop: w.prelude}
					if w.atRule {
						n.kind = 'a'
					}
					if cur == nil {
						top = n
					} else {
						cur.nodes = []*tnode{n}
					}
					cur = n
				}
				cur.nodes = inner
				branches = append(branches, top)
			}
			r.nodes = branches
		}
		out = append(out, &compiled{rule: r, order: order, count: count, raw: c.raw})
	}
	return out
}

func (b *builder) substituteSpacing(s string) string {
	for {
		i := strings.Index(s, "--spacing(")
		if i < 0 {
			return s
		}
		start := i + len("--spacing(")
		depth, end := 1, -1
		for j := start; j < len(s); j++ {
			if s[j] == '(' {
				depth++
			} else if s[j] == ')' {
				if depth--; depth == 0 {
					end = j
					break
				}
			}
		}
		if end < 0 {
			return s
		}
		arg := strings.TrimSpace(s[start:end])
		mult, _ := b.theme.resolve("", false, "--spacing")
		var r string
		if n, _, ok := dimension(arg); ok && n == 0 {
			r = "0px"
		} else if ok && n == 1 {
			r = mult
		} else {
			r = "calc(" + mult + " * " + arg + ")"
		}
		s = s[:i] + r + s[end+1:]
	}
}

// ---- building -------------------------------------------------------------

func (b *builder) build(rawCandidates []string) string {
	type match struct {
		raw        string
		candidates []*candidate
	}
	var matches []match
	seen := map[string]bool{}
	var usedVariants []*variantUse
	variantSeen := map[string]bool{}
	for _, raw := range rawCandidates {
		if seen[raw] {
			continue
		}
		seen[raw] = true
		cs := b.parseCandidate(raw)
		if len(cs) == 0 {
			continue
		}
		matches = append(matches, match{raw, cs})
		for _, v := range cs[0].variants {
			if k := v.key(); !variantSeen[k] {
				variantSeen[k] = true
				usedVariants = append(usedVariants, v)
			}
		}
	}

	// Each variant gets a bit, in variant order; equal variants share one.
	slices.SortStableFunc(usedVariants, compareVariants)
	bit := map[string]int{}
	idx := 0
	for i, v := range usedVariants {
		if i > 0 && compareVariants(usedVariants[i-1], v) != 0 {
			idx++
		}
		bit[v.key()] = idx
	}
	words := idx/64 + 1

	var all []*compiled
	for _, m := range matches {
		for _, c := range m.candidates {
			for _, cc := range b.compile(c) {
				cc.variants = make([]uint64, words)
				for _, v := range c.variants {
					n := bit[v.key()]
					cc.variants[n/64] |= 1 << (n % 64)
				}
				all = append(all, cc)
			}
		}
	}
	slices.SortStableFunc(all, func(a, z *compiled) int {
		for i := words - 1; i >= 0; i-- {
			if a.variants[i] != z.variants[i] {
				if a.variants[i] < z.variants[i] {
					return -1
				}
				return 1
			}
		}
		off := 0
		for off < len(a.order) && off < len(z.order) && a.order[off] == z.order[off] {
			off++
		}
		ao, zo := 1<<30, 1<<30
		if off < len(a.order) {
			ao = a.order[off]
		}
		if off < len(z.order) {
			zo = z.order[off]
		}
		if ao != zo {
			return ao - zo
		}
		if a.count != z.count {
			return z.count - a.count
		}
		return compareNatural(a.raw, z.raw)
	})
	return b.write(all)
}

// An optimizer prepares the rules for printing, as Tailwind's optimizeAst:
// it drops what does not print, hoists the @property rules, notes the theme
// variables used and adds fallbacks for color-mix().
type optimizer struct {
	b          *builder
	properties []*property
	seen       map[string]bool
	colorMix   [][2]*tnode // parent, declaration
}

func (o *optimizer) transform(n *tnode, parent *tnode, supportsColorMix bool) {
	switch n.kind {
	case 'd':
		if n.prop == "--tw-sort" {
			return
		}
		if strings.Contains(n.value, "var(") {
			o.b.trackVariables(n.value)
		}
		if !supportsColorMix && strings.Contains(n.value, "color-mix(") {
			o.colorMix = append(o.colorMix, [2]*tnode{parent, n})
		}
		parent.nodes = append(parent.nodes, n)
	case 'r', 'a':
		if n.kind == 'a' && strings.HasPrefix(n.prop, "@supports") && strings.Contains(n.prop, "color-mix(") {
			supportsColorMix = true
		}
		c := &tnode{kind: n.kind, prop: n.prop}
		for _, child := range n.nodes {
			o.transform(child, c, supportsColorMix)
		}
		if len(c.nodes) > 0 || n.kind == 'a' && strings.HasPrefix(n.prop, "@layer") {
			parent.nodes = append(parent.nodes, c)
		}
	case 'p':
		if !o.seen[n.property.name] {
			o.seen[n.property.name] = true
			o.properties = append(o.properties, n.property)
		}
	}
}

func (b *builder) write(all []*compiled) string {
	o := &optimizer{b: b, seen: map[string]bool{}}
	layer := &tnode{kind: 'a', prop: "@layer utilities"}
	for _, cc := range all {
		o.transform(cc.rule, layer, false)
	}
	for _, pd := range o.colorMix {
		parent, d := pd[0], pd[1]
		i := slices.Index(parent.nodes, d)
		if i < 0 {
			continue
		}
		fallback, ok := b.colorMixFallback(d.value)
		if !ok {
			continue
		}
		f := &tnode{kind: 'd', prop: d.prop, value: fallback, important: d.important}
		supports := &tnode{kind: 'a', prop: "@supports (color: color-mix(in lab, red, red))", nodes: []*tnode{d}}
		parent.nodes = slices.Insert(slices.Delete(parent.nodes, i, i+1), i, f, supports)
	}

	var sb strings.Builder
	sb.WriteString("/*! tailwindcss v" + version + " | MIT License | https://tailwindcss.com */\n")
	if len(o.properties) > 0 {
		sb.WriteString("@layer properties;\n")
	}
	b.writeTheme(&sb)
	for _, n := range nest([]*tnode{layer}) {
		writeTree(&sb, n, 0)
	}
	for _, p := range o.properties {
		sb.WriteString("@property " + p.name + " {\n")
		for _, d := range p.decls {
			sb.WriteString("  " + d[0] + ": " + d[1] + ";\n")
		}
		sb.WriteString("}\n")
	}
	if len(o.properties) > 0 {
		sb.WriteString("@layer properties {\n  @supports ((-webkit-hyphens: none) and (not (margin-trim: inline))) or ((-moz-orient: inline) and (not (color:rgb(from red r g b)))) {\n    *, ::before, ::after, ::backdrop {\n")
		for _, p := range o.properties {
			sb.WriteString("      " + p.name + ": " + p.initial + ";\n")
		}
		sb.WriteString("    }\n  }\n}\n")
	}
	return sb.String()
}

func writeTree(sb *strings.Builder, n *tnode, depth int) {
	for i := 0; i < depth; i++ {
		sb.WriteString("  ")
	}
	switch n.kind {
	case 'd':
		sb.WriteString(n.prop)
		sb.WriteString(": ")
		sb.WriteString(n.value)
		if n.important {
			sb.WriteString(" !important")
		}
		sb.WriteString(";\n")
		return
	case 'a':
		if len(n.nodes) == 0 {
			sb.WriteString(n.prop + ";\n")
			return
		}
	}
	sb.WriteString(n.prop)
	sb.WriteString(" {\n")
	for _, c := range n.nodes {
		writeTree(sb, c, depth+1)
	}
	for i := 0; i < depth; i++ {
		sb.WriteString("  ")
	}
	sb.WriteString("}\n")
}

// ---- nesting --------------------------------------------------------------

// A nester flattens nested rules into rules with full selectors, hoisting
// at-rules above them and merging adjacent ones, as Tailwind's
// handleNesting.
type nester struct {
	selectors []string
	atRules   []string
	nodes     *tnode // the node emitted nodes go into, or nil
	seen      map[string]bool
	dedupe    []*tnode
	result    []*tnode
}

func nest(ast []*tnode) []*tnode {
	h := &nester{seen: map[string]bool{}}
	for _, n := range ast {
		h.visit(n)
	}
	for _, n := range h.dedupe {
		n.nodes = dedupeDecls(n.nodes)
	}
	return h.result
}

func hoistable(prelude string) bool {
	for _, p := range []string{"@container", "@layer", "@media", "@page", "@starting-style", "@supports", "@view-transition"} {
		if prelude == p || strings.HasPrefix(prelude, p+" ") || strings.HasPrefix(prelude, p+"(") {
			return true
		}
	}
	return false
}

func (h *nester) visit(n *tnode) {
	switch n.kind {
	case 'r':
		h.nodes = nil
		switch {
		case len(h.selectors) == 0:
			h.selectors = append(h.selectors, n.prop)
		case n.prop == "&":
			for _, c := range n.nodes {
				h.visit(c)
			}
			return
		default:
			h.selectors = append(h.selectors, nestSelector(n.prop, h.selectors[len(h.selectors)-1]))
		}
		if slices.ContainsFunc(n.nodes, func(c *tnode) bool { return c.kind == 'd' }) {
			for _, c := range n.nodes {
				h.emit(c)
			}
		} else {
			for _, c := range n.nodes {
				h.visit(c)
			}
		}
		h.nodes = nil
		h.selectors = h.selectors[:len(h.selectors)-1]
	case 'a':
		h.nodes = nil
		if !hoistable(n.prop) || len(n.nodes) == 0 && !strings.HasPrefix(n.prop, "@layer") {
			h.emit(n)
			return
		}
		if len(n.nodes) == 0 {
			h.emit(n)
			return
		}
		h.atRules = append(h.atRules, n.prop)
		for _, c := range n.nodes {
			h.visit(c)
		}
		h.nodes = nil
		h.atRules = h.atRules[:len(h.atRules)-1]
	case 'd':
		h.emit(n)
	}
}

func (h *nester) emit(n *tnode) {
	if h.nodes != nil {
		if n.kind == 'd' {
			if h.seen[n.prop] {
				h.markDedupe(h.nodes)
			} else {
				h.seen[n.prop] = true
			}
		}
		h.nodes.nodes = append(h.nodes.nodes, n)
		return
	}
	if len(h.selectors) == 0 && len(h.atRules) == 0 {
		if l := last(h.result); l != nil && l.kind == 'a' && n.kind == 'a' && len(l.nodes) == 0 && len(n.nodes) == 0 && l.prop == n.prop {
			return
		}
		h.result = append(h.result, n)
		return
	}
	clear(h.seen)
	if n.kind == 'd' {
		h.seen[n.prop] = true
	}
	target := &h.result
	off := 0
	if l := last(*target); l != nil && l.kind == 'a' {
		for _, at := range h.atRules {
			if l == nil || l.kind != 'a' || l.prop != at {
				break
			}
			off++
			target = &l.nodes
			l = last(l.nodes)
		}
	}
	var root *tnode
	if len(h.selectors) > 0 {
		sel := h.selectors[len(h.selectors)-1]
		if len(h.atRules)-off <= 0 {
			if l := last(*target); l != nil && l.kind == 'r' && l.prop == sel {
				l.nodes = append(l.nodes, n)
				h.nodes = l
				h.markDedupe(l)
				return
			}
		}
		root = &tnode{kind: 'r', prop: sel, nodes: []*tnode{n}}
		h.nodes = root
	}
	for i := len(h.atRules) - 1; i >= off; i-- {
		if root == nil {
			root = &tnode{kind: 'a', prop: h.atRules[i], nodes: []*tnode{n}}
			h.nodes = root
		} else {
			root = &tnode{kind: 'a', prop: h.atRules[i], nodes: []*tnode{root}}
		}
	}
	if root != nil {
		*target = append(*target, root)
	} else {
		*target = append(*target, n)
		h.nodes = &tnode{} // as in Tailwind, later nodes are not kept
	}
}

func (h *nester) markDedupe(n *tnode) {
	if !slices.Contains(h.dedupe, n) {
		h.dedupe = append(h.dedupe, n)
	}
}

func last(ns []*tnode) *tnode {
	if len(ns) == 0 {
		return nil
	}
	return ns[len(ns)-1]
}

// dedupeDecls drops declarations repeated later with the same value.
func dedupeDecls(ns []*tnode) []*tnode {
	seen := map[string]bool{}
	keep := make([]bool, len(ns))
	for i := len(ns) - 1; i >= 0; i-- {
		n := ns[i]
		if n.kind != 'd' {
			keep[i] = true
			continue
		}
		id := n.prop + "\x00" + n.value
		if n.important {
			id += "\x00!"
		}
		if !seen[id] {
			seen[id] = true
			keep[i] = true
		}
	}
	out := ns[:0:0]
	for i, n := range ns {
		if keep[i] {
			out = append(out, n)
		}
	}
	return out
}

// nestSelector puts a nested rule's selector in its parent's, sel: & is
// sel, and a selector without & is a descendant of sel.
func nestSelector(nested, parent string) string {
	parentList := len(segment(parent, ',')) > 1
	parentComplex := isComplexSelector(parent)
	parts := segment(nested, ',')
	for i, s := range parts {
		s = strings.TrimSpace(s)
		if !strings.Contains(s, "&") {
			if parentList {
				parts[i] = ":is(" + parent + ") " + s
			} else {
				parts[i] = parent + " " + s
			}
			continue
		}
		parts[i] = substituteParent(dropUniversal(s), parent, parentList, parentComplex)
	}
	return strings.Join(parts, ", ")
}

// isComplexSelector reports whether s has a combinator at its top level.
func isComplexSelector(s string) bool {
	depth := 0
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '\\':
			i++
		case '(', '[':
			depth++
		case ')', ']':
			depth--
		case ' ', '>', '+', '~':
			if depth == 0 {
				return true
			}
		}
	}
	return false
}

// substituteParent replaces each & in a selector with the parent, wrapped
// in :is() where the parent would change the selector's meaning otherwise.
func substituteParent(s, parent string, parentList, parentComplex bool) string {
	var b strings.Builder
	depth := 0
	compoundStart := true // whether & would start a compound selector
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\\':
			b.WriteByte(c)
			if i+1 < len(s) {
				b.WriteByte(s[i+1])
			}
			i++
			compoundStart = false
			continue
		case '&':
			wrap := parentList
			if !wrap && !compoundStart && depth == 0 && parentComplex {
				wrap = true
			}
			if wrap {
				b.WriteString(":is(" + parent + ")")
			} else {
				b.WriteString(parent)
			}
			compoundStart = false
			continue
		case '(', '[':
			depth++
		case ')', ']':
			depth--
		case ' ', '>', '+', '~':
			if depth == 0 {
				// Combinators print with a space on each side.
				j := i
				comb := byte(' ')
				for ; j < len(s) && strings.IndexByte(" >+~", s[j]) >= 0; j++ {
					if s[j] != ' ' {
						comb = s[j]
					}
				}
				if comb == ' ' {
					b.WriteByte(' ')
				} else {
					b.WriteByte(' ')
					b.WriteByte(comb)
					b.WriteByte(' ')
				}
				i = j - 1
				compoundStart = true
				continue
			}
		}
		b.WriteByte(c)
		compoundStart = c == ' ' || c == '>' || c == '+' || c == '~' || c == '(' || c == ','
	}
	return b.String()
}

// dropUniversal drops a * that starts a compound selector with more in it,
// as *:hover is :hover.
func dropUniversal(s string) string {
	if !strings.Contains(s, "*") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' && i+1 < len(s) {
			b.WriteByte(c)
			b.WriteByte(s[i+1])
			i++
			continue
		}
		if c == '*' && i+1 < len(s) && (s[i+1] == ':' || s[i+1] == '[' || s[i+1] == '.' || s[i+1] == '#') && (i == 0 || strings.IndexByte(" >+~(,", s[i-1]) >= 0) {
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// trackVariables marks the theme variables a value uses.
func (b *builder) trackVariables(v string) {
	vars, ok := b.vars[v]
	if !ok {
		ast := parseValue(v)
		walkValues(&ast, func(n *value) (int, []*value) {
			if n.kind != 'f' || n.text != "var" {
				return walkContinue, nil
			}
			walkValues(&n.nodes, func(c *value) (int, []*value) {
				if c.kind == 'w' && strings.HasPrefix(c.text, "--") {
					vars = append(vars, c.text)
				}
				return walkContinue, nil
			})
			return walkSkip, nil
		})
		b.vars[v] = vars
	}
	for _, name := range vars {
		b.used[name] = true
	}
}

// writeTheme writes the theme variables the utilities use, and those they
// depend on.
func (b *builder) writeTheme(sb *strings.Builder) {
	deps := map[string][]string{}
	for _, e := range b.theme.entries {
		if e.opts&optReference == 0 && strings.Contains(e.value, "var(") {
			ast := parseValue(e.value)
			walkValues(&ast, func(n *value) (int, []*value) {
				if n.kind == 'w' && strings.HasPrefix(n.text, "--") {
					deps[n.text] = append(deps[n.text], e.key)
				}
				return walkContinue, nil
			})
		}
	}
	var isUsed func(k string, seen map[string]bool) bool
	isUsed = func(k string, seen map[string]bool) bool {
		if seen[k] {
			return true
		}
		seen[k] = true
		if e := b.theme.index[k]; e != nil && e.opts&optStatic != 0 || b.used[k] {
			return true
		}
		for _, d := range deps[k] {
			if isUsed(d, seen) {
				return true
			}
		}
		return false
	}
	started := false
	for _, e := range b.theme.entries {
		if e.opts&optReference != 0 || !isUsed(e.key, map[string]bool{}) {
			continue
		}
		if !started {
			sb.WriteString("@layer theme {\n  :root, :host {\n")
			started = true
		}
		sb.WriteString("    " + e.key + ": " + e.value + ";\n")
	}
	if started {
		sb.WriteString("  }\n}\n")
	}
}

// colorMixFallback returns a value for browsers without color-mix() in
// oklab, with the theme colors it mixes inlined, as Tailwind does.
func (b *builder) colorMixFallback(v string) (string, bool) {
	ast := parseValue(v)
	requires := false
	walkValues(&ast, func(n *value) (int, []*value) {
		if n.kind != 'f' || n.text != "color-mix" {
			return walkContinue, nil
		}
		unresolvable, current := false, false
		walkValues(&n.nodes, func(c *value) (int, []*value) {
			if c.kind == 'w' && strings.ToLower(c.text) == "currentcolor" {
				current, requires = true, true
				return walkContinue, nil
			}
			vn := c
			inlined := ""
			seen := map[string]bool{}
			for vn != nil {
				if vn.kind != 'f' || vn.text != "var" || len(vn.nodes) == 0 || vn.nodes[0].kind != 'w' {
					return walkContinue, nil
				}
				name := vn.nodes[0].text
				if seen[name] {
					unresolvable = true
					return walkContinue, nil
				}
				seen[name] = true
				requires = true
				val, ok := b.theme.resolveValue("", false, name)
				if !ok || val == "" {
					unresolvable = true
					return walkContinue, nil
				}
				if strings.ToLower(val) == "currentcolor" {
					current = true
					return walkContinue, nil
				}
				inlined = val
				vn = nil
				if strings.HasPrefix(val, "var(") {
					vn = parseValue(val)[0]
				}
			}
			return walkReplace, []*value{{kind: 'w', text: inlined}}
		})
		if unresolvable || current {
			sep := -1
			for i, c := range n.nodes {
				if c.kind == 's' && strings.Contains(strings.TrimSpace(c.text), ",") {
					sep = i
					break
				}
			}
			if sep < 0 || sep+1 >= len(n.nodes) {
				return walkContinue, nil
			}
			return walkReplace, []*value{n.nodes[sep+1]}
		}
		if requires && len(n.nodes) > 2 {
			if cs := n.nodes[2]; cs.kind == 'w' && (cs.text == "oklab" || cs.text == "oklch" || cs.text == "lab" || cs.text == "lch") {
				cs.text = "srgb"
			}
		}
		return walkContinue, nil
	})
	if !requires {
		return "", false
	}
	return valueString(ast), true
}
