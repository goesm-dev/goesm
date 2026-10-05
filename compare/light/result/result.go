// Package result validates order lines as ../../result does, with error
// types of its own instead of fmt.Errorf: each one's Error method builds
// its message by concatenation, and Unwrap keeps errors.Is working.
// fmt formats any value through reflect, which a bundle then carries.
// js/impl/result.mjs is the same with neverthrow.
package result

import (
	"errors"
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

// A fieldError is an error about a field: "name: err", or "name: value:
// err" for a value out of range.
type fieldError struct {
	name, value string
	err         error
}

func (e *fieldError) Error() string {
	if e.value != "" {
		return e.name + ": " + e.value + ": " + e.err.Error()
	}
	return e.name + ": " + e.err.Error()
}

func (e *fieldError) Unwrap() error { return e.err }

func field(fields map[string]string, name string) (string, error) {
	v, ok := fields[name]
	if !ok {
		return "", &fieldError{name: name, err: ErrMissing}
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
		return 0, &fieldError{name: name, err: err}
	}
	if n < lo || n > hi {
		return 0, &fieldError{name: name, value: strconv.Itoa(n), err: ErrRange}
	}
	return n, nil
}

// ParseOrder parses a line such as "sku=AB-12;qty=3;price=1200".
func ParseOrder(line string) (Order, error) {
	fields := map[string]string{}
	for part := range strings.SplitSeq(line, ";") {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			return Order{}, errors.New("malformed field " + strconv.Quote(part))
		}
		fields[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	sku, err := field(fields, "sku")
	if err != nil {
		return Order{}, err
	}
	if len(sku) < 3 || !strings.Contains(sku, "-") {
		return Order{}, &fieldError{name: "sku " + strconv.Quote(sku), err: ErrRange}
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
			r.Errors = append(r.Errors, "line "+strconv.Itoa(i+1)+": "+err.Error())
			continue
		}
		r.Valid++
		r.Total += o.Qty * o.Price
	}
	return r
}
