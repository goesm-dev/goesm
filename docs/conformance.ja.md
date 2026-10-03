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

Go 1.27.0 の `test/` ディレクトリ、Node.js 22、2026-10-03 時点の main（このスイートが最初に見つけたバグの大半の修正と複素数のサポートを入れた #5 のあと）での結果です。

| ディレクトリ | 通過率 | skip |
|---|---|---|
| `test/` | 46.3% (63/136) | 9 |
| `chan/` | 58.8% (10/17) | 0 |
| `fixedbugs/` | 60.6% (373/616) | 30 |
| `interface/` | 72.7% (8/11) | 0 |
| `ken/` | 82.5% (33/40) | 0 |
| `typeparam/` | 50.4% (71/141) | 0 |
| **合計** | **58.1% (558/961)** | 39 |
| import のないテスト | 94.7% (485/512) | |

`GOESM_CONFORMANCE_NATIVE=1` で確認すると、native の `go run` は対象テストのすべてで `.out` を再現します。例外は go コマンドを呼び出す（`os/exec`）11 本で、これはどのみち goesm ではビルドできません。

標準ライブラリのパッケージを import するテストの、パッケージ別の通過率です（複数を import するテストはそれぞれに数えます）。

| パッケージ | 通過率 |
|---|---|
| `fmt` | 0.0% (0/209) |
| `runtime` | 26.4% (29/110) |
| `reflect` | 0.0% (0/68) |
| `os` | 0.0% (0/58) |
| `unsafe` | 23.6% (13/55) |
| `strings` | 26.2% (11/42) |
| `math` | 20.7% (6/29) |
| `time` | 0.0% (0/19) |
| `strconv` | 18.8% (3/16) |
| `sync` | 20.0% (3/15) |

この表は毎回の実行結果にも出力されます。大きな阻害要因は次の 2 つです。

* **`fmt` のリフレクション**: `fmt` はコンパイルできるようになりましたが、`fmt` を使うテストはすべて実行時に "internal/abi.TypeOf is not supported yet" で panic します（246 本）。`fmt` は `reflect` を通して書式化するので、`internal/abi` の裏に goesm の型記述子が必要です。
* **`os` の出力**: `js/wasm` では `os.Stdout` への書き込みが `syscall/js` を経由するため、50 本が "syscall/js.valueGet is not supported yet" で panic します。

import のない 512 本のうち、失敗した 27 本は次のように分類できます。

| 分類 | テスト |
|---|---|
| 未実装: 後方への `goto` | `ken/label`、`fixedbugs/bug005`、`bug178`、`issue40367`、`issue75569` |
| 未実装: その他 | `convert4`（スライスから配列ポインタへの変換）、`range4` と `fixedbugs/issue71675`（range-over-func 本体の `defer`）、`typeparam/issue54537`（型パラメータ変数のアドレス） |
| 64 ビット整数（既知の差異） | `intcvt`、`printbig`、`divmod`（タイムアウト）、`fixedbugs/issue2615`、`issue4448`、`issue43480`、`issue50854`、`issue70481`、`issue23305` |
| 間接的な `recover`（既知の差異） | `fixedbugs/issue73916`、`issue73916b`、`issue73917`、`issue73920` |
| 複素数 | `fixedbugs/issue79812`（float の型パラメータに対する `T(0 + 0i)` が複素数の値になる）、`issue5793`（多値を返す複素数の呼び出しの lowering で internal error） |
| generics | `typeparam/typeswitch3`（`reading 'methods'`） |
| リソース | `fixedbugs/issue34395`（100 MiB の配列リテラルのビルドに 4 GB 以上必要）、`issue13169`（10 万回のチャネル送信が 20 秒以内に終わらない） |
