// Command otelc-app is instrumented by otelc (OpenTelemetry's compile-time
// instrumentation) in TestOtelc: its HTTP requests become client spans,
// children of the span it starts by hand, also in a goroutine it starts
// (otelc propagates the span through goroutine-local storage), and its log
// line carries the span's IDs.
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"

	"go.opentelemetry.io/otel"
	_ "go.opentelemetry.io/otel/sdk/trace" // instrumented: spans in goroutine-local storage
)

func get(url string) string {
	resp, err := http.Get(url)
	if err != nil {
		return "error: " + err.Error()
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return fmt.Sprintf("%d %s", resp.StatusCode, b)
}

func main() {
	log.SetFlags(0)
	log.SetOutput(os.Stdout)
	url := os.Args[1]
	_, span := otel.Tracer("otelc-app").Start(context.Background(), "parent")
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		fmt.Println("in goroutine:", get(url+"?from=goroutine"))
	}()
	wg.Wait()
	fmt.Println("in span:", get(url+"?from=main"))
	log.Printf("logged in span")
	span.End()
	fmt.Println("after span:", get(url+"?from=after"))
}
