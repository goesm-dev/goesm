# ライブラリとの比較

[English](README.md)

Web アプリが普段は人気の JavaScript ライブラリに任せる処理を、Go で書いて goesm でコンパイルすると、サイズと速度はどうなるか。ここではそれを比べる。各比較は同じ処理を 2 回実装する。1 つは goesm でコンパイルする Go パッケージで、もう 1 つはそのライブラリを使う JavaScript である。両者は同じ方法でバンドルし、同じ処理を JavaScript から呼んで計測する。JavaScript 側は処理が呼ぶものだけを import する。これと同じく、Go 側のバンドルは処理が呼ぶ関数だけを export する。このため、アプリの場合と同じく、バンドラーは残りを削除できる。両者の結果が一致しない場合、計測は失敗する。

| ライブラリ | Go パッケージ | 両者が行う処理 |
| --- | --- | --- |
| [luxon](https://moment.github.io/luxon/) 3.7.2 | [`datetime`](datetime/datetime.go)、`time` を使う | オフセット付きの RFC 3339 の日時を解析し、暦日で加算し、2 つの日時の間の日数を数え、表示用に整形し、カレンダー 1 か月分の 42 日を並べる |
| [neverthrow](https://github.com/supermacro/neverthrow) 8.2.0 | [`result`](result/result.go)、`errors`・`fmt`・`strconv` を使う | 注文 500 行を検証する。各段階は文脈を付けてラップしたエラーを返す。有効な行の合計を出す |
| [connect-es](https://connectrpc.com/docs/web/) 2.2.0 | [`rpc`](rpc/rpc.go)、connect-go と protobuf-go を使う | Connect サービスの unary 呼び出しを、protobuf のバイナリ形式で `fetch` 経由で 50 回行う |
| [React](https://react.dev/) 19.3.0 | [`render`](render/render.go)、`html/template` を使う | 商品 100 件のページを HTML にレンダリングする。React は `renderToString`、Go はテンプレートを使う |
| [VitePress](https://vitepress.dev/) | [`markdown`](markdown/markdown.go)、[goldmark](https://github.com/yuin/goldmark) を使う | CommonMark の文書を HTML にする。JS 側は VitePress が使う markdown-it 15.0.2 |
| [Astro](https://astro.build/) | 同じ `markdown` | 同じ文書を、Astro が使う remark と rehype（unified 11）で HTML にする。Astro は Markdown を Node.js 上で処理するため、Node.js 向けにビルドする |
| [Vue](https://vuejs.org/) | [`reactive`](reactive/reactive.go)、ref・computed・effect を Go で実装したもの | 20 段の computed の連鎖 50 本、ダイヤモンド型の依存、それらを読む effect を作り、ref を 2,000 回変更する。JS 側は @vue/reactivity 3.5.43 |
| [Tailwind CSS](https://tailwindcss.com/) 4.3.3 | [`utility`](utility/utility.go)、ユーティリティファーストの CSS を生成する Go の実装 | ランディングページ [js/page.html](js/page.html) の 154 個のクラス名に対するスタイルシートを、Tailwind の既定のテーマから作る。Go 側の CSS は Tailwind の出力とバイト単位で一致しなければならない |
| [marked](https://marked.js.org/) 18.1.0 | [`gfm`](gfm/gfm.go)、goldmark の GFM 拡張を使う | GitHub Flavored Markdown の文書（表、タスクリスト、打ち消し線、自動リンク）を HTML にする |
| [smol-toml](https://github.com/squirrelchat/smol-toml) 1.9.1 | [`invoice`](invoice/invoice.go)、[go-toml](https://github.com/pelletier/go-toml) v2 を使う | 明細 40 行と領収書を持つ請求書を TOML から型付きの値に読み、合計を出し、TOML に書き戻す |
| [@noble/post-quantum](https://github.com/paulmillr/noble-post-quantum) 0.7.1 | [`pqjwt`](pqjwt/pqjwt.go)、`crypto/mldsa` を使う | シードから ML-DSA-65 の鍵を作り、その鍵で署名した JWT を発行し、10 個のトークンを検証する |
| [@bufbuild/protobuf](https://github.com/bufbuild/protobuf-es) 2.16.0 | [`protomsg`](protomsg/protomsg.go)、protobuf-go を使う | protobuf のバイナリ形式の応答 20 件を生成された型に読み、要約し、タスクを完了にして再びエンコードする |

フレームワークについては、ページが最も依存する部分を比べる。React はレンダリング、VitePress と Astro は Markdown のレンダラー、Vue はリアクティビティ、Tailwind はページのクラス名を CSS にするコンパイラを対象にする。Go 側は、ネイティブのプログラム向けに書くのと同じ普通の Go のコードである。`%w` 付きの `fmt.Errorf`、`time.Parse`、生成された protobuf の型、`html/template` を使う。Go には比べられるリアクティビティの仕組みがないため、`reactive` はこの比較のために Vue 3.5 と同じアルゴリズムで書いたものである。計算が読む ref や computed の一覧と、ref や computed を購読する計算の一覧は、同じリンクをつないだ双方向リストであり、リンクは版数を持つ。このため、前回と同じものを読んだ計算はリンクを再利用し、読んだものが変わっていない computed は再計算しない。`utility` も同様にこの比較のために書いたものである。テーマの CSS を読み、Tailwind と同じ方法でユーティリティを組み立てる。Tailwind の静的なユーティリティ、静的なバリアント、プロパティの並び順は、[js/gen-utility.mjs](js/gen-utility.mjs) が tailwindcss のパッケージから生成する表から読む。対象は一般的なページが使うユーティリティと、値を取らないバリアントである。`group-*`、`peer-*`、`not-*`、`max-*` などの複合的なバリアントや値を取るバリアントは扱わない。JavaScript 側はライブラリの通常の API を使う。Markdown のレンダラーはいくつかの文字のエスケープの仕方が異なるため、そのエスケープを戻してから HTML を比べる。入力と処理は [js/libs.mjs](js/libs.mjs) に、JavaScript 版は [js/impl/](js/impl) にある。

3 つの比較には、[light/](light) に 2 つ目の Go パッケージがあり、表では「(light)」と示す。同じ処理を、バンドルを意識して書いたものであり、1 つ目のパッケージを大きくしている標準ライブラリのパッケージを使わない。[`light/result`](light/result/result.go) は、任意の値を `reflect` で整形する `fmt.Errorf` を使わず、独自のエラー型でメッセージを組み立てる。[`light/rpc`](light/rpc/rpc.go) は、connect-go、protobuf のランタイム、`net/http` を使わず、2 つのメッセージを手書きでエンコードとデコードし、`syscall/js` 経由で `fetch` を呼ぶ。[`light/render`](light/render/render.go) は、`html/template` でテンプレートを解釈せず、templ が生成するコードと同じようにコンポーネントごとの関数で `strings.Builder` にページを書き、`html.EscapeString` でエスケープする。これらの結果も、ライブラリの結果と一致しなければならない。

## 結果

<!-- compare:start -->
| 比較対象 | Go パッケージ | goesm gzip | JS gzip | 比 |
| --- | --- | --- | --- | --- |
| luxon | `datetime` | 19.8 KiB | 21.7 KiB | 0.91× |
| neverthrow | `result` | 41.0 KiB | 2.4 KiB | 17.18× |
| neverthrow (light) | `light/result` | 15.5 KiB | 2.4 KiB | 6.49× |
| connect-es | `rpc` | 949.6 KiB | 32.9 KiB | 28.90× |
| connect-es (light) | `light/rpc` | 14.1 KiB | 32.9 KiB | 0.43× |
| react | `render` | 215.1 KiB | 64.3 KiB | 3.34× |
| react (light) | `light/render` | 11.5 KiB | 64.3 KiB | 0.18× |
| vitepress | `markdown` | 150.8 KiB | 40.4 KiB | 3.73× |
| astro | `markdown` | 150.8 KiB | 47.2 KiB | 3.19× |
| vue | `reactive` | 7.0 KiB | 5.3 KiB | 1.32× |
| tailwind | `utility` | 54.0 KiB | 72.2 KiB | 0.75× |
| marked | `gfm` | 158.9 KiB | 13.4 KiB | 11.85× |
| smol-toml | `invoice` | 100.7 KiB | 6.6 KiB | 15.23× |
| @noble/post-quantum | `pqjwt` | 101.7 KiB | 9.5 KiB | 10.66× |
| @bufbuild/protobuf | `protomsg` | 197.6 KiB | 18.1 KiB | 10.92× |

| 比較対象 | node 26.10.0 goesm | node 26.10.0 JS | 比 | bun 1.4.2 goesm | bun 1.4.2 JS | 比 |
| --- | --- | --- | --- | --- | --- | --- |
| luxon | 3.3 ms | 7.4 ms | 0.44× | 5.7 ms | 7.5 ms | 0.76× |
| neverthrow | 0.39 ms | 0.37 ms | 1.05× | 0.50 ms | 0.47 ms | 1.07× |
| neverthrow (light) | 0.35 ms | 0.36 ms | 0.96× | 0.54 ms | 0.46 ms | 1.17× |
| connect-es | 17 ms | 2.2 ms | 7.93× | 20 ms | 1.8 ms | 11.14× |
| connect-es (light) | 1.5 ms | 2.2 ms | 0.71× | 1.1 ms | 2.0 ms | 0.54× |
| react | 2.4 ms | 1.1 ms | 2.07× | 2.7 ms | 1.7 ms | 1.57× |
| react (light) | 0.15 ms | 1.4 ms | 0.11× | 0.17 ms | 1.7 ms | 0.10× |
| vitepress | 0.30 ms | 0.19 ms | 1.56× | 0.52 ms | 0.14 ms | 3.78× |
| astro | 0.32 ms | 1.9 ms | 0.17× | 0.41 ms | 2.5 ms | 0.17× |
| vue | 4.8 ms | 5.4 ms | 0.89× | 5.2 ms | 4.6 ms | 1.13× |
| tailwind | 2.0 ms | 4.1 ms | 0.50× | 2.8 ms | 3.5 ms | 0.80× |
| marked | 2.1 ms | 0.80 ms | 2.59× | 3.7 ms | 1.3 ms | 2.75× |
| smol-toml | 8.2 ms | 0.96 ms | 8.55× | 16 ms | 0.69 ms | 23.51× |
| @noble/post-quantum | 57 ms | 28 ms | 2.04× | 46 ms | 47 ms | 0.99× |
| @bufbuild/protobuf | 13 ms | 1.9 ms | 6.72× | 9.9 ms | 1.9 ms | 5.28× |
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

`GOESM=/path/to/goesm node build.mjs` とすると別の goesm のバイナリを使う。`node build.mjs luxon` や `node run.mjs -quick luxon` のように指定すると、一部の比較だけを実行する。計測は、マシンでほかに重い処理が動いていないときに行う。Bun のガベージコレクションの動作が変わるため、`BUN_OPTIONS=--smol` は指定しない。バンドルは esbuild で minify し、ブラウザ向けの ES モジュールとして作る。Go 側にあるわずかな `node:` の import は外部参照のまま残す。

[proto/](proto) の Go と TypeScript のコードは `buf generate` で生成する。使うプラグインは [buf.gen.yaml](buf.gen.yaml) にある。
