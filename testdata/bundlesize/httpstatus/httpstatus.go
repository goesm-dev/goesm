// Package httpstatus uses only net/http's constants, as code that maps
// status codes to error kinds does (see TestBundleSize).
package httpstatus

import "net/http"

// Kind names the kind of failure an HTTP status reports.
func Kind(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "invalid"
	case http.StatusUnauthorized, http.StatusForbidden:
		return "denied"
	case http.StatusNotFound:
		return "missing"
	case http.StatusTooManyRequests, http.StatusServiceUnavailable:
		return "retry"
	}
	return "unknown"
}
