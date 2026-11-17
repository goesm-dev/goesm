// Package alpha waits while it initializes.
package alpha

import (
	"time"

	"programs/splitinit/registry"
)

// Seen are the names registered before alpha's.
var Seen string

func init() {
	time.Sleep(2 * time.Millisecond)
	ch := make(chan string)
	go func() { ch <- "alpha" }()
	Seen = registry.Add(<-ch)
}
