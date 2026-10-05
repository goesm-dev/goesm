// Command gjson exercises a popular pure-Go library; TestUseCaseLibraries
// compares its output under goesm with native Go.
package main

import (
	"fmt"

	"github.com/tidwall/gjson"
)

const j = `{"name":{"first":"Tom","last":"Anderson"},"age":37,"children":["Sara","Alex","Jack"],"fav.movie":"Deer Hunter","friends":[{"first":"Dale","last":"Murphy","age":44,"nets":["ig","fb","tw"]},{"first":"Roger","last":"Craig","age":68,"nets":["fb","tw"]},{"first":"Jane","last":"Murphy","age":47,"nets":["ig","tw"]}],"big":12345678901234567890,"f":1.5e3,"u":"é😀"}`

func main() {
	for _, p := range []string{"name.last", "age", "children", "children.#", "children.1", "child*.2", "c?ildren.0", `fav\.movie`, "friends.#.first", "friends.1.last", `friends.#(last=="Murphy").first`, `friends.#(last=="Murphy")#.first`, `friends.#(age>45)#.last`, `friends.#(nets.#(=="fb"))#.first`, "children|@reverse", "children|@reverse|0", "@pretty:{\"indent\":\" \"}", "{name.first,age}", "big", "f", "u", "missing"} {
		r := gjson.Get(j, p)
		fmt.Printf("%s => %s | %v %v\n", p, r.Raw, r.Type, r.Exists())
	}
	fmt.Println(gjson.Get(j, "age").Int(), gjson.Get(j, "big").Uint(), gjson.Get(j, "f").Float(), gjson.Get(j, "u").String())
	gjson.Get(j, "friends").ForEach(func(k, v gjson.Result) bool { fmt.Println(k.Int(), v.Get("first")); return true })
	fmt.Println(gjson.Valid(j), gjson.Valid("{"), gjson.GetMany(j, "age", "name.first"))
}
