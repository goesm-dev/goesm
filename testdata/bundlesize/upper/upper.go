// Package upper uses only strings.ToUpper (see TestBundleSize).
package upper

import "strings"

func Upper(s string) string { return strings.ToUpper(s) }
