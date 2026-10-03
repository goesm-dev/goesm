# cart

[日本語](README.ja.md)

The shopping cart from the top-level README. `cart.go` is plain Go; `index.mjs` builds a `[]Item` from JS, calls `Total`, `Discount` and `Receipt`, and prints [output.txt](output.txt).

```sh
../bin/goesm build -o cart/dist ./cart && node cart/index.mjs   # in examples/
```

Note `Discount`: `2408 * 85 / 100` is 2046 because Go integer division truncates; goesm keeps that (JS would give 2046.8).
