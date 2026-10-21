//go:build goesm

// goesm's patch of package crypto/internal/fips140's self-tests: FIPS 140
// mode cannot be enabled in a goesm program (see the patch of
// crypto/internal/fips140/check), so the self-tests never run. Without the
// patch, the init functions of the hash, cipher and key derivation packages,
// which pass their self-tests to CAST, keep those algorithms in every bundle
// that imports them, as net/http does through crypto/tls.
package fips140

// CAST runs the named Cryptographic Algorithm Self-Test if operated in FIPS
// mode, which goesm does not support.
func CAST(name string, f func() error) {}

// PCT runs the named Pairwise Consistency Test if operated in FIPS mode,
// which goesm does not support.
func PCT(name string, f func() error) {}
