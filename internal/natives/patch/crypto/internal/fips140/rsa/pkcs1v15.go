//go:build goesm

// goesm's patch of crypto/internal/fips140/rsa's pkcs1v15.go: verifying a
// signature raises it to the public exponent with the runtime's BigInts
// (encryptPublic), as the signature, the exponent and the modulus are all
// public. Encryption keeps bigmod's Montgomery multiplications, as the
// plaintext is secret.
package rsa

import "crypto/internal/fips140/bigmod"

func verifyPKCS1v15(pub *PublicKey, hash string, hashed []byte, sig []byte) error {
	if fipsApproved, err := checkPublicKey(pub); err != nil {
		return err
	} else if !fipsApproved {
		fips140.RecordNonApproved()
	}

	// RFC 8017 Section 8.2.2: If the length of the signature S is not k
	// octets (where k is the length in octets of the RSA modulus n), output
	// "invalid signature" and stop.
	if pub.Size() != len(sig) {
		return ErrVerification
	}

	em, err := encryptPublic(pub, sig)
	if err != nil {
		return ErrVerification
	}

	expected, err := pkcs1v15ConstructEM(pub, hash, hashed)
	if err != nil {
		return ErrVerification
	}
	if !bytes.Equal(em, expected) {
		return ErrVerification
	}

	return nil
}

// encryptPublic is encrypt for a public input, a signature being verified.
func encryptPublic(pub *PublicKey, sig []byte) ([]byte, error) {
	m, err := bigmod.NewNat().SetBytes(sig, pub.N)
	if err != nil {
		return nil, err
	}
	return bigmod.NewNat().ExpPublicVarTime(m, uint(pub.E), pub.N).Bytes(pub.N), nil
}
