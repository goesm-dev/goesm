# ベンチマーク

[English](README.md)

このディレクトリでは、goesm の出力を、JavaScript ホストで Go を動かす他の方法と比較します。

| | 出力 | ビルド方法 | バージョン |
| --- | --- | --- | --- |
| **goesm** | Go パッケージごとの ES モジュール。ここでは 1 つにバンドルして使う | `goesm build -minify ./kernels` | このチェックアウトの goesm |
| **GopherJS** | JavaScript のプログラム | `gopherjs build -m ./jsmain` | GopherJS 1.21.0、Go 1.21.13 |
| **Go wasm** | WebAssembly と `wasm_exec.js` | `GOOS=js GOARCH=wasm go build -ldflags=-s ./jsmain` | Go 1.27.1 |
| **TinyGo wasm** | WebAssembly と `wasm_exec.js` | `tinygo build -target=wasm -opt=2 -no-debug ./jsmain` | TinyGo 0.42.0 |

数値の目安として、基準を 2 つ載せています。1 つは**ネイティブ Go** で、同じカーネルを `go build` でこのマシン向けにコンパイルしたものです。もう 1 つは**手書き JS** で、同じ処理を慣用的な JavaScript で手書きした [js/handwritten.mjs](js/handwritten.mjs) です。手書き JS は、JS エンジン自体が各処理をどこまで速く実行できるかを示します。

最新の結果は [results/results.md](results/results.md) にあります。生データは [results/results.json](results/results.json) にあり、要約はルートの [README](../README.ja.md#性能) にあります。

## 計測内容

すべての実装が、同じ Go のソースであるパッケージ [kernels](kernels) を実行します。

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
| MapInt | 200,000 | `map[int]int` の挿入、検索、削除 |
| MapString | 500,000 | `map[string]int` での集計 |
| Strings | 100,000 | `strings.Builder`、`strconv`、`Split`、`Join` |
| Sort | 100,000 | `sort.Ints`、`sort.Strings` |
| JSON | 2,000 レコード | `encoding/json` の Marshal と Unmarshal |
| Sprintf | 50,000 | `fmt.Sprintf` |
| Channels | 100,000 個の値 | goroutine、バッファなしチャネル、`sync.WaitGroup` |
| Add | 100,000 回 | JS からの呼び出し: 数値 2 つを渡して 1 つ受け取る |
| Upper | 100,000 回 | JS からの呼び出し: `strings.ToUpper`、文字列を渡して受け取る |
| Handle | 10,000 回 | JS からの呼び出し: JSON のリクエストハンドラ、文字列を渡して受け取る |

各カーネルはサイズを受け取り、チェックサムを返します。ハーネスはすべての実装のチェックサムをネイティブ Go の値と照合します。そのため、結果に載る数値はどれも、正しい計算にかかった時間です。

最後の 3 つのカーネルは、ライブラリ API の形をしています。JavaScript から小さな呼び出しを何度も行い、そのたびに境界で引数と結果を変換します。Handle のハンドラは、リクエストを `encoding/json` でデコードし、集計し、結果をエンコードします。これらのカーネルは、[`callInputs`](js/suite.mjs) の 100 種類の入力を順に渡す JS のループとして計測し、1 回の呼び出しあたりの時間で報告します。この時間は、処理と境界越えを合わせた、JS から 1 回呼ぶコストです。ネイティブ Go は、[kernels/api.go](kernels/api.go) の `CallChecksum` で同じループを Go で実行します。ネイティブ Go の時間は処理だけのコストで、Add の値はありません。

### 崖のカーネル

[kernels/cliffs.go](kernels/cliffs.go) と [kernels/pull.go](kernels/pull.go) のカーネルは、JS へのコンパイラがネイティブ Go や同じ処理の手書き JS より極端に遅くしやすい Go の書き方を測ります。幾何平均と合計にはこれらを含めません。レポートでは別の表に示し、各実装の最も遅いカーネルの集計には含めます。

| カーネル | サイズ | 対象 |
| --- | ---: | --- |
| Parallel | 100,000 | 4 つの goroutine に分けた CPU 処理。ネイティブ Go は 4 スレッドで、JS は順番に実行する |
| Rand64 | 1,000,000 | メソッド経由での、構造体フィールドの `uint64` 演算 |
| MaybeBlocking | 1,000,000 | 別の実装がブロックするインターフェースのメソッド呼び出し |
| Pull | 50,000 ステップ | `iter.Pull`。ネイティブ Go はコルーチンで実行する。手書き JS はジェネレーターを使う |
| RSASign | 署名 4 回 | `crypto/rsa` による 2048 ビットの PKCS #1 v1.5 署名と多倍長演算。手書き JS は Web Crypto を使う |

**合計**は全カーネルの中央値の和で、各カーネルを 1 回ずつ実行するのにかかる時間を表します。呼び出し系のカーネルについては、ループ全体の時間を足します。幾何平均はどのカーネルも同じ重みで扱います。一方、合計は、遅いカーネルをアプリケーションで体感するときと同じ重みで扱います。

- **時間の計り方**: ハーネスはカーネルごとに、3 回以上かつ 300 ms 以上ウォームアップします。そのあと 1 回ずつの呼び出しを 10 回以上かつ合計 1 秒以上計測し、中央値を報告します。遅い呼び出しについては、3 回以上かつ 10 秒以上で計測を終えます。JS 側のハーネスは [js/harness.mjs](js/harness.mjs)、ネイティブ Go のハーネスは [native/main.go](native/main.go) です。
- **分離**: 各実装は、Node.js と Bun ではそれぞれ別のプロセスで、Chromium では別のページで、順番に実行します。プロセスには、`NODE_OPTIONS` と `BUN_OPTIONS` を除いた環境を渡します。これらに含まれるランタイムのオプションは、測る対象を変えてしまうためです。たとえば Bun の `--smol` はヒープを小さくするので、GC が増えます。
- **呼び出し方**: goesm の出力は Go の関数をそのまま export する ES モジュールなので、ハーネスは `kernels` を import して `Fib(30)` を直接呼びます。GopherJS、Go wasm、TinyGo は、パッケージではなくプログラムをビルドします。そのため、[jsmain](jsmain) が `syscall/js` の `js.FuncOf` でカーネルを `globalThis.goBench` に公開します。これは、これらのプログラムを JS から呼ぶときの一般的な方法です。`syscall/js` のコールバックはブロックできないので、これらの実装では Channels が Promise を返し、専用の goroutine で動きます。goesm では、Channels 自体が async 関数になります。
- **呼び出し系カーネル**: 各実装で最も速い呼び方を使います。goesm の export には JS の文字列をそのまま渡します。Go のバイト列の文字列との変換は、export のラッパーが行います。Go wasm では `syscall/js` の呼び出しに数マイクロ秒かかり、TinyGo では約 1 ミリ秒かかります。そのため [jsmain/export_wasm.go](jsmain/export_wasm.go) は、Add、Upper、Handle を素の WebAssembly 関数として export します。Go では `//go:wasmexport` を、TinyGo では `//export` を使います。TinyGo の `//go:wasmexport` は、`main` がブロックしている間、1 回の呼び出しに約 0.2 ms かかるからです。文字列は、`TextEncoder.encodeInto` と `TextDecoder` で UTF-8 にして、線形メモリ経由で渡します。GopherJS は `syscall/js` を通しますが、GopherJS ではそれがそのまま JS の呼び出しになります。
- **起動時間**: 出力の読み込みを始めてから、最初の関数が呼べるようになるまでの時間です。読み込みには、ファイルの読み込みまたは fetch、コンパイル、パッケージの初期化、`main` の実行が含まれます。goesm の出力ディレクトリには、ES モジュールのパッケージとして配布するときと同じく、`"type": "module"` を指定した `package.json` を置きます。これがないと、Node.js はモジュール形式を判定するためにもう一度パースし、ここでは約 30 ms 余計にかかります。
- **サイズ**: ページが読み込む必要のあるファイルのサイズです。goesm ではバンドルしたモジュール、GopherJS ではスクリプト、Go と TinyGo では `.wasm` と `wasm_exec.js` が対象です。サイズには、カーネルが使う標準ライブラリの `fmt`、`encoding/json`、`sort`、`strconv`、`strings`、`sync`、`math` が含まれます。

### 公平性について

- `int` は、GopherJS と TinyGo の wasm ターゲットでは 32 ビット、それ以外では 64 ビットです。goesm では JS の number です。どの実装でも同じ計算になるように、カーネルは `int` の値を 2^31 未満に収めます。オーバーフローがアルゴリズムの一部になる箇所では、`uint32` か `uint64` を使います。
- TinyGo は、速度を優先する `-opt=2` でビルドします。TinyGo の既定の `-opt=z` は、サイズを優先します。Go wasm には速度とサイズを切り替える指定がないので、既定のままビルドします。
- ネイティブ Go は goroutine を複数のスレッドで動かしますが、他の実装はシングルスレッドです。スレッドをまたいでバッファなしチャネルで値を受け渡すと、1 スレッド上で goroutine を切り替えるよりコストがかかります。Channels でネイティブ Go が最速でないのはこのためです。
- 手書き JS は Go ではありません。手書き JS は、型付き配列、`Map`、エンジンのネイティブ実装である `JSON.stringify` と `JSON.parse`、テンプレート文字列を使い、境界チェックや nil チェックを自前では行いません。Channels に相当するカーネルはありません。
- [kernels](kernels) は、GopherJS 1.21 でコンパイルできるように、Go 1.21 の言語と標準ライブラリの範囲で書いています。[gopherjs.mod](gopherjs.mod) は、GopherJS がビルドに使うモジュールファイルです。Pull は Go 1.23 の `iter` を使うので、GopherJS 以外でだけビルドします。
- jsmain は、Channels と同じく Parallel と Pull も専用の goroutine で実行し、Promise を返します。これらはブロックするからです。

### goesm が手書き JS より遅いところ

goesm は、同じ処理を JS で手書きした場合と同じ速度とメモリ使用量で Go を動かすことを目指しています。Node.js では、手書きの版がある 17 のカーネルのうち 12 で、手書き JS との差が 10% 以内です。Bun と Chromium では、比率が計測ごとに大きく変わります。遅いままのカーネルとその理由は次のとおりです。

- **Upper** は 1.9〜2.5 倍です。Go の文字列はバイト列で、JS の文字列は UTF-16 のコード単位の列です。JS から Go に渡る文字列は、すべて ASCII 以外の文字を含むかどうかを検査します。Node.js では、この検査が Upper の 1 回 110 ns のうち約 40 ns を占めます。`len`、インデックス、スライスにはバイト列の形が必要なので、この検査は省けません。
- **JSON** は 1.4〜1.6 倍、**Handle** は 1.1〜1.9 倍です。Go の `encoding/json` の規則は `JSON.parse` と異なります。`1.0` や `1e2` のような数は `int` のフィールドではエラーになります。同じキーが繰り返されると、同じ構造体やスライスにもう一度デコードします。`\u` のエスケープは UTF-8 になります。そのため goesm は JS で書いた独自のパーサーでデコードし、その時間は `JSON.parse` の約 1.2 倍です。さらに、スライスのヘッダーや JS の `Map` を使う map などの Go の値を組み立てます。
- **Interfaces** は、Node.js と Chromium で 1.1〜1.2 倍、Bun で 1.7 倍です。インターフェース値は、型 switch と型アサーションのために動的な型を持ち、メソッド呼び出しもこの型を通ります。Bun では、同じクラス構成と呼び出しを手で書くと、手書き JS との差は 5% 以内に収まります。ただし、カーネルのモジュール全体を読み込むと、GC が扱うヒープが大きくなり、そのコードは 20〜30% 遅くなります。Bun での残りの差の原因は、まだ特定できていません。
- **BinaryTrees** は 1.0〜1.2 倍です。生成するコードは手書き JS と同じで、割り当てるバイト数も同じです。単独で計ると、Node.js と Bun で手書き JS の 0.95〜1.05 倍の時間で終わります。全体の計測では、それまでのカーネルが残した GC の状態によって時間が変わります。
- **Pull** の 2〜5 倍と **MaybeBlocking** の 1.4〜1.6 倍は、意図して測っている崖です。`iter.Pull` は、シーケンスが `yield` を直接呼ぶだけであれば JS のジェネレーターになります。それ以外のシーケンスは goroutine 上のコルーチンとして動きます。ブロックしうる呼び出しでは、結果が Promise かどうかを確かめる必要があります。
- **RSASign** は約 110 倍です。手書き JS はネイティブコードである Web Crypto を呼びます。goesm は Go の `crypto/rsa` を、double で表した 32 ビットのリムで実行します。BigInt を使うと定数時間になりません。

メモリも同じ傾向です。Node.js で `v8.GCProfiler` を使って測った 1 回の呼び出しあたりの割り当てバイト数は、ほとんどのカーネルで手書き JS との差が 5% 以内です。例外は、JSON の 1.8 倍、Handle の 1.7 倍、Strings の 1.4 倍、Sort の 1.4 倍、Pull の 3.6 倍です。JSON と Handle は、スライスのヘッダー、`Map` を使う map、構造体のデコード先となるコピーを割り当てます。Strings では、`strings.Builder` への書き込みのたびに文字列を連結します。Sort では、手書き JS が `Int32Array` を使うのに対し、Go の `[]int` は number の配列になります。

## 実行方法

ツールのバージョンは固定しています。Go、Node.js、Bun のバージョンはリポジトリの [mise.toml](../mise.toml) で、TinyGo のバージョンは[このディレクトリの mise.toml](mise.toml) で固定しています。GopherJS は、`build.sh` が固定したバージョンを `go install` します。Chromium は、[package.json](package.json) で固定した `playwright-core` で操作します。

```sh
cd bench
mise install                   # Go、Node.js、Bun、TinyGo を入れる
npm ci                         # Chromium の計測に使う playwright-core を入れる
mise exec -- sh build.sh       # すべてを out/ にビルドする
mise exec -- node js/compare.mjs            # results/results.json、results/results.md、results/charts/*.svg を書き出す
node js/report.mjs results/results.json -readme   # 要約とグラフを ../README.md と ../README.ja.md にコピーする
```

`compare.mjs` には、`-runtimes node,bun,chromium`、`-impls goesm,gopherjs,gowasm,tinygo,js`、`-kernels Fib,Sieve`、`-quick` を指定できます。`-quick` は、ウォームアップと計測の回数を減らします。Chromium は Playwright と同じ場所から探すので、`PLAYWRIGHT_BROWSERS_PATH` を設定するか、`npx playwright-core install chromium` で入れておきます。`CHROMIUM_PATH` を指定すると、その場所の Chromium を使います。`node js/run.mjs goesm` や `bun js/run.mjs tinygo Fib` のように、1 つの実装だけを実行することもできます。`bench/` を配信して `js/browser.html?impl=goesm` を開けば、任意のブラウザでも計測できます。

## int64.mjs

[int64.mjs](int64.mjs) は、別のマイクロベンチマークです。goesm が `int64` と `uint64` の表現として検討した、BigInt、uint32 2 つ、オブジェクト、ハイブリッドの各方式を比べます。詳しくは [ARCHITECTURE.ja.md](../ARCHITECTURE.ja.md) を参照してください。
