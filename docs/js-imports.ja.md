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

- **モジュール**は import の指定子です。`./` か `../` で始まるパスは Go のファイルからの相対パスで、ファイルが存在しなければなりません。goesm は、生成したモジュールの位置から見た相対パスに書き換えて出力します。`chart.js` や `node:path` のようなそれ以外の指定子はバンドラーが解決し、その際は Go パッケージのディレクトリにあるファイルからの import と同じ扱いになります。
- **エクスポート名**は取り込むエクスポートの名前です。`default` を書くか何も書かなければデフォルトエクスポートを、`*` を書けばモジュールの名前空間オブジェクトを取り込みます。
- **await** は、Promise を返す関数に付けます。Go からはチャネルの受信と同じく Promise が決着するまでブロックする関数として呼べ、その関数を呼ぶ側の関数も JavaScript では非同期になります。

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

値はコピーされるので、相手側でスライスやマップや構造体を変更しても元の値は変わりません。DOM 要素やクラスのインスタンスのように同一性を保ちたい値は `js.Value` で受け渡します。複数の戻り値は、JavaScript の関数が返す配列の要素に対応します。チャネルや複素数のように JavaScript に対応する型がない型を使うと、ディレクティブの位置でコンパイルエラーになります。

## エラー

最後の戻り値が `error` の関数では、JavaScript の関数が投げた例外と Promise の reject がその error として返り、ほかの戻り値はゼロ値になります。正常に戻った場合の error は nil です。`error` の戻り値がない関数では、例外は panic になり、`recover` で止められます。どちらの場合も、Go パッケージが `syscall/js` を import していればエラーは `js.Error` になり、`errors.As` で投げられた値を取り出せます。import していない場合も、メッセージが同じ `JavaScript error: <メッセージ>` のエラーになります。

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

引数が数値か ASCII の文字列か、それらをフィールドに持つ構造体であれば、`//goesm:import` 経由の呼び出しは JavaScript から同じ関数を呼ぶ場合より数ナノ秒多くかかるだけです。同じ呼び出しを `syscall/js` で書くと、日本語の文字列で 3 倍、構造体では最大 160 倍の時間がかかります。計測は [bench/jsimport](../bench/jsimport) で行いました。

| 呼び出し | JS → JS | Go → JS、`//goesm:import` | Go → JS、`syscall/js` |
| --- | ---: | ---: | ---: |
| `add(int, int) int` | 2.2 ns | 3.0 ns | 163 ns |
| `strlen(string) int`、ASCII | 2.6 ns | 7.8 ns | 146 ns |
| `strlen(string) int`、日本語 | 5.0 ns | 117 ns | 317 ns |
| `upper(string) string`、ASCII | 30 ns | 48 ns | 280 ns |
| `total(struct) int` | 7.3 ns | 5.3 ns | 840 ns |
| `sum([]float64) float64`、8 要素 | 17 ns | 37 ns | 1,406 ns |

計測環境は 4 vCPU の Intel Xeon 2.80 GHz のクラウド VM 上の Node.js 22 で、Bun でもほぼ同じ結果になります。ASCII 以外の文字を含む文字列は呼び出しのたびに UTF-8 と UTF-16 の間で変換され、これが残っているコストです。スライスは要素ごとにコピーされます。

## 制限

- `//goesm:import` を含むパッケージは goesm でしかビルドできません。`go build` は本体のない関数をエラーにします。`go vet` と gopls はそのまま受け付けます。
- `goesm build` は esbuild でバンドルするので、`.ts`、`.js`、`.mjs` は扱えますが `.vue` は扱えません。Vue コンポーネントは、Vite でビルドする gosfc から使います。
- Go に返された JavaScript の関数は `js.Value` になり、`Invoke` で呼びます。
- 生成されるモジュールでは、取り込んだ関数の TypeScript の型は `any` です。
- Go の宣言は手で書きます。`.d.ts` から生成する仕組みはありません。
