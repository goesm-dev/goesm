package lower

import (
	"fmt"
	"go/token"
	"strconv"
	"strings"

	"github.com/goesm-dev/goesm/internal/sourcemap"
)

// Source positions travel with generated code from the start. Expression
// lowering returns strings; a Go position is attached to a string by
// embedding a marker (posMark) that the writer strips while recording a
// generated->original mapping at the exact output column. NUL never occurs in
// generated code otherwise (string literals escape control characters).

const (
	markStart = '\x00'
	markEnd   = '\x01'
)

type posTable struct {
	pos []token.Pos
}

func (t *posTable) mark(p token.Pos) string {
	if !p.IsValid() {
		return ""
	}
	t.pos = append(t.pos, p)
	return string(markStart) + strconv.Itoa(len(t.pos)-1) + string(markEnd)
}

type genMapping struct {
	line, col int
	pos       token.Pos
}

// writer accumulates one section of a generated module.
type writer struct {
	tab    *posTable
	buf    strings.Builder
	line   int
	col    int // UTF-16 units
	indent int
	maps   []genMapping
	// raw writers keep position markers in their output; their content is
	// later embedded (e.g. a function literal inside an expression) and the
	// enclosing writer resolves the markers.
	raw bool
}

func newWriter(tab *posTable) *writer { return &writer{tab: tab} }

func newRawWriter(tab *posTable) *writer { return &writer{tab: tab, raw: true} }

func (w *writer) write(s string) {
	if w.raw {
		w.buf.WriteString(s)
		return
	}
	for len(s) > 0 {
		i := strings.IndexAny(s, "\x00\n")
		if i < 0 {
			w.buf.WriteString(s)
			w.col += sourcemap.UTF16Len(s)
			return
		}
		w.buf.WriteString(s[:i])
		w.col += sourcemap.UTF16Len(s[:i])
		if s[i] == '\n' {
			w.buf.WriteByte('\n')
			w.line++
			w.col = 0
			s = s[i+1:]
			continue
		}
		j := strings.IndexByte(s[i:], markEnd)
		id, _ := strconv.Atoi(s[i+1 : i+j])
		w.maps = append(w.maps, genMapping{w.line, w.col, w.tab.pos[id]})
		s = s[i+j+1:]
	}
}

// ln writes one indented line.
func (w *writer) ln(format string, args ...any) {
	w.write(strings.Repeat("  ", w.indent))
	if len(args) > 0 {
		format = fmt.Sprintf(format, args...)
	}
	w.write(format)
	w.write("\n")
}

// append appends another writer's content, shifting its mappings.
func (w *writer) append(o *writer) {
	if w.col != 0 {
		w.write("\n")
	}
	for _, m := range o.maps {
		w.maps = append(w.maps, genMapping{m.line + w.line, m.col, m.pos})
	}
	w.buf.WriteString(o.buf.String())
	w.line += o.line
	w.col = o.col
}

func (w *writer) String() string { return w.buf.String() }

// stripMarks removes position markers from s (for contexts where no mapping
// is wanted, e.g. comments).
func stripMarks(s string) string {
	for {
		i := strings.IndexByte(s, markStart)
		if i < 0 {
			return s
		}
		j := strings.IndexByte(s[i:], markEnd)
		s = s[:i] + s[i+j+1:]
	}
}
