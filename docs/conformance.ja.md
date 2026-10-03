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

Go 1.27.0 の `test/` ディレクトリ、Node.js 22:

| ディレクトリ | 通過率 | skip |
|---|---|---|
| `test/` | 32.4% (44/136) | 9 |
| `chan/` | 41.2% (7/17) | 0 |
| `fixedbugs/` | 50.6% (312/616) | 30 |
| `interface/` | 72.7% (8/11) | 0 |
| `ken/` | 70.0% (28/40) | 0 |
| `typeparam/` | 41.8% (59/141) | 0 |
| **合計** | **47.7% (458/961)** | 39 |
| import のないテスト | 87.5% (448/512) | |

`GOESM_CONFORMANCE_NATIVE=1` で確認すると、native の `go run` は対象テストのすべてで `.out` を再現します。例外は go コマンドを呼び出す（`os/exec`）11 本で、これはどのみち goesm ではビルドできません。

標準ライブラリのパッケージを import するテストをパッケージ別に数えると次のとおりです（複数を import するテストはそれぞれに数えます。通過があるのは `unsafe` だけで、`unsafe.Sizeof` などの定数しか使わないテストです）。

| パッケージ | テスト数 | 通過 |
|---|---|---|
| `fmt` | 209 | 0 |
| `runtime` | 110 | 0 |
| `reflect` | 68 | 0 |
| `os` | 58 | 0 |
| `unsafe` | 55 | 8 |
| `strings` | 42 | 0 |
| `math` | 29 | 0 |
| `time` | 19 | 0 |
| `strconv` | 16 | 0 |
| `sync` | 15 | 0 |

この表は毎回の実行結果にも出力されます。

失敗の大半は標準ライブラリを import するテストです（`fmt` だけを import するテストが 107 本あります）。goesm はまだこれらをコンパイルできません（ARCHITECTURE.ja.md §9）。import のない 512 本のうち、失敗した 64 本は次のように分類できます。

| 分類 | テスト |
|---|---|
| 未実装: 複素数 | `convT2X`、`print`、`ken/cplx0`、`cplx1`、`cplx2`、`cplx5`、`fixedbugs/bug329`、`bug401`、`bug491`、`issue5793`、`issue58671`、`issue79812` |
| 未実装: `goto` | `ken/label`、`fixedbugs/bug005`、`bug178`、`issue13684`、`issue40367`、`issue4748`、`issue75569` |
| 未実装: その他 | `convert4`（スライスから配列ポインタへの変換）、`range4` と `fixedbugs/issue71675`（range-over-func 内のラベルつき分岐）、`typeparam/issue54537`（型パラメータ変数のアドレス） |
| 64 ビット整数（既知の差異） | `intcvt`、`printbig`、`divmod`（タイムアウト）、`fixedbugs/issue2615`、`issue4448`、`issue43480`、`issue50854`、`issue70481`、`issue23305` |
| 間接的な `recover`（既知の差異） | `fixedbugs/issue73916`、`issue73916b`、`issue73917`、`issue73920` |
| 多値の式で初期化されるパッケージ変数（`var a, ok = m[k]`、`var x, y = f()`、`var v, ok = i.(T)`） | `fixedbugs/bug227`、`bug244`、`bug291`、`issue53619` |
| 再帰型（`type S []S`、`type Chan[T any] chan Chan[T]`）で goesm がスタックオーバーフローする | `ddd`、`fixedbugs/issue17039`、`typeparam/issue47901` |
| 型パラメータ上のメソッド呼び出し / generic な型のメソッド集合（`reading 'methods'`、ARCHITECTURE の既知の課題） | `typeparam/issue49421`、`issue53419`、`issue54225`、`shape1`、`typeswitch3`、`fixedbugs/issue54348` |
| その他の generics のバグ | `typeparam/interfacearg`（internal error）、`issue50833`（型 `P` の複合リテラル）、`issue44688`、`issue376214` |
| メソッド式（昇格メソッド、リテラル型のレシーバ） | `method`、`method7` |
| `new(expr)`（Go 1.26）が `new(T)` として lower される | `newexpr` |
| nil interface に対する `defer x.M()` は defer 文の時点で panic すべき | `fixedbugs/issue15975` |
| panic すべき nil 参照（`&*p`、nil ポインタのメソッド値） | `nilptr2`、`method5` |
| サイズ 0 の値（`[0]int`、`struct{}`） | `zerosize`、`fixedbugs/bug352` |
| range の代入順序: `for i, x[i] = range y` の `x[i]` は代入前の `i` を使うべき | `range` |
| 遅すぎる（20 秒以内に終了しない） | `fixedbugs/issue13169`（10 万回のチャネル送信）、`issue34395`（100 MiB の配列リテラル） |
