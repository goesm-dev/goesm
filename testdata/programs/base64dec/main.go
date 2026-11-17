// Command base64dec decodes base64 of every length up to a few blocks, with
// newlines, padding and corrupt digits at each position, with each encoding,
// strict or not.
package main

import (
	"encoding/base64"
	"fmt"
	"strings"
)

func main() {
	encs := []struct {
		name string
		enc  *base64.Encoding
	}{
		{"std", base64.StdEncoding},
		{"url", base64.URLEncoding},
		{"rawstd", base64.RawStdEncoding},
		{"rawurl", base64.RawURLEncoding},
		{"strict", base64.StdEncoding.Strict()},
		{"rawurlstrict", base64.RawURLEncoding.Strict()},
	}
	data := make([]byte, 40)
	for i := range data {
		data[i] = byte(i*37 + 11)
	}
	for _, e := range encs {
		for n := 0; n <= len(data); n++ {
			s := e.enc.EncodeToString(data[:n])
			b, err := e.enc.DecodeString(s)
			ok := err == nil && string(b) == string(data[:n])
			// With newlines every 5 digits.
			var nl strings.Builder
			for i, c := range s {
				if i > 0 && i%5 == 0 {
					nl.WriteString("\r\n")
				}
				nl.WriteRune(c)
			}
			b2, err2 := e.enc.DecodeString(nl.String())
			// A buffer of exactly the decoded length.
			dst := make([]byte, e.enc.DecodedLen(len(s)))
			m, err3 := e.enc.Decode(dst, []byte(s))
			fmt.Println(e.name, n, ok, string(b2) == string(data[:n]), err2, m, err3)
			// A corrupt digit at each position.
			for i := 0; i < len(s); i += 3 {
				bad := s[:i] + "!" + s[i+1:]
				out, err := e.enc.DecodeString(bad)
				fmt.Println(" corrupt", i, len(out), err)
			}
		}
		// Non-canonical trailing bits and stray padding.
		for _, s := range []string{"QQ", "QR", "QR==", "QUI", "QUJ", "QUJ=", "QUJD=", "=QUJD", "QU=I", "QUJDRA", "QUJDRA=="} {
			out, err := e.enc.DecodeString(s)
			fmt.Printf(" %s %q %v\n", s, out, err)
		}
	}
}
