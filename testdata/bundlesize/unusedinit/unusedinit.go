// Package unusedinit imports a package that imports encoding/json and
// net/url and uses only a function that needs neither (see TestBundleSize).
package unusedinit

import "bundlesize/unusedinit/codec"

func Sum(a, b int) int { return codec.Add(a, b) }
