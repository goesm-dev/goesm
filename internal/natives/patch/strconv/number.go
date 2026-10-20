//go:build goesm

// goesm's additions to package strconv's number.go: FormatFloat,
// AppendFloat and ParseFloat of a float64 on the engine's number
// formatting and parsing (formatFloat64 and parseFloat64 in
// runtime/src/fmt.ts). The lowering calls them for the calls that name
// bitSize 64 and, for FormatFloat and AppendFloat, a format 'e', 'E',
// 'f', 'g' or 'G' (internal/lower/strconvcall.go), so a program that
// converts only such floats leaves out internal/strconv's own conversions
// and their tables.
package strconv

// formatFloat64JS is FormatFloat(f, fmt, prec, 64) for fmt 'e', 'E', 'f',
// 'g' and 'G'.
func formatFloat64JS(f float64, fmt byte, prec int) string

// parseFloat64JS parses s as ParseFloat(s, 64) does. code is 0, 1 for
// ErrSyntax or 2 for ErrRange.
func parseFloat64JS(s string) (f float64, code int)

// parseFloat64 is ParseFloat(s, 64).
func parseFloat64(s string) (float64, error) {
	f, code := parseFloat64JS(s)
	switch code {
	case 1:
		return 0, syntaxError("ParseFloat", s)
	case 2:
		return f, rangeError("ParseFloat", s)
	}
	return f, nil
}
