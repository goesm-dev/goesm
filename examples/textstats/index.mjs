// Uses the Go package example.com/examples/textstats from JavaScript.
// Build it first (see README.md): goesm build -o textstats/dist ./textstats
import * as ts from "./dist/textstats.js";

const text = "The Go toolchain is the authority. The output is ES modules; the input is Go.";
console.log(ts.TopWords(text, 3).join(" "));
console.log(ts.Slug("Go パッケージを ES Module に!"));

// A Go function with results (int, error) returns the int, or throws the
// error as a GoError. Passed back to Go, the GoError is the Go error again.
for (const input of ["1, 2, 3", "1, x", " "]) {
  try {
    console.log(`SumCSV(${JSON.stringify(input)}) = ${ts.SumCSV(input)}`);
  } catch (err) {
    const msg = err.message.replaceAll("\n", " / ");
    console.log(`SumCSV(${JSON.stringify(input)}) failed: ${msg} (empty: ${ts.IsEmpty(err)})`);
  }
}
