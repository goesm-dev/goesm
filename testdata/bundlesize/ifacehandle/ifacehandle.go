// Package ifacehandle returns values of type any to JavaScript, which can
// hold a handle of any type, and imports a package that keeps a value of
// its own interface type in Go (see TestBundleSize).
package ifacehandle

import "bundlesize/ifacehandle/items"

// Get returns the name of the current item, or nil.
func Get(key string) any {
	if key == "name" {
		return items.Name()
	}
	return nil
}
