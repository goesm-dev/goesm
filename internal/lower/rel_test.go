package lower

import "testing"

func TestRelSpecifier(t *testing.T) {
	for _, c := range [][3]string{
		{"example.com/app/main", "example.com/app/mathx.ts", "./mathx.ts"},
		{"example.com/app/main", RuntimeFile, "../../@goesm/runtime/index.ts"},
		{"strings", "internal/bytealg.ts", "./internal/bytealg.ts"},
		{"internal/bytealg", "strings.ts", "../strings.ts"},
		{"strings", RuntimeFile, "./@goesm/runtime/index.ts"},
		{"a/b", "a/b/c.ts", "./b/c.ts"},
		{"a/b/c", "a/b.ts", "../b.ts"},
	} {
		if got := relSpecifier(c[0], c[1]); got != c[2] {
			t.Errorf("relSpecifier(%q, %q) = %q, want %q", c[0], c[1], got, c[2])
		}
	}
}
