# goesm

[English](README.md)

Go package を、現代の web 向けのネイティブな ES module に compile します。

`goesm` を使うと、普通の Go package を書き、Go module の ecosystem を使い、それを Vite、Vue、Astro などの ESM ベースの toolchain から使える JavaScript module に compile できます。

```go
package cart

type Item struct {
	Price    int
	Quantity int
}

func Total(items []Item) int {
	total := 0

	for _, item := range items {
		total += item.Price * item.Quantity
	}

	return total
}
```

目的は、browser 向けの「Go 風の言語」を作ることではありません。

入力は Go です。

`go.mod`、`go.sum`、package の import、generics、interface、pointer、map、goroutine、channel、`defer`、`panic` / `recover`、reflection などの Go の意味論は、すべて Go 側に属します。

## アーキテクチャ

`goesm` は Go toolchain を source 言語の権威として使います。

```text
.go
 │
 ▼
Go parser / type checker / package loader
 │
 ▼
goesm
Go semantics → TypeScript
 │
 ▼
TypeScript
 │
 ▼
Vite / Rolldown / その他の host toolchain
 │
 ▼
ES Modules
```

TypeScript は実装の詳細であり、中間 target です。

`goesm` は Go parser、Go の型システム、Go Modules、JavaScript の build ecosystem を置き換えようとはしません。

代わりに、Go の意味論を現代の web tooling がすでに理解できる表現へ lowering することに集中します。

## Go プログラムではなく Go package

`goesm` のプロジェクトに `main` package は必要ありません。

これは意図的です。

Go package それ自体が ES module になれます。

```text
example.com/app/cart
        │
        ▼
      goesm
        │
        ▼
   JavaScript ESM
```

アプリケーションの entry point は別の場所にあってかまいません。

たとえば:

```text
index.ts
   │
   ├── Go package
   ├── Go package
   └── Go package
```

あるいは:

```text
Astro / Vue / Vite
        │
        ▼
   Go packages
```

これは TypeScript の ecosystem に近いモデルです。package は再利用可能な module を提供し、どこから実行が始まるかは周囲のアプリケーションが決めます。

`package main` は単体の Go アプリケーションには今後も役立ちますが、`goesm` の基本的な compile 単位ではありません。

基本単位は **Go package** です。

## 構成例

```text
app/
├── go.mod
├── src/
│   ├── cart/
│   │   ├── cart.go
│   │   ├── price.go
│   │   └── model.go
│   └── user/
│       └── user.go
└── web/
    └── index.ts
```

Go package は普通の Go package のままです。

```go
import "example.com/app/src/cart"
```

`.go` ファイルの import、独自の module 構文、JavaScript 風の相対 import は導入しません。

## 目標

`goesm` は既存の Go の開発体験を保つことを目指します。

```text
go.mod
go.sum
go.work
go fmt
gopls
go test
go vet
golangci-lint
govulncheck
```

compiler は並行する tooling を作るのではなく、Go の ecosystem を再利用すべきです。

JavaScript との境界では、別の bundler を実装するのではなく、現代の web tooling を再利用すべきです。

## 目標としないこと

`goesm` は次のものではありません。

- Go に着想を得た別言語
- WebAssembly runtime
- 新しい package manager
- Vite や Rolldown の代替
- Vue や Astro のような framework

framework 固有の統合は別の adapter が担います。

たとえば `gosfc` は次のようなコードを

```vue
<script setup lang="go">
import "example.com/app/src/cart"

total := cart.Total(items)
</script>
```

`goesm` に繋ぎ、SFC と template の層は引き続き Vue が担当します。

## 状況

`goesm` は現在実験段階です。

最初の作業は、型検査済みの普通の Go package を TypeScript に lowering し、Go の意味論と package 境界を保ったままネイティブな ES module として使えることを実証することに集中しています。

この repository の PoC は go/packages + go/types で package を読み込み、TypeScript と小さな runtime (`@goesm/runtime`) に lowering し、esbuild の Go API で ES module を生成します。設計と実装済み・未実装の範囲は [ARCHITECTURE.ja.md](ARCHITECTURE.ja.md)、GopherJS との違いは [docs/gopherjs-comparison.ja.md](docs/gopherjs-comparison.ja.md)、生成される TypeScript / JavaScript は [docs/example-output.ja.md](docs/example-output.ja.md) を参照してください。

### インストール

goesm は Go toolchain (package の読み込みに `go list` を使います) の Go 1.27 以上が必要です。古い `go` でも `GOTOOLCHAIN` により 1.27 が自動でダウンロードされます。Node.js や npm は不要です。runtime (`@goesm/runtime`) は binary に embed され、`goesm build` の出力に bundle されるので、npm package をインストールする必要はありません。

おすすめは module の tool として追加する方法です。コードと同じ toolchain で goesm が build されます。

```sh
go get -tool github.com/goesm-dev/goesm/cmd/goesm@latest
go tool goesm build ./cart       # dist/cart.js
```

`PATH` にインストールする場合:

```sh
go install github.com/goesm-dev/goesm/cmd/goesm@latest
goesm build ./cart
```

Linux / macOS / Windows (amd64 と arm64) の build 済み binary を各 [GitHub release](https://github.com/goesm-dev/goesm/releases) に添付しています。これらは Go 1.27 で build しているので、module がより新しい Go を使う場合は `go get -tool` を使ってください。`goesm version` は goesm の version と、build に使った Go を表示します。

goesm が実験段階のあいだ、release は `v0.0.1-beta.N` という名前の prerelease です。`@latest` は最新のものに解決され、`@v0.0.1-beta.1` のように固定もできます。

### 使い方

入力は普通の Go module の中の、普通の Go package pattern です。

```sh
cd testdata/example
go run ../../cmd/goesm build ./main          # dist/main.js (+ .go を指す .js.map)
go run ../../cmd/goesm build -split ./main   # Go package ごとに 1 つの ES module
go run ../../cmd/goesm emit-ts ./main        # 生成された TypeScript を確認
```

```js
import { Result } from "./dist/main.js";
Result(); // 3
```

[examples/](examples) に、そのまま動く例 (上のカート、標準 library の利用、goroutine) と、それを import する JS があります。

標準 library の package は自身の Go source から compile します。gc runtime に結び付いた少数の package (`runtime`、`internal/reflectlite`、`sync`) は goesm が持つ Go source で置き換えます。goesm がまだ lowering できない標準 library の関数は呼ばれると panic する stub になり、`build` / `emit-ts` がその数を表示します (`-v` で一覧)。

goesm は自身を build した toolchain の go/types を使うので、module が使う toolchain で build してください (たとえば `tool` directive と `go tool goesm`)。この repository の fixture は Go 1.27 を使います。

### テスト

```sh
npm ci --prefix test   # TestOxlint 用の oxlint (手元では任意、CI では必須)
go test ./...          # Go 1.27 以上と Node.js 22 以上が必要
```

- `TestGolden` は fixture の引数なし exported 関数をすべて native Go と goesm が生成した ESM (Node) の両方で実行し、結果の一致を要求します。
- `TestJS` は build した bundle に対して `test/js/*.test.mjs` (node:test) を実行します。
- `TestKnownGaps` は文書化した native Go との差分がまだ存在することを固定します。
- `TestExamples` は `examples/*` を build し、各 `index.mjs` を Node (インストールされていれば Bun でも) で実行して `output.txt` と比較します。
- `TestOxlint` は fixture から build した ESM (bundle と split) を oxlint の correctness ルールで検査し、指摘が 1 件でもあれば失敗します。生成コードのために無効にしている 4 ルールとその理由は `test/lint_test.go` にあります。
- `TestStdlibStatus -v` は標準 library のどの package が lowering でき、そのうち何個の関数が stub かを報告します。
