// Package runtime embeds the @goesm/runtime TypeScript sources so that the
// goesm binary is self-contained. The runtime is written in TypeScript and is
// compiled by esbuild together with the generated code; goesm never executes it.
package runtime

import "embed"

// Files holds runtime/src/*.ts.
//
//go:embed package.json src/*.ts
var Files embed.FS
