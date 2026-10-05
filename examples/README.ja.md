# goesm の examples

[English](README.md)

各ディレクトリは module `example.com/examples` の普通の Go package です。goesm が build した ES module を import する `index.mjs` と、その出力 (`output.txt`) が入っています。出力は `TestExamples` が Node と (インストールされていれば) Bun で確認しています。

| example | 内容 |
| --- | --- |
| [cart](cart) | README のショッピングカート: struct、slice、`strconv` / `strings.Builder` |
| [textstats](textstats) | Go source から compile した標準 library: `strings`、`strconv`、`sort`、`unicode`、`errors` (`Is`、`Join`)、`(値, error)` の戻り値 |
| [workers](workers) | goroutine、channel、`sync.WaitGroup` と `sync.Mutex`。block する関数は Promise を返す |

## 実行方法

```sh
# repository の root で
go build -o bin/goesm ./cmd/goesm
cd examples
../bin/goesm build -o cart/dist ./cart
node cart/index.mjs        # または: bun cart/index.mjs
```

`goesm build` は、まだ lowering できず呼ぶと panic する stub になった標準 library 関数の数を表示します。これらの examples はどれも呼びません。`-v` で一覧が出ます。

## Go のコードの呼び出し

`index.mjs` は、export された Go の関数を普通の JavaScript の値で呼びます ([docs/js-exports.ja.md](../docs/js-exports.ja.md))。

* 文字列は JS の文字列、スライスは配列、構造体はプレーンオブジェクトです。たとえば `cart.Total([{ Name: "apple", Price: 120, Quantity: 3 }])` のように呼びます。
* 複数の戻り値は配列で返ります。最後の戻り値の `error` は `GoError` として投げられ、Go に渡し直すと元の Go のエラーに戻ります (`ts.IsEmpty(err)`)。
* チャネル操作やミューテックスの待ちでブロックしうる関数は `async` になり、Promise を返します。
