// Package sourcemap builds the generated-TypeScript -> original-.go source map
// (Source Map v3) that goesm attaches to every generated module.
//
// goesm only produces this first hop. esbuild reads it from the generated
// file's sourceMappingURL comment and composes it into the final
// JavaScript -> .go map, so final map emission stays esbuild's job.
package sourcemap

import (
	"encoding/base64"
	"encoding/json"
	"sort"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Mapping maps a generated position to an original position. All lines and
// columns are 0-based; columns are UTF-16 code units as the spec requires.
type Mapping struct {
	GenLine, GenCol int
	Source          int
	SrcLine, SrcCol int
}

// Builder collects sources and mappings.
type Builder struct {
	sources  []string
	contents []string
	srcIndex map[string]int
	lines    map[int][]string // per source: lines, for byte->UTF-16 column conversion
	Mappings []Mapping
}

func NewBuilder() *Builder {
	return &Builder{srcIndex: map[string]int{}, lines: map[int][]string{}}
}

// AddSource registers a source file and returns its index.
func (b *Builder) AddSource(name, content string) int {
	if i, ok := b.srcIndex[name]; ok {
		return i
	}
	i := len(b.sources)
	b.sources = append(b.sources, name)
	b.contents = append(b.contents, content)
	b.srcIndex[name] = i
	b.lines[i] = strings.Split(content, "\n")
	return i
}

// SourceIndex returns the index of a registered source.
func (b *Builder) SourceIndex(name string) (int, bool) {
	i, ok := b.srcIndex[name]
	return i, ok
}

// UTF16Col converts a 1-based byte column on a 1-based line of source i
// (as reported by go/token) into a 0-based UTF-16 column.
func (b *Builder) UTF16Col(i, line, byteCol int) int {
	if byteCol < 1 { // unknown column (a //line directive without one)
		return 0
	}
	lines := b.lines[i]
	if line-1 < 0 || line-1 >= len(lines) {
		return byteCol - 1
	}
	l := lines[line-1]
	if byteCol-1 > len(l) {
		return byteCol - 1
	}
	return UTF16Len(l[:byteCol-1])
}

// UTF16Len returns the length of s in UTF-16 code units.
func UTF16Len(s string) int {
	n := 0
	for _, r := range s {
		if r == utf8.RuneError {
			n++
			continue
		}
		n += len(utf16.Encode([]rune{r}))
	}
	return n
}

// Add records a mapping.
func (b *Builder) Add(m Mapping) { b.Mappings = append(b.Mappings, m) }

type mapJSON struct {
	Version        int      `json:"version"`
	File           string   `json:"file,omitempty"`
	Sources        []string `json:"sources"`
	SourcesContent []string `json:"sourcesContent"`
	Names          []string `json:"names"`
	Mappings       string   `json:"mappings"`
}

// JSON encodes the map.
func (b *Builder) JSON(file string) []byte {
	ms := append([]Mapping(nil), b.Mappings...)
	sort.SliceStable(ms, func(i, j int) bool {
		if ms[i].GenLine != ms[j].GenLine {
			return ms[i].GenLine < ms[j].GenLine
		}
		return ms[i].GenCol < ms[j].GenCol
	})
	var sb strings.Builder
	line, prevCol, prevSrc, prevSrcLine, prevSrcCol := 0, 0, 0, 0, 0
	first := true
	for _, m := range ms {
		for line < m.GenLine {
			sb.WriteByte(';')
			line++
			prevCol = 0
			first = true
		}
		if !first {
			sb.WriteByte(',')
		}
		first = false
		vlq(&sb, m.GenCol-prevCol)
		vlq(&sb, m.Source-prevSrc)
		vlq(&sb, m.SrcLine-prevSrcLine)
		vlq(&sb, m.SrcCol-prevSrcCol)
		prevCol, prevSrc, prevSrcLine, prevSrcCol = m.GenCol, m.Source, m.SrcLine, m.SrcCol
	}
	out, _ := json.Marshal(mapJSON{
		Version:        3,
		File:           file,
		Sources:        b.sources,
		SourcesContent: b.contents,
		Names:          []string{},
		Mappings:       sb.String(),
	})
	return out
}

// InlineComment returns a sourceMappingURL comment embedding the map.
func (b *Builder) InlineComment(file string) string {
	return "//# sourceMappingURL=data:application/json;base64," + base64.StdEncoding.EncodeToString(b.JSON(file)) + "\n"
}

const b64 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"

func vlq(sb *strings.Builder, v int) {
	u := v << 1
	if v < 0 {
		u = (-v << 1) | 1
	}
	for {
		digit := u & 31
		u >>= 5
		if u > 0 {
			digit |= 32
		}
		sb.WriteByte(b64[digit])
		if u == 0 {
			break
		}
	}
}
