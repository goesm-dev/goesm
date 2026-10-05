// Package edge is an HTTP backend as one http.Handler: the REST API of
// package api, the Connect service of package greet, and a few endpoints for
// what edge runtimes provide (environment variables, crypto, streaming,
// outbound requests). The same Handler runs under Cloudflare Workers through
// the runtime's fetchHandler, and under Node.js and Bun with
// http.ListenAndServe (package server).
package edge

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"usecases/api"
	"usecases/greet"
)

func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", api.New())
	mux.Handle("/greet.v1.GreetService/", greet.Handler())
	mux.HandleFunc("GET /env", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "GREETING=%q", os.Getenv("GREETING"))
	})
	mux.HandleFunc("GET /sign", func(w http.ResponseWriter, r *http.Request) {
		m := hmac.New(sha256.New, []byte("key"))
		m.Write([]byte(r.URL.Query().Get("msg")))
		fmt.Fprint(w, hex.EncodeToString(m.Sum(nil)))
	})
	mux.HandleFunc("GET /events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for i := range 3 {
			fmt.Fprintf(w, "data: %d\n\n", i)
			if err := http.NewResponseController(w).Flush(); err != nil {
				fmt.Fprintln(w, "flush:", err)
			}
			time.Sleep(10 * time.Millisecond)
		}
	})
	mux.HandleFunc("GET /proxy", func(w http.ResponseWriter, r *http.Request) {
		resp, err := http.Get(r.URL.Query().Get("url"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(w, "upstream %d: %s", resp.StatusCode, b)
	})
	return mux
}
