// Package cart is the shopping cart from the README: plain Go, used from
// JavaScript as an ES module.
package cart

import (
	"strconv"
	"strings"
)

// Item is one line of the cart. Prices are in cents.
type Item struct {
	Name     string
	Price    int
	Quantity int
}

// Total is the sum of price × quantity over the items.
func Total(items []Item) int {
	total := 0
	for _, item := range items {
		total += item.Price * item.Quantity
	}
	return total
}

// Discount takes percent off a total, rounding down as Go integer division
// does.
func Discount(total, percent int) int {
	return total * (100 - percent) / 100
}

// Receipt formats one line per item and the total.
func Receipt(items []Item) string {
	var b strings.Builder
	for _, it := range items {
		b.WriteString(it.Name + " x" + strconv.Itoa(it.Quantity) + "  " + money(it.Price*it.Quantity) + "\n")
	}
	b.WriteString("total  " + money(Total(items)))
	return b.String()
}

func money(cents int) string {
	frac := strconv.Itoa(cents % 100)
	if len(frac) < 2 {
		frac = "0" + frac
	}
	return strconv.Itoa(cents/100) + "." + frac
}
