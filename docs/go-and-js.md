# Where Go and JavaScript differ

[日本語](go-and-js.ja.md)

goesm keeps Go's semantics, so Go code gives the results it gives natively, also where JavaScript computes something differently. Code ported from JavaScript meets these differences in regular expressions, Unicode normalization, rounding, the last bit of some `math` functions, and string offsets. The package `github.com/goesm-dev/goesm/jscompat` gives Go code JavaScript's behaviour where it is needed. Its functions on strings and numbers are plain Go and build everywhere; the others need a `js` build and call the engine, with goesm through `//goesm:import`, with Go's WebAssembly port through `syscall/js`.

## Regular expressions

Go's `regexp` has RE2's syntax: there is no lookahead, lookbehind or backreference, and a pattern that uses one does not compile. goesm already matches a constant pattern that `regexp` accepts with the engine's `RegExp` when that is safe ([use-cases.md](use-cases.md#not-supported-yet)), so such patterns need nothing else. A pattern in JavaScript's syntax is a `jscompat.RegExp`:

```go
var price = jscompat.MustCompileRegExp(`(?<=¥)(\d+)(?!\d*円)`, "u")

price.MatchString("¥1200")            // true
price.FindStringSubmatch("税込¥1200") // ["1200" "1200"]
```

Its methods take and return Go strings and follow JavaScript's rules: a group that did not take part is `""`, empty matches next to a match count (`x*` finds `""`, `"xx"` and `""` in `"axx"`), and `ReplaceAllString` uses JavaScript's replacement patterns (`$1`, `$<name>`, `$&`). It has no methods returning indices, which would be UTF-16 offsets (see below).

## Unicode normalization

The standard library has no Unicode normalization. `golang.org/x/text/unicode/norm` works with goesm but adds its tables to the bundle. `jscompat.Normalize(s, "NFKC")` is JavaScript's `String.prototype.normalize`, with the forms `"NFC"`, `"NFD"`, `"NFKC"` and `"NFKD"`.

## Rounding and Math functions

`math.Round` rounds halves away from zero, and JavaScript's `Math.round` toward positive infinity: `math.Round(-2.5)` is -3, `Math.round(-2.5)` is -2. `jscompat.Round` rounds as `Math.round` does, negative zero included, and is plain Go.

`math.Cos`, `math.Sin`, `math.Exp`, `math.Log` and the other functions that are not exactly rounded use Go's algorithms, so they give native Go's results; the engine's `Math` functions may differ in the last bit. `jscompat.Cos`, `Sin`, `Tan`, `Exp`, `Log`, `Pow` and `Atan2` call the engine's, for results that must match JavaScript code exactly. `math.Sqrt`, `math.Floor` and the other exactly rounded functions are the same in both.

## String offsets

A Go string is UTF-8 bytes, and a JavaScript string UTF-16 code units: `"あ"` has length 3 in Go and 1 in JavaScript, and `"😀"` 4 and 2. Offsets that come from the browser, such as `selectionStart` of an input element or the indices of `String.prototype.indexOf`, count UTF-16 code units. `jscompat.UTF16Offset(s, i)` converts a byte offset of `s` to a UTF-16 offset, `jscompat.ByteOffset(s, u)` the other way, and `jscompat.UTF16Len(s)` is JavaScript's `length` of `s`. An offset between the two halves of a surrogate pair has no Go counterpart; `ByteOffset` returns the start of the character.

```go
start := jscompat.ByteOffset(text, input.Get("selectionStart").Int())
end := jscompat.ByteOffset(text, input.Get("selectionEnd").Int())
selected := text[start:end]
```
