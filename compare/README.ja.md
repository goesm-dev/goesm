# ライブラリとの比較

[English](README.md)

Web アプリが普段は人気の JavaScript ライブラリに任せる処理を、Go で書いて goesm でコンパイルすると、サイズと速度はどうなるか。ここではそれを比べる。各比較は同じ処理を 2 回実装する。1 つは goesm でコンパイルする Go パッケージで、もう 1 つはそのライブラリを使う JavaScript である。両者は同じ方法でバンドルし、同じ処理を JavaScript から呼んで計測する。両者の結果が一致しない場合、計測は失敗する。

| ライブラリ | Go パッケージ | 両者が行う処理 |
| --- | --- | --- |
| [luxon](https://moment.github.io/luxon/) 3.7.2 | [`datetime`](datetime/datetime.go)、`time` を使う | オフセット付きの RFC 3339 の日時を解析し、暦日で加算し、2 つの日時の間の日数を数え、表示用に整形し、カレンダー 1 か月分の 42 日を並べる |
| [neverthrow](https://github.com/supermacro/neverthrow) 8.2.0 | [`result`](result/result.go)、`errors`・`fmt`・`strconv` を使う | 注文 500 行を検証する。各段階は文脈を付けてラップしたエラーを返す。有効な行の合計を出す |
| [connect-es](https://connectrpc.com/docs/web/) 2.2.0 | [`rpc`](rpc/rpc.go)、connect-go と protobuf-go を使う | Connect サービスの unary 呼び出しを、protobuf のバイナリ形式で `fetch` 経由で 50 回行う |
| [React](https://react.dev/) 19.3.0 | [`render`](render/render.go)、`html/template` を使う | 商品 100 件のページを HTML にレンダリングする。React は `renderToString`、Go はテンプレートを使う |
| [VitePress](https://vitepress.dev/) | [`markdown`](markdown/markdown.go)、[goldmark](https://github.com/yuin/goldmark) を使う | CommonMark の文書を HTML にする。JS 側は VitePress が使う markdown-it 15.0.2 |
| [Astro](https://astro.build/) | 同じ `markdown` | 同じ文書を、Astro が使う remark と rehype（unified 11）で HTML にする。Astro は Markdown を Node.js 上で処理するため、Node.js 向けにビルドする |
| [Vue](https://vuejs.org/) | [`reactive`](reactive/reactive.go)、ref・computed・effect を Go で実装したもの | 20 段の computed の連鎖 50 本、ダイヤモンド型の依存、それらを読む effect を作り、ref を 2,000 回変更する。JS 側は @vue/reactivity 3.5.43 |

フレームワークについては、ページが最も依存する部分を比べる。React はレンダリング、VitePress と Astro は Markdown のレンダラー、Vue はリアクティビティを対象にする。Go 側は、ネイティブのプログラム向けに書くのと同じ普通の Go のコードである。`%w` 付きの `fmt.Errorf`、`time.Parse`、生成された protobuf の型、`html/template` を使う。Go には比べられるリアクティビティの仕組みがないため、`reactive` はこの比較のために Vue に倣って書いたものである。JavaScript 側はライブラリの通常の API を使う。Markdown のレンダラーはいくつかの文字のエスケープの仕方が異なるため、そのエスケープを戻してから HTML を比べる。入力と処理は [js/libs.mjs](js/libs.mjs) に、JavaScript 版は [js/impl/](js/impl) にある。

## 結果

<!-- compare:start -->
| 比較対象 | Go パッケージ | goesm gzip | JS gzip | 比 |
| --- | --- | --- | --- | --- |
| luxon | `datetime` | 34.7 KiB | 21.7 KiB | 1.60× |
| neverthrow | `result` | 133.4 KiB | 2.4 KiB | 55.86× |
| connect-es | `rpc` | 1403.0 KiB | 32.9 KiB | 42.70× |
| react | `render` | 354.8 KiB | 64.3 KiB | 5.51× |
| vitepress | `markdown` | 285.5 KiB | 40.4 KiB | 7.07× |
| astro | `markdown` | 285.5 KiB | 47.2 KiB | 6.04× |
| vue | `reactive` | 10.7 KiB | 5.3 KiB | 2.03× |

| 比較対象 | node 26.10.0 goesm | node 26.10.0 JS | 比 | bun 1.4.2 goesm | bun 1.4.2 JS | 比 |
| --- | --- | --- | --- | --- | --- | --- |
| luxon | 2.2 ms | 8.9 ms | 0.24× | 4.5 ms | 8.1 ms | 0.55× |
| neverthrow | 0.81 ms | 0.51 ms | 1.59× | 1.1 ms | 0.50 ms | 2.30× |
| connect-es | 32 ms | 2.7 ms | 11.65× | 27 ms | 1.9 ms | 14.36× |
| react | 2.9 ms | 1.8 ms | 1.59× | 3.3 ms | 1.9 ms | 1.72× |
| vitepress | 0.63 ms | 0.23 ms | 2.80× | 1.1 ms | 0.16 ms | 7.05× |
| astro | 0.60 ms | 2.0 ms | 0.31× | 1.00 ms | 2.7 ms | 0.37× |
| vue | 42 ms | 6.0 ms | 7.01× | 62 ms | 5.6 ms | 11.00× |
<!-- compare:end -->

サイズは minify した ES モジュールのバンドルを gzip のレベル 9 で圧縮したものである。時間は、ウォームアップ後に処理を 1 回実行した時間の中央値である。connect-es の処理では、エンコード済みの応答を返す `fetch` を同じプロセス内に置く。このため、計測するのはクライアントの処理だけ、すなわちリクエストのエンコード、プロトコルの処理、応答のデコードである。

## 実行方法

```sh
cd compare/js
npm ci
node build.mjs                      # このチェックアウトの goesm をビルドし、各比較の両者を ../out にビルドする
node run.mjs > ../results/node.jsonl
bun run.mjs > ../results/bun.jsonl
node report.mjs ../results/node.jsonl ../results/bun.jsonl   # 上の表を更新する
```

`GOESM=/path/to/goesm node build.mjs` とすると別の goesm のバイナリを使う。`node build.mjs luxon` や `node run.mjs -quick luxon` のように指定すると、一部の比較だけを実行する。計測は、マシンでほかに重い処理が動いていないときに行う。バンドルは esbuild で minify し、ブラウザ向けの ES モジュールとして作る。Go 側にあるわずかな `node:` の import は外部参照のまま残す。

[proto/](proto) の Go と TypeScript のコードは `buf generate` で生成する。使うプラグインは [buf.gen.yaml](buf.gen.yaml) にある。
