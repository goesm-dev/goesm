# Go conformance スイート

[English](conformance.md)

Go の意味論の authority は Go なので、goesm の正しさは Go 自身のテストで判定します。`TestGoConformance`（`test/conformance_test.go`）は Go 配布物の `test/` ディレクトリから `// run` テストを取り出し、1 本ずつ goesm でビルドして ES module を Node.js で実行し、Go 自身のテストランナー（`cmd/internal/testdir`）が gc を検査するのと同じ方法で判定します。プログラムが正常終了し、stdout と stderr を合わせた出力がテストの隣の `.out` ファイルと一致すること（`.out` がなければ出力が空であること）が条件です。GopherJS も同じ方法で自身を検証しています。

Node や Bun、ブラウザのテストスイートは使いません。それらは JS エンジンを検証するもので、goesm の検証にはなりません。JS エンジンは生成された ESM を動かす実行環境にすぎません。

## 実行方法

```sh
GOESM_CONFORMANCE=1 go test ./test -run TestGoConformance -v
```

Node.js 22 以上と、完全な Go 配布物（go.dev/dl や `actions/setup-go` のもの）の `test/` ディレクトリが必要です。`GOTOOLCHAIN` でダウンロードされた toolchain には `test/` が含まれないので、その場合は `GOESM_GOROOT_TEST` で指定します。

```sh
curl -sSL https://go.dev/dl/go1.27.0.linux-amd64.tar.gz | tar xz -C /tmp
GOTOOLCHAIN=go1.27.0 GOESM_CONFORMANCE=1 GOESM_GOROOT_TEST=/tmp/go/test \
  go test ./test -run TestGoConformance -v
```

| 変数 | 意味 |
|---|---|
| `GOESM_CONFORMANCE=1` | スイートを有効にする（約 1000 本のプログラムをビルドする。4 コアで約 5 分） |
| `GOESM_GOROOT_TEST` | test ディレクトリ（既定は `$(go env GOROOT)/test`） |
| `GOESM_CONFORMANCE_DIRS` | 対象サブディレクトリをカンマ区切りで（既定は `.,ken,chan,interface,typeparam,fixedbugs`） |
| `GOESM_CONFORMANCE_RUN` | テスト名に対する正規表現（例: `^ken/`、`typeswitch`） |
| `GOESM_CONFORMANCE_NATIVE=1` | 各テストを native の `go run` でも実行し、native の出力が `.out` と食い違うテストを除外する（ハーネス自体の検証用） |
| `GOESM_CONFORMANCE_OUT` | テストごとの結果を TSV で書き出す（状態、import、Node での実行時間、理由） |
| `GOESM_CONFORMANCE_UPDATE=1` | baseline を書き直す |

タイムアウトしたテスト（Node で 20 秒、ビルドで 2 分）は、並列実行のあとに単独でもう一度実行します。CI ランナーが混んでいて起動が遅れただけのものを失敗にしないためで、単独でもタイムアウトすれば失敗です。

各テストは 1 パッケージだけのモジュール（`go` ディレクティブは実行中の toolchain のもの）にコピーされ、`goesm build` でビルドされます。実行は小さなドライバが bundle を import し、Go のプログラムと同じく `main` が返った時点で終了します。回復されない panic では終了コード 2 になります。

対象はレシピが `// run` だけのテストです。引数や go コマンドのフラグつき（`// run -gcflags=...`）、複数ファイルの `rundir`、コンパイラ専用のレシピ（`errorcheck`、`compile`、`asmcheck`）は対象外です。ビルド制約で `js/wasm`（goesm のターゲット）が除外されるテストは skip として数えます。

## Baseline

`test/conformance/passing.txt` に通過するテストを列挙しています。列挙されたテストが失敗すると回帰としてスイートが失敗します。列挙されていないテストが通過した場合は報告されるので、`GOESM_CONFORMANCE_UPDATE=1` で一覧を更新します。CI では独立した `conformance` ジョブとして実行します。

## 結果

Go 1.27.0 の `test/` ディレクトリ、Node.js 22、2026-10-04 時点の main（このスイートに対する 2 回目の修正のあと）での結果です。

| ディレクトリ | 通過率 | skip |
|---|---|---|
| `test/` | 86.8% (118/136) | 9 |
| `chan/` | 100.0% (17/17) | 0 |
| `fixedbugs/` | 92.4% (569/616) | 30 |
| `interface/` | 100.0% (11/11) | 0 |
| `ken/` | 100.0% (40/40) | 0 |
| `typeparam/` | 98.6% (139/141) | 0 |
| **合計** | **93.0% (894/961)** | 39 |
| import のないテスト | 98.8% (506/512) | |

`GOESM_CONFORMANCE_NATIVE=1` で確認すると、native の `go run` は対象テストのすべてで `.out` を再現します。例外は go コマンドを呼び出す（`os/exec`）11 本で、これはどのみち goesm ではビルドできません。

標準ライブラリのパッケージを import するテストの、パッケージ別の通過率です（複数を import するテストはそれぞれに数えます）。

| パッケージ | 通過率 |
|---|---|
| `fmt` | 90.4% (189/209) |
| `runtime` | 69.1% (76/110) |
| `reflect` | 79.4% (54/68) |
| `os` | 87.9% (51/58) |
| `unsafe` | 67.3% (37/55) |
| `strings` | 66.7% (28/42) |
| `math` | 96.6% (28/29) |
| `time` | 100.0% (19/19) |
| `strconv` | 87.5% (14/16) |
| `sync` | 100.0% (15/15) |

この表は毎回の実行結果にも出力されます。失敗した 67 本は次のように分類できます。

| 分類 | テスト |
|---|---|
| アドレス空間がない: field offset 以外の `unsafe` のポインタ演算、`uintptr` からポインタへの変換、`unsafe.Pointer` を介したメモリの読み替え | `cmp`、`strcopy`、`unsafebuiltins` など 19 本（goesm がビルド時か実行時に報告） |
| `runtime.Caller`、スタックトレース、PC テーブル | `inline_literal`、`devirtualization_nil_panics`、`fixedbugs/bug347`、`issue4562`、`issue5856`、`issue7690`、`issue14646`、`issue18149`、`issue21879`、`issue22083`、`issue22662`、`issue27201`、`issue29504`、`issue33724`、`issue56990`、`issue58300`、`issue58300b`、`issue79762` |
| GC の観測（finalizer、`MemStats`、liveness） | `init1`、`stackobj`、`stackobj3`、`fixedbugs/issue15281`、`issue27518b`、`issue32477`、`issue46725`、`issue54343` |
| 未実装 | `range4` と `fixedbugs/issue71675`（range-over-func 本体の `defer`）、`fixedbugs/issue72063` と `typeparam/nested`（型パラメータに依存するローカル型）、`fixedbugs/issue30606`、`issue30606b`、`issue49110`（`reflect.StructOf`）、`fixedbugs/issue73748a`、`issue73748b`（`runtime/trace`） |
| メソッドを包む関数値を通した `recover`（既知の差異） | `fixedbugs/issue73917`、`issue73920`。`recover` と `recover1` は再帰呼び出しや reflect で作った deferred 呼び出しも検査する |
| 64 ビットの `int`（JS の number で、2^53 未満で正確） | `divmod`（タイムアウト）、`fixedbugs/issue30116u` |
| アドレスとメモリレイアウト | `nilptr`、`fixedbugs/bug260`、`bug348`、`issue29190`（JS の配列の上限より長い、サイズ 0 の要素のスライス） |
| リソース | `fixedbugs/issue34395`（100 MiB の配列リテラルのビルドに 4 GB 以上必要）、`issue25897a`、`issue30977`、`issue78081`（20 秒以内に終わらない） |
| 環境 | `winbatch`（GOROOT の `src/all.bat` を読む） |
