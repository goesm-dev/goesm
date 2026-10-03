# cart

[English](README.md)

トップの README にあるショッピングカートです。`cart.go` は普通の Go で、`index.mjs` は JS から `[]Item` を作って `Total`、`Discount`、`Receipt` を呼び、[output.txt](output.txt) の内容を出力します。

```sh
../bin/goesm build -o cart/dist ./cart && node cart/index.mjs   # examples/ で実行
```

`Discount` に注目: Go の整数除算は切り捨てなので `2408 * 85 / 100` は 2046 になり、goesm もそれを保ちます (JS のままなら 2046.8)。
