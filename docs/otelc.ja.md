# compile-time instrumentation (otelc)

OpenTelemetry の Go 向け compile-time instrumentation である [otelc](https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation) は、そのまま goesm で使えます。program の HTTP request、log、span は、native build と同じように Node.js、Bun、browser で trace されます。

## 使い方

`otelc setup` は goesm の target である `GOOS=js GOARCH=wasm` 向けに、goesm が使う toolchain で module を準備する必要があります。そのうえで `goesm build` (または `emit-ts`) に `go build` と同じく otelc を `-toolexec` program として渡します:

```sh
GOOS=js GOARCH=wasm otelc setup
goesm build -toolexec "otelc toolexec" ./cmd/app
OTEL_TRACES_EXPORTER=console node dist/app.js
```

`GOFLAGS` の `-toolexec` でも動きます (`GOFLAGS=-toolexec='otelc toolexec' goesm build ./cmd/app`)。host 向けに setup した module (`GOOS`/`GOARCH` なしの `otelc setup`) は js/wasm 向けには build できません。setup は別々に (たとえば別の checkout で) 用意してください。

## native Go と一致するもの

`TestOtelc` (`go test ./test -run TestOtelc`、`GOESM_TEST_OTELC` に otelc の binary を指定) は [testdata/otelc](../testdata/otelc) を native と goesm で build し、console exporter が出す telemetry が Node.js と Bun で同じであることを確認します:

* otelc の `net/http` client instrumentation が作る span (名前、kind、attribute、status)
* OpenTelemetry API で開始した span と、その下にネストする HTTP span (その中で起動した goroutine からのものも含む。otelc は現在の span を goroutine-local storage で伝播します)
* 現在の span と相関した `log` の行 (`trace_id=... span_id=...`) と、SDK 自身の `slog` 出力

resource は program が動く host を表します: `os.type` は `js`、`process.runtime.name` は `goesm` で、Node.js と Bun では `process.executable.name` は script です。

## 仕組み

* `-toolexec`: goesm は otelc を通して target 向けの build を実行し、各 compile が見る source (otelc は関数を hook 呼び出しに書き換え、file を追加します) を記録して、その source を lower します。[ARCHITECTURE.ja.md §3](../ARCHITECTURE.ja.md) を参照してください。
* hook には `//go:linkname` で到達し、goroutine-local storage は goesm の goroutine 上の otelc の `runtime` API で、OpenTelemetry SDK の依存 (`//go:embed`、protobuf の `unsafe` header struct) は [ARCHITECTURE.ja.md §7 と §9](../ARCHITECTURE.ja.md) のとおり lower されます。

## 制限

* OTLP exporter はまだ動きません: protobuf-go で encode しますが、生成 message の高速経路は struct field の offset による pointer 演算を行い、goesm はこれを lower できません (それらの関数は stub になり、export は panic します)。console exporter か `OTEL_TRACES_EXPORTER=none` を使ってください。
* Node.js と Bun では HTTP server は listen できません (`net.Listen` は js/wasm port の process 内 network)。そのため server instrumentation が効くのは process 内の server だけで、その client は `fetch` を使えません。
* 他の library (database driver、gRPC など) の instrumentation は未検証です。goesm が lower できない library の関数は呼ぶと panic する stub になります (`goesm build -v` で一覧できます)。
