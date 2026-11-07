// Command jsonerrtype looks at the Type field of the errors of decoding
// JSON through reflect.Value only, without fmt, reflect.TypeOf or
// Value.Type: goesm's runtime decodes these calls and makes the errors
// itself, with the TypeOf that package reflect hands it.
package main

import (
	"encoding/json"
	"os"
	"reflect"
	"strconv"
)

func main() {
	var i int
	err := json.Unmarshal([]byte(`"x"`), &i)
	os.Stdout.WriteString(err.Error() + "\n")
	f := reflect.ValueOf(err).Elem().Field(1)
	os.Stdout.WriteString(strconv.FormatBool(f.IsNil()) + "\n")
}
