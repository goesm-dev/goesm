# goesm への貢献

[English](CONTRIBUTING.md)

ご協力ありがとうございます。goesm は実験段階なので、native Go と goesm で振る舞いが違う Go プログラムを示す issue も、コードと同じくらい歓迎します。

## 方針

* **Go が基準です。** 構文解析、module の解決、型検査は Go toolchain (go/packages + go/types) が行います。goesm は独自の構文、module system、型システムを追加しません。`go vet` が通る package が、goesm が扱うべき入力です。
* **native Go が正解です。** `go run` と同じ結果なら正しい振る舞いです。既知の差分は [ARCHITECTURE.ja.md](ARCHITECTURE.ja.md) §11 にあり、`TestKnownGaps` で固定しています。
* **compiler plugin はありません。** 依存 package を import しても goesm の中でコードは実行されません。標準 library の置換と natives は goesm 内の固定の集合で、`$GOROOT/src` にだけ適用されます (ARCHITECTURE.ja.md §10)。
* **出力は普通の ESM です。** 出力する TypeScript tree は、plugin なしでどの bundler や TypeScript 対応 runtime でも動き、strict な `tsc` で型検査が通る必要があります。主な対象は browser の ESM で、Node.js と Bun の両方でも動く必要があるので、runtime は Node 専用の API を使いません。
* **性能が重要です。** goesm は WebAssembly より速い選択肢として存在するので、表現を選ぶときは生成される JavaScript の速度を考慮してください。

## 環境構築

[mise](https://mise.jdx.dev) が `mise.toml` で pin した Go、Node.js、Bun を入れます。CI も同じ version を使います。

```sh
mise install
npm ci --prefix test   # test/package.json で pin した tsc、oxlint、workerd
```

mise を使わない場合は Go 1.27 以上と Node.js 22.18 以上が必要です。Bun は任意です。ツールの version を変えるときは、`mise.toml` か `test/package.json` で正確な version に pin してください (`latest` は使いません)。

## テスト

```sh
go test ./...
```

| テスト | 確認すること |
| --- | --- |
| `TestGolden` | `testdata/semantics/{basics,generics,goroutines,panics,stdlibuse}` の引数なし exported 関数すべてが、native Go と goesm が build した ESM で同じ値を返す |
| `TestJS` | build した bundle に対する `test/js/*.test.mjs` (node:test) |
| `TestKnownGaps` | 文書化した native Go との差分がまだ存在する (`testdata/semantics/gaps`) |
| `TestExamples` | `examples/*` が Node.js (インストールされていれば Bun でも) で動き、`output.txt` どおりに出力する |
| `TestTSC` | 出力した TypeScript が strict な `tsc` で型検査を通る |
| `TestOxlint` | build した ESM に oxlint の correctness の指摘がない |
| `TestPrograms` | `testdata/programs` の command が native Go と goesm (Node.js と Bun) で同じ出力をし、同じ status で終了する |
| `TestToolexec` | `-toolexec` program による module と標準 library の書き換えが、`go build` と同じように goesm の出力に反映される (`testdata/toolexec`) |
| `TestFetch` | HTTP client が `fetch` を使い、素の `net.Dialer` を持つ Transport でも同様で、独自の dialer は引き続き呼ばれる (`testdata/fetch`、local の server に対して) |
| `TestUseCase*` | [docs/use-cases.ja.md](docs/use-cases.ja.md) のユースケース (`testdata/usecases`) が native と同じように動く: CLI、ビルドツール、SSR、よく使われるライブラリ、`http.ListenAndServe` (Node.js と Bun) と Cloudflare Workers の fetch handler (workerd) で動く HTTP と Connect の server |
| `TestStdlibStatus -v` | 標準 library のどの package が lowering でき、何個の関数が stub かを報告する |

`TestTSC` と `TestOxlint` は `npm ci --prefix test` をしていないと skip されます。CI では `GOESM_REQUIRE_TOOLS=1` を設定しているので skip できません。CI は `gofmt -l .` と `go vet ./...` も確認します。

### otelc

`TestOtelc` は `testdata/otelc` を host 向けと goesm 向けに otelc で計装し、telemetry を比べます ([docs/otelc.ja.md](docs/otelc.ja.md))。`GOESM_TEST_OTELC` に otelc の binary を指定したときだけ動き、`otelc setup` が追加する module のために network が必要です。

```sh
GOBIN=/tmp/otelc go install go.opentelemetry.io/otelc/tool/cmd/otelc@v1.1.0
GOESM_TEST_OTELC=/tmp/otelc/otelc go test ./test -run TestOtelc -v
```

### Go conformance suite

`TestGoConformance` は Go 配布物の `test/` directory にある `// run` テストを goesm で build し、Go 自身の test runner と同じように出力を `.out` file と比べます。約 1000 個のプログラムを build するので、明示的に有効にしたときだけ動きます。

```sh
GOESM_CONFORMANCE=1 go test ./test -run TestGoConformance -v
GOESM_CONFORMANCE=1 GOESM_CONFORMANCE_RUN='^ken/' go test ./test -run TestGoConformance -v   # 一部だけ
```

完全な Go 配布物の `test/` directory が必要です。`mise install` で入れた Go にはありますが、`GOTOOLCHAIN` でダウンロードした toolchain にはありません。失敗したケースを 1 つ再現するには、新しい module にコピーして native Go と比べます。

```sh
mkdir /tmp/case && cp "$(go env GOROOT)/test/ken/chan.go" /tmp/case/main.go
cd /tmp/case && go mod init case && go run . > native.txt 2>&1
goesm build ./ && node dist/case.js > goesm.txt 2>&1
diff native.txt goesm.txt
```

環境変数、`test/conformance/passing.txt` の baseline とその更新方法は [docs/conformance.ja.md](docs/conformance.ja.md) を参照してください。

## Pull request

* **意味論の修正には必ず golden fixture を付けてください。** バグを再現する引数なしの exported 関数を `testdata/semantics/` の package に追加すると、`TestGolden` が native Go と比べます。修正で conformance テストが通るようになったら baseline も更新してください。
* 意図して native Go と違うままにする場合は、代わりに `testdata/semantics/gaps` と ARCHITECTURE.md §11 に追加してください。
* 生成コードは `TestTSC` と `TestOxlint` を通る状態に保ってください。green にするためにルールを無効にしないでください。
* ドキュメントは英語の file と、その隣の日本語の `.ja.md` があります。両方を更新してください。
* push する前に `gofmt` と `go test ./...` を実行してください。

## ライセンス

goesm は [BSD 3-Clause License](LICENSE) で公開しています。貢献したものは、このライセンスで提供されることに同意したものとみなします。
