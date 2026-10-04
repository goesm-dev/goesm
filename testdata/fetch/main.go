// Command fetch sends HTTP requests through Transports with different
// dialers, in TestFetch: under goesm, a Transport without dialers or with a
// plain net.Dialer's uses fetch, and one with a custom dialer dials with it.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"
)

func get(c *http.Client, url string) string {
	resp, err := c.Get(url)
	if err != nil {
		return "error: " + errors.Unwrap(err).Error()
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return fmt.Sprintf("%d %s", resp.StatusCode, b)
}

func main() {
	url := os.Args[1]
	fmt.Println("default:", get(http.DefaultClient, url+"/default"))
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	fmt.Println("net.Dialer:", get(&http.Client{Transport: &http.Transport{
		DialContext: dialer.DialContext,
	}}, url+"/dialer"))
	fmt.Println("custom:", get(&http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return nil, errors.New("custom dialer called")
		},
	}}, url+"/custom"))
}
