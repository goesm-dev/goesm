# Go から DOM を使う

[English](dom.md)

goesm でコンパイルした Go のコードは、DOM とその他のブラウザ API を `syscall/js` から使います。`syscall/js` は Go 自身の WebAssembly 版と同じパッケージなので、`GOOS=js` 向けに書かれたコードとライブラリは変更なしで goesm でもビルドできます。よく使う `syscall/js` の呼び出しは、goesm が JavaScript のプロパティアクセスと関数呼び出しに直接コンパイルします。goesm は独自の DOM パッケージを持ちません。

```go
package main

import (
	"strconv"
	"syscall/js"
)

func main() {
	doc := js.Global().Get("document")
	button := doc.Call("createElement", "button")
	button.Set("textContent", "0")
	count := 0
	button.Call("addEventListener", "click", js.FuncOf(func(this js.Value, args []js.Value) any {
		count++
		button.Set("textContent", strconv.Itoa(count))
		return nil
	}))
	doc.Get("body").Call("append", button)
}
```

`goesm build -minify ./counter` は `dist/counter.js` を書き出し、ページはこれをモジュールスクリプトとして読み込みます。このモジュールは gzip で 9 KiB です。同じプログラムを Go の WebAssembly 版でビルドすると、gzip で 567 KiB になります。

```html
<script type="module" src="dist/counter.js"></script>
```

## 型付きの DOM ライブラリ

[honnef.co/go/js/dom/v2](https://pkg.go.dev/honnef.co/go/js/dom/v2) は、`Document`、`Element` と各 HTML 要素の型、イベントなどの Go の型で DOM を包むライブラリです。goesm でもそのまま動きます。

```go
import "honnef.co/go/js/dom/v2"

func main() {
	doc := dom.GetWindow().Document()
	button := doc.CreateElement("button")
	button.SetTextContent("0")
	count := 0
	button.AddEventListener("click", false, func(dom.Event) {
		count++
		button.SetTextContent(strconv.Itoa(count))
	})
	doc.QuerySelector("body").AppendChild(button)
}
```

型とメソッドが多いため、モジュールは大きくなります。上のカウンターは gzip で 50 KiB で、Go の WebAssembly 版では 704 KiB です。

## イベントハンドラ

`js.FuncOf` は、Go の関数を呼ぶ JavaScript の関数を作ります。Go の関数は、最初に待つところまで同期的に実行されます。待つ処理とは、チャネル、`time.Sleep`、HTTP リクエスト、`await` を付けた import などです。詳しくは [js-imports.ja.md](js-imports.ja.md) にまとめています。待つハンドラは JavaScript に Promise を返し、DOM はその Promise を無視します。そのため、`preventDefault` と `stopPropagation` は最初に待つ前に呼びます。

```go
form.Call("addEventListener", "submit", js.FuncOf(func(this js.Value, args []js.Value) any {
	args[0].Call("preventDefault") // 下のリクエストより前に呼ぶ
	resp, err := http.Post("/api/orders", "application/json", body)
	// ...
	return nil
}))
```

`js.FuncOf` で作った関数は、JavaScript の関数と一緒にガベージコレクタが回収します。そのため `Release` は不要で、呼んでも何もしません。goroutine とタイマーはページのイベントループの上で動きます。この実行モデルは [concurrency.ja.md](concurrency.ja.md) で説明しています。

## 性能

`Value.Get`、`Set`、`SetIndex`、`Call`、`Invoke`、`New` は、すべての引数が真偽値、数値、文字列、`js.Value`、`js.Func`、`nil` のいずれかであれば、JavaScript のプロパティアクセスと関数呼び出しに直接コンパイルされます。プロパティ名とメソッド名が ASCII の定数であれば、変換せずにそのまま使います。通常はこの場合に当たります。スライスやマップなどその他の引数は、Go と同じく実行時に `js.ValueOf` で変換します。

Node.js では、`syscall/js` 経由の呼び出しは JavaScript から同じ関数を呼ぶ場合より 20〜50 ns ほど多くかかります。計測結果は [js-imports.ja.md](js-imports.ja.md#性能) に載せています。イベントハンドラやページの組み立てでは無視できる差です。大きなデータの要素ごとに境界を越えるコード、たとえばグラフを 1 点ずつ描くコードは、TypeScript で書いて `//goesm:import` から呼ぶほうが速くなります。`//goesm:import` はスライスや構造体を 1 回の呼び出しでまとめて渡します。

```go
//goesm:import "./chart.ts" drawPoints
func drawPoints(canvas js.Value, points []Point)
```

## UI フレームワークと組み合わせる

コンポーネントの多いアプリケーションでは、状態と DOM の同期はフレームワークに任せ、Go はロジックを受け持つ構成が向いています。[gosfc](https://github.com/goesm-dev/gosfc) を使うと、Vue コンポーネントの `<script setup lang="go">` と Astro のページに Go を書けます。React などのフレームワークは、goesm の出力を ES モジュールとして import します。詳しくは [use-cases.ja.md](use-cases.ja.md) にまとめています。
