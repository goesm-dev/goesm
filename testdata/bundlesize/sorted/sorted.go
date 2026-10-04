// Package sorted uses only sort.Ints (see TestBundleSize).
package sorted

import "sort"

func Sort(a []int) { sort.Ints(a) }
