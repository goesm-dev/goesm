// Package jsondecode only decodes JSON into a plain struct type, which
// the runtime does without encoding/json.
package jsondecode

import "encoding/json"

type Item struct {
	Name  string `json:"name"`
	Price int    `json:"price"`
}

// Total returns the sum of the prices of the items in s.
func Total(s string) int {
	var items []Item
	if err := json.Unmarshal([]byte(s), &items); err != nil {
		return -1
	}
	t := 0
	for _, it := range items {
		t += it.Price
	}
	return t
}
