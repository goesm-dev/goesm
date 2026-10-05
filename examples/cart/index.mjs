// Uses the Go package example.com/examples/cart from JavaScript.
// Build it first (see README.md): goesm build -o cart/dist ./cart
import * as cart from "./dist/cart.js";

// Arguments and results are plain JavaScript values: a []Item is an array
// of objects, a string a JS string.
const items = [
  { Name: "apple", Price: 120, Quantity: 3 },
  { Name: "bread", Price: 250, Quantity: 1 },
  { Name: "coffee", Price: 899, Quantity: 2 },
];

const total = cart.Total(items);
console.log("total:", total);
console.log("15% off:", cart.Discount(total, 15));
console.log(cart.Receipt(items));
