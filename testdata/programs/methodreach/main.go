// Command methodreach checks what the method tables keep when an interface
// method is never called, as the Dump methods of example.com/reachtree: the
// entries stay for type assertions. It also checks how regexp parses
// Unicode classes of patterns known only at run time.
package main

import (
	"fmt"
	"os"
	"regexp"

	tree "example.com/reachtree"
)

type Dumper interface{ Dump(level int) string }

var word = `[a-z]+`

func main() {
	var n tree.Node = &tree.Elem{Kids: []tree.Node{tree.Text("a"), tree.Wrap{Node: tree.Text("b")}}}
	_, isDumper := n.(Dumper)
	_, wrapDumper := any(tree.Wrap{Node: tree.Text("c")}).(Dumper)
	var x any = tree.Text("d")
	_, textNode := x.(tree.Node)
	fmt.Println(n.Kind(), isDumper, wrapDumper, textNode, tree.Wrap{Node: tree.Text("e")}.Kind(), n)

	// A pattern from a variable, without a Unicode class.
	fmt.Println(regexp.MustCompile("^" + word + "$").MatchString("abc"))
	// Patterns known only at run time may have one.
	class := `\p{Greek}+`
	if len(os.Args) > 5 {
		class = word
	}
	re := regexp.MustCompile(class)
	fmt.Println(re.FindString("abc αβγ def"))
	_, err := regexp.Compile(class[:len(class)-3] + "Nope}")
	fmt.Println(err)
}
