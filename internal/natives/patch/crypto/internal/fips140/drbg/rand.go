//go:build goesm

// goesm's patch of package crypto/internal/fips140/drbg: FIPS 140 mode
// cannot be enabled in a goesm program (see the patch of
// crypto/internal/fips140/check), so Read always reads from the operating
// system, as Go's does outside FIPS mode. Without the patch, the CTR_DRBG
// that FIPS mode reads through keeps AES in every program that calls
// crypto/rand.Read.
package drbg

import (
	"crypto/internal/fips140"
	"crypto/internal/sysrand"
)

// Read fills b with cryptographically secure random bytes.
func Read(b []byte) {
	if testingReader != nil {
		fips140.RecordNonApproved()
		// Avoid letting b escape in the non-testing case.
		bb := make([]byte, len(b))
		testingReader.Read(bb)
		copy(b, bb)
		return
	}
	sysrand.Read(b)
}
