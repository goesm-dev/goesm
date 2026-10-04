# goesm

[![CI](https://github.com/goesm-dev/goesm/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/goesm-dev/goesm/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/goesm-dev/goesm.svg)](https://pkg.go.dev/github.com/goesm-dev/goesm)
[![Go version](https://img.shields.io/github/go-mod/go-version/goesm-dev/goesm)](go.mod)
[![Release](https://img.shields.io/github/v/release/goesm-dev/goesm?include_prereleases&sort=semver)](https://github.com/goesm-dev/goesm/releases)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue)](LICENSE)

[English](README.md)

Go のパッケージを、ネイティブな ES モジュールにコンパイルします。出力は素の JavaScript（TypeScript として出力）で、WebAssembly は使いません。

goesm は、普通の Go モジュールにある普通の Go パッケージを、パッケージごとに 1 つの ES モジュールに変換します。Vite、Rolldown、esbuild、Bun、Node.js、ブラウザからそのまま import できます。export された Go の関数は JavaScript の関数に、export された型は TypeScript の型付きのクラスになります。Go の意味論（整数演算、スライス、マップ、インターフェース、goroutine、`defer` / `panic` / `recover`、ジェネリクス、リフレクション）は保たれ、ネイティブ Go と突き合わせて検証しています。

> [!NOTE]
> goesm は実験段階です。現時点で動くもの・動かないものは[現状](#現状)を参照してください。

```go
package cart

type Item struct {
	Name     string
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

func Discount(total, percent int) int {
	return total * (100 - percent) / 100
}
```

```ts
import { Discount, Item, Total, $runtime as rt } from "./goesm-ts/example.com/app/cart.ts";

const items = rt.sliceLit([new Item(rt.fromJSString("apple"), 120, 3), new Item(rt.fromJSString("bread"), 250, 1)]);
Total(items);          // 610
Discount(2408, 15);    // 2046: Go の整数除算。2046.8 ではない
```

## goesm を使う理由

- **Go の関数がそのまま JavaScript の関数になります。** WebAssembly のインスタンスも、`wasm_exec.js` も、非同期のインスタンス化も、`syscall/js` 経由の値の変換もありません。`Discount` の呼び出しコストは普通の JS 関数の呼び出しと同じで、数値・真偽値・構造体はそのまま境界を越えます。
- **Go パッケージ 1 つが ES モジュール 1 つになります。** 出力は、相対 `.ts` 指定子で互いを import する TypeScript モジュールのツリーです。tree shaking、コード分割、minify、（`.go` ファイルまで戻る）ソースマップはホストのバンドラーが受け持ち、TypeScript からは Go の API の型が見えます。
- **Go は Go のままです。** goesm はフロントエンドとして Go のツールチェーン自体（go/packages、go/types）を使います。同じコードに対して `go.mod`、`go.work`、`gopls`、`go vet`、`go test` がそのまま使え、goesm 独自の構文はありません。標準ライブラリも Go 自身のソースからコンパイルします。
- **ネイティブ Go と突き合わせて検証しています。** すべてのフィクスチャの結果を `go run` と比較し、Go 自身のテストスイート（`$GOROOT/test`）も goesm で実行しています。

## 現状

PoC は Go 言語の大部分と、標準ライブラリのかなりの部分をコンパイルできます。

- **言語**: 関数とクロージャ、構造体、配列、スライス、マップ、ポインタ、インターフェース、型 switch、ジェネリクス（Go 1.27 のジェネリックメソッドを含む）、メソッド値、`defer` / `panic` / `recover`、goroutine、チャネル、`select`、整数と関数に対する range、（前方への）`goto`、ラベル付き文、パッケージの初期化順序。
- **標準ライブラリ**（Go のソースからコンパイル）: `strings`、`strconv`、`unicode`、`sort`、`slices`、`maps`、`errors`、`math`、`math/bits`、`fmt`、`reflect`、`encoding/json`、`sync`、`time`（ホストのタイマー上で動作）、`os` の標準入出力など。goesm がまだ変換できない関数は、呼ぶと panic するスタブになります。`goesm build -v` で一覧できます。
- **Go のテストスイート**: `$GOROOT/test` の実行可能なテスト 961 件のうち 888 件で、ネイティブ Go と同じ出力になります（[docs/conformance.ja.md](docs/conformance.ja.md)）。

まだできないこと（詳細は [ARCHITECTURE.ja.md §11](ARCHITECTURE.ja.md)）:

- `int` と `uint` は JS の number です。2^53 未満では正確ですが、64 ビットのオーバーフローで折り返しません。`int64` と `uint64` は正確です（BigInt）。
- JS 呼び出し ABI はまだありません。Go の文字列とスライスはランタイムのオブジェクトなので、各モジュールが再 export しているランタイム（`rt.fromJSString`、`rt.sliceLit`、`rt.toArray` など）で手作業で変換します。ブロックしうる関数は Promise を返します。
- `iter.Pull`、後方への `goto`、goroutine ごとの `recover` の状態、ホストに保留中の処理があるときのデッドロック検出、DOM バインディング。

## インストール

goesm には Go のツールチェーン（Go 1.27 以降。古い `go` は `GOTOOLCHAIN` で 1.27 を自動でダウンロードします）が必要です。Node.js や npm は不要です。ランタイム（`@goesm/runtime`）はバイナリに埋め込まれていて、出力に書き出されます。

自分のモジュールのツールとして追加すると、自分のコードと同じツールチェーンでビルドされます。

```sh
go get -tool github.com/goesm-dev/goesm/cmd/goesm@latest
go tool goesm emit-ts ./cart     # goesm-ts/<cart の import パス>.ts + goesm-ts/@goesm/runtime/
```

`PATH` にインストールすることもできます。

```sh
go install github.com/goesm-dev/goesm/cmd/goesm@latest
```

ビルド済みバイナリはありません。goesm はどのみち `go` を実行しますし、自分のツールチェーンでビルドすれば goesm の go/types がモジュールの使う Go と揃います。実験段階の間、リリースは `v0.0.1-beta.N` という名前のプレリリースです（[GitHub Releases](https://github.com/goesm-dev/goesm/releases)）。`@latest` は最新のものを指します。`goesm version` で goesm のバージョンと、ビルドに使った Go を表示します。

## 使い方

### `goesm emit-ts`: バンドラー向けの TypeScript ツリー

入力は、普通の Go モジュールでの普通の Go パッケージパターンです。

```sh
cd testdata/example
go run ../../cmd/goesm emit-ts -o goesm-ts ./main   # -o の既定は goesm-ts
```

```text
goesm-ts/
├── example.com/app/main.ts    import * as mathx from "./mathx.ts"
├── example.com/app/mathx.ts   import * as $rt from "../../@goesm/runtime/index.ts"
└── @goesm/runtime/
    ├── index.ts
    └── ...                    その他のランタイムのファイル
```

Go パッケージ `p` のモジュールは `<dir>/<p>.ts` です（標準ライブラリも同じで、`strings.ts`、`internal/bytealg.ts` など）。ランタイムは `<dir>/@goesm/runtime/` です（Go の import パスは `@` で始まれません）。モジュールどうしは `.ts` で終わる相対指定子で import し合うので、リゾルバーもプラグインもバンドラーの設定も要りません。エントリのパッケージを自分のコードから import します。

```ts
// Vite プロジェクトの index.ts。または直接実行: bun index.ts / node index.ts（Node.js 22.18 以降）
import { Result } from "./goesm-ts/example.com/app/main.ts";
console.log(Result()); // 3
```

ツリーを import するコードを `tsc` で型検査するには `allowImportingTsExtensions` を有効にします。生成される TypeScript と JavaScript は [docs/example-output.ja.md](docs/example-output.ja.md) にあります。

### `goesm build`: バンドル済みの ES モジュール

`goesm build` は同じツリーを一時ディレクトリに書き出し、esbuild の Go API でバンドルします。バンドラーがない場合（と goesm 自身のテスト）向けです。

```sh
go run ../../cmd/goesm build ./main            # dist/main.js（+ .go を指す .js.map）
go run ../../cmd/goesm build -minify ./main
go run ../../cmd/goesm build -split ./main     # Go パッケージごとに 1 つの ES モジュール: dist/example.com/app/main.js など
```

```js
import { Result } from "./dist/main.js";
Result(); // 3
```

### JavaScript から Go を呼ぶ

- export された関数と型は、パッケージのモジュールの export になり、TypeScript 上も Go の型を持ちます（`Total(items: $rt.S<Item>): number`）。
- 数値と真偽値は JS の number と boolean、`int64` / `uint64` は BigInt です。構造体は、フィールドを順に受け取るコンストラクタを持つクラスです（`new Item(name, price, quantity)`）。
- Go の文字列はバイト列です。渡すときは `rt.fromJSString(s)`、受け取るときは `rt.toJSString(s)` を使います。スライスは `rt.sliceLit([...])` で渡し、`rt.toArray(s)` で受け取ります。複数の戻り値は配列で、`error` は Go のインターフェース値で返ります。
- ブロックしうる関数（チャネル操作、`time.Sleep`、ミューテックスの待ち）は `async function` で Promise を返します。それ以外は同期関数です。

`rt` はランタイムで、すべてのモジュールが `$runtime` として再 export しています。[examples/](examples) に、呼び出し側の JavaScript 付きで実行できる例（cart、標準ライブラリの利用、goroutine）があります。

## 性能

[bench/](bench) では、同じ Go のカーネルを goesm、[GopherJS](https://github.com/gopherjs/gopherjs)、Go 公式の `GOOS=js GOARCH=wasm`、[TinyGo](https://tinygo.org) の wasm ターゲットでコンパイルし、Node.js、Bun、Chromium で実行します。すべての結果をネイティブ Go と照合し、起動時間と出力サイズも計測します。ネイティブ Go と、同じカーネルを JavaScript で手書きしたものを基準として載せています。各カーネルの内容、出力のビルド方法と呼び出し方、公平性についての注意は [bench/README.ja.md](bench/README.ja.md) に、ランタイムごとの全数値は [bench/results/results.md](bench/results/results.md) にあります。

<!-- bench:start -->
<!-- bench:end -->

## 仕組み

```text
.go ──► go/packages + go/types ──► goesm: Go の意味論 → TypeScript ──► バンドラー / ランタイム ──► ES モジュール
         （Go のツールチェーン）                                         （Vite、Rolldown、esbuild、Bun、Node.js）
```

goesm は、ソース言語については Go のツールチェーンを、出力については現代の JS ツールを権威として扱います。Go のパーサー、型システム、モジュールを置き換えることも、バンドラーを実装することもしません。型検査済みの Go を TypeScript と、TypeScript で書いた小さなランタイムに変換します。主な設計判断は次のとおりで、すべて [ARCHITECTURE.ja.md](ARCHITECTURE.ja.md) で説明しています。

- コンパイルの単位は Go のプログラムではなく Go のパッケージです。goesm のプロジェクトに `package main` は要らず、どこから実行を始めるかは JS のアプリケーションが決めます。
- goroutine は協調的です。プログラム全体の解析でブロックしうる関数を見つけ、それらはブロック箇所に `await` を置いた `async` 関数になります。それ以外はすべて素の同期 JavaScript のままです。
- 標準ライブラリのパッケージは Go 自身のソースからコンパイルします。gc ランタイムに結びついた少数のパッケージ（`runtime`、`reflect`、`internal/reflectlite`、`sync`、`syscall/js`）は goesm 独自の Go ソースに置き換え、Go の本体を持たない関数はランタイムで実装します。
- Go の各型は実行時の型記述子を持ち、インターフェース、ジェネリクス（型辞書）、構造体をキーにしたマップ、リフレクションがそれを使います。

設計を GopherJS と比べたものは [docs/gopherjs-comparison.ja.md](docs/gopherjs-comparison.ja.md) にあります。

### 目標と目標外

goesm は、Go の開発体験（`go.mod`、`go.work`、`go fmt`、`gopls`、`go test`、`go vet`、`golangci-lint`、`govulncheck`）をそのまま保ち、出力側では JavaScript エコシステムのツールを再利用することを目指します。

Go 風の言語、WebAssembly ランタイム、パッケージマネージャー、Vite や Rolldown の代替、フレームワークではありません。フレームワークとの統合は別のアダプターの役割です。たとえば `<script setup lang="go">` を持つ Vue SFC は、Go の部分を goesm に渡し、テンプレート層は Vue が受け持つ、という形にできます。

## ドキュメント

- [ARCHITECTURE.ja.md](ARCHITECTURE.ja.md): 設計、値の表現、goroutine、標準ライブラリ、実装済みの範囲とネイティブ Go との違い
- [docs/example-output.ja.md](docs/example-output.ja.md): 生成される TypeScript と JavaScript
- [docs/conformance.ja.md](docs/conformance.ja.md): Go 自身のテストスイートを goesm で実行する
- [docs/gopherjs-comparison.ja.md](docs/gopherjs-comparison.ja.md): GopherJS との違い
- [bench/README.ja.md](bench/README.ja.md): ベンチマーク
- [examples/](examples): 実行できる例
- [CONTRIBUTING.ja.md](CONTRIBUTING.ja.md): 開発環境、方針、プルリクエストに必要なもの
- [docs/releasing.ja.md](docs/releasing.ja.md): リリースの手順

## 開発

```sh
mise install           # mise.toml で固定したバージョンの Go、Node.js、Bun（CI と同じ）
npm ci --prefix test   # TestTSC / TestOxlint 用の tsc と oxlint（ローカルでは任意、CI では必須）
go test ./...          # Go 1.27 以降と Node.js 22.18 以降が必要。Bun は任意
```

`TestGolden` はフィクスチャの引数なしの export 関数をすべてネイティブ Go と goesm でビルドした ESM で実行し、結果が一致することを確認します。残りは `TestExamples`、`TestJS`、`TestTSC`（出力した TypeScript の strict な型検査）、`TestOxlint`、オプトインの `TestGoConformance` が確認します。各テストとプルリクエストに必要なものは [CONTRIBUTING.ja.md](CONTRIBUTING.ja.md) にあります。

## ライセンス

goesm は [BSD 3-Clause License](LICENSE) で公開しています。`emit-ts` と `build` の出力には、コードが使う Go の標準ライブラリのパッケージを Go のソースからコンパイルしたものが含まれ、それらは [Go 自身の BSD スタイルのライセンス](https://go.dev/LICENSE) に従います。
