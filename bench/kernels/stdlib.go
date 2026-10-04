package kernels

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// MapInt inserts n int keys into a map, then looks each one up and deletes
// half of them.
func MapInt(n int) int {
	m := make(map[int]int)
	for i := 0; i < n; i++ {
		m[i*7] = i
	}
	sum := 0
	for i := 0; i < n; i++ {
		sum += m[i*7] & 0xff
		if i%2 == 0 {
			delete(m, i*7)
		}
	}
	return sum + len(m)
}

// MapString counts n words drawn from a vocabulary of 1000 in a
// map[string]int: string hashing and comparison.
func MapString(n int) int {
	words := make([]string, 1000)
	for i := range words {
		words[i] = "word" + strconv.Itoa(i*7919)
	}
	counts := make(map[string]int)
	r := lcg(1)
	for i := 0; i < n; i++ {
		counts[words[r.next()%1000]]++
	}
	best := 0
	for _, c := range counts {
		if c > best {
			best = c
		}
	}
	return best*10000 + len(counts)
}

// Strings builds a comma-separated line of n numbers with strings.Builder
// and strconv, splits it, and joins it back in upper case.
func Strings(n int) int {
	var b strings.Builder
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString("id")
		b.WriteString(strconv.Itoa(i))
	}
	parts := strings.Split(b.String(), ",")
	total := 0
	for _, p := range parts {
		if strings.HasSuffix(p, "7") {
			total += len(strings.ToUpper(p))
		}
	}
	return total + len(strings.Join(parts, ";"))
}

// Sort sorts n pseudo-random ints with sort.Ints and n strings with
// sort.Strings.
func Sort(n int) int {
	r := lcg(42)
	ints := make([]int, n)
	strs := make([]string, n)
	for i := range ints {
		ints[i] = int(r.next() >> 2)
		strs[i] = strconv.Itoa(int(r.next() % 100000))
	}
	sort.Ints(ints)
	sort.Strings(strs)
	sum := 0
	for i := 0; i < n; i += n / 100 {
		sum = (sum + ints[i]%1000 + len(strs[i])) % 1000000007
	}
	return sum + len(strs[n/2])
}

// Record is the value JSON encodes and decodes.
type Record struct {
	ID     int               `json:"id"`
	Name   string            `json:"name"`
	Email  string            `json:"email"`
	Active bool              `json:"active"`
	Score  float64           `json:"score"`
	Tags   []string          `json:"tags"`
	Attrs  map[string]string `json:"attrs"`
}

// JSON marshals n records with encoding/json and unmarshals them again.
func JSON(n int) int {
	records := make([]Record, n)
	for i := range records {
		records[i] = Record{
			ID:     i,
			Name:   "user" + strconv.Itoa(i),
			Email:  "user" + strconv.Itoa(i) + "@example.com",
			Active: i%3 == 0,
			Score:  float64(i) * 1.25,
			Tags:   []string{"a", "b", strconv.Itoa(i % 10)},
			Attrs:  map[string]string{"k": "v" + strconv.Itoa(i)},
		}
	}
	data, err := json.Marshal(records)
	if err != nil {
		panic(err)
	}
	var back []Record
	if err := json.Unmarshal(data, &back); err != nil {
		panic(err)
	}
	sum := len(data)
	for _, r := range back {
		sum += r.ID%7 + len(r.Tags) + len(r.Attrs["k"])
	}
	return sum
}

// Sprintf formats n lines with fmt.Sprintf: fmt's reflection-driven
// formatting of ints, strings and floats.
func Sprintf(n int) int {
	total := 0
	for i := 0; i < n; i++ {
		total += len(fmt.Sprintf("%d:%s:%.2f:%x|%v", i, "item", float64(i)/3, i*31, i%2 == 0))
	}
	return total
}

// Channels passes n values through a pipeline of three goroutines connected
// by unbuffered channels and waits for it with a sync.WaitGroup: goroutine
// switches.
func Channels(n int) int {
	src := make(chan int)
	mid := make(chan int)
	out := make(chan int)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for v := range src {
			mid <- v * 2
		}
		close(mid)
	}()
	go func() {
		defer wg.Done()
		for v := range mid {
			out <- v + 1
		}
		close(out)
	}()
	go func() {
		for i := 0; i < n; i++ {
			src <- i
		}
		close(src)
	}()
	sum := 0
	for v := range out {
		sum = (sum + v) % 1000000007
	}
	wg.Wait()
	return sum
}

// Add is the trivial function the call-overhead benchmark calls from JS.
func Add(a, b int) int {
	return a + b
}
