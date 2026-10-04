# compile-time instrumentation (otelc)

OpenTelemetry の Go 向け compile-time instrumentation である [otelc](https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation) は、そのまま goesm で使えます。program の HTTP request、log、span は、native build と同じように Node.js、Bun、browser で trace されます。

## 使い方

`otelc setup` は goesm の target である `GOOS=js GOARCH=wasm` 向けに、goesm が使う toolchain で module を準備する必要があります。そのうえで `goesm build` (または `emit-ts`) に `go build` と同じく otelc を `-toolexec` program として渡します:

```sh
GOOS=js GOARCH=wasm otelc setup
goesm build -toolexec "otelc toolexec" ./cmd/app
OTEL_TRACES_EXPORTER=console node dist/app.js
```

`GOFLAGS` の `-toolexec` でも動きます (`GOFLAGS="'-toolexec=otelc toolexec'" goesm build ./cmd/app`。go command と同じく goesm は `GOFLAGS` を空白で区切るので、quote は flag 全体を囲みます)。host 向けに setup した module (`GOOS`/`GOARCH` なしの `otelc setup`) は js/wasm 向けには build できません。setup は別々に (たとえば別の checkout で) 用意してください。

## native Go と一致するもの

`TestOtelc` (`go test ./test -run TestOtelc`、`GOESM_TEST_OTELC` に otelc の binary を指定) は [testdata/otelc](../testdata/otelc) を native と goesm で build し、console exporter が出す telemetry が Node.js と Bun で同じであることを確認します:

* otelc の `net/http` client instrumentation が作る span (名前、kind、attribute、status)
* OpenTelemetry API で開始した span と、その下にネストする HTTP span (その中で起動した goroutine からのものも含む。otelc は現在の span を goroutine-local storage で伝播します)
* 現在の span と相関した `log` の行 (`trace_id=... span_id=...`) と、SDK 自身の `slog` 出力
* OTLP/HTTP (`OTEL_TRACES_EXPORTER=otlp`、`OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf`) で export した同じ span: collector が受け取る protobuf-go で encode された request は、同じ span、attribute、親子関係に decode されます

resource は program が動く host を表します: `os.type` は `js`、`process.runtime.name` は `goesm` で、Node.js と Bun では `process.executable.name` は script です。

## 仕組み

* `-toolexec`: goesm は otelc を通して target 向けの build を実行し、各 compile が見る source (otelc は関数を hook 呼び出しに書き換え、file を追加します) を記録して、その source を lower します。[ARCHITECTURE.ja.md §3](../ARCHITECTURE.ja.md) を参照してください。
* hook には `//go:linkname` で到達し、goroutine-local storage は goesm の goroutine 上の otelc の `runtime` API で、OpenTelemetry SDK の依存 (`//go:embed`、protobuf-go の `unsafe` header struct と field offset による pointer 演算) は [ARCHITECTURE.ja.md §7 と §9](../ARCHITECTURE.ja.md) のとおり lower されます。
* OTLP/HTTP exporter の `http.Transport` は `net.Dialer` で dial し、js/wasm port はそれに従って process 内の network に dial します。goesm はそのような request を `http.DefaultTransport` の request と同じく `fetch` で送ります (独自の dialer を持つ Transport は引き続きその dialer で dial します)。

## 制限

* gRPC の OTLP (`OTEL_EXPORTER_OTLP_PROTOCOL=grpc`) は HTTP/2 の接続が必要ですが、JS の host は program にそれを渡しません。`http/protobuf` を使ってください。別 origin の collector は browser の CORS request を許可する必要があります。
* batch span processor (既定) は timer と provider の shutdown 時に export します。native build と同じく、処理の直後に終了する program では queue に残った span は失われます。`OTEL_GO_SIMPLE_SPAN_PROCESSOR=true` なら span の終了ごとに export します。
* Node.js と Bun では HTTP server は listen できません (`net.Listen` は js/wasm port の process 内 network)。そのため server instrumentation が効くのは process 内の server だけで、その client は `fetch` を使えません。
* 他の library (database driver、gRPC など) の instrumentation は未検証です。goesm が lower できない library の関数は呼ぶと panic する stub になります (`goesm build -v` で一覧できます)。
