# Go から JavaScript を呼ぶ

[English](js-imports.md)

goesm でコンパイルした Go のコードは、任意の ES モジュールの関数を呼び、値を使えます。対象はプロジェクト内の TypeScript や JavaScript のファイル、`node_modules` のパッケージ、そして [gosfc](https://github.com/goesm-dev/gosfc) 経由の Vue コンポーネントです。Go 側で取り込むものを Go の型で宣言すると、goesm が境界で値を変換します。

```ts
// format.ts
export function formatPrice(yen: number, currency: string): string {
  return new Intl.NumberFormat("ja-JP", { style: "currency", currency }).format(yen);
}
```

```go
package shop

//goesm:import "./format.ts" formatPrice
func formatPrice(yen int, currency string) string

func Label(price int) string {
	return "価格: " + formatPrice(price, "JPY")
}
```

## ディレクティブ

`//goesm:import` は、本体のない関数宣言か、値のないパッケージ変数の宣言の直前に書きます。

```go
//goesm:import "<モジュール>" [<エクスポート名>] [await]
```

- **モジュール**は import の指定子です。`./` か `../` で始まるパスは Go のファイルからの相対パスで、ファイルが存在しなければなりません。goesm は、生成したモジュールの位置から見た相対パスに書き換えて出力します。`chart.js` のようなそれ以外の指定子はパッケージを指します。`goesm build` は、実行したディレクトリとその親ディレクトリにある `node_modules` からパッケージを探します。`goesm emit-ts` の出力では指定子がそのまま残り、利用側のバンドラーが解決します。`node:path`、`bun:sqlite`、`cloudflare:sockets` のように JavaScript のランタイムに組み込まれたモジュールは、`goesm build` の出力でも import のまま残り、出力を実行する環境で解決されます。
- **エクスポート名**は取り込むエクスポートの名前です。`default` を書くか何も書かなければデフォルトエクスポートを、`*` を書けばモジュールの名前空間オブジェクトを取り込みます。
- **await** は、Promise を返す関数に付けます。Go からはチャネルの受信と同じく Promise が決着するまでブロックする関数として呼べ、その関数を呼ぶ側の関数も JavaScript では非同期になります。Promise を返す関数には `await` が必要です。付けずに宣言すると、Promise が返った時点で呼び出しが panic します。ただし、戻り値が `js.Value` の場合は Promise をそのまま受け取り、戻り値がない場合は Promise を待たずに戻ります。

```go
//goesm:import "chart.js" Chart
var Chart js.Value // クラスなので Chart.New(canvas, config) で生成する

//goesm:import "./api.ts" fetchUser await
func fetchUser(id string) (User, error)

//goesm:import "./config.ts" VERSION
var version string
```

パッケージ変数は、そのパッケージのほかの変数より先に、Go パッケージの初期化時点でモジュールがエクスポートしている値で初期化されます。

## 変換

引数は Go から JavaScript へ、戻り値は JavaScript から Go へ、宣言の型に従って変換されます。goesm が JavaScript の関数について知る手がかりは Go の宣言だけで、TypeScript の型は読みません。

| Go | JavaScript |
| --- | --- |
| `bool`、整数、浮動小数点数 | boolean、number |
| `int64`、`uint64` | bigint |
| `string` | string |
| `js.Value`、`js.Func` | 値そのもの |
| `[]byte` | `Uint8Array` |
| ほかのスライスと配列 | 配列 |
| `map[string]T` | プレーンなオブジェクト |
| 構造体と構造体へのポインタ | エクスポートされたフィールドを持つプレーンなオブジェクト。プロパティ名は `encoding/json` と同じく `json` タグの名前かフィールド名。埋め込んだ構造体と構造体へのポインタのフィールドは外側のオブジェクトのプロパティになる |
| 関数。引数としてのみ渡せる | 関数。その関数の引数と戻り値も同じ規則で変換される |
| `any` | JavaScript へは動的な値の型に従って変換し、Go へは `encoding/json` が `any` にデコードする形に変換する |
| 可変長引数 | 個別の引数 |

値はコピーされるので、相手側でスライスやマップや構造体を変更しても元の値は変わりません。JavaScript から Go を呼ぶ方向も、同じ変換表を逆向きに使います ([js-exports.ja.md](js-exports.ja.md))。DOM 要素やクラスのインスタンスのように同一性を保ちたい値は `js.Value` で受け渡します。複数の戻り値は、JavaScript の関数が返す配列の要素に対応します。チャネルや複素数のように JavaScript に対応する型がない型を使うと、ディレクティブの位置でコンパイルエラーになります。

## エラー

最後の戻り値が `error` の関数では、JavaScript の関数が投げた例外と Promise の reject がその error として返り、ほかの戻り値はゼロ値になります。正常に戻った場合の error は nil です。`error` の戻り値がない関数では、例外は panic になり、`recover` で止められます。どちらの場合も、Go パッケージが `syscall/js` を import していればエラーは `js.Error` になり、`errors.As` で投げられた値を取り出せます。import していない場合も、メッセージが同じ `JavaScript error: <メッセージ>` のエラーになります。文字列や `null` のようにオブジェクトでない値が投げられた場合は、その値を文字列にしたものをメッセージとし、元の値を `cause` に持つ `Error` に包みます。

```go
//goesm:import "./lib.ts" parsePrice
func parsePrice(s string) (float64, error)

_, err := parsePrice("abc")
var jerr js.Error
if errors.As(err, &jerr) {
	fmt.Println(jerr.Get("name").String()) // Error
}
```

## 性能

引数が数値か ASCII の文字列か、それらをフィールドに持つ構造体であれば、`//goesm:import` 経由の呼び出しは JavaScript から同じ関数を呼ぶ場合より数ナノ秒多くかかるだけです。同じ呼び出しを `syscall/js` で書くと、数値と文字列では 20〜50 ns 多くかかり、スライスと構造体では 8〜40 倍の時間がかかります。`syscall/js` のコードは、スライスと構造体を要素やプロパティごとに組み立てるためです。`syscall/js` の性能は [dom.ja.md](dom.ja.md#性能) でも説明しています。計測は [bench/jsimport](../bench/jsimport) で行いました。

| 呼び出し | JS → JS | Go → JS、`//goesm:import` | Go → JS、`syscall/js` |
| --- | ---: | ---: | ---: |
| `add(int, int) int` | 2.6 ns | 3.1 ns | 23 ns |
| `strlen(string) int`、ASCII | 2.9 ns | 9.1 ns | 36 ns |
| `strlen(string) int`、日本語 | 5.3 ns | 124 ns | 158 ns |
| `upper(string) string`、ASCII | 29 ns | 44 ns | 83 ns |
| `total(struct) int` | 8.1 ns | 5.0 ns | 218 ns |
| `sum([]float64) float64`、8 要素 | 18 ns | 35 ns | 278 ns |

計測環境は 4 vCPU の Intel Xeon 2.10 GHz のクラウド VM 上の Node.js 22 で、7 回の計測の中央値です。Bun では、数値と文字列の呼び出しはほぼ同じ時間で、構造体とスライスの呼び出しは 2〜4 倍の時間がかかります。ASCII 以外の文字を含む文字列は呼び出しのたびに UTF-8 と UTF-16 の間で変換され、これが残っているコストです。スライスは要素ごとにコピーされます。

## 制限

- `//goesm:import` を含むパッケージは goesm でしかビルドできません。`go build` は本体のない関数をエラーにします。`go vet` と gopls はそのまま受け付けます。
- `goesm build` は esbuild でバンドルするので、`.ts`、`.js`、`.mjs` は扱えますが `.vue` は扱えません。Vue コンポーネントは、Vite でビルドする gosfc から使います。
- Go に返された JavaScript の関数は `js.Value` になり、`Invoke` で呼びます。
- JavaScript に渡した Go の関数がブロックする場合、つまりスリープやチャネルの待機、`await` を付けた import の呼び出しを含む場合、その関数は JavaScript に Promise を返します。`xs.map(f)` のように結果を await せずに呼ぶ JavaScript のコードは、値の代わりに Promise を受け取ります。
- 生成されるモジュールでは、取り込んだ関数の TypeScript の型は `any` です。
- Go の宣言は手で書きます。`.d.ts` から生成する仕組みはありません。
