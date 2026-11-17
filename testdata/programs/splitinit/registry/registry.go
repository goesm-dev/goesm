// Package registry is a registry that packages fill as they initialize.
package registry

import (
	"strings"
	"sync"
)

var (
	mu    sync.RWMutex
	names []string
)

// Add registers name and returns the names registered before it.
func Add(name string) string {
	mu.Lock()
	defer mu.Unlock()
	before := strings.Join(names, ",")
	names = append(names, name)
	return before
}

// Names returns the names registered so far.
func Names() string {
	mu.RLock()
	defer mu.RUnlock()
	return strings.Join(names, ",")
}
