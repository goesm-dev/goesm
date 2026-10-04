//go:build goesm

// goesm's patch of package crypto/internal/fips140/check: the gc linker
// records the FIPS 140 module's code and data sections (Linkinfo) for this
// init to checksum, but a goesm program has no address space to read them
// from, so the module cannot be verified and FIPS 140 mode cannot be enabled.
package check

func init() {
	if fips140.Enabled {
		panic("fips140: goesm cannot verify the FIPS 140 module")
	}
}
