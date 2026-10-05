//go:build goesm

// goesm's patch of package net/http: the standard library talks to a fake
// in-process network instead of the Fetch API under Node.js, for the wasm
// port's tests (go.dev/issue/57613). goesm programs run on real JS hosts:
// Node.js, Bun and browsers all have fetch, so the client always uses it.
package http

import (
	"context"
	"net"
	"syscall/js"
)

var jsFetchDisabled = false

// fetchTransport is the receiver of the original RoundTrip for the requests
// RoundTrip sends with fetch: a Transport without dialers, which the fetch
// path reads nothing else of.
var fetchTransport = &Transport{}

// RoundTrip sends the request with fetch, like the js/wasm port's
// RoundTrip, also when the Transport's dialers are those of a net.Dialer
// that only tunes timeouts and keep-alives. The port dials with them, which
// reaches nothing but its in-process network; fetch is what such a
// Transport means on a JS host (the port's DefaultTransport has no
// DialContext for the same reason). Exporters such as OpenTelemetry's
// OTLP/HTTP build their Transport that way.
//
// The original RoundTrip is kept and called, so code that instruments it
// (otelc) runs once either way.
//
//goesm:original roundTripPort
func (t *Transport) RoundTrip(req *Request) (*Response, error) {
	if t.Dial != nil || t.DialContext != nil || t.DialTLS != nil || t.DialTLSContext != nil {
		if t.DialTLS != nil || t.DialTLSContext != nil ||
			(t.Dial != nil && !plainDialer(dialerOf(t.Dial))) ||
			(t.DialContext != nil && !plainDialer(dialerOfContext(t.DialContext))) {
			return t.roundTripPort(req)
		}
		t = fetchTransport
	}
	if fetchManualRedirect && !jsFetchMissing && req.Header.Get(jsFetchRedirect) == "" {
		req = req.Clone(req.Context())
		req.Header.Set(jsFetchRedirect, "manual")
	}
	return t.roundTripPort(req)
}

// fetchManualRedirect reports whether fetch hands redirects to the caller
// with redirect: "manual", as it does outside browsers (Node.js, Bun, Deno,
// Cloudflare Workers). There RoundTrip asks for it, so that http.Client
// follows redirects itself, as natively: CheckRedirect, the cookie jar and
// the limit of 10 apply. A browser only returns an opaque response for a
// manual redirect, so there fetch follows them.
var fetchManualRedirect = func() bool {
	g := js.Global()
	if g.Get("document").Truthy() {
		return false
	}
	return g.Get("process").Truthy() || g.Get("Deno").Truthy() || g.Get("Bun").Truthy() ||
		g.Get("navigator").Truthy() && g.Get("navigator").Get("userAgent").String() == "Cloudflare-Workers"
}()

// dialerOf and dialerOfContext return the receiver of dial if it is a
// method value of (*net.Dialer).Dial or DialContext, or nil.
func dialerOf(dial func(network, addr string) (net.Conn, error)) *net.Dialer
func dialerOfContext(dial func(ctx context.Context, network, addr string) (net.Conn, error)) *net.Dialer

// plainDialer reports whether d dials nothing but the address it is given.
func plainDialer(d *net.Dialer) bool {
	return d != nil && d.LocalAddr == nil && d.Resolver == nil && d.Control == nil && d.ControlContext == nil
}
