# Go conformance スイート

[English](conformance.md)

Go の意味論を決めるのは Go 自身なので、goesm の正しさは Go 自身のテストで判定します。`test/conformance_test.go` にある `TestGoConformance` は、Go 配布物の `test/` ディレクトリから `// run` テストを取り出します。そして 1 本ずつ goesm でビルドし、できた ES module を Node.js で実行します。判定には、Go 自身のテストランナーである `cmd/internal/testdir` が gc を検査するのと同じ方法を使います。合格の条件は、プログラムが正常終了し、stdout と stderr を合わせた出力がテストの隣にある `.out` ファイルと一致することです。`.out` がない場合は、出力が空であることが条件です。GopherJS も同じ方法で自身を検証しています。

Node や Bun、ブラウザのテストスイートは使いません。それらは JS エンジンを検証するもので、goesm の検証にはなりません。JS エンジンは生成された ESM を動かす実行環境にすぎません。

## 実行方法

```sh
GOESM_CONFORMANCE=1 go test ./test -run TestGoConformance -v
```

実行には、Node.js 22 以上と、完全な Go 配布物の `test/` ディレクトリが必要です。go.dev/dl や `actions/setup-go` で入れた Go は、完全な配布物です。`GOTOOLCHAIN` でダウンロードされた toolchain には `test/` が含まれないので、その場合は `GOESM_GOROOT_TEST` で `test/` の場所を指定します。

```sh
curl -sSL https://go.dev/dl/go1.27.0.linux-amd64.tar.gz | tar xz -C /tmp
GOTOOLCHAIN=go1.27.0 GOESM_CONFORMANCE=1 GOESM_GOROOT_TEST=/tmp/go/test \
  go test ./test -run TestGoConformance -v
```

| 変数 | 意味 |
|---|---|
| `GOESM_CONFORMANCE=1` | スイートを有効にする。約 1000 本のプログラムをビルドし、4 コアで約 5 分かかる |
| `GOESM_GOROOT_TEST` | test ディレクトリ。既定は `$(go env GOROOT)/test` |
| `GOESM_CONFORMANCE_DIRS` | 対象のサブディレクトリのカンマ区切りのリスト。既定は `.,ken,chan,interface,typeparam,fixedbugs` |
| `GOESM_CONFORMANCE_RUN` | テスト名に対する正規表現。たとえば `^ken/` や `typeswitch` |
| `GOESM_CONFORMANCE_NATIVE=1` | 各テストを native の `go run` でも実行し、native の出力が `.out` と食い違うテストを除外する。ハーネス自体の検証に使う |
| `GOESM_CONFORMANCE_OUT` | テストごとの状態、import、Node での実行時間、理由を TSV で書き出す |
| `GOESM_CONFORMANCE_UPDATE=1` | baseline を書き直す |

Node での実行が 20 秒、またはビルドが 2 分を超えたテストは、並列実行のあとに単独でもう一度実行します。これは、CI ランナーが混んでいて起動が遅れただけのテストを失敗にしないためです。単独で実行してもタイムアウトしたテストは失敗とします。

各テストは、1 パッケージだけのモジュールにコピーされ、`goesm build` でビルドされます。モジュールの `go` ディレクティブには、実行中の toolchain のバージョンを使います。実行時には小さなドライバが bundle を import し、Go のプログラムと同じく `main` が返った時点で終了します。回復されない panic が起きた場合、終了コードは 2 になります。

対象は、レシピが `// run` だけのテストです。`// run -gcflags=...` のように引数や go コマンドのフラグが付いたテスト、複数ファイルの `rundir`、`errorcheck`、`compile`、`asmcheck` といったコンパイラ専用のレシピは対象外です。goesm のターゲットは `js/wasm` なので、ビルド制約で `js/wasm` が除外されるテストは skip として数えます。

## Baseline

`test/conformance/passing.txt` には、通過するテストを列挙しています。列挙されたテストが失敗すると、スイートは回帰として失敗します。列挙されていないテストが通過すると、スイートはそれを報告します。その場合は、`GOESM_CONFORMANCE_UPDATE=1` で一覧を更新します。CI では独立した `conformance` ジョブとして実行します。

## 結果

次の表は、Go 1.27.0 の `test/` ディレクトリと Node.js 22 を使い、2026-10-04 時点の main で実行した結果です。この時点の main には、このスイートに対する 3 回目の修正が入っています。

| ディレクトリ | 通過率 | skip |
|---|---|---|
| `test/` | 87.5% (119/136) | 9 |
| `chan/` | 100.0% (17/17) | 0 |
| `fixedbugs/` | 92.7% (571/616) | 30 |
| `interface/` | 100.0% (11/11) | 0 |
| `ken/` | 100.0% (40/40) | 0 |
| `typeparam/` | 99.3% (140/141) | 0 |
| **合計** | **93.4% (898/961)** | 39 |
| import のないテスト | 99.2% (508/512) | |

`GOESM_CONFORMANCE_NATIVE=1` で確認すると、native の `go run` は、対象テストのすべてで `.out` を再現します。例外は、`os/exec` で go コマンドを呼び出す 11 本です。これらのテストは、いずれにしても goesm ではビルドできません。

次の表は、標準ライブラリのパッケージを import するテストの、パッケージ別の通過率です。複数のパッケージを import するテストは、それぞれのパッケージで数えます。

| パッケージ | 通過率 |
|---|---|
| `fmt` | 91.4% (191/209) |
| `runtime` | 69.1% (76/110) |
| `reflect` | 80.9% (55/68) |
| `os` | 87.9% (51/58) |
| `unsafe` | 67.3% (37/55) |
| `strings` | 66.7% (28/42) |
| `math` | 96.6% (28/29) |
| `time` | 100.0% (19/19) |
| `strconv` | 87.5% (14/16) |
| `sync` | 100.0% (15/15) |

この表は毎回の実行結果にも出力されます。失敗した 63 本は次のように分類できます。

| 分類 | テスト |
|---|---|
| アドレス空間がない: field offset 以外の `unsafe` のポインタ演算、`uintptr` からポインタへの変換、`unsafe.Pointer` を介したメモリの読み替え | `cmp`、`strcopy`、`unsafebuiltins` など 19 本。goesm はビルド時か実行時にこれを報告する |
| `runtime.Caller`、スタックトレース、PC テーブル | `inline_literal`、`devirtualization_nil_panics`、`fixedbugs/bug347`、`issue4562`、`issue5856`、`issue7690`、`issue14646`、`issue18149`、`issue21879`、`issue22083`、`issue22662`、`issue27201`、`issue29504`、`issue33724`、`issue56990`、`issue58300`、`issue58300b`、`issue79762` |
| GC の観測: finalizer、`MemStats`、liveness | `init1`、`stackobj`、`stackobj3`、`fixedbugs/issue15281`、`issue27518b`、`issue32477`、`issue46725`、`issue54343` |
| 未実装: `reflect.StructOf` | `fixedbugs/issue30606`、`issue30606b`、`issue49110` |
| 未実装: `runtime/trace` | `fixedbugs/issue73748a`、`issue73748b` |
| 既知の差異: メソッドを包む関数値を通した `recover` | `fixedbugs/issue73917`、`issue73920`、`recover`、`recover1`。`recover` と `recover1` は、再帰呼び出しや reflect で作った deferred 呼び出しも検査する |
| 64 ビットの `int`: goesm の `int` は JS の number で、2^53 未満でのみ正確 | `divmod`、`fixedbugs/issue30116u`。`divmod` はタイムアウトする |
| アドレスとメモリレイアウト | `nilptr`、`fixedbugs/bug260`、`bug348`、`issue29190`。`issue29190` は、JS の配列の上限より長い、サイズ 0 の要素のスライスを使う |
| リソース | `fixedbugs/issue34395`、`issue25897a`、`issue30977`、`issue78081`。`issue34395` は 100 MiB の配列リテラルのビルドに 4 GB 以上のメモリを使い、残りの 3 本は 20 秒以内に終わらない |
| 環境: GOROOT の `src/all.bat` を読む | `winbatch` |
