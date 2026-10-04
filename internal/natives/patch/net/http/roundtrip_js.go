//go:build goesm

// goesm's patch of package net/http: the standard library talks to a fake
// in-process network instead of the Fetch API under Node.js, for the wasm
// port's tests (go.dev/issue/57613). goesm programs run on real JS hosts:
// Node.js, Bun and browsers all have fetch, so the client always uses it.
package http

var jsFetchDisabled = false
