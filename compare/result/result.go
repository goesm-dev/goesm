// Package result validates order lines the way code built on neverthrow
// does, with Go's own error values instead of Result objects: each step
// returns an error, wrapped with the context it failed in.
// js/impl/result.mjs is the same with neverthrow.
package result

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Order is one validated order line.
type Order struct {
	SKU   string
	Qty   int
	Price int
}

// Report is what Check returns: the valid orders' total, and the errors.
type Report struct {
	Valid  int      `json:"valid"`
	Total  int      `json:"total"`
	Errors []string `json:"errors"`
}

var (
	ErrMissing = errors.New("missing field")
	ErrRange   = errors.New("out of range")
)

func field(fields map[string]string, name string) (string, error) {
	v, ok := fields[name]
	if !ok {
		return "", fmt.Errorf("%s: %w", name, ErrMissing)
	}
	return v, nil
}

func number(fields map[string]string, name string, lo, hi int) (int, error) {
	s, err := field(fields, name)
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	if n < lo || n > hi {
		return 0, fmt.Errorf("%s: %d: %w", name, n, ErrRange)
	}
	return n, nil
}

// ParseOrder parses a line such as "sku=AB-12;qty=3;price=1200".
func ParseOrder(line string) (Order, error) {
	fields := map[string]string{}
	for part := range strings.SplitSeq(line, ";") {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			return Order{}, fmt.Errorf("malformed field %q", part)
		}
		fields[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	sku, err := field(fields, "sku")
	if err != nil {
		return Order{}, err
	}
	if len(sku) < 3 || !strings.Contains(sku, "-") {
		return Order{}, fmt.Errorf("sku %q: %w", sku, ErrRange)
	}
	qty, err := number(fields, "qty", 1, 99)
	if err != nil {
		return Order{}, err
	}
	price, err := number(fields, "price", 0, 1_000_000)
	if err != nil {
		return Order{}, err
	}
	return Order{SKU: sku, Qty: qty, Price: price}, nil
}

// Check validates every line and totals the valid ones.
func Check(lines []string) Report {
	r := Report{Errors: []string{}}
	for i, line := range lines {
		o, err := ParseOrder(line)
		if err != nil {
			r.Errors = append(r.Errors, fmt.Sprintf("line %d: %v", i+1, err))
			continue
		}
		r.Valid++
		r.Total += o.Qty * o.Price
	}
	return r
}
