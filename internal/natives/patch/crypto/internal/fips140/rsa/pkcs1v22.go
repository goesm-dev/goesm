//go:build goesm

// goesm's patch of crypto/internal/fips140/rsa's pkcs1v22.go: verifying a
// PSS signature uses encryptPublic (the patch of pkcs1v15.go).
package rsa

func verifyPSS(pub *PublicKey, hash hash.Hash, digest []byte, sig []byte, saltLength int) error {
	fipsSelfTest()
	fips140.RecordApproved()
	checkApprovedHash(hash)
	if fipsApproved, err := checkPublicKey(pub); err != nil {
		return err
	} else if !fipsApproved {
		fips140.RecordNonApproved()
	}

	if len(sig) != pub.Size() {
		return ErrVerification
	}

	emBits := pub.N.BitLen() - 1
	emLen := (emBits + 7) / 8
	em, err := encryptPublic(pub, sig)
	if err != nil {
		return ErrVerification
	}

	// Like in signPSSWithSalt, deal with mismatches between emLen and the size
	// of the modulus. The spec would have us wire emLen into the encoding
	// function, but we'd rather always encode to the size of the modulus and
	// then strip leading zeroes if necessary. This only happens for weird
	// modulus sizes anyway.
	for len(em) > emLen && len(em) > 0 {
		if em[0] != 0 {
			return ErrVerification
		}
		em = em[1:]
	}

	return emsaPSSVerify(digest, em, emBits, saltLength, hash)
}
