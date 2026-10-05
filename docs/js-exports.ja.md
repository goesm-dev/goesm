# JavaScript から Go を呼ぶ

[English](js-exports.md)

ビルドした Go パッケージのエクスポートされた関数は、普通の JavaScript の値を受け取って返す JavaScript の関数になります。goesm は、宣言の Go の型に従って、引数と戻り値を境界で変換します。変換表は [`//goesm:import`](js-imports.ja.md) と同じもので、向きが逆になります。手で変換する必要はありません。

```go
package shop

type Item struct {
	Name  string `json:"name"`
	Price int    `json:"price"`
}

func Total(items []Item) int { ... }
func Label(it Item) string   { ... }
func Parse(s string) (Item, error) { ... }
```

```ts
import { Label, Parse, Total } from "./goesm-ts/example.com/app/shop.ts";

Total([{ name: "りんご", price: 120 }, { name: "パン", price: 250 }]); // 370
Label({ name: "みかん", price: 80 });                              // JS の文字列
try {
  Parse("x");
} catch (e) {
  console.log(e.name, e.message); // GoError と、err.Error() の文字列
}
```

ビルドしたパッケージとは、`goesm emit-ts ./shop` や `goesm build ./shop` の `./shop` のようにコマンドラインで指定したパッケージです。gosfc の `go:` の import で指定したパッケージもこれにあたります。このパッケージのモジュールを import してください。このパッケージが import するパッケージのモジュールもツリーに含まれますが、それらは生成されたモジュールから使うためのもので、関数は Go の内部の値の表現のままエクスポートされます。

## 変換

| Go | JavaScript |
| --- | --- |
| `bool`、`int`、`float64` などの数値 | boolean、number。変換せずにそのまま渡す |
| `int64`、`uint64` | bigint。引数には number も渡せる |
| `string` | string |
| `[]byte` | `Uint8Array`。引数には数値の配列も渡せる |
| その他のスライスと配列 | 配列 |
| `map[string]T` | プレーンオブジェクト |
| その他のマップ | `Map` |
| 構造体 | エクスポートされたフィールドを持つプレーンオブジェクト。名前は `encoding/json` と同じく、`json` タグの名前かフィールド名になる。埋め込んだ構造体のフィールドは外側のオブジェクトのフィールドになる。引数で省いたフィールドはゼロ値になる |
| メソッドを持つ構造体型へのポインタ | Go のオブジェクトそのもの。ハンドルとして扱う (後述) |
| その他のポインタ | 指す先の値を変換したもの。nil は `null`。引数では、変換した値のコピーを指すポインタになる |
| `error` | 最後の戻り値の `error` は例外として投げる (後述)。それ以外の位置では `GoError` か `null` |
| `any` | 動的な値を変換したもの。引数は `encoding/json` が `any` にデコードするときと同じ形になる |
| その他のインターフェース | Go の値。ハンドルとして扱う |
| 戻り値の `http.Handler` | fetch ハンドラ `(request: Request) => Promise<Response>` |
| 関数 | 関数。その関数の引数と戻り値も同じ規則で変換し、最後の `error` の戻り値は例外として投げる |
| チャネル、複素数、`unsafe.Pointer` | Go の値。ハンドルとして扱う |
| `js.Value`、`js.Func` | その値そのもの |
| 可変長引数 | 個別の引数 |
| 複数の戻り値 | 最後の `error` を除いた戻り値の配列 |

値はコピーされます。一方で配列やオブジェクトを変更しても、もう一方には反映されません。JavaScript が受け取った Go の値は、その型が期待される場所にそのまま渡せます。パッケージの構造体クラスのオブジェクトはコピーされ、スライスとマップはそのまま使われます。

数値は検査しません。`int` に小数を渡すとそのまま Go に届くので、整数を渡してください。TypeScript ではどちらも `number` 型です。

## ハンドル

Go のコードは、状態を持つオブジェクトを、メソッドを持つ構造体型へのポインタで渡すのが普通です。そのため JavaScript は Go のオブジェクトそのものを受け取り、そのメソッドを呼びます。

```go
type Cart struct{ owner string; items []Item }

func NewCart(owner string) *Cart { return &Cart{owner: owner} }
func (c *Cart) Add(items ...Item) { c.items = append(c.items, items...) }
func (c *Cart) Total() int        { return Total(c.items) }
```

```ts
const cart = NewCart("かなで");
cart.Add({ name: "みかん", price: 100 }, { name: "柿", price: 80 });
cart.Total(); // 180
```

ハンドルのメソッドは、パッケージの関数と同じ規則で引数と戻り値を変換します。各メソッドは `Cart$Add(cart, item)` という名前のモジュールの関数としても使えます。export された関数が返す `*strings.Builder` のように、他のパッケージの型のハンドルも、その型の公開メソッドを持ちます。ハンドルのフィールドには Go の内部表現が入っています。Go の文字列は JS の文字列ではありません。データは、変換を行うメソッドや関数を通して読んでください。

インターフェースが期待される場所に渡したハンドルや、パッケージの構造体クラスのオブジェクトは、その型がインターフェースのメソッドを持っていれば、そのインターフェースの値になります。

## エラーと panic

最後の戻り値が `error` の関数は、他の戻り値を返すか、エラーを `GoError` として投げます。`GoError` は `name` が `"GoError"` の `Error` で、`message` は `err.Error()` の文字列です。`error` が期待される場所に `GoError` を渡すと、元の Go のエラーに戻ります。そのため `errors.Is` や `errors.As` が使えます。それ以外の値は、同じメッセージを持つエラーになります。

```go
var ErrEmpty = errors.New("empty input")

func SumCSV(s string) (int, error)
func IsEmpty(err error) bool { return errors.Is(err, ErrEmpty) }
```

```ts
try {
  SumCSV(" ");
} catch (e) {
  IsEmpty(e); // true
}
```

JavaScript まで届いた panic は、`name` が `"GoPanic"` の `Error` として投げられます。メッセージには panic の値が入り、スタックトレースは `.go` ファイルを指します。

## ブロックする関数

チャネル、ミューテックス、`time.Sleep`、`await` 付きで import した JavaScript の Promise などでブロックする Go の関数は、変換した戻り値の Promise を返します。エラーの戻り値は Promise を reject します。JavaScript から Go に渡した関数は同期的に呼ばれ、その戻り値がそのまま使われます。

## エッジのサーバー

戻り値の `http.Handler` は fetch ハンドラになります。Cloudflare Workers、`Deno.serve`、`Bun.serve`、Service Worker で使えます。

```go
func Handler() http.Handler { ... }
```

```ts
import { Handler } from "./goesm-ts/example.com/app/api.ts";
export default { fetch: Handler() };
```

## TypeScript

エクスポートには、変換表の JavaScript の型が付きます。たとえば `Total(items: Array<{ name?: string; price?: number }> | null): number` のようになり、ハンドルにはそのクラスの型が、ブロックする関数には Promise の型が付きます。引数の型が違う呼び出しは TypeScript がエラーにします。

## 性能

数値はそのまま渡すので、呼び出しのコストは JavaScript の関数の呼び出しと同じです。文字列は、渡すときに UTF-16 から UTF-8 に、受け取るときに UTF-8 から UTF-16 に変換します。ASCII の文字列は、走査したうえでそのまま渡します。スライス、マップ、構造体は要素ごとにコピーします。この費用は [bench](../bench) の呼び出し系カーネルで測っています。`Add`、`Upper`、`Handle` は、数値や文字列を渡して Go を 10 万回または 1 万回呼び出します。
