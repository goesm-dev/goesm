# 変更履歴

[English](CHANGELOG.md)

goesm の各バージョンの変更点を、新しい順に記載します。タグを付けたバージョンはすべて semver の prerelease なので、`go install github.com/goesm-dev/goesm/cmd/goesm@latest` は最新のものを選びます。GitHub の各リリースのノートは、このファイルと [CHANGELOG.md](CHANGELOG.md) の該当する節から作られます。手順は [docs/releasing.ja.md](docs/releasing.ja.md) にあります。

## Unreleased

v0.0.1-beta.3 以降に `main` に入った変更です。差分は [v0.0.1-beta.3...main](https://github.com/goesm-dev/goesm/compare/v0.0.1-beta.3...main) で確認できます。

生成コードの速度とバンドルサイズを、手書きの JavaScript にさらに近づける変更です。Go のコードの書き方も、それを呼ぶ JavaScript の書き方も変わりません。

### 性能

- インターフェース経由でメソッドが呼ばれる型のインターフェース値は、その型のメソッドを持つ box クラスのインスタンスになりました。インターフェース呼び出しはその値に対するメソッド呼び出しになります。構造体へのポインタはそれ自身がインターフェース値になるので、変換してもアロケーションは発生しません。[#86](https://github.com/goesm-dev/goesm/pull/86) [#88](https://github.com/goesm-dev/goesm/pull/88)
- int64 や uint64 の 1 つのフィールドへの代入が続く箇所では、ローカル変数にコピーして計算し、最後に 1 回だけフィールドに書き戻します。これにより、V8 は値をマシンワードのまま扱います。Node.js での Rand64 は 33 ms から 14 ms になり、BigInt を使う手書きの JavaScript と同じ速さになりました。[#78](https://github.com/goesm-dev/goesm/pull/78)
- `iter.Pull` と `Pull2` は、単純なシーケンスのリテラルを goroutine の切り替えではなく JavaScript のジェネレーターで進めます。`iter.Pull` を使うプログラムでも、fmt、encoding/json、sort は同期のまま動きます。[#80](https://github.com/goesm-dev/goesm/pull/80) [#83](https://github.com/goesm-dev/goesm/pull/83)
- ループが添字でアクセスするスライスは、配列、オフセット、長さをループの前に 1 回だけ読み込みます。Node.js での NBody は、手書きの JavaScript の 1.20 倍から 1.03 倍になりました。[#87](https://github.com/goesm-dev/goesm/pull/87)
- 型が分かっているポインタへの `json.Unmarshal` は、box を作らずにデコードします。JSON の配列は最終的な長さで作ります。1 回の呼び出しあたりのアロケーション量は、手書きの JavaScript と比べて JSON カーネルで 2.33 倍から 1.84 倍に、Handle で 2.16 倍から 1.66 倍になりました。[#89](https://github.com/goesm-dev/goesm/pull/89)
- `slices.Sort` は、32 ビットに収まる整数を Int32Array で、文字列のスライスをその場でソートします。Sort カーネルは手書きの JavaScript と比べて、Node.js で 1.12 倍から 1.00 倍に、Bun で 1.23 倍から 1.07 倍になりました。[#90](https://github.com/goesm-dev/goesm/pull/90)
- [#84](https://github.com/goesm-dev/goesm/pull/84) 時点のベンチマークでは、goesm の幾何平均はネイティブ Go の 1.03〜1.10 倍です。合計時間は、すべてのランタイムで 4 つのコンパイラーのうち最短です。[#84](https://github.com/goesm-dev/goesm/pull/84)

### バンドルサイズ

- regexp のパターンがすべてコンパイル時に分かるプログラムでは、変換後のパターンが同じマッチを線形時間で見つけられる場合に、エンジンの RegExp でマッチします。このとき regexp のパーサーとエンジンはバンドルに入りません。`strconv.Atoi`、`strings.TrimSpace`、`strings.Split` は、unicode のテーブルや NumError のメソッドを引き込まなくなりました。日付を解析するカレンダーのパッケージでは、strings と strconv による増分が gzip で 10.2 KB から 1.9 KB に、regexp による増分が 55.6 KB から 4.5 KB に減りました。[#85](https://github.com/goesm-dev/goesm/pull/85)
- メソッドテーブルは、Go のリンカーと同じく、到達可能なコードがインターフェース経由で呼ぶメソッドだけを残します。unicode のカテゴリーとスクリプトのテーブルは、それを使いうる regexp パターンがなければ取り除かれます。Markdown のバンドルは gzip で 215 KiB から 163 KiB になりました。[#82](https://github.com/goesm-dev/goesm/pull/82)
- 副作用がないとみなすパッケージ変数の初期化式が増えました。空の init 関数は出力せず、strconv の 128 ビットの 10 のべき乗は必要なときに計算します。neverthrow との比較で使う標準ライブラリ版のバンドルは、gzip で 101 KiB から 52 KiB になりました。[#81](https://github.com/goesm-dev/goesm/pull/81)
- 定数の書式を持つ `fmt.Errorf` と `fmt.Sprintf` は、`%w`、`%q`、エラーの引数も文字列連結に変換します。構造体の記述子も小さくなりました。Vue との比較でのサイズは、Vue の 2.16 倍から 1.22 倍になりました。[#79](https://github.com/goesm-dev/goesm/pull/79)

### 修正

- 名前が ASCII 以外の大文字で始まる構造体フィールドは、export されるようになりました。ASCII 以外の文字で始まる非公開のフィールドは、export されなくなりました。[#81](https://github.com/goesm-dev/goesm/pull/81)

### 比較スイートと CI

- compare/ のリアクティビティは Vue 3.5 のアルゴリズムに合わせました。標準ライブラリを使う版に加えて、バンドルサイズを意識して書いた Go パッケージの版も比較します。[#79](https://github.com/goesm-dev/goesm/pull/79) [#81](https://github.com/goesm-dev/goesm/pull/81)
- マージ後に走る pull request の CI が、`main` への push の CI を取り消さなくなりました。[#77](https://github.com/goesm-dev/goesm/pull/77)

## v0.0.1-beta.3

2026-10-06 にタグを付けました。v0.0.1-beta.2 からの差分は [v0.0.1-beta.2...v0.0.1-beta.3](https://github.com/goesm-dev/goesm/compare/v0.0.1-beta.2...v0.0.1-beta.3) で確認できます。

JavaScript から Go の export を普通の JavaScript の値で呼べるようになりました。バンドルは大きく縮み、syscall/js の呼び出しは DOM を扱えるほど速くなりました。新しい比較スイートで goesm と人気の JavaScript ライブラリを比べ、そこで見つかった高速化もこのバージョンに含めています。

### 互換性のない変更

- **export された関数とメソッドは、普通の JavaScript の値を受け取り、返します。** [#68](https://github.com/goesm-dev/goesm/pull/68)
  - 文字列は JS の文字列に、スライスは配列になります。構造体は、encoding/json と同じ規則でフィールド名をプロパティ名にした普通のオブジェクトになります。キーが文字列のマップは普通のオブジェクトに、それ以外のマップは Map になります。
  - 最後の結果が `error` の場合、エラーは `GoError` として throw されます。その `GoError` を Go に渡すと、元の Go のエラーに戻ります。
  - `http.Handler` を返す関数からは fetch ハンドラーが得られます。書き方は `export default { fetch: Handler() }` です。
  - メソッドを持つ構造体型へのポインタ、空でないインターフェース、チャネルはハンドルとして渡ります。ハンドルのメソッドも同じ規則で値を変換します。
  - TypeScript からは JavaScript の型が見えます。
  - **移行方法:** 手書きの変換を削除してください。`rt.fromJSString`、`rt.toJSString`、`rt.sliceLit`、`rt.toArray`、`rt.icall(err, "Error")` の呼び出しは不要になり、エントリーモジュールはランタイムを `$runtime` として再 export しなくなりました。詳しくは [docs/js-exports.ja.md](docs/js-exports.ja.md) を参照してください。

### 新機能

- 引数が真偽値、数値、文字列、`js.Value`、`js.Func`、`nil` だけの `syscall/js` の呼び出しは、JavaScript のプロパティアクセスと関数呼び出しに直接コンパイルされます。Node.js で syscall/js 経由の `add(1, 2)` は 173 ns から 23 ns になりました。`honnef.co/go/js/dom/v2` のような `GOOS=js` 向けのライブラリは、そのまま動きます。Go から DOM を使う方法は [docs/dom.ja.md](docs/dom.ja.md) にあります。[#72](https://github.com/goesm-dev/goesm/pull/72)
- compare/ は、同じ処理を goesm でビルドした Go のパッケージと、人気のライブラリを使う JavaScript の両方で実行し、gzip サイズと Node.js と Bun での時間を比べます。対象は luxon、neverthrow、connect-es、React、VitePress の markdown-it、Astro の remark、Vue のリアクティビティ、Tailwind CSS です。[#71](https://github.com/goesm-dev/goesm/pull/71) [#76](https://github.com/goesm-dev/goesm/pull/76)

### バンドルサイズ

- メソッドテーブルは、Go のリンカーと同じく、動的に呼ばれうるメソッドだけを載せます。副作用のない初期化式を持つパッケージ変数は、使われなければ取り除かれます。time は syscall/js を使わずにホストの UTC オフセットを読みます。[#70](https://github.com/goesm-dev/goesm/pull/70)
- エントリーモジュールはランタイムを再 export しなくなりました。[#68](https://github.com/goesm-dev/goesm/pull/68)

| `goesm build -minify`、gzip -9 | v0.0.1-beta.2 | v0.0.1-beta.3 |
| --- | ---: | ---: |
| 標準ライブラリなし | 23,619 B | 3,561 B |
| `time` の関数 6 つ | 47,774 B | 13,510 B |
| `fmt.Sprintf` | 140,505 B | 94,778 B |
| `regexp`、`strconv`、`time` | 131,053 B | 93,887 B |

### 性能

- fmt の `Sprintf` と `Errorf` は、エラーと Stringer の `%v` と `%s`、`Errorf` の 1 つの `%w` を文字列連結で整形します。`range []byte(s)` は文字列をコピーせずに走査し、`utf8.Valid` は 64 ビットのワードを読まなくなりました。[#71](https://github.com/goesm-dev/goesm/pull/71)
- 64 バイト以上の ASCII 以外を含む文字列は、エンジンの UTF-8 エンコーダーとデコーダーで JavaScript との境界を越えます。省略できる境界チェックが増え、構造体のスライスを走査する range ループは、要素をコピーせず、使うフィールドだけを読みます。[#73](https://github.com/goesm-dev/goesm/pull/73)
- ゼロ値はモジュールの定数になりました。インターフェースに変換した構造体のポインタはそのインターフェース値を再利用し、フィールドと要素へのポインタは WeakMap ではなくオブジェクト自身に保持します。[#74](https://github.com/goesm-dev/goesm/pull/74)
- 一部の引数を通してだけブロックする関数には同期版が作られ、ブロックしない引数での呼び出しはそちらを使います。net/http を import するプログラムでも、`fmt.Sprintf` が非同期にならなくなりました。[#75](https://github.com/goesm-dev/goesm/pull/75)
- 比較スイートでは、[#71](https://github.com/goesm-dev/goesm/pull/71) で Vue のワークロードが 102 ms から 42 ms になりました。[#73](https://github.com/goesm-dev/goesm/pull/73) で VitePress の goldmark 版の時間は Node.js で 0.62 倍、Bun で 0.47 倍になり、[#74](https://github.com/goesm-dev/goesm/pull/74) で Vue のワークロードはさらに 2.2〜3.2 倍速くなりました。connect-es は [#74](https://github.com/goesm-dev/goesm/pull/74) と [#75](https://github.com/goesm-dev/goesm/pull/75) でそれぞれ 10〜20% 速くなりました。[#71](https://github.com/goesm-dev/goesm/pull/71) [#73](https://github.com/goesm-dev/goesm/pull/73) [#74](https://github.com/goesm-dev/goesm/pull/74) [#75](https://github.com/goesm-dev/goesm/pull/75)

### 修正

- protobuf の repeated のメッセージフィールドで要素が失われていました。10 件のメッセージを含むレスポンスで、その一部が nil になっていました。[#69](https://github.com/goesm-dev/goesm/pull/69)
- `fmt.Sprintf` の高速パスが fmt 本来の処理に切り替わるとき、引数の `Error` と `String` のメソッドを 2 回呼んでいました。[#73](https://github.com/goesm-dev/goesm/pull/73)

## v0.0.1-beta.2

2026-10-05 にタグを付けました。v0.0.1-beta.1 からの差分は [v0.0.1-beta.1...v0.0.1-beta.2](https://github.com/goesm-dev/goesm/compare/v0.0.1-beta.1...v0.0.1-beta.2) で確認できます。

新しい `//goesm:import` ディレクティブで、Go のコードから JavaScript と TypeScript を呼べるようになりました。

### 新機能

- 本体のない関数宣言と、値のないパッケージ変数で、ES モジュールの export を import できます。[#58](https://github.com/goesm-dev/goesm/pull/58)

  ```go
  //goesm:import "./format.ts" formatPrice
  func formatPrice(yen int, currency string) string
  ```

  - 引数と結果は Go の型に従って変換されます。対象は文字列、数値、スライス、マップ、encoding/json と同じ規則でフィールド名を付けた構造体、関数、`js.Value`、`any` です。
  - 最後の結果が `error` の場合、throw された例外や reject を受け取ります。`await` を付けると、Promise を返す関数がブロックする Go の関数になります。
  - 呼び出しのコストは、JavaScript から同じ関数を呼ぶ場合より数ナノ秒多い程度で、syscall/js 経由の 3〜160 分の 1 です。
  - 詳しくは [docs/js-imports.ja.md](docs/js-imports.ja.md) を参照してください。
- `await` を付けずに import した関数が Promise を返すと、ディレクティブを示すメッセージで panic します。以前は Promise が誤った値に変換されていました。throw された文字列、null、数値は `Error` になります。[#67](https://github.com/goesm-dev/goesm/pull/67)
- `goesm build` は `node:`、`bun:`、`cloudflare:` の import をバンドルせずに残します。`//goesm:import` で指定したモジュールが見つからない場合は、goesm の内部エラーではなく利用者のエラーとして報告します。[#67](https://github.com/goesm-dev/goesm/pull/67)

### 性能

- 構造体のフィールドなど BigInt で表される int64 と uint64 のビット演算は、V8 でマシンワードのまま計算されるようになりました。Node.js での Rand64 は 137 ms から 51 ms になりました。[#64](https://github.com/goesm-dev/goesm/pull/64)
- `iter.Pull` のコルーチンは、チャネル操作ではなく Promise の resolve で切り替わります。Node.js で 1 つの値あたり約 1.7 µs から 0.75 µs になりました。[#65](https://github.com/goesm-dev/goesm/pull/65)
- ASCII 以外を含む文字列の UTF-8 と UTF-16 の変換は、Node.js で速くなり、Bun では数倍速くなりました。[#58](https://github.com/goesm-dev/goesm/pull/58)
- 生成されたエンコーダーによる `json.Marshal` は、テキストの確認に正規表現の走査ではなく、エンコードした UTF-8 を使います。[#66](https://github.com/goesm-dev/goesm/pull/66)

## v0.0.1-beta.1

2026-10-05 にタグを付けました。v0.0.1-beta.0 からの差分は [v0.0.1-beta.0...v0.0.1-beta.1](https://github.com/goesm-dev/goesm/compare/v0.0.1-beta.0...v0.0.1-beta.1) で確認できます。

このバージョンでは、Go の値の表現とコードの変換方法を見直し、出力が手書きの JavaScript と同じ速さで動くようにしました。[#53](https://github.com/goesm-dev/goesm/pull/53) 時点のベンチマークでは、goesm の幾何平均はネイティブ Go の 1.00〜1.07 倍で、手書きの JavaScript は 0.91〜1.08 倍です。サポート範囲をユースケースごとに定め、HTTP サーバーと差分ビルドにも対応しました。

### 新機能

- [docs/use-cases.ja.md](docs/use-cases.ja.md) に、Node.js、エッジ、ブラウザ、JavaScript や TypeScript から import するライブラリのそれぞれで動く典型的なコードをまとめました。各項目はテストか手作業で確認しています。[#49](https://github.com/goesm-dev/goesm/pull/49)
  - `http.ListenAndServe` は `node:http`、`Bun.serve`、`Deno.serve` で動きます。`rt.fetchHandler` は `http.Handler` を Cloudflare Workers 向けの fetch ハンドラーに変換します。
  - net/http のクライアントは、ブラウザ以外でリダイレクトをたどります。
  - 生成ファイルの先頭に `// @ts-nocheck` を付けるので、Next.js や Vite のような strict な設定のプロジェクトでもそのまま使えます。呼び出し側からは、引き続き Go の API の型が見えます。
- 差分ビルドに対応しました。goesm は変換したパッケージごとの TypeScript モジュールを、ユーザーキャッシュディレクトリの `goesm/modules` か `$GOESMCACHE` に保存し、変更のあったパッケージだけを変換し直します。`GOESMCACHE=off` でキャッシュを無効にできます。[#54](https://github.com/goesm-dev/goesm/pull/54)
- [docs/concurrency.ja.md](docs/concurrency.ja.md) で並行処理のモデルを説明しました。goroutine はプリエンプションなしで 1 つのスレッド上で動きます。`runtime.Gosched` はホストのタイマーと I/O にも処理を譲るようになり、これを使ってポーリングするループが動きます。[#57](https://github.com/goesm-dev/goesm/pull/57) [#59](https://github.com/goesm-dev/goesm/pull/59)
- `js.FuncOf` のコールバックが返した Promise は、そのまま JavaScript に渡ります。[#51](https://github.com/goesm-dev/goesm/pull/51)

### 表現の見直しと性能

- インターフェースのメソッドを型ごとのプロトタイプに置き、V8 がインターフェース呼び出しをインライン化できるようにしました。[#34](https://github.com/goesm-dev/goesm/pull/34)
- ローカル変数の `[]bool` はバイト列で表します。[#35](https://github.com/goesm-dev/goesm/pull/35) [#44](https://github.com/goesm-dev/goesm/pull/44)
- `slices.Sort` と、それを使う `sort.Ints` と `sort.Strings` は、整数と文字列をエンジンのソートで並べます。[#36](https://github.com/goesm-dev/goesm/pull/36)
- ループの添字が範囲内だと分かる箇所では、境界チェックを省きます。[#37](https://github.com/goesm-dev/goesm/pull/37) [#44](https://github.com/goesm-dev/goesm/pull/44)
- `strings.Builder` は文字列の連結で文字列を組み立て、`strings.Split` と `Join` はエンジンの実装を使います。[#38](https://github.com/goesm-dev/goesm/pull/38)
- キーが真偽値、整数、文字列、チャネルのマップは、キーをそのまま JavaScript の Map に使います。[#39](https://github.com/goesm-dev/goesm/pull/39)
- よく使う書式指定子の `fmt.Sprintf` は文字列連結で整形し、書式が定数の呼び出しはコンパイル時に文字列連結に変換します。[#40](https://github.com/goesm-dev/goesm/pull/40) [#42](https://github.com/goesm-dev/goesm/pull/42) [#50](https://github.com/goesm-dev/goesm/pull/50)
- encoding/json は単純な値を 1 回の走査でエンコードとデコードします。静的に型が分かる `json.Marshal` の呼び出しには、エンコーダーを生成します。[#41](https://github.com/goesm-dev/goesm/pull/41) [#42](https://github.com/goesm-dev/goesm/pull/42) [#52](https://github.com/goesm-dev/goesm/pull/52) [#60](https://github.com/goesm-dev/goesm/pull/60)
- 頻繁に使う int64 と uint64 のローカル変数は、BigInt ではなく 2 つの int32 で保持します。`GOESM_SPLIT64=off` でこの変換を無効にできます。[#50](https://github.com/goesm-dev/goesm/pull/50)
- JavaScript との境界を越える ASCII の文字列は、再度の走査を省きます。ASCII 以外を含む文字列の変換も速くなりました。[#45](https://github.com/goesm-dev/goesm/pull/45) [#56](https://github.com/goesm-dev/goesm/pull/56)
- すぐに完了するチャネル操作は await しなくなり、math/big のワード演算も速くなりました。[#62](https://github.com/goesm-dev/goesm/pull/62)
- ブロックしないコードは、`iter.Pull` やブロックしうる呼び出しの近くにあっても同期のまま残ります。[#61](https://github.com/goesm-dev/goesm/pull/61)
- bench/ に、Go では速く JavaScript への変換では遅くなりうる処理を測るカーネルと、各実装が最も遅くなるカーネルの表を追加しました。[#63](https://github.com/goesm-dev/goesm/pull/63)

### バンドルサイズ

- 副作用のない初期化式を持つパッケージ変数と、どこからも使われない型は、バンドラーが取り除けるようになりました。`strings.ToUpper` だけを使うライブラリは gzip で 93 KB から 27 KB に、fmt の hello world は 222 KB から 132 KB になりました。[#43](https://github.com/goesm-dev/goesm/pull/43)

### 修正

- `^ab*` のように `^` で固定したパターンに量指定子が続く regexp が、何にもマッチしませんでした。goldmark の GFM タスクリストも動くようになりました。[#33](https://github.com/goesm-dev/goesm/pull/33)
- キーをハッシュするマップのエントリーを上書きすると、gc と同じく新しいキーが残ります。[#39](https://github.com/goesm-dev/goesm/pull/39)
- TypeScript のプリミティブ型や JavaScript のグローバルと同じ名前の識別子と、添字で取り出した関数値の呼び出しを、正しくコンパイルします。[#49](https://github.com/goesm-dev/goesm/pull/49)
- ASCII 以外の長いテキストの変換で RangeError が起きることがありました。[#56](https://github.com/goesm-dev/goesm/pull/56)

## v0.0.1-beta.0

2026-10-05 にタグを付けました。最初にタグを付けたバージョンで、[#32](https://github.com/goesm-dev/goesm/pull/32) までのすべての変更を含みます。コミットの一覧は [v0.0.1-beta.0 までのコミット](https://github.com/goesm-dev/goesm/commits/v0.0.1-beta.0) で確認できます。

goesm は Go のパッケージをネイティブな ES モジュールにコンパイルします。出力は Go のパッケージごとに 1 つの TypeScript モジュールで、バンドラー、Node.js、Bun、ブラウザからそのまま import できます。WebAssembly は使いません。この最初のベータ版は、Go 言語の大部分と、標準ライブラリのかなりの部分を Go 自身のソースからコンパイルします。結果はネイティブ Go と突き合わせて検証しています。

### コマンド

- `goesm emit-ts` はホストのバンドラー向けに TypeScript のツリーを書き出し、`goesm build` は埋め込みの esbuild でそれを 1 つの ES モジュールにバンドルします。`-split` を付けると Go のパッケージごとにモジュールを書き出します。ソースマップは `.go` ファイルを指します。[#1](https://github.com/goesm-dev/goesm/pull/1)
- `-overlay` は go コマンドの overlay 形式を受け付けます。`//line` ディレクティブがあれば、型エラーとソースマップは元のファイルを指します。gosfc は Vue コンポーネントの Go ブロックにこの仕組みを使います。[#3](https://github.com/goesm-dev/goesm/pull/3)
- `-toolexec` で otelc などのツールを実行できます。この指定は `GOFLAGS` からも読み取ります。[#12](https://github.com/goesm-dev/goesm/pull/12)
- `goesm version` は、モジュールのバージョンと、goesm のビルドに使った Go のバージョンを表示します。goesm は `go get -tool` か `go install` でインストールします。バイナリと npm パッケージは配布しません。[#4](https://github.com/goesm-dev/goesm/pull/4)

### 言語とランタイム

- `main` パッケージは、Node.js と Bun の上でネイティブ Go と同じように動きます。標準出力、標準エラー出力、終了ステータス、終了ステータス 2 を伴う Go の `panic:` の報告、デッドロックのエラーが Go と一致します。`print` と `println` は、Go のランタイムと同じ形式で標準エラー出力に書きます。[#2](https://github.com/goesm-dev/goesm/pull/2) [#6](https://github.com/goesm-dev/goesm/pull/6)
- int64 と uint64 は BigInt で表し、Go と同じく桁あふれで折り返します。int、uint、uintptr は数値のままです。export された関数は、int64 と uint64 を `bigint` として受け取り、返します。[#7](https://github.com/goesm-dev/goesm/pull/7)
- complex64 と complex128、`new(expr)`、後方への `goto`、range-over-func の本体でのブロックする操作と `defer` と `goto`、ジェネリック関数内のローカル型、`iter.Pull` と `Pull2` が動きます。[#5](https://github.com/goesm-dev/goesm/pull/5) [#10](https://github.com/goesm-dev/goesm/pull/10) [#16](https://github.com/goesm-dev/goesm/pull/16)
- `recover` は、Go と同じく、defer された関数が直接呼んだときだけ panic を回復します。[#10](https://github.com/goesm-dev/goesm/pull/10)
- goroutine は 1 つのスレッド上の async 関数として動きます。`sync.Mutex.Lock` は同期的に動くので、ロックするだけのコードは非同期になりません。[#6](https://github.com/goesm-dev/goesm/pull/6)
- `//go:embed`、標準ライブラリ以外での `//go:linkname`、goroutine ローカルストレージ、フィールドのオフセットに対する `unsafe` のポインタ演算が動きます。[#12](https://github.com/goesm-dev/goesm/pull/12) [#13](https://github.com/goesm-dev/goesm/pull/13)
- goesm がまだ変換できない関数は、ビルドを失敗させずに、呼ぶと panic するスタブになります。スタブの一覧は `goesm build -v` で表示できます。[#12](https://github.com/goesm-dev/goesm/pull/12)

### 標準ライブラリ

- reflect を置き換えたので、fmt、encoding/json、reflect が Go のソースからコンパイルでき、ネイティブ Go と同じ結果を返します。[#8](https://github.com/goesm-dev/goesm/pull/8)
- time のタイマーはホストのタイマーの上で動きます。[#9](https://github.com/goesm-dev/goesm/pull/9)
- math/big が正確に動くので、RSA、ECDSA、ECDH、Ed25519、x509 はネイティブ Go と同じ結果を返します。[#16](https://github.com/goesm-dev/goesm/pull/16)
- Pure Go の crypto、crypto/rand、hash/maphash、unique、log/slog が動きます。os は Node.js と Bun の上で `node:fs` を使い、os/signal は `process.on` を使い、net/http のクライアントは fetch を使います。[#6](https://github.com/goesm-dev/goesm/pull/6) [#10](https://github.com/goesm-dev/goesm/pull/10) [#12](https://github.com/goesm-dev/goesm/pull/12)
- otelc で計装したプログラムが動き、Node.js、Bun、ブラウザから OTLP/HTTP でエクスポートできます。詳しくは [docs/otelc.ja.md](docs/otelc.ja.md) を参照してください。[#12](https://github.com/goesm-dev/goesm/pull/12) [#13](https://github.com/goesm-dev/goesm/pull/13)

### 性能

- 構造体のフィールドをクラスに宣言し、スライスの添字アクセスをインライン化し、`[]byte` を Uint8Array で表すようにしました。インターフェース呼び出しは呼び出し箇所ごとに動的な型のメソッドテーブルを引き、数値の整形はエンジンが行います。[#14](https://github.com/goesm-dev/goesm/pull/14) [#15](https://github.com/goesm-dev/goesm/pull/15) [#17](https://github.com/goesm-dev/goesm/pull/17) [#20](https://github.com/goesm-dev/goesm/pull/20) [#21](https://github.com/goesm-dev/goesm/pull/21) [#23](https://github.com/goesm-dev/goesm/pull/23) [#25](https://github.com/goesm-dev/goesm/pull/25) [#26](https://github.com/goesm-dev/goesm/pull/26) [#32](https://github.com/goesm-dev/goesm/pull/32)
- [#31](https://github.com/goesm-dev/goesm/pull/31) 時点のベンチマークでは、goesm の幾何平均はネイティブ Go の 2.6〜3.8 倍で、Go の WebAssembly 版は 2.7〜3.4 倍です。[#31](https://github.com/goesm-dev/goesm/pull/31)

### 適合性

- Go 自身の `$GOROOT/test` のテストスイートを goesm で実行し、961 件中 898 件が通ります。詳しくは [docs/conformance.ja.md](docs/conformance.ja.md) を参照してください。[#2](https://github.com/goesm-dev/goesm/pull/2) [#5](https://github.com/goesm-dev/goesm/pull/5) [#10](https://github.com/goesm-dev/goesm/pull/10) [#16](https://github.com/goesm-dev/goesm/pull/16)

### ドキュメントと開発環境

- README で goesm を GopherJS、Go の WebAssembly 版、TinyGo と比較し、ARCHITECTURE.ja.md でコンパイラーの仕組みを説明しました。[#1](https://github.com/goesm-dev/goesm/pull/1) [#11](https://github.com/goesm-dev/goesm/pull/11) [#18](https://github.com/goesm-dev/goesm/pull/18) [#19](https://github.com/goesm-dev/goesm/pull/19)
- ライセンスは BSD-3-Clause です。`mise.toml` で、開発に使う Go、Node.js、Bun のバージョンを固定しています。[#4](https://github.com/goesm-dev/goesm/pull/4)
