// Package invoice reads and writes an invoice kept as a TOML file with
// go-toml, as an app does that saves a document as TOML and opens it again.
// js/impl/invoice.mjs does the same with smol-toml.
package invoice

import (
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// File is an invoice: a header, the parties, the products it names and its
// lines, each with the receipts behind it.
type File struct {
	Format   int               `toml:"format"`
	Invoice  Header            `toml:"invoice"`
	Seller   Party             `toml:"seller"`
	Buyer    Party             `toml:"buyer"`
	Products map[string]string `toml:"products"`
	Lines    []Line            `toml:"lines"`
}

type Header struct {
	ID    string `toml:"id"`
	Date  string `toml:"date"`
	Title string `toml:"title"`
	Notes string `toml:"notes"`
	Taxed bool   `toml:"taxed"`
	Tax   int    `toml:"tax_percent"`
}

type Party struct {
	Name    string `toml:"name"`
	Address string `toml:"address"`
	Email   string `toml:"email"`
}

type Line struct {
	SKU      string    `toml:"sku"`
	Product  string    `toml:"product"`
	Text     string    `toml:"text"`
	Qty      int       `toml:"qty"`
	Price    int       `toml:"price"`
	Receipts []Receipt `toml:"receipts"`
}

type Receipt struct {
	Day      string  `toml:"day"`
	Currency string  `toml:"currency"`
	Amount   float64 `toml:"amount"`
	File     string  `toml:"file"`
}

// summary describes f in one line: its id, buyer, first product, lines,
// receipts, subtotal, tax and the sum of the receipts not in JPY.
func summary(f *File) string {
	subtotal, receipts, foreign := 0, 0, 0.0
	for _, l := range f.Lines {
		subtotal += l.Qty * l.Price
		receipts += len(l.Receipts)
		for _, r := range l.Receipts {
			if r.Currency != "JPY" {
				foreign += r.Amount
			}
		}
	}
	tax := 0
	if f.Invoice.Taxed {
		tax = subtotal * f.Invoice.Tax / 100
	}
	return strings.Join([]string{
		f.Invoice.ID, f.Buyer.Name, f.Products[f.Lines[0].Product],
		strconv.Itoa(len(f.Lines)), strconv.Itoa(receipts),
		strconv.Itoa(subtotal), strconv.Itoa(tax), strconv.FormatFloat(foreign, 'f', -1, 64),
	}, "|")
}

// Open reads the invoice src and summarizes it.
func Open(src string) (string, error) {
	var f File
	if err := toml.Unmarshal([]byte(src), &f); err != nil {
		return "", err
	}
	return summary(&f), nil
}

// Resave reads the invoice src, writes it out as TOML again and summarizes
// what reading that back gives.
func Resave(src string) (string, error) {
	var f File
	if err := toml.Unmarshal([]byte(src), &f); err != nil {
		return "", err
	}
	b, err := toml.Marshal(&f)
	if err != nil {
		return "", err
	}
	var g File
	if err := toml.Unmarshal(b, &g); err != nil {
		return "", err
	}
	return summary(&g), nil
}
