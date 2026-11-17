//go:build goesm

// goesm's patch of crypto/rsa's rsa.go: a public key's modulus gets its
// Montgomery constant from the runtime's BigInts (bigmod.NewPublicModulus).
// A public key is converted for every verification and encryption, and the
// constant-time computation of NewModulus took a fifth of verifying an RS256
// JWT.
package rsa

import "crypto/internal/fips140/bigmod"

func fipsPublicKey(pub *PublicKey) (*rsa.PublicKey, error) {
	N, err := bigmod.NewPublicModulus(pub.N.Bytes())
	if err != nil {
		return nil, err
	}
	return &rsa.PublicKey{N: N, E: pub.E}, nil
}
