// Package beta initializes without waiting, after alpha.
package beta

import "programs/splitinit/registry"

// Seen are the names registered before beta's.
var Seen = registry.Add("beta")
