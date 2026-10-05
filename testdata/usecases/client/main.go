// Command client sends a fixed set of requests to package edge's Handler at
// the URL in its first argument and prints what comes back: the REST API,
// the Connect service with each protocol, streaming, environment variables.
// The second argument, if any, is a URL for the /proxy endpoint to fetch.
package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"usecases/greet"
)

var client = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

func do(base, method, path, body string, hdr ...string) {
	req, err := http.NewRequest(method, base+path, strings.NewReader(body))
	if err != nil {
		panic(err)
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("%s %s: error\n", method, path)
		return
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	fmt.Printf("%s %s -> %d\n", method, path, resp.StatusCode)
	for _, k := range []string{"Content-Type", "Content-Length", "Location", "Allow", "Set-Cookie", "X-Powered-By", "X-Content-Type-Options"} {
		if v := resp.Header.Values(k); len(v) > 0 {
			fmt.Printf("  %s: %s\n", k, strings.Join(v, " | "))
		}
	}
	fmt.Printf("  %q\n", b)
}

func main() {
	base := os.Args[1]
	do(base, "GET", "/hello/bob?q=z", "")
	do(base, "POST", "/todos", `{"title":"write docs"}`, "Content-Type", "application/json")
	do(base, "POST", "/todos", `{bad`)
	do(base, "GET", "/todos/1", "")
	do(base, "GET", "/todos/9", "")
	do(base, "DELETE", "/todos/1", "")
	do(base, "POST", "/echo", "abc", "Content-Type", "text/plain", "Cookie", "session=s1")
	do(base, "POST", "/form", "a=1&b=2&b=3", "Content-Type", "application/x-www-form-urlencoded")
	do(base, "GET", "/redirect", "")
	do(base, "GET", "/slow", "")
	do(base, "HEAD", "/hello/head", "")
	do(base, "GET", "/env", "")
	do(base, "GET", "/sign?msg=hello", "")
	do(base, "GET", "/events", "")
	if len(os.Args) > 2 {
		do(base, "GET", "/proxy?url="+os.Args[2], "")
	}
	greet.Run(http.DefaultClient, base)
}
