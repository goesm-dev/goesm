package jscompat

import (
	"math"
	"testing"
)

func TestUTF16(t *testing.T) {
	s := "aあ😀b" // 1 + 3 + 4 + 1 bytes; 1 + 1 + 2 + 1 units
	if n := UTF16Len(s); n != 5 {
		t.Errorf("UTF16Len = %d, want 5", n)
	}
	for _, c := range []struct{ byteOff, u int }{{0, 0}, {1, 1}, {2, 1}, {4, 2}, {6, 2}, {8, 4}, {9, 5}, {99, 5}} {
		if got := UTF16Offset(s, c.byteOff); got != c.u {
			t.Errorf("UTF16Offset(%d) = %d, want %d", c.byteOff, got, c.u)
		}
	}
	for _, c := range []struct{ u, byteOff int }{{0, 0}, {1, 1}, {2, 4}, {3, 4}, {4, 8}, {5, 9}, {6, 9}} {
		if got := ByteOffset(s, c.u); got != c.byteOff {
			t.Errorf("ByteOffset(%d) = %d, want %d", c.u, got, c.byteOff)
		}
	}
}

func TestRound(t *testing.T) {
	// Math.round in V8.
	for _, c := range []struct{ x, want float64 }{
		{2.5, 3}, {-2.5, -2}, {-0.5, math.Copysign(0, -1)}, {0.49999999999999994, 0}, {-0.2, math.Copysign(0, -1)},
		{1.4, 1}, {-1.6, -2}, {4503599627370495.5, 4503599627370496}, {-4503599627370495.5, -4503599627370495}, {1e300, 1e300},
	} {
		got := Round(c.x)
		if got != c.want || math.Signbit(got) != math.Signbit(c.want) {
			t.Errorf("Round(%v) = %v, want %v", c.x, got, c.want)
		}
	}
	if !math.IsNaN(Round(math.NaN())) || !math.IsInf(Round(math.Inf(-1)), -1) {
		t.Error("Round of NaN or -Inf")
	}
}
