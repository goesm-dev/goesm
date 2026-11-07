// Package unusedre imports a package whose variables hold compiled regular
// expressions and uses none of them (see TestBundleSize).
package unusedre

import "bundlesize/unusedre/patterns"

func Sum(a, b int) int { return patterns.Add(a, b) }
