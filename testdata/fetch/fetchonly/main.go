// Command fetchonly sends HTTP requests through Transports without dialers
// of their own, in TestFetch: nothing in the program gives a Transport a
// custom dialer, so goesm leaves out the Transport's own connections and
// sends every request with fetch.
package main

import (
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
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DialContext = (&net.Dialer{Timeout: time.Second}).DialContext
	fmt.Println("clone:", get(&http.Client{Transport: t}, url+"/clone"))
}
