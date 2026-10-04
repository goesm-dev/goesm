//go:build goesm

// goesm's patch of crypto/internal/fips140/nistec's p256.go: goesm has no
// address space, so the precomputed table cannot alias the bytes of
// p256PrecomputedEmbed. It is decoded instead: each element is stored as
// four little-endian uint64 limbs of its Montgomery form m, which SetBytes
// cannot take directly (it converts to the Montgomery domain), so the value
// m is set and multiplied by R⁻¹ (whose Montgomery form is 1), giving m.
package nistec

func init() {
	var rInv fiat.P256Element
	if _, err := rInv.SetBytes([]byte{
		0xff, 0xff, 0xff, 0xfe, 0x00, 0x00, 0x00, 0x03, 0xff, 0xff, 0xff, 0xfd, 0x00, 0x00, 0x00, 0x02,
		0x00, 0x00, 0x00, 0x01, 0xff, 0xff, 0xff, 0xfe, 0x00, 0x00, 0x00, 0x03, 0x00, 0x00, 0x00, 0x00,
	}); err != nil {
		panic("nistec: internal error: bad R⁻¹")
	}
	tables := new([43]p256AffineTable)
	var be [32]byte
	src := p256PrecomputedEmbed[:]
	element := func(e *fiat.P256Element) {
		for i := range be {
			be[i] = src[31-i]
		}
		src = src[32:]
		if _, err := e.SetBytes(be[:]); err != nil {
			panic("nistec: internal error: bad precomputed table")
		}
		e.Mul(e, &rInv)
	}
	for t := range tables {
		for i := range tables[t] {
			element(&tables[t][i].x)
			element(&tables[t][i].y)
		}
	}
	p256GeneratorTables = tables
}
