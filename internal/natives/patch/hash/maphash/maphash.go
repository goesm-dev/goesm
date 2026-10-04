//go:build goesm

// goesm's patch of package hash/maphash: the gc implementation hashes raw
// memory with the runtime's hashers, which goesm has no address space for.
// The goesm runtime hashes byte strings and comparable values itself.
package maphash

func hashBytes(b []byte, seed uint64) uint64
func hashString(s string, seed uint64) uint64
func hashComparable(v any, seed uint64) uint64

func rthash(buf []byte, seed uint64) uint64 {
	if len(buf) == 0 {
		return seed
	}
	return hashBytes(buf, seed)
}

func rthashString(s string, state uint64) uint64 {
	if len(s) == 0 {
		return state
	}
	return hashString(s, state)
}

func comparableHash[T comparable](v T, seed Seed) uint64 {
	return hashComparable(any(v), seed.s)
}

func runtime_memhash(p unsafe.Pointer, seed, s uintptr) uintptr { panic("unreachable") }
