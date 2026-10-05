// Command uuid exercises a popular pure-Go library; TestUseCaseLibraries
// compares its output under goesm with native Go.
package main

import (
	"fmt"

	"github.com/google/uuid"
)

func main() {
	u, err := uuid.Parse("f47ac10b-58cc-4372-a567-0e02b2c3d479")
	fmt.Println(u, err, u.Version(), u.Variant())
	fmt.Println(uuid.NewSHA1(uuid.NameSpaceDNS, []byte("example.com")))
	fmt.Println(uuid.NewMD5(uuid.NameSpaceURL, []byte("https://go.dev")))
	r := uuid.New()
	fmt.Println("random v4:", r.Version(), len(r.String()), r != uuid.New())
	v7, err := uuid.NewV7()
	fmt.Println("v7:", v7.Version(), err)
	_, err = uuid.Parse("nope")
	fmt.Println(err)
	b, _ := u.MarshalText()
	fmt.Println(string(b), uuid.Must(uuid.FromBytes(u[:])) == u)
}
