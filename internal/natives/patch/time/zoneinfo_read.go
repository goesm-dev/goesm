//go:build goesm

// goesm's patch of package time's zoneinfo_read.go: loading a Location
// from tzdata enables the TZ rules of its extend string in lookup (see
// tzsetHook in patch zoneinfo.go).
package time

// LoadLocationFromTZData returns a Location with the given name
// initialized from the IANA Time Zone database-formatted data.
// The data should be in the format of a standard IANA time zone file
// (for example, the content of /etc/localtime on Unix systems).
//
//goesm:original loadLocationFromTZData
func LoadLocationFromTZData(name string, data []byte) (*Location, error) {
	tzsetHook = tzset
	return loadLocationFromTZData(name, data)
}
