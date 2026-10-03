// Uses the Go package example.com/examples/textstats from JavaScript.
// Build it first (see README.md): goesm build -o textstats/dist ./textstats
import * as ts from "./dist/textstats.js";

const rt = ts.$runtime;
const str = (s) => rt.fromJSString(s);
const strs = (slice) => rt.toArray(slice).map(rt.toJSString);

const text = "The Go toolchain is the authority. The output is ES modules; the input is Go.";
console.log(strs(ts.TopWords(str(text), 3)).join(" "));
console.log(rt.toJSString(ts.Slug(str("Go パッケージを ES Module に!"))));

// A Go function with results (int, error) returns a [value, error] pair.
// The error is a Go interface value; Error() is called through the runtime.
for (const input of ["1, 2, 3", "1, x", " "]) {
  const [sum, err] = ts.SumCSV(str(input));
  if (err === null) {
    console.log(`SumCSV(${JSON.stringify(input)}) = ${sum}`);
  } else {
    const msg = rt.toJSString(rt.icall(err, "Error")).replaceAll("\n", " / ");
    console.log(`SumCSV(${JSON.stringify(input)}) failed: ${msg} (empty: ${ts.IsEmpty(err)})`);
  }
}
