// Package codec decodes values and checks links, and adds numbers.
package codec

import (
	"encoding/json"
	"net/url"
)

// Item is a decoded value.
type Item struct {
	Name string `json:"name"`
	Link string `json:"link"`
}

// Decode decodes the items in data.
func Decode(data []byte) ([]Item, error) {
	var items []Item
	err := json.Unmarshal(data, &items)
	return items, err
}

// IsURL reports whether s is an absolute URL.
func IsURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme != "" && u.Host != ""
}

// Add returns a + b.
func Add(a, b int) int { return a + b }
