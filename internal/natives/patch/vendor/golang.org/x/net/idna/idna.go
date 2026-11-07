//go:build goesm

// goesm's patch of the vendored golang.org/x/net/idna: ToASCII, which
// net/http calls on a host that is not ASCII (httpguts.PunycodeHostPort),
// uses the Punycode profile, which maps and validates nothing. Its general
// process goes through the profiles' mapping and validation, which keep the
// IDNA and bidi tables (about 140 KB of JavaScript) in every program with an
// HTTP client. punycodeToASCII is process for the Punycode profile.
package idna

import "strings"

// ToASCII is a wrapper for Punycode.ToASCII.
func ToASCII(s string) (string, error) {
	return punycodeToASCII(s)
}

// punycodeToASCII is Punycode.process(s, true): the labels with the ACE
// prefix are decoded, and those that are not ASCII encoded.
func punycodeToASCII(s string) (string, error) {
	var err error
	labels := labelIter{orig: s}
	for ; !labels.done(); labels.next() {
		label := labels.label()
		if label == "" || !strings.HasPrefix(label, acePrefix) {
			continue
		}
		enc := label[len(acePrefix):]
		u, err2 := decode(enc)
		if err2 != nil {
			if err == nil {
				err = err2
			}
			// Spec says keep the old label.
			continue
		}
		if unicode16 && err == nil && len(u) > 0 && isASCII(u) {
			err = punyError(enc)
		}
		labels.set(u)
	}
	for labels.reset(); !labels.done(); labels.next() {
		label := labels.label()
		if !ascii(label) {
			a, err2 := encode(acePrefix, label)
			if err == nil {
				err = err2
			}
			labels.set(a)
		}
	}
	return labels.result(), err
}
