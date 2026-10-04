package kernels

import (
	"encoding/json"
	"strconv"
	"strings"
)

// The API kernels are small functions called many times from JavaScript
// with a string in and a string out, the shape of a typical library API:
// they measure the work together with crossing into the compiled code and
// converting the arguments and results. The suite calls each with the
// inputs of UpperInputs and HandleInputs in turn (see CallChecksum).

// Upper returns s in upper case.
func Upper(s string) string {
	return strings.ToUpper(s)
}

type apiItem struct {
	SKU   string `json:"sku"`
	Price int    `json:"price"`
	Qty   int    `json:"qty"`
}

type apiRequest struct {
	User  string    `json:"user"`
	Items []apiItem `json:"items"`
}

type apiResponse struct {
	User  string `json:"user"`
	Count int    `json:"count"`
	Total int    `json:"total"`
}

// Handle is a JSON request handler: it decodes an order, totals it and
// encodes the reply.
func Handle(req string) string {
	var r apiRequest
	if err := json.Unmarshal([]byte(req), &r); err != nil {
		return `{"error":` + strconv.Quote(err.Error()) + `}`
	}
	resp := apiResponse{User: r.User}
	for _, it := range r.Items {
		resp.Count += it.Qty
		resp.Total += it.Price * it.Qty
	}
	b, _ := json.Marshal(resp)
	return string(b)
}

// UpperInputs returns the strings Upper is called with, as js/suite.mjs
// builds them.
func UpperInputs() []string {
	in := make([]string, 100)
	for j := range in {
		in[j] = "user-" + strconv.Itoa(j) + " visited /items/" + strconv.Itoa(j*7919%10007) + "?ref=home"
	}
	return in
}

// HandleInputs returns the requests Handle is called with, as js/suite.mjs
// builds them.
func HandleInputs() []string {
	in := make([]string, 100)
	for j := range in {
		r := apiRequest{User: "user-" + strconv.Itoa(j)}
		for k := 0; k <= j%8; k++ {
			r.Items = append(r.Items, apiItem{
				SKU:   "S" + strconv.Itoa(j) + "-" + strconv.Itoa(k),
				Price: 100 + (j*37+k*101)%5000,
				Qty:   1 + (j+k)%5,
			})
		}
		b, _ := json.Marshal(r)
		in[j] = string(b)
	}
	return in
}

// CallChecksum calls f with n inputs in turn and folds the lengths and last
// bytes of the results, as the JS harness does around its calls.
func CallChecksum(f func(string) string, inputs []string, n int) int {
	acc := 0
	for i := 0; i < n; i++ {
		out := f(inputs[i%len(inputs)])
		acc = (acc*31 + len(out) + int(out[len(out)-1])) % 1000000007
	}
	return acc
}
