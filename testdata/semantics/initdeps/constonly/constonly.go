// Package constonly is imported only for a constant that go/types folds, so
// nothing in the importer references it at run time.
package constonly

import "example.com/sem/initdeps/registry"

const K = 7

func init() { registry.Inited = append(registry.Inited, "constonly") }
