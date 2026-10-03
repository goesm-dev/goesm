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

各テストは 1 パッケージだけのモジュール（`go` ディレクティブは実行中の toolchain のもの）にコピーされ、`goesm build` でビルドされます。実行は小さなドライバが bundle を import し、Go のプログラムと同じく `main` が返った時点で終了します。回復されない panic では終了コード 2 になります。

対象はレシピが `// run` だけのテストです。引数や go コマンドのフラグつき（`// run -gcflags=...`）、複数ファイルの `rundir`、コンパイラ専用のレシピ（`errorcheck`、`compile`、`asmcheck`）は対象外です。ビルド制約で `js/wasm`（goesm のターゲット）が除外されるテストは skip として数えます。

## Baseline

`test/conformance/passing.txt` に通過するテストを列挙しています。列挙されたテストが失敗すると回帰としてスイートが失敗します。列挙されていないテストが通過した場合は報告されるので、`GOESM_CONFORMANCE_UPDATE=1` で一覧を更新します。CI では独立した `conformance` ジョブとして実行します。

## 結果

Go 1.27.0 の `test/` ディレクトリ、Node.js 22、2026-10-03 時点の #1 の head（`errors`、`strings`、`strconv`、`sort`、`slices`、`maps`、`sync` を Go ソースからコンパイルする版）での結果です。

| ディレクトリ | 通過率 | skip |
|---|---|---|
| `test/` | 36.0% (49/136) | 9 |
| `chan/` | 58.8% (10/17) | 0 |
| `fixedbugs/` | 59.4% (366/616) | 30 |
| `interface/` | 72.7% (8/11) | 0 |
| `ken/` | 72.5% (29/40) | 0 |
| `typeparam/` | 47.5% (67/141) | 0 |
| **合計** | **55.0% (529/961)** | 39 |
| import のないテスト | 89.8% (460/512) | |

`GOESM_CONFORMANCE_NATIVE=1` で確認すると、native の `go run` は対象テストのすべてで `.out` を再現します。例外は go コマンドを呼び出す（`os/exec`）11 本で、これはどのみち goesm ではビルドできません。

標準ライブラリのパッケージを import するテストの、パッケージ別の通過率です（複数を import するテストはそれぞれに数えます）。

| パッケージ | 通過率 |
|---|---|
| `fmt` | 0.0% (0/209) |
| `runtime` | 26.4% (29/110) |
| `reflect` | 0.0% (0/68) |
| `os` | 0.0% (0/58) |
| `unsafe` | 21.8% (12/55) |
| `strings` | 23.8% (10/42) |
| `math` | 13.8% (4/29) |
| `time` | 0.0% (0/19) |
| `strconv` | 18.8% (3/16) |
| `sync` | 20.0% (3/15) |

この表は毎回の実行結果にも出力されます。大きな阻害要因は次の 2 つです。

* **複素数**: `fmt` は `complex128` に依存している（`strconv.FormatComplex`、複素数の `%v`）ため、`fmt` を使う 209 本はすべて "unsupported basic type complex128" で止まります。最も多くのテストを通せるようになる変更は複素数のサポートです。
* **`os` の出力**: `js/wasm` では `os.Stdout` への書き込みが `syscall/js` を経由するため、`os` を使う 45 本が "syscall/js.valueGet is not supported yet" で panic します。

import のない 512 本のうち、失敗した 52 本は次のように分類できます。

| 分類 | テスト |
|---|---|
| 未実装: 複素数 | `convT2X`、`print`、`ken/cplx0`、`cplx1`、`cplx2`、`cplx5`、`fixedbugs/bug329`、`bug401`、`bug491`、`issue5793`、`issue58671`、`issue79812` |
| 未実装: 後方への `goto` | `ken/label`、`fixedbugs/bug005`、`bug178`、`issue40367`、`issue75569` |
| 未実装: その他 | `convert4`（スライスから配列ポインタへの変換）、`range4` と `fixedbugs/issue71675`（range-over-func 本体の `defer`）、`typeparam/issue54537`（型パラメータ変数のアドレス） |
| 64 ビット整数（既知の差異） | `intcvt`、`printbig`、`divmod`（タイムアウト）、`fixedbugs/issue2615`、`issue4448`、`issue43480`、`issue50854`、`issue70481`、`issue23305` |
| 間接的な `recover`（既知の差異） | `fixedbugs/issue73916`、`issue73916b`、`issue73917`、`issue73920` |
| 再帰型（`type S []S`、`type Chan[T any] chan Chan[T]`）で goesm がスタックオーバーフローする | `ddd`、`fixedbugs/issue17039`、`typeparam/issue47901` |
| generics | `typeparam/typeswitch3`（`reading 'methods'`）、`interfacearg`（internal error）、`issue50833`（型 `P` の複合リテラル）、`issue376214` |
| メソッド式（昇格メソッド、リテラル型のレシーバ） | `method`、`method7` |
| `new(expr)`（Go 1.26）が `new(T)` として lower される | `newexpr` |
| nil interface に対する `defer x.M()` は defer 文の時点で panic すべき | `fixedbugs/issue15975` |
| panic すべき nil 参照（`&*p`、nil ポインタのメソッド値） | `nilptr2`、`method5` |
| サイズ 0 の値（`[0]int`、`struct{}`） | `zerosize`、`fixedbugs/bug352` |
| range の代入順序: `for i, x[i] = range y` の `x[i]` は代入前の `i` を使うべき | `range` |
| 遅すぎる（20 秒以内に終了しない） | `fixedbugs/issue13169`（10 万回のチャネル送信）、`issue34395`（100 MiB の配列リテラル） |
