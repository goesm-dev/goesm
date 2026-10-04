# ベンチマーク

[English](README.md)

このディレクトリでは、goesm が生成する JavaScript を、JavaScript ホストで Go を動かす他の方法と比較します。

| | 出力 | ビルド方法 |
| --- | --- | --- |
| **goesm** | Go パッケージごとの ES モジュール（ここではバンドル済み） | `goesm build -minify ./kernels`（このチェックアウトの goesm） |
| **GopherJS** | JavaScript のプログラム | `gopherjs build -m ./jsmain`（GopherJS 1.21.0、Go 1.21.13） |
| **Go wasm** | WebAssembly + `wasm_exec.js` | `GOOS=js GOARCH=wasm go build -ldflags=-s ./jsmain`（Go 1.27.1） |
| **TinyGo wasm** | WebAssembly + `wasm_exec.js` | `tinygo build -target=wasm -opt=2 -no-debug ./jsmain`（TinyGo 0.42.0） |

数値の目安として、基準を 2 つ載せています。**ネイティブ Go**（同じカーネルを `go build` でこのマシン向けにコンパイルしたもの）と、**手書き JS**（同じ処理を慣用的な JavaScript で手書きしたもの、[js/handwritten.mjs](js/handwritten.mjs)）です。手書き JS は、各処理を JS エンジン自体がどこまで速く実行できるかを示します。

最新の結果は [results/results.md](results/results.md)（生データは [results/results.json](results/results.json)）にあり、ルートの [README](../README.ja.md#性能) に要約があります。

## 計測内容

すべての実装が同じ Go のソース、パッケージ [kernels](kernels) を実行します。

| カーネル | サイズ | 対象 |
| --- | ---: | --- |
| Fib | 30 | 再帰呼び出し、int 演算 |
| Sieve | 2,000,000 | `[]bool`、密なループ |
| Mandelbrot | 400×400 | float64 のループ |
| NBody | 100,000 ステップ | ポインタ経由の構造体の float64 フィールド |
| FNV32 | 8 MB | uint32 の乗算と xor |
| FNV64 | 8 MB | uint64 の乗算と xor |
| BinaryTrees | 深さ 14 | メモリ確保、GC |
| Interfaces | 1,000,000 | インターフェースのメソッド呼び出し |
| MapInt | 200,000 | `map[int]int` の挿入・検索・削除 |
| MapString | 500,000 | `map[string]int` での集計 |
| Strings | 100,000 | `strings.Builder`、`strconv`、`Split`、`Join` |
| Sort | 100,000 | `sort.Ints`、`sort.Strings` |
| JSON | 2,000 レコード | `encoding/json` の Marshal + Unmarshal |
| Sprintf | 50,000 | `fmt.Sprintf` |
| Channels | 100,000 個の値 | goroutine、バッファなしチャネル、`sync.WaitGroup` |
| Add | 100,000 回 | JS からの呼び出し: 数値 2 つを渡して 1 つ受け取る |
| Upper | 100,000 回 | JS からの呼び出し: `strings.ToUpper`、文字列を渡して受け取る |
| Handle | 10,000 回 | JS からの呼び出し: JSON のリクエストハンドラ（`encoding/json` でデコード、集計、エンコード）、文字列を渡して受け取る |

各カーネルはサイズを受け取り、チェックサムを返します。ハーネスはすべての実装のチェックサムをネイティブ Go の値と照合するので、結果に載る数値はどれも正しい計算にかかった時間です。

最後の 3 つはライブラリ API の形をしています。JavaScript から小さな呼び出しを何度も行い、そのたびに境界で引数と結果を変換します。[`callInputs`](js/suite.mjs)（100 種類の入力）を順に渡す JS のループとして計測し、1 回あたりで報告するので、JS から 1 回呼ぶコスト（処理と境界越えの合計）がわかります。ネイティブ Go は同じループを Go で実行します（[kernels/api.go](kernels/api.go) の `CallChecksum`）。これは処理だけのコストで、Add はありません。

**合計**は全カーネルの中央値の和（呼び出し系カーネルはループ全体）で、各カーネルを 1 回ずつ実行するのにかかる時間です。幾何平均はどのカーネルも同じ重みで扱いますが、合計は遅いカーネルをアプリケーションで体感するのと同じ重みで扱います。

- **時間の計り方**: カーネルごとに、ハーネス（[js/harness.mjs](js/harness.mjs)。ネイティブ Go は [native/main.go](native/main.go)）は 3 回以上かつ 300 ms 以上ウォームアップしたあと、1 回ずつの呼び出しを 10 回以上かつ合計 1 秒以上（遅い呼び出しは 3 回以上かつ 10 秒以上）計測し、中央値を報告します。
- **分離**: 各実装はそれぞれ別のプロセス（Node.js、Bun）またはページ（Chromium）で、順番に実行します。
- **呼び出し方**: goesm の出力は Go の関数をそのまま export する ES モジュールなので、ハーネスは `kernels` を import して `Fib(30)` を直接呼びます。GopherJS、Go wasm、TinyGo はパッケージではなくプログラムをビルドするので、[jsmain](jsmain) が `syscall/js`（`js.FuncOf`）でカーネルを `globalThis.goBench` に公開します。これらのプログラムを JS から呼ぶときの一般的な方法です。`syscall/js` のコールバックはブロックできないので、これらでは Channels は Promise を返し、専用の goroutine で動きます。goesm では Channels 自体が async 関数になります。
- **呼び出し系カーネル**: 各実装で最も速い呼び方を使います。goesm は export をそのまま呼び、文字列はランタイムの `fromJSString` / `toJSString` で変換します（Go の文字列はバイト列です）。Go と TinyGo の wasm では `syscall/js` の呼び出しに数マイクロ秒（Go）から約 1 ミリ秒（TinyGo）かかるので、[jsmain/export_wasm.go](jsmain/export_wasm.go) が Add、Upper、Handle を素の WebAssembly 関数として export し（Go は `//go:wasmexport`、TinyGo は `//export`。TinyGo の `//go:wasmexport` は `main` がブロックしている間 1 回あたり約 0.2 ms かかります）、文字列は `TextEncoder.encodeInto` と `TextDecoder` で UTF-8 として線形メモリ経由で渡します。GopherJS は `syscall/js` を通しますが、GopherJS ではそれがそのまま JS の呼び出しです。
- **起動時間**: 出力の読み込み（ファイルの読み込みまたは fetch、コンパイル、パッケージ初期化と `main` の実行）を始めてから、最初の関数が呼べるようになるまでの時間です。
- **サイズ**: ページが読み込む必要のあるファイルのサイズです。goesm はバンドルしたモジュール、GopherJS はスクリプト、Go と TinyGo は `.wasm` と `wasm_exec.js` です。カーネルが使う標準ライブラリ（`fmt`、`encoding/json`、`sort`、`strconv`、`strings`、`sync`、`math`）を含みます。

### 公平性について

- `int` は GopherJS と TinyGo の wasm ターゲットでは 32 ビット、それ以外では 64 ビットです。goesm では JS の number です。どの実装でも同じ計算になるよう、カーネルは `int` の値を 2^31 未満に収め、オーバーフローがアルゴリズムの一部になる箇所では `uint32` / `uint64` を使います。
- TinyGo は `-opt=2`（速度優先。既定の `-opt=z` はサイズ優先）でビルドします。Go wasm には速度とサイズを切り替える指定がないので既定のままです。
- ネイティブ Go は goroutine を複数のスレッドで動かし、他の実装はシングルスレッドです。スレッドをまたいでバッファなしチャネルで値を受け渡すのは、1 スレッド上で goroutine を切り替えるより高くつきます。Channels でネイティブ Go が最速でないのはこのためです。
- 手書き JS は Go ではありません。型付き配列、`Map`、`JSON.stringify` / `JSON.parse`（エンジンのネイティブ実装）、テンプレート文字列を使い、境界チェックや nil チェックを自前では行いません。Channels に相当するものはありません。
- GopherJS 1.21 でコンパイルできるよう、[kernels](kernels) は Go 1.21 の言語と標準ライブラリの範囲に収めています（[gopherjs.mod](gopherjs.mod) は GopherJS がビルドに使うモジュールファイルです）。

## 実行方法

ツールのバージョンは固定しています。Go、Node.js、Bun はリポジトリの [mise.toml](../mise.toml)、TinyGo は[このディレクトリの mise.toml](mise.toml) です。GopherJS は `build.sh` が固定バージョンを `go install` し、Chromium は [package.json](package.json) で固定した `playwright-core` で操作します。

```sh
cd bench
mise install                   # Go、Node.js、Bun、TinyGo
npm ci                         # Chromium の計測に使う playwright-core
mise exec -- sh build.sh       # すべてを out/ にビルド
mise exec -- node js/compare.mjs            # results/results.{json,md}、results/charts/*.svg
node js/report.mjs results/results.json -readme   # 要約とグラフを ../README(.ja).md にコピー
```

`compare.mjs` には `-runtimes node,bun,chromium`、`-impls goesm,gopherjs,gowasm,tinygo,js`、`-kernels Fib,Sieve`、`-quick`（ウォームアップと計測回数を減らす）を指定できます。Chromium は Playwright と同じ方法で探します（`PLAYWRIGHT_BROWSERS_PATH`、または `npx playwright-core install chromium`）。`CHROMIUM_PATH` で上書きできます。1 つの実装だけを実行することもでき（`node js/run.mjs goesm`、`bun js/run.mjs tinygo Fib`）、`bench/` を配信して `js/browser.html?impl=goesm` を開けば任意のブラウザでも動きます。

## int64.mjs

[int64.mjs](int64.mjs) は、goesm が `int64` / `uint64` の表現として検討した方式（BigInt、uint32 2 つ、オブジェクト、ハイブリッド）を比べる別のマイクロベンチマークです。[ARCHITECTURE.ja.md](../ARCHITECTURE.ja.md) を参照してください。
