// Package api is an HTTP API as an http.Handler.
package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type Todo struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Done  bool   `json:"done"`
}

type store struct {
	mu    sync.Mutex
	next  int
	todos map[int]Todo
}

func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		w.Header().Set("X-Powered-By", "goesm")
		next.ServeHTTP(w, r)
		_ = time.Since(start)
	})
}

func New() http.Handler {
	s := &store{next: 1, todos: map[int]Todo{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /hello/{name}", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "hello %s, q=%s", r.PathValue("name"), r.URL.Query().Get("q"))
	})
	mux.HandleFunc("POST /todos", func(w http.ResponseWriter, r *http.Request) {
		var t Todo
		if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		t.ID = s.next
		s.next++
		s.todos[t.ID] = t
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(t)
	})
	mux.HandleFunc("GET /todos/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.Atoi(r.PathValue("id"))
		s.mu.Lock()
		t, ok := s.todos[id]
		s.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(t)
	})
	mux.HandleFunc("POST /echo", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		c, err := r.Cookie("session")
		cv := ""
		if err == nil {
			cv = c.Value
		}
		http.SetCookie(w, &http.Cookie{Name: "seen", Value: "1", Path: "/"})
		fmt.Fprintf(w, "%s %s len=%d ct=%s cookie=%s", r.Method, r.URL.Path, len(b), r.Header.Get("Content-Type"), cv)
	})
	mux.HandleFunc("POST /form", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		fmt.Fprintf(w, "a=%s b=%v", r.FormValue("a"), r.Form["b"])
	})
	mux.HandleFunc("GET /redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/hello/r", http.StatusFound)
	})
	mux.HandleFunc("GET /slow", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(50 * time.Millisecond):
			fmt.Fprint(w, "slow done")
		case <-r.Context().Done():
		}
	})
	return logging(mux)
}
