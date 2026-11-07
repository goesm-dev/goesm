// Package httpget sends a request with net/http's client, which goesm
// sends with fetch (see TestBundleSize).
package httpget

import (
	"io"
	"net/http"
)

func Get(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return string(b), err
}
