// Command server serves package edge's Handler with http.ListenAndServe on
// the port in $PORT, until SIGINT or SIGTERM shuts it down gracefully.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"usecases/edge"
)

func main() {
	srv := &http.Server{Addr: "127.0.0.1:" + os.Getenv("PORT"), Handler: edge.Handler()}
	stopped := make(chan struct{})
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		fmt.Println("shutdown:", srv.Shutdown(context.Background()))
		close(stopped)
	}()
	fmt.Println("listening")
	err := srv.ListenAndServe()
	if !errors.Is(err, http.ErrServerClosed) {
		fmt.Println("error:", err)
		os.Exit(1)
	}
	<-stopped
	fmt.Println("stopped:", err)
}
