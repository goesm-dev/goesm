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

TypeScript は goesm の出力であり、host toolchain の入力です。Go package ごとに 1 つの ES module になる生成コードで、手で書いたり編集したりするものではありません。

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

この repository の PoC は go/packages + go/types で package を読み込み、Go package ごとに 1 つの、ESM としてそのまま build できる TypeScript file の tree と、TypeScript で書かれた小さな runtime (`@goesm/runtime`) に lowering します。この tree は任意の bundler (Vite、Rolldown、esbuild) や TypeScript を扱える runtime (Bun、type stripping を使う Node.js) がそのまま読み込めます。`goesm build` は同じ tree を esbuild の Go API で bundle する便宜的な command です。設計と実装済み・未実装の範囲は [ARCHITECTURE.ja.md](ARCHITECTURE.ja.md)、GopherJS との違いは [docs/gopherjs-comparison.ja.md](docs/gopherjs-comparison.ja.md)、生成される TypeScript / JavaScript は [docs/example-output.ja.md](docs/example-output.ja.md) を参照してください。

### インストール

goesm は Go toolchain (package の読み込みに `go list` を使います) の Go 1.27 以上が必要です。古い `go` でも `GOTOOLCHAIN` により 1.27 が自動でダウンロードされます。Node.js や npm は不要です。runtime (`@goesm/runtime`) は binary に embed され、出力に書き出される (`emit-ts` では `<dir>/@goesm/runtime/`、`build` では bundle) ので、npm package をインストールする必要はありません。

おすすめは module の tool として追加する方法です。コードと同じ toolchain で goesm が build されます。

```sh
go get -tool github.com/goesm-dev/goesm/cmd/goesm@latest
go tool goesm emit-ts ./cart     # goesm-ts/<cart の import path>.ts と goesm-ts/@goesm/runtime/
go tool goesm build ./cart       # bundle した dist/cart.js が欲しい場合
```

`PATH` にインストールする場合:

```sh
go install github.com/goesm-dev/goesm/cmd/goesm@latest
goesm emit-ts ./cart
```

build 済み binary は配布していません。goesm はどのみち `go` を実行しますし、自分の toolchain で build すれば goesm の go/types が module の Go とずれません。`goesm version` は goesm の version と、build に使った Go を表示します。release notes は [GitHub Releases](https://github.com/goesm-dev/goesm/releases) にあります。

goesm が実験段階のあいだ、release は `v0.0.1-beta.N` という名前の prerelease です。`@latest` は最新のものに解決され、`@v0.0.1-beta.1` のように固定もできます。

### 使い方

入力は普通の Go module の中の、普通の Go package pattern です。`goesm emit-ts` が TypeScript の tree を書き出します。

```sh
cd testdata/example
go run ../../cmd/goesm emit-ts -o goesm-ts ./main   # -o の既定値は goesm-ts
```

```text
goesm-ts/
├── example.com/app/main.ts    import * as mathx from "./mathx.ts"
├── example.com/app/mathx.ts   import * as $rt from "../../@goesm/runtime/index.ts"
└── @goesm/runtime/
    ├── index.ts
    ├── natives.ts
    └── ...                    その他の runtime file
```

Go package `p` の module は `<dir>/<p>.ts` です (標準 library の package も同様: `strings.ts`、`internal/bytealg.ts`)。runtime は `<dir>/@goesm/runtime/` に置かれます。Go の import path は `@` で始まらないので、runtime が package と衝突することはありません。module 同士は `.ts` で終わる相対 specifier で import し合うため、resolver、plugin、bundler の設定は不要です。自分のコードから entry package を import します。

```ts
// Vite project の index.ts、または直接実行: bun index.ts / node index.ts (Node.js 22.18 以上)
import { Result } from "./goesm-ts/example.com/app/main.ts";
console.log(Result()); // 3
```

この tree を import するコードを `tsc` で型検査するには `allowImportingTsExtensions` を有効にします。

`goesm build` は便宜的な command です (テストもこれを使います)。同じ tree を一時 directory に書き出し、esbuild の Go API で bundle します。

```sh
go run ../../cmd/goesm build ./main          # dist/main.js (+ .go を指す .js.map)
go run ../../cmd/goesm build -split ./main   # Go package ごとに 1 つの ES module: dist/example.com/app/main.js など
```

```js
import { Result } from "./dist/main.js";
Result(); // 3
```

[examples/](examples) に、そのまま動く例 (上のカート、標準 library の利用、goroutine) と、それを import する JS があります。

標準 library の package は自身の Go source から compile します。gc runtime に結び付いた少数の package (`runtime`、`internal/reflectlite`、`sync`) は goesm が持つ Go source で置き換えます。goesm がまだ lowering できない標準 library の関数は呼ばれると panic する stub になり、`build` / `emit-ts` がその数を表示します (`-v` で一覧)。

goesm は自身を build した toolchain の go/types を使うので、module が使う toolchain で build してください (たとえば `tool` directive と `go tool goesm`)。この repository の fixture は Go 1.27 を使います。

### テスト

開発環境の構築、方針、pull request に必要なことは [CONTRIBUTING.ja.md](CONTRIBUTING.ja.md) を参照してください。

```sh
mise install           # mise.toml で pin した Go、Node.js、Bun を入れる (CI も同じ version)
npm ci --prefix test   # TestTSC / TestOxlint 用の tsc と oxlint (手元では任意、CI では必須)
go test ./...          # Go 1.27 以上と Node.js 22 以上が必要、Bun は任意
```

- `TestGolden` は fixture の引数なし exported 関数をすべて native Go と goesm が生成した ESM (Node) の両方で実行し、結果の一致を要求します。
- `TestJS` は build した bundle に対して `test/js/*.test.mjs` (node:test) を実行します。
- `TestKnownGaps` は文書化した native Go との差分がまだ存在することを固定します。
- `TestExamples` は `examples/*` を build し、各 `index.mjs` を Node (インストールされていれば Bun でも) で実行して `output.txt` と比較します。
- `TestOxlint` は fixture から build した ESM (bundle と split) を oxlint の correctness ルールで検査し、指摘が 1 件でもあれば失敗します。生成コードのために無効にしている 4 ルールとその理由は `test/lint_test.go` にあります。
- `TestStdlibStatus -v` は標準 library のどの package が lowering でき、そのうち何個の関数が stub かを報告します。
- `TestTSC` は出力した TypeScript (fixture、examples、runtime) を strict mode の tsc で型検査します。あわせて、exported な Go API が TypeScript から Go の型で見えることを consumer で確認します。`TestOxlint` と同じく `npm ci --prefix test` が必要で、CI では必須です。native Go との結果比較は引き続き意味論の gate です。
- `TestGoConformance`（`GOESM_CONFORMANCE=1` で有効）は Go 本体の `test/` ディレクトリにある `// run` テストを goesm で実行し、native Go が出すべき出力と照合します。[docs/conformance.ja.md](docs/conformance.ja.md) を参照してください。

## ライセンス

goesm は [BSD 3-Clause License](LICENSE) で公開しています。`emit-ts` と `build` の出力には、コードが使う Go 標準 library の package が Go の source から compile されて含まれます。それらは [Go 自身の BSD 系ライセンス](https://go.dev/LICENSE) に従います。
