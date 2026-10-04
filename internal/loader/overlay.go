package loader

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The go command refuses overlays for files beneath GOMODCACHE: the
// sources of dependency modules and of a toolchain the go command
// downloaded (GOTOOLCHAIN). Compile-time instrumentation (goesm -toolexec)
// rewrites exactly such files, so goesm applies that part of an overlay
// itself: the go command lists the packages as they are on disk, and goesm
// parses the files of the overlay instead, adds the added ones to their
// package, and type-checks the result (loadMem).

// memOverlay is the part of an overlay goesm applies itself.
type memOverlay struct {
	files map[string][]byte   // replaced and added files
	added map[string][]string // package dir -> added files, sorted
}

// splitOverlay splits overlay into the files the go command accepts and the
// rest, beneath modcache.
func splitOverlay(overlay map[string][]byte, modcache string) (map[string][]byte, *memOverlay) {
	if modcache == "" || len(overlay) == 0 {
		return overlay, nil
	}
	prefix := filepath.Clean(modcache) + string(filepath.Separator)
	goOverlay := map[string][]byte{}
	mem := &memOverlay{files: map[string][]byte{}, added: map[string][]string{}}
	for f, data := range overlay {
		if !strings.HasPrefix(f, prefix) {
			goOverlay[f] = data
			continue
		}
		mem.files[f] = data
		if _, err := os.Stat(f); os.IsNotExist(err) {
			dir := filepath.Dir(f)
			mem.added[dir] = append(mem.added[dir], f)
		}
	}
	if len(mem.files) == 0 {
		return overlay, nil
	}
	for _, added := range mem.added {
		sort.Strings(added)
	}
	if len(goOverlay) == 0 {
		goOverlay = nil
	}
	return goOverlay, mem
}

// file returns the contents of filename in the overlay.
func (m *memOverlay) file(filename string) ([]byte, bool) {
	if m == nil {
		return nil, false
	}
	d, ok := m.files[filename]
	return d, ok
}
