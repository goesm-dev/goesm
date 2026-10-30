//go:build purego

// goesm's patch of the vendored golang.org/x/crypto/internal/alias (used by
// chacha20poly1305): the purego variant compares element addresses from
// reflect.Value.Pointer, which goesm has no address space for, so every pair
// of slices looked overlapping. Two slices overlap when they share a backing
// array and their element ranges intersect.
package alias

// AnyOverlap reports whether x and y share memory at any (not necessarily
// corresponding) index. The memory beyond the slice length is ignored.
func AnyOverlap(x, y []byte) bool
