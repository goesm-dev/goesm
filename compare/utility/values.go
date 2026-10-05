package utility

import (
	"math"
	"strconv"
	"strings"
)

// segment splits s at each top-level sep, outside parentheses, brackets,
// braces and quotes.
func segment(s string, sep byte) []string {
	var parts []string
	var stack []byte
	last := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if len(stack) == 0 && c == sep {
			parts = append(parts, s[last:i])
			last = i + 1
			continue
		}
		switch c {
		case '\\':
			i++
		case '\'', '"':
			for i++; i < len(s); i++ {
				if s[i] == '\\' {
					i++
				} else if s[i] == c {
					break
				}
			}
		case '(':
			stack = append(stack, ')')
		case '[':
			stack = append(stack, ']')
		case '{':
			stack = append(stack, '}')
		case ')', ']', '}':
			if len(stack) > 0 && stack[len(stack)-1] == c {
				stack = stack[:len(stack)-1]
			}
		}
	}
	return append(parts, s[last:])
}

// validArbitrary reports whether s may be an arbitrary value: balanced
// brackets and no top-level semicolon.
func validArbitrary(s string) bool {
	var stack []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\\':
			i++
		case '\'', '"':
			for i++; i < len(s); i++ {
				if s[i] == '\\' {
					i++
				} else if s[i] == c {
					break
				}
			}
		case '(':
			stack = append(stack, ')')
		case '[':
			stack = append(stack, ']')
		case ']', '}', ')':
			if len(stack) == 0 {
				return false
			}
			if stack[len(stack)-1] == c {
				stack = stack[:len(stack)-1]
			}
		case ';':
			if len(stack) == 0 {
				return false
			}
		}
	}
	return true
}

// A value is a CSS value parsed into words, separators and functions.
type value struct {
	kind  byte // 'w' word, 's' separator, 'f' function
	text  string
	nodes []*value
}

func isSeparator(c byte) bool {
	switch c {
	case ':', ',', '=', '>', '<', '\n', ' ', '\t':
		return true
	}
	return false
}

func parseValue(s string) []*value {
	if strings.Contains(s, "\r\n") {
		s = strings.ReplaceAll(s, "\r\n", "\n")
	}
	var ast []*value
	var stack []*value
	var buf strings.Builder
	push := func(v *value) {
		if len(stack) > 0 {
			p := stack[len(stack)-1]
			p.nodes = append(p.nodes, v)
		} else {
			ast = append(ast, v)
		}
	}
	flush := func() {
		if buf.Len() > 0 {
			push(&value{kind: 'w', text: buf.String()})
			buf.Reset()
		}
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\':
			buf.WriteByte(c)
			if i+1 < len(s) {
				buf.WriteByte(s[i+1])
			}
			i++
		case c == '/':
			flush()
			push(&value{kind: 'w', text: "/"})
		case isSeparator(c):
			flush()
			end := i + 1
			for end < len(s) && isSeparator(s[end]) {
				end++
			}
			push(&value{kind: 's', text: s[i:end]})
			i = end - 1
		case c == '\'' || c == '"':
			start := i
			for j := i + 1; j < len(s); j++ {
				if s[j] == '\\' {
					j++
				} else if s[j] == c {
					i = j
					break
				}
			}
			buf.WriteString(s[start : i+1])
		case c == '(':
			f := &value{kind: 'f', text: buf.String()}
			buf.Reset()
			push(f)
			stack = append(stack, f)
		case c == ')':
			if len(stack) > 0 {
				tail := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				if buf.Len() > 0 {
					tail.nodes = append(tail.nodes, &value{kind: 'w', text: buf.String()})
					buf.Reset()
				}
			} else {
				buf.Reset()
			}
		default:
			buf.WriteByte(c)
		}
	}
	if buf.Len() > 0 {
		ast = append(ast, &value{kind: 'w', text: buf.String()})
	}
	return ast
}

func writeValues(b *strings.Builder, ast []*value) {
	for _, v := range ast {
		b.WriteString(v.text)
		if v.kind == 'f' {
			b.WriteByte('(')
			writeValues(b, v.nodes)
			b.WriteByte(')')
		}
	}
}

func valueString(ast []*value) string {
	var b strings.Builder
	writeValues(&b, ast)
	return b.String()
}

// Walk results, as Tailwind's walk: go into the node's children, skip
// them, replace the node and visit the replacements, or stop.
const (
	walkContinue = iota
	walkSkip
	walkReplace
	walkStop
)

func walkValues(nodes *[]*value, fn func(v *value) (int, []*value)) bool {
	for i := 0; i < len(*nodes); {
		v := (*nodes)[i]
		action, repl := fn(v)
		switch action {
		case walkStop:
			return false
		case walkReplace:
			ns := append(append(append([]*value{}, (*nodes)[:i]...), repl...), (*nodes)[i+1:]...)
			*nodes = ns
			continue
		case walkContinue:
			if len(v.nodes) > 0 && !walkValues(&v.nodes, fn) {
				return false
			}
		}
		i++
	}
	return true
}

// underscores turns _ into a space and \_ into _.
func underscores(s string, keepUnderscore bool) string {
	if !strings.Contains(s, "_") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\' && i+1 < len(s) && s[i+1] == '_':
			b.WriteByte('_')
			i++
		case c == '_' && !keepUnderscore:
			b.WriteByte(' ')
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

func decodeValues(ast []*value) {
	for _, v := range ast {
		switch v.kind {
		case 'f':
			if v.text == "url" || strings.HasSuffix(v.text, "_url") {
				v.text = underscores(v.text, false)
				continue
			}
			if v.text == "var" || strings.HasSuffix(v.text, "_var") || v.text == "theme" || strings.HasSuffix(v.text, "_theme") {
				v.text = underscores(v.text, false)
				for i, n := range v.nodes {
					if i == 0 && n.kind == 'w' {
						n.text = underscores(n.text, true)
						continue
					}
					decodeValues(v.nodes[i : i+1])
				}
				continue
			}
			v.text = underscores(v.text, false)
			decodeValues(v.nodes)
		default:
			v.text = underscores(v.text, false)
		}
	}
}

// decodeArbitrary turns the text between brackets in a class name into CSS.
func decodeArbitrary(s string) string {
	if !strings.Contains(s, "(") {
		return underscores(s, false)
	}
	ast := parseValue(s)
	decodeValues(ast)
	return spaceMathOperators(valueString(ast))
}

var mathFunctions = []string{"calc", "min", "max", "clamp", "mod", "rem", "sin", "cos", "tan", "asin", "acos", "atan", "atan2", "pow", "sqrt", "hypot", "log", "exp", "round"}

func isMathFunction(s string) bool {
	for _, f := range mathFunctions {
		if s == f {
			return true
		}
	}
	return false
}

func hasMathFn(s string) bool {
	if !strings.Contains(s, "(") {
		return false
	}
	for _, f := range mathFunctions {
		if strings.Contains(s, f+"(") {
			return true
		}
	}
	return false
}

func isDigit(c byte) bool  { return c >= '0' && c <= '9' }
func isLower(c byte) bool  { return c >= 'a' && c <= 'z' }
func isAlpha(c byte) bool  { return isLower(c) || c >= 'A' && c <= 'Z' }
func isMathOp(c byte) bool { return c == '+' || c == '*' || c == '/' || c == '-' }

// at returns s[i], or 0 out of range, as JavaScript's charCodeAt gives NaN.
func at(s string, i int) byte {
	if i < 0 || i >= len(s) {
		return 0
	}
	return s[i]
}

// spaceMathOperators puts spaces around the operators in math functions:
// calc(1px+2px) is calc(1px + 2px).
func spaceMathOperators(s string) string {
	found := false
	for _, f := range mathFunctions {
		if strings.Contains(s, f) {
			found = true
			break
		}
	}
	if !found {
		return s
	}
	b := make([]byte, 0, len(s)+8)
	var formattable []bool
	valuePos, lastValuePos := -1, -1
	top := func() bool { return len(formattable) > 0 && formattable[len(formattable)-1] }
	for i := 0; i < len(s); i++ {
		c := s[i]
		if isDigit(c) {
			valuePos = i
		} else if valuePos >= 0 && (c == '%' || isAlpha(c)) {
			valuePos = i
		} else {
			lastValuePos = valuePos
			valuePos = -1
		}
		switch {
		case c == '(':
			b = append(b, c)
			start := i
			for j := i - 1; j >= 0 && (isDigit(s[j]) || isLower(s[j])); j-- {
				start = j
			}
			fn := s[start:i]
			formattable = append(formattable, isMathFunction(fn) || top() && fn == "")
		case c == ')':
			b = append(b, c)
			if len(formattable) > 0 {
				formattable = formattable[:len(formattable)-1]
			}
		case c == ',' && top():
			b = append(b, ", "...)
		case c == ' ' && top() && len(b) > 0 && b[len(b)-1] == ' ':
		case isMathOp(c) && top():
			trimmed := strings.TrimRight(string(b), " \t\n\r\f")
			prev, prevPrev := at(trimmed, len(trimmed)-1), at(trimmed, len(trimmed)-2)
			next := at(s, i+1)
			switch {
			case (prev == 'e' || prev == 'E') && isDigit(prevPrev):
				b = append(b, c)
			case isMathOp(prev):
				b = append(b, c)
			case prev == '(' || prev == ',':
				b = append(b, c)
			case at(s, i-1) == ' ':
				b = append(b, c, ' ')
			case isDigit(prev) || isDigit(next) || prev == ')' || next == '(' || isMathOp(next) || lastValuePos >= 0 && lastValuePos == i-1:
				b = append(b, ' ', c, ' ')
			default:
				b = append(b, c)
			}
		default:
			b = append(b, c)
		}
	}
	return string(b)
}

// jsNumber converts s to a number as JavaScript's Number does, for the
// decimal forms class names use.
func jsNumber(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, true
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !isDigit(c) && c != '.' && c != '-' && c != '+' && c != 'e' && c != 'E' {
			return 0, false
		}
	}
	f, err := strconv.ParseFloat(s, 64)
	return f, err == nil
}

// jsString formats f as JavaScript's String does.
func jsString(f float64) string {
	if f == 0 {
		return "0"
	}
	if a := math.Abs(f); a >= 1e21 || a < 1e-6 {
		s := strconv.FormatFloat(f, 'g', -1, 64)
		return strings.Replace(strings.Replace(s, "e-0", "e-", 1), "e+0", "e+", 1)
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func isPositiveInteger(s string) bool {
	n, ok := jsNumber(s)
	return ok && n == math.Trunc(n) && n >= 0 && jsString(n) == s
}

func isStrictPositiveInteger(s string) bool {
	n, ok := jsNumber(s)
	return ok && n == math.Trunc(n) && n > 0 && jsString(n) == s
}

func isMultipleOf(s string, divisor float64) bool {
	n, ok := jsNumber(s)
	return ok && n >= 0 && math.Mod(n, divisor) == 0 && jsString(n) == s
}

func isValidSpacingMultiplier(s string) bool { return isMultipleOf(s, 0.25) }
func isValidOpacityValue(s string) bool      { return isMultipleOf(s, 0.25) }

// numberPrefix returns the length of the number at the start of s, as
// [+-]?\d*\.?\d+(?:[eE][+-]?\d+)?, or -1.
func numberPrefix(s string) int {
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	ds := i
	for i < len(s) && isDigit(s[i]) {
		i++
	}
	end := -1
	if i > ds {
		end = i
	}
	if i < len(s) && s[i] == '.' {
		j := i + 1
		for j < len(s) && isDigit(s[j]) {
			j++
		}
		if j > i+1 {
			end = j
		}
	}
	if end < 0 {
		return -1
	}
	if end < len(s) && (s[end] == 'e' || s[end] == 'E') {
		j := end + 1
		if j < len(s) && (s[j] == '+' || s[j] == '-') {
			j++
		}
		k := j
		for k < len(s) && isDigit(s[k]) {
			k++
		}
		if k > j {
			end = k
		}
	}
	return end
}

// numberWith reports whether s is a number followed by one of units.
func numberWith(s string, units []string) bool {
	n := numberPrefix(s)
	if n < 0 {
		return false
	}
	rest := s[n:]
	for _, u := range units {
		if rest == u {
			return true
		}
	}
	return false
}

var (
	noUnit      = []string{""}
	percentUnit = []string{"%"}
	lengthUnits = []string{"cm", "mm", "Q", "in", "pc", "pt", "px", "em", "ex", "ch", "rem", "lh", "rlh", "vw", "vh", "vmin", "vmax", "vb", "vi", "svw", "svh", "lvw", "lvh", "dvw", "dvh", "cqw", "cqh", "cqi", "cqb", "cqmin", "cqmax"}
	angleUnits  = []string{"deg", "rad", "grad", "turn"}
)

func isNumber(s string) bool     { return numberWith(s, noUnit) || hasMathFn(s) }
func isPercentage(s string) bool { return numberWith(s, percentUnit) || hasMathFn(s) }

func isLength(s string) bool {
	return numberWith(s, lengthUnits) || len(s) >= 10 && strings.EqualFold(s[:10], "--spacing(") || hasMathFn(s)
}

func isFraction(s string) bool {
	if hasMathFn(s) {
		return true
	}
	n := numberPrefix(s)
	if n < 0 {
		return false
	}
	rest := strings.TrimLeft(s[n:], " \t\n\r\f")
	if !strings.HasPrefix(rest, "/") {
		return false
	}
	rest = strings.TrimLeft(rest[1:], " \t\n\r\f")
	return numberWith(rest, noUnit)
}

var colorNames = func() map[string]bool {
	m := map[string]bool{}
	for _, n := range strings.Split(namedColors, "\n") {
		m[n] = true
	}
	return m
}()

func isNamedColor(s string) bool { return colorNames[strings.ToLower(s)] }

var colorFunctions = []string{"rgba", "rgb", "hsla", "hsl", "hwb", "color", "oklab", "oklch", "lab", "lch", "light-dark", "color-mix", "--alpha"}

func isColor(s string) bool {
	if len(s) > 0 && s[0] == '#' {
		return true
	}
	if i := strings.IndexByte(s, '('); i > 0 {
		fn := strings.ToLower(s[:i])
		for _, f := range colorFunctions {
			if fn == f {
				return true
			}
		}
	}
	return isNamedColor(s)
}

func isURL(s string) bool { return strings.HasPrefix(s, "url(") && strings.HasSuffix(s, ")") }

func isImage(s string) bool {
	count := 0
	for _, part := range segment(s, ',') {
		switch {
		case strings.HasPrefix(part, "var("):
			continue
		case isURL(part):
		case isGradient(part):
		case strings.HasPrefix(part, "element(") || strings.HasPrefix(part, "image(") || strings.HasPrefix(part, "cross-fade(") || strings.HasPrefix(part, "image-set("):
		default:
			return false
		}
		count++
	}
	return count > 0
}

func isGradient(s string) bool {
	s = strings.TrimPrefix(s, "repeating-")
	for _, k := range []string{"conic", "linear", "radial"} {
		if strings.HasPrefix(s, k+"-gradient(") {
			return true
		}
	}
	return false
}

func isLineWidth(s string) bool {
	for _, v := range segment(s, ' ') {
		if !isLength(v) && !isNumber(v) && v != "thin" && v != "medium" && v != "thick" {
			return false
		}
	}
	return true
}

func isBackgroundPosition(s string) bool {
	count := 0
	for _, part := range segment(s, ' ') {
		switch part {
		case "center", "top", "right", "bottom", "left":
			count++
			continue
		}
		if strings.HasPrefix(part, "var(") {
			continue
		}
		if isLength(part) || isPercentage(part) {
			count++
			continue
		}
		return false
	}
	return count > 0
}

func isBackgroundSize(s string) bool {
	count := 0
	for _, size := range segment(s, ',') {
		if size == "cover" || size == "contain" {
			count++
			continue
		}
		values := segment(size, ' ')
		if len(values) != 1 && len(values) != 2 {
			return false
		}
		all := true
		for _, v := range values {
			if v != "auto" && !isLength(v) && !isPercentage(v) {
				all = false
			}
		}
		if all {
			count++
		}
	}
	return count > 0
}

func isFamilyName(s string) bool {
	count := 0
	for _, part := range segment(s, ',') {
		if len(part) > 0 && isDigit(part[0]) {
			return false
		}
		if strings.HasPrefix(part, "var(") {
			continue
		}
		count++
	}
	return count > 0
}

func isGenericName(s string) bool {
	switch s {
	case "serif", "sans-serif", "monospace", "cursive", "fantasy", "system-ui", "ui-serif", "ui-sans-serif", "ui-monospace", "ui-rounded", "math", "emoji", "fangsong":
		return true
	}
	return false
}

func isAbsoluteSize(s string) bool {
	switch s {
	case "xx-small", "x-small", "small", "medium", "large", "x-large", "xx-large", "xxx-large":
		return true
	}
	return false
}

func checkType(t, s string) bool {
	switch t {
	case "color":
		return isColor(s)
	case "length":
		return isLength(s)
	case "percentage":
		return isPercentage(s)
	case "ratio":
		return isFraction(s)
	case "number":
		return isNumber(s)
	case "integer":
		return isPositiveInteger(s)
	case "url":
		return isURL(s)
	case "position":
		return isBackgroundPosition(s)
	case "bg-size":
		return isBackgroundSize(s)
	case "line-width":
		return isLineWidth(s)
	case "image":
		return isImage(s)
	case "family-name":
		return isFamilyName(s)
	case "generic-name":
		return isGenericName(s)
	case "absolute-size":
		return isAbsoluteSize(s)
	case "relative-size":
		return s == "larger" || s == "smaller"
	case "angle":
		return numberWith(s, angleUnits)
	}
	return false
}

// inferDataType returns the first of types that s has, or "".
func inferDataType(s string, types ...string) string {
	if strings.HasPrefix(s, "var(") {
		return ""
	}
	for _, t := range types {
		if checkType(t, s) {
			return t
		}
	}
	return ""
}

// dimension parses a number with an optional unit, like 64rem.
func dimension(s string) (n float64, unit string, ok bool) {
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	ds := i
	for i < len(s) && isDigit(s[i]) {
		i++
	}
	if i < len(s) && s[i] == '.' {
		i++
		fs := i
		for i < len(s) && isDigit(s[i]) {
			i++
		}
		if i == fs {
			return 0, "", false
		}
	} else if i == ds {
		return 0, "", false
	}
	num := s[:i]
	unit = s[i:]
	if unit != "%" {
		for j := 0; j < len(unit); j++ {
			if !isAlpha(unit[j]) {
				return 0, "", false
			}
		}
	}
	n, err := strconv.ParseFloat(num, 64)
	return n, unit, err == nil
}

// escapeClass escapes a class name for a selector, as CSS.escape.
func escapeClass(s string) string {
	if s == "-" {
		return `\-`
	}
	simple := true
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= 0x80 || c == '-' || c == '_' || isDigit(c) || isAlpha(c)) || i == 0 && isDigit(c) || i == 1 && isDigit(c) && s[0] == '-' {
			simple = false
			break
		}
	}
	if simple {
		return s
	}
	var b strings.Builder
	for i, r := range s {
		switch {
		case r == 0:
			b.WriteRune('�')
		case r >= 1 && r <= 0x1f || r == 0x7f || i == 0 && r >= '0' && r <= '9' || i == 1 && r >= '0' && r <= '9' && s[0] == '-':
			b.WriteByte('\\')
			b.WriteString(strconv.FormatInt(int64(r), 16))
			b.WriteByte(' ')
		case r >= 0x80 || r == '-' || r == '_' || r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z':
			b.WriteRune(r)
		default:
			b.WriteByte('\\')
			b.WriteRune(r)
		}
	}
	return b.String()
}

// compareNatural orders strings with their numbers compared as numbers, as
// Tailwind sorts class names.
func compareNatural(a, z string) int {
	n := len(a)
	if len(z) < n {
		n = len(z)
	}
	for i := 0; i < n; i++ {
		ac, zc := a[i], z[i]
		if isDigit(ac) && isDigit(zc) {
			ae, ze := i+1, i+1
			for ae < len(a) && isDigit(a[ae]) {
				ae++
			}
			for ze < len(z) && isDigit(z[ze]) {
				ze++
			}
			an, zn := a[i:ae], z[i:ze]
			af, _ := strconv.ParseFloat(an, 64)
			zf, _ := strconv.ParseFloat(zn, 64)
			if af != zf {
				if af < zf {
					return -1
				}
				return 1
			}
			if an < zn {
				return -1
			}
			if an > zn {
				return 1
			}
			continue
		}
		if ac != zc {
			return int(ac) - int(zc)
		}
	}
	return len(a) - len(z)
}
