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

## 今の JS 側で必要なこと

JS 呼び出し ABI はまだありません (ARCHITECTURE.ja.md §7)。そのため `index.mjs` は、各 module が `$runtime` として re-export している runtime を使って値を手で変換しています。

* Go の string は byte 列: 渡すときは `rt.fromJSString(s)`、受け取るときは `rt.toJSString(s)`。
* slice: 渡すときは `rt.sliceLit([...])`、受け取るときは `rt.toArray(s)`。
* struct は位置引数の constructor を持つ class: `new cart.Item(name, price, quantity)`。
* 多値は配列で返り、`error` は Go の interface 値として返る (`rt.icall(err, "Error")`)。
* block し得る関数 (channel 操作、mutex 待ち) は `async` になり Promise を返す。
