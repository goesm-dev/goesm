// Uses the Go package example.com/examples/cart from JavaScript.
// Build it first (see README.md): goesm build -o cart/dist ./cart
import * as cart from "./dist/cart.js";

// The JS calling ABI is not implemented yet: Go strings are byte strings
// and Go slices are runtime objects, so values are converted by hand with
// the runtime the module re-exports.
const rt = cart.$runtime;
const str = (s) => rt.fromJSString(s);

const items = rt.sliceLit([
  new cart.Item(str("apple"), 120, 3),
  new cart.Item(str("bread"), 250, 1),
  new cart.Item(str("coffee"), 899, 2),
]);

const total = cart.Total(items);
console.log("total:", total);
console.log("15% off:", cart.Discount(total, 15));
console.log(rt.toJSString(cart.Receipt(items)));
