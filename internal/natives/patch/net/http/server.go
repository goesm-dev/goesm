//go:build goesm

// goesm's patch of package net/http's server: a JS host has no sockets for
// net.Listen (the js/wasm port listens on a fake in-process network), but it
// has HTTP servers of its own, and the Fetch API's Request and Response.
// hostServe runs a Handler for one Request and resolves its Response; it is
// what the runtime's fetchHandler calls (Cloudflare Workers, Deno.serve,
// Bun.serve, service workers), and what ListenAndServe hands to the host's
// server (node:http under Node.js, Bun.serve, Deno.serve).
package http

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strconv"
	"sync"
	"syscall/js"
	"time"
)

func init() {
	hostSetServer(js.FuncOf(hostServeJS))
}

// hostSetServer gives the runtime the function that serves a request with a
// Handler (runtime/src/http.ts).
func hostSetServer(serve js.Func)

// hostHandlerOf returns the Go value a JS value holds: the Handler the
// runtime passes back to hostServeJS.
func hostHandlerOf(v js.Value) Handler

// hostServeJS is hostServe as a JS function of (handler, request, info),
// where info is undefined or an object with the remote address and the
// Server, as ListenAndServe's host server passes it. It returns the
// Response, or an Error if the handler panicked before it wrote the header.
func hostServeJS(this js.Value, args []js.Value) any {
	var s *Server
	remote := ""
	signal := true
	if len(args) > 2 && args[2].Truthy() {
		remote = args[2].Get("remoteAddr").String()
		s = hostServers[args[2].Get("server").Int()]
		// Deno.serve's request signal also aborts after a response, and
		// Deno warns about it when the signal is read.
		signal = !args[2].Get("noSignal").Truthy()
	}
	return hostServe(s, hostHandlerOf(args[0]), args[1], remote, signal)
}

// hostServe runs h for the Fetch API Request req, or a request-like object
// with the same method, url, headers (or an array of [name, value] pairs),
// body, arrayBuffer() and signal, and returns the Response once the handler
// has finished or flushed: a handler that flushes (Server-Sent Events,
// streaming RPCs) gets a streamed body, the others one buffered body. The
// request body is read in full before the handler runs. With signal, the
// request's context is canceled when req.signal aborts.
func hostServe(s *Server, h Handler, req js.Value, remote string, signal bool) js.Value {
	ctx, cancel := context.WithCancel(context.Background())
	if s != nil {
		if s.BaseContext != nil {
			ctx, cancel = context.WithCancel(s.BaseContext(hostListenerOf(s)))
		}
		ctx = context.WithValue(ctx, ServerContextKey, s)
	}
	if signal {
		if sig := req.Get("signal"); sig.Truthy() {
			sig.Call("addEventListener", "abort", js.FuncOf(func(js.Value, []js.Value) any {
				cancel()
				return nil
			}))
		}
	}
	r, err := hostRequest(ctx, req, remote)
	if err != nil {
		cancel()
		return js.Global().Get("Response").New("400 Bad Request: "+err.Error(), map[string]any{"status": 400})
	}
	w := &hostResponseWriter{req: r, header: Header{}, ready: make(chan struct{})}
	if h == nil {
		h = DefaultServeMux
	}
	if r.RequestURI == "*" && r.Method == "OPTIONS" {
		h = globalOptionsHandler{}
	}
	go func() {
		defer cancel()
		defer func() {
			if err := recover(); err != nil {
				if err != ErrAbortHandler {
					msg := fmt.Sprintf("http: panic serving %v: %v\n", remote, err)
					if s != nil {
						s.logf("%s", msg)
					} else {
						os.Stderr.WriteString(msg)
					}
				}
				w.abort(err)
				return
			}
			w.finish()
		}()
		h.ServeHTTP(w, r)
	}()
	<-w.ready
	return w.response
}

// hostRequest is the server-side *Request of a Fetch API Request.
func hostRequest(ctx context.Context, req js.Value, remote string) (*Request, error) {
	u, err := url.Parse(req.Get("url").String())
	if err != nil {
		return nil, err
	}
	// Array.from takes both a Headers object and an array of [name, value]
	// pairs. The pairs are read in Go: a JS callback cannot wait for Go code
	// that may block.
	header := Header{}
	pairs := js.Global().Get("Array").Call("from", req.Get("headers"))
	for i, n := 0, pairs.Length(); i < n; i++ {
		kv := pairs.Index(i)
		k := CanonicalHeaderKey(kv.Index(0).String())
		header[k] = append(header[k], kv.Index(1).String())
	}
	var body []byte
	if b := req.Get("body"); b.Truthy() {
		buf, err := hostAwait(req.Call("arrayBuffer"))
		if err != nil {
			return nil, err
		}
		a := js.Global().Get("Uint8Array").New(buf)
		body = make([]byte, a.Get("length").Int())
		js.CopyBytesToGo(body, a)
	}
	requestURI := u.EscapedPath()
	if u.RawQuery != "" || u.ForceQuery {
		requestURI += "?" + u.RawQuery
	}
	r := &Request{
		Method:        req.Get("method").String(),
		URL:           &url.URL{Path: u.Path, RawPath: u.RawPath, RawQuery: u.RawQuery, ForceQuery: u.ForceQuery},
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        header,
		Host:          u.Host,
		RemoteAddr:    remote,
		RequestURI:    requestURI,
		ContentLength: int64(len(body)),
		Body:          NoBody,
		ctx:           ctx,
	}
	if hv, ok := header["Host"]; ok {
		if len(hv) > 0 {
			r.Host = hv[0]
		}
		delete(header, "Host")
	}
	if len(body) > 0 {
		r.Body = io.NopCloser(bytes.NewReader(body))
	}
	if u.Scheme == "https" {
		r.TLS = &tls.ConnectionState{HandshakeComplete: true, ServerName: u.Hostname()}
	}
	return r, nil
}

// hostAwait waits for the JS promise p.
func hostAwait(p js.Value) (js.Value, error) {
	ch := make(chan js.Value, 1)
	errc := make(chan error, 1)
	ok := js.FuncOf(func(this js.Value, args []js.Value) any {
		ch <- args[0]
		return nil
	})
	fail := js.FuncOf(func(this js.Value, args []js.Value) any {
		errc <- js.Error{Value: args[0]}
		return nil
	})
	p.Call("then", ok, fail)
	select {
	case v := <-ch:
		return v, nil
	case err := <-errc:
		return js.Undefined(), err
	}
}

// hostResponseWriter is the ResponseWriter of hostServe. Writes are
// buffered until the handler returns, when they become the Response's body,
// or until it flushes, when the Response gets a ReadableStream that every
// later write and flush goes to.
type hostResponseWriter struct {
	req         *Request
	header      Header
	status      int
	wroteHeader bool
	buf         []byte
	stream      js.Value // the ReadableStream's controller once streaming
	done        bool
	response    js.Value
	ready       chan struct{}
}

func (w *hostResponseWriter) Header() Header { return w.header }

func (w *hostResponseWriter) WriteHeader(code int) {
	if w.done {
		return
	}
	if w.wroteHeader {
		os.Stderr.WriteString("http: superfluous response.WriteHeader call\n")
		return
	}
	checkWriteHeaderCode(code)
	if code >= 100 && code <= 199 && code != StatusSwitchingProtocols {
		return // 1xx informational responses are not sent
	}
	w.wroteHeader = true
	w.status = code
}

func (w *hostResponseWriter) Write(p []byte) (int, error) {
	if w.done {
		return 0, ErrHandlerTimeout
	}
	if !w.wroteHeader {
		w.WriteHeader(StatusOK)
	}
	if !bodyAllowedForStatus(w.status) {
		return 0, ErrBodyNotAllowed
	}
	if w.stream.Truthy() {
		if len(p) > 0 {
			w.stream.Call("enqueue", hostBytes(p))
		}
		return len(p), nil
	}
	w.buf = append(w.buf, p...)
	return len(p), nil
}

func (w *hostResponseWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

// Flush sends the header and what was written so far, and makes the
// response a stream.
func (w *hostResponseWriter) Flush() {
	if w.done {
		return
	}
	if !w.wroteHeader {
		w.WriteHeader(StatusOK)
	}
	if w.stream.Truthy() {
		return
	}
	var ctrl js.Value
	start := js.FuncOf(func(this js.Value, args []js.Value) any {
		ctrl = args[0]
		return nil
	})
	rs := js.Global().Get("ReadableStream").New(map[string]any{"start": start})
	w.stream = ctrl
	if len(w.buf) > 0 {
		ctrl.Call("enqueue", hostBytes(w.buf))
	}
	w.commit(rs)
	w.buf = nil
}

// finish completes the response when the handler returns.
func (w *hostResponseWriter) finish() {
	if !w.wroteHeader {
		w.WriteHeader(StatusOK)
	}
	if w.stream.Truthy() {
		w.done = true
		w.stream.Call("close")
		return
	}
	var body any
	if bodyAllowedForStatus(w.status) && w.status != StatusResetContent {
		if w.header.Get("Content-Length") == "" && w.header.Get("Transfer-Encoding") == "" && (len(w.buf) > 0 || w.req.Method != "HEAD") {
			w.header.Set("Content-Length", strconv.Itoa(len(w.buf)))
		}
		if w.req.Method != "HEAD" {
			body = hostBytes(w.buf)
		}
	}
	w.commit(body)
	w.done = true
}

// abort ends the response after a panic: before the header was sent, the
// host gets an error (Workers answer 500, ListenAndServe closes the
// connection, as Go's server does); after, the stream fails.
func (w *hostResponseWriter) abort(v any) {
	msg := fmt.Sprint(v)
	if w.stream.Truthy() {
		w.done = true
		w.stream.Call("error", js.Global().Get("Error").New(msg))
		return
	}
	if !w.done {
		w.done = true
		w.response = js.Global().Get("Error").New("goesm: http handler panicked: " + msg)
		close(w.ready)
	}
}

// commit creates the Response, with the header as it is now, and hands it
// to hostServe.
func (w *hostResponseWriter) commit(body any) {
	h := w.header
	if _, ok := h["Content-Type"]; !ok && bodyAllowedForStatus(w.status) {
		if len(w.buf) > 0 {
			h.Set("Content-Type", DetectContentType(w.buf))
		}
	}
	if _, ok := h["Date"]; !ok {
		h.Set("Date", time.Now().UTC().Format(TimeFormat))
	}
	headers := js.Global().Get("Headers").New()
	for k, vs := range h {
		for _, v := range vs {
			headers.Call("append", k, v)
		}
	}
	init := map[string]any{"status": w.status, "headers": headers}
	if _, ok := h["Content-Encoding"]; ok {
		// The handler encoded the body itself: Cloudflare Workers would
		// otherwise encode it again.
		init["encodeBody"] = "manual"
	}
	w.response = js.Global().Get("Response").New(body, init)
	close(w.ready)
}

// hostBytes copies b into a new Uint8Array.
func hostBytes(b []byte) js.Value {
	a := js.Global().Get("Uint8Array").New(len(b))
	js.CopyBytesToJS(a, b)
	return a
}

// hostServers are the Servers whose ListenAndServe runs, by the number the
// host server passes to hostServeJS.
var (
	hostMu      sync.Mutex
	hostServers = map[int]*Server{}
	hostNext    = 1
)

// ListenAndServe listens on the TCP network address s.Addr with the host's
// HTTP server (node:http under Node.js, Bun.serve, Deno.serve) and serves
// its requests with s.Handler, each in its own goroutine.
//
// If s.Addr is blank, ":http" is used. ListenAndServe always returns a
// non-nil error. After [Server.Shutdown] or [Server.Close], the returned
// error is [ErrServerClosed].
func (s *Server) ListenAndServe() error {
	if s.shuttingDown() {
		return ErrServerClosed
	}
	addr := s.Addr
	if addr == "" {
		addr = ":http"
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return &net.OpError{Op: "listen", Net: "tcp", Err: err}
	}
	p, err := net.LookupPort("tcp", port)
	if err != nil {
		return &net.OpError{Op: "listen", Net: "tcp", Err: err}
	}
	hostMu.Lock()
	id := hostNext
	hostNext++
	hostServers[id] = s
	hostMu.Unlock()
	l := &hostListener{id: id, done: make(chan error, 1), addr: &net.TCPAddr{Port: p}}
	var ln net.Listener = l
	if !s.trackListener(&ln, true) {
		return ErrServerClosed
	}
	defer s.trackListener(&ln, false)
	defer func() {
		hostMu.Lock()
		delete(hostServers, id)
		hostMu.Unlock()
	}()
	h := s.Handler
	if h == nil {
		h = DefaultServeMux
	}
	listening := func(ip string, port int) {
		l.addr = &net.TCPAddr{IP: net.ParseIP(ip), Port: port}
	}
	done := func(msg string) {
		var err error
		if msg != "" {
			err = &net.OpError{Op: "listen", Net: "tcp", Addr: l.addr, Err: errors.New(msg)}
		}
		select {
		case l.done <- err:
		default:
		}
	}
	l.close = hostListen(id, host, p, h, listening, done)
	err = <-l.done
	if s.shuttingDown() {
		return ErrServerClosed
	}
	if err == nil {
		err = ErrServerClosed
	}
	return err
}

// hostListen starts the host's HTTP server on host:port. It calls
// listening(address, port) once it listens and done(error message, or "")
// when it stops, and returns the function that stops it.
func hostListen(id int, host string, port int, h Handler, listening func(ip string, port int), done func(msg string)) (stop func())

// hostListener is the net.Listener of a ListenAndServe, so that Close and
// Shutdown, which close a Server's listeners, stop the host's server.
type hostListener struct {
	id    int
	addr  *net.TCPAddr
	close func()
	once  sync.Once
	done  chan error
}

func (l *hostListener) Accept() (net.Conn, error) {
	return nil, errors.New("net/http: the host's server accepts the connections")
}

func (l *hostListener) Close() error {
	l.once.Do(l.close)
	return nil
}

func (l *hostListener) Addr() net.Addr { return l.addr }

// hostListenerOf returns a listener of s for BaseContext.
func hostListenerOf(s *Server) net.Listener {
	s.mu.Lock()
	defer s.mu.Unlock()
	for ln := range s.listeners {
		return *ln
	}
	return nil
}
