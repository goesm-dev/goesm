//go:build goesm

// Package bbig is goesm's replacement for crypto/internal/boring/bbig. The
// original reinterprets a big.Int's words as uints through unsafe; under
// goesm big.Word is uint32, not uint, so the words are copied. BoringCrypto
// is never enabled under goesm.
package bbig

import (
	"crypto/internal/boring"
	"math/big"
)

func Enc(b *big.Int) boring.BigInt {
	if b == nil {
		return nil
	}
	x := b.Bits()
	out := make(boring.BigInt, len(x))
	for i, w := range x {
		out[i] = uint(w)
	}
	return out
}

func Dec(b boring.BigInt) *big.Int {
	if b == nil {
		return nil
	}
	x := make([]big.Word, len(b))
	for i, w := range b {
		x[i] = big.Word(w)
	}
	return new(big.Int).SetBits(x)
}
