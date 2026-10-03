// Uses the Go package example.com/examples/workers from JavaScript.
// Build it first (see README.md): goesm build -o workers/dist ./workers
import * as workers from "./dist/workers.js";

console.log("Square(12):", workers.Square(12));

// SumSquares blocks on channels, so goesm made it async.
const p = workers.SumSquares(100, 4);
console.log("SumSquares returns a Promise:", p instanceof Promise);
console.log("SumSquares(100, 4):", await p);
console.log("Count(8, 1000):", await workers.Count(8, 1000));
