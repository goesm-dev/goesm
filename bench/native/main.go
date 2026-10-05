// Command native runs the kernels as native Go, the reference the JS
// harness checks results against and compares times with. It reads the
// suite (a JSON array of {"name", "arg"}) on stdin and writes one JSON object
// per kernel to stdout, measured the way js/harness.mjs measures: warm up,
// then time single calls until enough samples are taken, and report the
// median.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	"example.com/bench/kernels"
)

var byName = map[string]func(int) int{
	"Fib":           kernels.Fib,
	"Sieve":         kernels.Sieve,
	"Mandelbrot":    kernels.Mandelbrot,
	"FNV32":         kernels.FNV32,
	"FNV64":         kernels.FNV64,
	"NBody":         kernels.NBody,
	"BinaryTrees":   kernels.BinaryTrees,
	"Interfaces":    kernels.Interfaces,
	"MapInt":        kernels.MapInt,
	"MapString":     kernels.MapString,
	"Strings":       kernels.Strings,
	"Sort":          kernels.Sort,
	"JSON":          kernels.JSON,
	"Sprintf":       kernels.Sprintf,
	"Channels":      kernels.Channels,
	"Parallel":      kernels.Parallel,
	"Rand64":        kernels.Rand64,
	"MaybeBlocking": kernels.MaybeBlocking,
	"Pull":          kernels.Pull,
	"RSASign":       kernels.RSASign,
}

// calls are the API kernels: e.Arg calls with the inputs in turn.
var calls = map[string]struct {
	f      func(string) string
	inputs []string
}{
	"Upper":  {kernels.Upper, kernels.UpperInputs()},
	"Handle": {kernels.Handle, kernels.HandleInputs()},
}

type entry struct {
	Name string `json:"name"`
	Arg  int    `json:"arg"`
}

type result struct {
	Name    string    `json:"name"`
	Result  int       `json:"result"`
	Median  float64   `json:"median"`
	Min     float64   `json:"min"`
	Samples int       `json:"samples"`
	Times   []float64 `json:"-"`
}

func main() {
	warmup := flag.Duration("warmup", 300*time.Millisecond, "minimum warm-up time per kernel")
	minTime := flag.Duration("time", time.Second, "minimum measuring time per kernel")
	minSamples := flag.Int("samples", 10, "minimum samples per kernel")
	flag.Parse()

	var suite []entry
	if err := json.NewDecoder(os.Stdin).Decode(&suite); err != nil {
		fmt.Fprintln(os.Stderr, "native: reading the suite:", err)
		os.Exit(1)
	}
	enc := json.NewEncoder(os.Stdout)
	for _, e := range suite {
		if e.Name == "Add" {
			r := measure(func() int {
				s := 0
				for i := 0; i < e.Arg; i++ {
					s = kernels.Add(s, i) & 0xffff
				}
				return s
			}, *warmup, *minTime, *minSamples)
			r.Name = e.Name
			enc.Encode(r)
			continue
		}
		if call, ok := calls[e.Name]; ok {
			r := measure(func() int { return kernels.CallChecksum(call.f, call.inputs, e.Arg) }, *warmup, *minTime, *minSamples)
			r.Name = e.Name
			enc.Encode(r)
			continue
		}
		f, ok := byName[e.Name]
		if !ok {
			fmt.Fprintln(os.Stderr, "native: unknown kernel", e.Name)
			os.Exit(1)
		}
		r := measure(func() int { return f(e.Arg) }, *warmup, *minTime, *minSamples)
		r.Name = e.Name
		enc.Encode(r)
	}
}

func measure(f func() int, warmup, minTime time.Duration, minSamples int) result {
	var r result
	for start, n := time.Now(), 0; n < 3 || time.Since(start) < warmup; n++ {
		r.Result = f()
	}
	var total time.Duration
	for (len(r.Times) < minSamples || total < minTime) && !(len(r.Times) >= 3 && total >= 10*minTime) {
		t := time.Now()
		got := f()
		d := time.Since(t)
		if got != r.Result {
			fmt.Fprintf(os.Stderr, "native: result changed from %d to %d\n", r.Result, got)
			os.Exit(1)
		}
		total += d
		r.Times = append(r.Times, float64(d.Nanoseconds())/1e6)
		if len(r.Times) >= 1000 {
			break
		}
	}
	sorted := append([]float64(nil), r.Times...)
	sort.Float64s(sorted)
	r.Median = median(sorted)
	r.Min = sorted[0]
	r.Samples = len(sorted)
	return r
}

func median(s []float64) float64 {
	if len(s)%2 == 1 {
		return s[len(s)/2]
	}
	return (s[len(s)/2-1] + s[len(s)/2]) / 2
}
