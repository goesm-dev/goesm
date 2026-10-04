//go:build goesm

// goesm's patch of package os: Executable is the program's script under a
// host that runs one (Node.js and Bun), which is also os.Args[0].
package os

// hostExecutable returns the absolute path of the program's script, or "".
func hostExecutable() string

func executable() (string, error) {
	if p := hostExecutable(); p != "" {
		return p, nil
	}
	return "", errors.New("Executable not implemented for " + runtime.GOOS)
}
