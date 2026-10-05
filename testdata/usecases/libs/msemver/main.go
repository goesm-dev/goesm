// Command msemver exercises a popular pure-Go library; TestUseCaseLibraries
// compares its output under goesm with native Go.
package main

import (
	"fmt"
	"sort"

	"github.com/Masterminds/semver/v3"
)

func main() {
	raw := []string{"1.2.3", "1.0", "v2.0.0-beta.1", "1.10.0", "0.9.9+build.7", "nope"}
	var vs []*semver.Version
	for _, r := range raw {
		v, err := semver.NewVersion(r)
		if err != nil {
			fmt.Println(r, "error:", err)
			continue
		}
		vs = append(vs, v)
	}
	sort.Sort(semver.Collection(vs))
	fmt.Println(vs)
	c, err := semver.NewConstraint(">= 1.2, < 2.0 || ^2.0.0-0")
	fmt.Println(err)
	for _, v := range vs {
		ok, errs := c.Validate(v)
		fmt.Println(v, c.Check(v), ok, len(errs))
	}
	v := semver.MustParse("1.2.3")
	fmt.Println(v.IncMinor(), v.IncPatch(), v.Major(), v.Compare(semver.MustParse("1.2.4")))
	c2, _ := semver.NewConstraint("~1.2.x")
	fmt.Println(c2, c2.Check(semver.MustParse("1.2.9")), c2.Check(semver.MustParse("1.3.0")))
}
