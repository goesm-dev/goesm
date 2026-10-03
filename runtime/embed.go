// Package runtime embeds the @goesm/runtime TypeScript sources so that the
// goesm binary is self-contained. The runtime is written in TypeScript and is
// emitted next to the generated modules (@goesm/runtime/*.ts), for whichever
// bundler consumes them; goesm never executes it.
package runtime

import (
	"embed"
	"regexp"
	"strings"
	"sync"
)

// Files holds runtime/src/*.ts.
//
//go:embed src/*.ts
var Files embed.FS

// NativeName is the export of runtime/src/natives.ts implementing the
// standard library function with the given types.Func.FullName.
func NativeName(fullName string) string {
	var b strings.Builder
	b.WriteString("native$")
	for _, r := range fullName {
		if r == '_' || 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || '0' <= r && r <= '9' {
			b.WriteRune(r)
		} else {
			b.WriteByte('$')
		}
	}
	return b.String()
}

var natives = sync.OnceValue(func() map[string]bool {
	src, _ := Files.ReadFile("src/natives.ts")
	set := map[string]bool{}
	for _, m := range regexp.MustCompile(`function (native\$[\w$]+)\(|(native\$[\w$]+) = `).FindAllSubmatch(src, -1) {
		set[string(m[1])+string(m[2])] = true
	}
	return set
})

// HasNative reports whether the runtime implements the native NativeName(fullName).
func HasNative(fullName string) bool { return natives()[NativeName(fullName)] }
