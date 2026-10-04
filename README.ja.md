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

- **言語**: 関数とクロージャ、構造体、配列、スライス、マップ、ポインタ、インターフェース、型 switch、ジェネリクス（Go 1.27 のジェネリックメソッドを含む）、メソッド値、`defer` / `panic` / `recover`、goroutine、チャネル、`select`、整数と関数に対する range、`goto`、ラベル付き文、パッケージの初期化順序。
- **標準ライブラリ**（Go のソースからコンパイル）: `strings`、`strconv`、`unicode`、`sort`、`slices`、`maps`、`errors`、`math`、`math/bits`、`fmt`、`reflect`、`encoding/json`、`sync`、`time`（ホストのタイマー上で動作）、`os` の標準入出力など。goesm がまだ変換できない関数は、呼ぶと panic するスタブになります。`goesm build -v` で一覧できます。
- **compile-time instrumentation**: `goesm build -toolexec "otelc toolexec"` で OpenTelemetry の [otelc](https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation) が計装したプログラムをビルドでき、ネイティブのビルドと同じ span を console や OTLP/HTTP の collector に出力します（[docs/otelc.ja.md](docs/otelc.ja.md)）。package 間の `//go:linkname` と `//go:embed` も動きます。
- **Go のテストスイート**: `$GOROOT/test` の実行可能なテスト 961 件のうち 898 件で、ネイティブ Go と同じ出力になります（[docs/conformance.ja.md](docs/conformance.ja.md)）。

まだできないこと（詳細は [ARCHITECTURE.ja.md §11](ARCHITECTURE.ja.md)）:

- `int` と `uint` は JS の number です。2^53 未満では正確ですが、64 ビットのオーバーフローで折り返しません。`int64` と `uint64` は正確です（BigInt）。
- JS 呼び出し ABI はまだありません。Go の文字列とスライスはランタイムのオブジェクトなので、各モジュールが再 export しているランタイム（`rt.fromJSString`、`rt.sliceLit`、`rt.toArray` など）で手作業で変換します。ブロックしうる関数は Promise を返します。
- goroutine ごとの `recover` の状態、ホストに保留中の処理があるときのデッドロック検出、DOM バインディング。

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

出力は拡張子 `.js` の ES モジュールです。Node.js では、`package.json` に `"type": "module"` があるパッケージから読み込んでください。そうでないと Node.js はモジュール形式を判定するために各モジュールをもう一度パースし、大きなバンドルでは起動に数十ミリ秒が加わります。

### JavaScript から Go を呼ぶ

- export された関数と型は、パッケージのモジュールの export になり、TypeScript 上も Go の型を持ちます（`Total(items: $rt.S<Item>): number`）。
- 数値と真偽値は JS の number と boolean、`int64` / `uint64` は BigInt です。構造体は、フィールドを順に受け取るコンストラクタを持つクラスです（`new Item(name, price, quantity)`）。
- Go の文字列はバイト列です。渡すときは `rt.fromJSString(s)`、受け取るときは `rt.toJSString(s)` を使います。スライスは `rt.sliceLit([...])` で渡し、`rt.toArray(s)` で受け取ります。複数の戻り値は配列で、`error` は Go のインターフェース値で返ります。
- ブロックしうる関数（チャネル操作、`time.Sleep`、ミューテックスの待ち）は `async function` で Promise を返します。それ以外は同期関数です。

`rt` はランタイムで、すべてのモジュールが `$runtime` として再 export しています。[examples/](examples) に、呼び出し側の JavaScript 付きで実行できる例（cart、標準ライブラリの利用、goroutine）があります。

## 性能

[bench/](bench) では、同じ Go のカーネルを goesm、[GopherJS](https://github.com/gopherjs/gopherjs)、Go 公式の `GOOS=js GOARCH=wasm`、[TinyGo](https://tinygo.org) の wasm ターゲットでコンパイルし、Node.js、Bun、Chromium で実行します。すべての結果をネイティブ Go と照合し、起動時間と出力サイズも計測します。ネイティブ Go と、同じカーネルを JavaScript で手書きしたものを基準として載せています。各カーネルの内容、出力のビルド方法と呼び出し方、公平性についての注意は [bench/README.ja.md](bench/README.ja.md) に、ランタイムごとの全数値は [bench/results/results.md](bench/results/results.md) にあります。

<!-- bench:start -->
- Intel(R) Xeon(R) Processor @ 2.80GHz (4 threads), linux 6.18.44-fc-v64
- goesm 4c26158, go version go1.27.1 linux/amd64
- GopherJS 1.21.0+go1.21.13
- tinygo version 0.42.0 linux/amd64 (using go version go1.27.1 and LLVM version 22.1.4)
- Node.js v26.10.0, Bun 1.4.2, Chromium 141.0.7390.37
- 2026-10-04 計測。300 ms 以上ウォームアップしたあと、カーネルごとに 10 回以上かつ 1000 ms 以上計測した中央値

![ネイティブ Go に対する遅さ](bench/results/charts/slowdown.ja.svg)

![合計時間](bench/results/charts/total.ja.svg)

ネイティブ Go に対する遅さ（各カーネルの時間の比の幾何平均。小さいほど速い）:

| ランタイム | 手書き JS | goesm | GopherJS | Go wasm | TinyGo wasm |
| --- | ---: | ---: | ---: | ---: | ---: |
| Node.js v26.10.0 | 0.9× | 3.5× | 11.3× | 3.0× | **1.7×** |
| Bun 1.4.2 | 1.2× | 4.4× | 9.5× | 2.9× | **1.9×** |
| Chromium 141.0.7390.37 | 0.9× | 3.2× | 9.2× | 3.1× | **1.8×** |

全カーネルを 1 回ずつ実行した合計時間（ms、中央値の和。呼び出し系カーネルはループ全体。* はないカーネルを除いた値。小さいほど速い）:

| ランタイム | 手書き JS | goesm | GopherJS | Go wasm | TinyGo wasm |
| --- | ---: | ---: | ---: | ---: | ---: |
| Node.js v26.10.0 | 311* | 1853 | 11198 | 1458 | **855** |
| Bun 1.4.2 | 949* | 3309 | 10377 | 1415 | **998** |
| Chromium 141.0.7390.37 | 284* | 1726 | 9552 | 1618 | **968** |

Node.js v26.10.0 での 1 回あたりの時間（ms、中央値。小さいほど速い）:

| カーネル | 対象 | ネイティブ Go | 手書き JS | goesm | GopherJS | Go wasm | TinyGo wasm |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Fib | 再帰呼び出し、int 演算 | 4.68 | 9.92 | 10.1 | 9.42 | 14.9 | **4.02** |
| Sieve | []bool、密なループ | 8.63 | 10.9 | 38.4 | 78.8 | 14.0 | **10.3** |
| Mandelbrot | float64 のループ | 15.3 | 16.2 | 16.1 | 15.7 | 16.2 | **14.8** |
| NBody | ポインタ経由の構造体の float64 フィールド | 8.49 | 10.7 | 34.9 | 967 | 12.2 | **8.56** |
| FNV32 | uint32 の乗算と xor | 11.2 | 11.4 | 13.4 | 28.7 | 24.9 | **11.0** |
| FNV64 | uint64 の乗算と xor | 11.1 | 46.5 | 45.6 | 337 | 25.2 | **10.6** |
| BinaryTrees | メモリ確保、GC | 66.1 | 48.1 | **56.0** | 76.6 | 262 | 127 |
| Interfaces | インターフェースのメソッド呼び出し | 22.6 | 12.3 | 49.8 | 41.7 | 99.4 | **29.6** |
| MapInt | map[int]int の挿入・検索・削除 | 31.2 | 27.3 | 60.1 | **48.7** | 101 | 114 |
| MapString | map[string]int での集計 | 9.45 | 23.6 | **31.1** | 150 | 34.1 | 77.1 |
| Strings | strings.Builder、strconv、Split、Join | 10.2 | 12.0 | 85.2 | 633 | 34.3 | **13.7** |
| Sort | sort.Ints、sort.Strings | 33.7 | 51.1 | 162 | 204 | 120 | **39.1** |
| JSON | encoding/json の Marshal + Unmarshal | 9.63 | 2.80 | 111 | 674 | 30.9 | **30.9** |
| Sprintf | fmt.Sprintf | 17.2 | 7.02 | 215 | 1604 | 71.5 | **43.2** |
| Channels | goroutine、バッファなしチャネル | 94.4 | — | 110 | 362 | 301 | **29.3** |
| Add (ns/回) | JS からの呼び出し: 数値 2 つを渡して 1 つ受け取る | — | 0.59 | **0.59** | 3671 | 6.13 | 2.09 |
| Upper (ns/回) | JS からの呼び出し: strings.ToUpper、文字列を渡して受け取る | 157 | 58.6 | 1289 | 15500 | 1104 | **898** |
| Handle (ns/回) | JS からの呼び出し: JSON のリクエストハンドラ、文字列を渡して受け取る | 4391 | 1588 | 68563 | 405035 | **18635** | 20166 |
| **合計 ms（全カーネルを 1 回ずつ）** | | 413* | 311* | 1853 | 11198 | 1458 | **855** |
| **ネイティブ Go 比の幾何平均** | | 1× | 0.9× | 3.5× | 11.3× | 3.0× | **1.7×** |

起動時間（出力を読み込み始めてから関数を呼べるようになるまで、ms）:

| ランタイム | goesm | GopherJS | Go wasm | TinyGo wasm |
| --- | ---: | ---: | ---: | ---: |
| Node.js | 98.7 | 111 | 48.6 | **23.2** |
| Bun | 154 | 97.6 | 53.3 | **19.0** |
| Chromium | 103 | 76.6 | 96.6 | **28.6** |

出力サイズ（カーネル一式と、使っている標準ライブラリ）:

| | goesm | GopherJS | Go wasm | TinyGo wasm |
| --- | ---: | ---: | ---: | ---: |
| ファイル | `kernels.js` | `bench.js` | `bench.wasm` + `wasm_exec.js` | `bench.wasm` + `wasm_exec.js` |
| 非圧縮 | 1193 KiB | 1153 KiB | 4393 KiB | **1104 KiB** |
| gzip -9 | 307 KiB | **228 KiB** | 1211 KiB | 406 KiB |
| brotli -11 | 236 KiB | **169 KiB** | 894 KiB | 299 KiB |
<!-- bench:end -->

この数値から読み取れる goesm の現状:

- **合計時間は TinyGo が最速で、Go wasm、goesm の順に続き、GopherJS は大きく離れています。** 全カーネルを 1 回ずつ実行すると、goesm は 1.7〜3.3 秒、Go wasm は 1.4〜1.6 秒、TinyGo は 0.9〜1.0 秒、GopherJS は 9.6〜11.2 秒かかります。ネイティブ Go に対する幾何平均では、goesm が 3.2〜4.4 倍（最初の最適化の前は 4.2〜4.6 倍）、Go wasm が 2.9〜3.1 倍、TinyGo が 1.7〜1.9 倍、GopherJS が 9.2〜11.3 倍です。
- **JavaScript から呼ぶコストは、数値なら goesm ではゼロですが、文字列と `encoding/json` ではまだ wasm より高くつきます。** goesm の関数は JS の関数そのものなので、`Add` のコストは手書き JS と同じ 0.6〜0.7 ns です。素の wasm export 経由では TinyGo が 2〜3 ns、Go wasm が 5〜8 ns（`syscall/js` 経由ならマイクロ秒単位）、GopherJS は 3〜4 µs です。文字列を渡して受け取る `Upper` では、goesm も wasm も 1 回あたり 1〜2 µs 程度です（Bun 上の goesm は 2.9 µs、手書き JS は 0.05〜0.07 µs）。JSON のリクエストハンドラは、goesm が Node.js と Chromium で 1 回 63〜69 µs（Bun では 107 µs）、wasm が 18〜27 µs です。リフレクションで動く goesm の `encoding/json` が最も遅い部分です。
- **goesm が強いところと弱いところ。** メモリ確保の多い BinaryTrees ではどのランタイムでも最速で、Fib、Mandelbrot、FNV32、FNV64 では手書き JS と同等です。ネイティブ Go から最も離れているのは Strings、Sprintf、JSON（Node.js で 8〜13 倍）で、どれもバイト文字列、リフレクション、インターフェースを使う標準ライブラリのコードです。起動は 99〜154 ms で wasm の 19〜97 ms より遅く、サイズは brotli で 236 KiB です（GopherJS 169 KiB、TinyGo 299 KiB、Go wasm 894 KiB）。
- **差は JavaScript ではなく goesm の変換にあります。** 手書き JS は Node.js と Chromium でネイティブ Go とほぼ同じ速さなので、goesm との差は goesm が生成するコードとランタイムにあり、引き続きそこを最適化していきます。

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
- [docs/otelc.ja.md](docs/otelc.ja.md): OpenTelemetry の compile-time instrumentation (otelc) を goesm で使う
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
