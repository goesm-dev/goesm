// Command jsonmarshalwait converts the result of json.Marshal to a string
// in a function where the call may block: the value's MarshalJSON waits.
package main

import (
	"encoding/json"
	"fmt"
	"time"
)

type slow struct{}

func (slow) MarshalJSON() ([]byte, error) {
	time.Sleep(time.Millisecond)
	return []byte(`"slow"`), nil
}

func toJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func main() {
	for _, v := range []any{slow{}, []int{1, 2}, map[string]int{"a": 1}} {
		s := toJSON(v)
		fmt.Println(s, len(s), s+"!" == toJSON(v)+"!")
	}
}
