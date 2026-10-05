// Command semver exercises a popular pure-Go library; TestUseCaseLibraries
// compares its output under goesm with native Go.
package main

import (
	"fmt"

	"golang.org/x/mod/semver"
)

func main() {
	vs := []string{"v1.10.0", "v1.2.3", "v1.2.3-rc.1", "v2.0.0+incompatible", "v1.2", "bad", "v0.0.1-beta.10", "v0.0.1-beta.9"}
	for _, v := range vs {
		fmt.Println(v, semver.IsValid(v), semver.Canonical(v), semver.Major(v), semver.MajorMinor(v), semver.Prerelease(v), semver.Build(v))
	}
	semver.Sort(vs)
	fmt.Println(vs)
	fmt.Println(semver.Compare("v1.2.3", "v1.10.0"), semver.Max("v1.2.3", "v1.2.3-rc.1"))
}
