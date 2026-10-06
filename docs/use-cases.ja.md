# ユースケース

[English](use-cases.md)

goesm の対応範囲は、言語機能ではなくユースケースで定めます。方針は次のとおりです。

- **対応:** 以下の各ユースケースで典型的に書かれるコード、つまりそのユースケースで書かれるコードのおよそ 75 パーセンタイルまでは、ネイティブと同じように動きます。各行に確認方法を記します。大半は CI でネイティブ Go と比較しています。
- **未対応:** それより珍しいコード、つまりおよそ 95 パーセンタイルまでは、動くこともありますが保証しません。分かっている不足は[一覧](#未対応)にまとめます。
- **対象外:** JavaScript のホストが提供できないものです。cgo、生のメモリ、gc ランタイムの内部がこれに当たります。

ユースケースは実行場所で分類します。実行場所は Node.js、エッジ、ブラウザの 3 つです。これに加えて、3 つすべてにまたがるユースケースとして、Go のライブラリを JavaScript や TypeScript から import して使う形と、JavaScript や TypeScript を Go から呼ぶ形を扱います。

## Node.js: CLI、サーバー、SSR、ビルドツール

ここでの Node.js は Bun を含み、サーバーについては Deno も含みます。ファイル、環境変数、標準入出力、シグナル、goroutine 同士の待ち合わせはすべて動きます。

| ユースケース | 典型的なコード | 状態 | 確認方法 |
| --- | --- | --- | --- |
| CLI | `flag`、`os` の引数・環境変数・終了ステータス・ファイル、`path/filepath` の `WalkDir` と `Glob`、標準入力に対する `bufio.Scanner`、`encoding/csv`、`encoding/json`、`regexp`、`text/template`、`log`、`log/slog` | 対応 | `TestUseCaseCLI` の `testdata/usecases/cli`。Node.js と Bun で実行します |
| cobra を使う CLI | `spf13/cobra` のサブコマンド、フラグ、ヘルプ、エラー、シェル補完 | 対応 | `TestUseCaseCLI` の `testdata/usecases/cobra` |
| ビルドツール | `yuin/goldmark` による Markdown 変換、`gopkg.in/yaml.v3`、`BurntSushi/toml`、`compress/gzip`、`crypto/sha256`、ディレクトリの読み書き | 対応 | `TestUseCaseBuildTool` の `testdata/usecases/site`。[goesm.dev](https://github.com/goesm-dev/goesm.dev) も自身のサイトをこの形でビルドしています |
| サーバーサイドレンダリング | `embed` と組み合わせた `html/template` を、JS フレームワークのサーバー側から呼ぶ形 | 対応 | `TestUseCaseBuildTool` の `testdata/usecases/ssr`。Next.js の Server Component は手作業で確認しました |
| HTTP サーバー | `http.ListenAndServe` と `ServeMux` のメソッド・パスのパターン、JSON、フォーム、cookie、リダイレクト、ミドルウェア、`context`、Server-Sent Events、シグナルを受けての `Shutdown` | 対応 | `TestUseCaseServer` の `testdata/usecases/server`。Node.js と Bun で実行します。Deno は手作業で確認しました |
| Connect のサーバー | `connectrpc.com/connect` のハンドラ。Connect、Connect JSON、gRPC-Web の各プロトコルで、unary とサーバーストリーミング、エラー、ヘッダーを扱います | 対応 | `TestUseCaseServer` の `testdata/usecases/greet` |
| HTTP と Connect のクライアント | `fetch` を使う `net/http` のクライアント、ネイティブと同じく `http.Client` 自身がたどるリダイレクト、`connectrpc.com/connect` のクライアント | 対応 | `TestUseCaseServer`、`TestFetch` |

`http.ListenAndServe` はホスト自身のサーバーを起動します。Node.js では `node:http`、Bun では `Bun.serve`、Deno では `Deno.serve` を使います。各リクエストは専用の goroutine で処理します。

## エッジ: Cloudflare Workers などの fetch ハンドラ型のランタイム

`http.Handler` は、次のように Worker の fetch ハンドラになります。

```ts
import { Handler } from "./goesm-ts/example.com/app/api.ts";
export default { fetch: Handler() };
```

| ユースケース | 典型的なコード | 状態 | 確認方法 |
| --- | --- | --- | --- |
| Workers 上の HTTP API | Node.js と同じ `http.Handler`。`ServeMux`、JSON、cookie、リダイレクト、`context` を使います | 対応 | Cloudflare の Workers ランタイムである workerd で実行する `TestUseCaseEdge` の `testdata/usecases/edge` |
| Workers 上の Connect のサーバー | Connect、Connect JSON、gRPC-Web の unary とサーバーストリーミング | 対応 | `TestUseCaseEdge` |
| 設定とシークレット | `nodejs_compat` フラグを有効にした Worker で、テキストバインディングとシークレットを `os.Getenv` で読みます | 対応 | `TestUseCaseEdge` |
| 外部へのリクエスト、暗号、ストリーミング | `fetch` を使う `http.Get` と Connect のクライアント、`crypto/hmac`、`crypto/sha256`、`crypto/rand`、レスポンスの flush による Server-Sent Events | 対応 | `TestUseCaseEdge` |
| Next.js の edge の Route Handler | `export const runtime = "edge"` を指定した Route Handler から Go を呼びます | 対応 | 手作業で確認しました |

`fetchHandler` は `Deno.serve`、`Bun.serve`、Service Worker でも使えます。リクエストのボディはハンドラの実行前に全部読み込みます。レスポンスはハンドラが戻った時点で送り、ハンドラが flush した場合はその時点からストリームにします。

## ブラウザ: CSR

| ユースケース | 典型的なコード | 状態 | 確認方法 |
| --- | --- | --- | --- |
| コンポーネントから呼ぶドメインロジック | 構造体、メソッド、`errors.Is`・`As`・`Join` によるエラー処理、ジェネリクス、`encoding/json`、`regexp`、`strings`、`strconv`、`time` | 対応 | Vite でビルドし、Chromium で手作業で確認しました |
| Go からの DOM 操作 | `syscall/js` または `honnef.co/go/js/dom/v2` による要素の作成と検索、`js.FuncOf` によるイベントリスナー、入力値の読み取り、タイマーと goroutine。使い方は [dom.ja.md](dom.ja.md) にまとめています | 対応 | Chromium で手作業で確認しました |
| ブラウザでの Connect のクライアント | Connect、Connect JSON、gRPC-Web の unary とサーバーストリーミング、期限とエラー。ストリーミングは届いた順に受け取れます | 対応 | Vite でビルドし、Chromium で手作業で確認しました |
| React、Preact、Next.js | TSX から Go の関数を呼びます。描画中、イベントハンドラ、エフェクト、Next.js の Client Component と Server Component と Route Handler から呼び、Turbopack と webpack の両方でビルドします | 対応 | Vite 8 上の React 19 と Preact、Next.js 16 を Chromium で手作業で確認しました |

出力は TypeScript で書かれた ES モジュールなので、Vite、Rolldown、Turbopack、webpack、esbuild はプラグインなしでバンドルできます。Go は JSX の中に書くのではなく、他のモジュールと同じように JSX から呼びます。生成するファイルはすべて `// @ts-nocheck` で始まります。そのため、`noUnusedLocals` や ES2017 のターゲットといったプロジェクト側の厳しい設定は生成コードを検査し直さず、export された型だけが呼び出し側に届きます。残る設定は 2 つです。1 つは、モジュールを `.ts` 付きの名前で import する場合に TypeScript の `allowImportingTsExtensions` が必要になることで、拡張子なしで import すれば不要です。もう 1 つは、goesm の出力を `node_modules` のパッケージとして配布する場合に Next.js の `transpilePackages` が必要になることです。

Go のコードがページの JavaScript に加える量を、`goesm build -minify` で minify して gzip した大きさで示します。Connect のクライアントだけは Vite 8 で計測しました。

| ページが使うもの | gzip |
| --- | ---: |
| `syscall/js` による DOM 操作。[dom.ja.md](dom.ja.md) のカウンターのボタン | 9 KiB |
| `strings.Fields`、`Join`、`ToLower` を使う関数 | 15 KiB |
| 同じカウンターを `honnef.co/go/js/dom/v2` で書いたもの | 50 KiB |
| `fmt` による hello world | 90 KiB |
| `reflect` を伴う `encoding/json` による構造体への JSON のデコード | 193 KiB |
| protobuf と `net/http` を伴う Connect のクライアント | 1.3 MiB |

## JavaScript と TypeScript から使う Go のライブラリ

ビルドした Go のパッケージが export する関数は、上のどの実行場所でも、そのモジュールの export として TypeScript の型付きで使えます。

| ユースケース | 典型的なコード | 状態 | 確認方法 |
| --- | --- | --- | --- |
| export された Go の関数を TS から呼ぶ | 文字列、配列、プレーンオブジェクトを渡して受け取る関数、メソッドを持つハンドル、配列になる複数の戻り値、例外になる `error`、Promise を返すブロックしうる関数 | 対応 | `TestTSC`、`TestJS`、`TestExamples` |
| よく使われる純 Go のライブラリ | `google/uuid`、`golang.org/x/mod/semver`、`Masterminds/semver`、`shopspring/decimal`、`go-playground/validator`、`expr-lang/expr`、`tidwall/gjson`、`golang.org/x/text`、`yuin/goldmark`、`gopkg.in/yaml.v3` | 対応 | `TestUseCaseLibraries` の `testdata/usecases/libs` と `TestUseCaseBuildTool` |

引数と戻り値は Go の型に従って境界で変換されます。文字列は JS の文字列、スライスは配列、構造体はプレーンオブジェクトのまま渡して受け取れます。詳しくは [js-exports.ja.md](js-exports.ja.md) にまとめています。

## Go から使う JavaScript と TypeScript

Go のコードは、`//goesm:import` で宣言した ES モジュールの関数と値を、上のどの実行場所でも使えます。詳しくは [js-imports.ja.md](js-imports.ja.md) にまとめています。

| ユースケース | 典型的なコード | 状態 | 確認方法 |
| --- | --- | --- | --- |
| プロジェクトの TS・JS の関数を Go から呼ぶ | 文字列、スライス、マップ、構造体、関数を渡す呼び出し、Promise を待つ呼び出し、例外を error として受け取る呼び出し | 対応 | `TestJSImport` の `testdata/jsimport`、Node.js と Bun |
| npm パッケージのクラスや値を Go から使う | `js.Value` で受け取ったクラスの `New` と `Call` | 対応 | 手作業 |
| Vue コンポーネントを Go から使う | gosfc の Go ブロックで取り込んだコンポーネントをテンプレートで使う | gosfc で対応 | gosfc のテスト |

## 未対応

75 パーセンタイルから 95 パーセンタイルの間で分かっている不足を挙げます。これらを必要とするコードも、一部は動くことがあります。

- **ハンドルのフィールド:** メソッドを持つ構造体型へのポインタは Go のオブジェクトそのままで JavaScript に渡るので、そのフィールドには Go の内部表現が入っています。データはメソッドや関数を通して読みます。
- **バンドルの大きさ。** `fmt.Sprintf` を使うパッケージは gzip 後に約 95 KiB になります。その大半は、`fmt` がすべての引数に使う `reflect` です。`errors.New` 以外の関数呼び出しで初期化するパッケージ変数があると、import した側が何も使わなくても、そのパッケージはバンドルに残ります。`regexp.MustCompile` がその例です。`net/http` のクライアントは、リクエストを `fetch` で送るにもかかわらず TLS と HTTP/2 のコードを残します。`time`、`strings`、`strconv` の一部だけを使うコードは小さく収まります。`time` の 6 つの関数を使うパッケージは 13 KiB です。コンパイル時にわかるパターンの `regexp` はエンジンの `RegExp` で照合し、増えるのは約 5 KiB です。実行時に与えるパターンと、`(\w+)@(\w+)` のようにバックトラックが線形時間を超え得るパターンでは Go のエンジンが残り、約 60 KiB になります。
- **HTTP サーバー:** HTTP/2 と gRPC 本来のプロトコル、TLS による `ListenAndServeTLS`、WebSocket と `Hijack`、トレーラー、ハンドラの実行中に読むストリーミングのリクエストボディ、クライアントストリーミングと双方向ストリーミングの RPC、`net.Listener` を渡す `Serve` はまだありません。Connect と gRPC-Web は動きます。
- **fetch 以外の Workers の機能:** KV、D1、R2、Durable Objects などのバインディングは `syscall/js` を通してしか使えません。`ctx.waitUntil` にはつながっていないので、レスポンスの後も動いている goroutine は止められることがあります。
- **ネットワークとプロセス:** 生の TCP や UDP に対する `net.Dial` と `net.Listen`、`pgx` や `go-sql-driver/mysql` のように TCP で接続するデータベースドライバ、`os/exec` は使えません。
- **ブラウザでのタイムゾーン:** ブラウザで `time.LoadLocation` を使うには `import _ "time/tzdata"` が必要です。`time.Local` は、Go 自身の js/wasm 移植と同じく、起動時に決まる固定のオフセットです。
- **64 ビットの `int` と `uint`:** 2^53 未満では正確ですが、オーバーフローしても折り返しません。`int64` と `uint64` は正確です。
- **並列実行:** goroutine は 1 本の JavaScript スレッドを共有し、待つ箇所でだけ切り替わります。これは `GOMAXPROCS=1` でプリエンプションのない Go と同じです。待たずに長く計算する Go のコードは、他の goroutine、タイマー、リクエストを待たせます。`runtime.Gosched` を呼ばずにフラグをポーリングするコードは止まったままになります。並列に動かす処理は Worker に切り出します。実行モデル、データ競合、メモリ安全性は [docs/concurrency.ja.md](concurrency.ja.md) にまとめています。
- **panic:** goroutine の中で誰も recover しない panic は、Go と同じくプロセスを終了させます。Go のコードが Next.js のような Node.js のサーバーにライブラリとして組み込まれている場合も同じです。`js.FuncOf` のコールバックの中の panic は、呼び出し元ではなく `reportError` に渡ります。
- **配布:** Node.js は `node_modules` の下の `.ts` ファイルから型を取り除かないので、npm パッケージとして公開した goesm の出力にはバンドラが必要です。ソースマップは `.go` ファイルを絶対パスで示します。
- **ツール:** `syscall/js` を import するパッケージは、`GOOS=js GOARCH=wasm` を指定しない限り、build、vet、gopls での読み込みができません。

## 対象外

- cgo、`plugin`、標準ライブラリ以外のアセンブリ。
- [ARCHITECTURE.ja.md §7](../ARCHITECTURE.ja.md) に書いた範囲を超える `unsafe`。無関係なレイアウト間でのメモリの再解釈や、ポインタより長く生きるアドレスがこれに当たります。
- gc ランタイムにしか答えられないもの。`runtime.Caller` と `Stack`、ファイナライザとガベージコレクタの観測、goroutine のプリエンプションがこれに当たります。
- WebAssembly での出力。goesm は JavaScript にコンパイルします。
