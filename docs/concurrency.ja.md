# 並行実行とメモリ安全性

[English](concurrency.md)

goesm は、goroutine を `GOMAXPROCS=1` の Go と同じように、プリエンプションなしで実行します。Go で正しいコードは、goesm でも正しく動きます。Go との違いは 3 つあります。goroutine が並列に動かないこと、待たない goroutine が他の goroutine の実行を妨げること、データ競合のあるコードが偶然動く場合があることです。メモリ安全性は Go と同等で、`unsafe` で扱える範囲はネイティブより狭くなります。

## goroutine の動き方

- goroutine は JavaScript の `async` 関数です。プログラムのすべての goroutine は、1 本の JavaScript スレッドを共有します。このスレッドは、ブラウザではページのメインスレッド、Node.js、Bun、Deno ではプロセスのスレッド、Worker ではプログラムを読み込んだ Worker のスレッドです。
- 待つ可能性のある関数は `async` 関数になり、待つ可能性のある箇所はそれぞれ `await` になります。待つ箇所にあたるのは、チャネル操作、`select`、`time.Sleep`、I/O、別の goroutine が待ちをまたいで保持しうる `sync.Mutex` のロック、およびそれらを呼ぶ関数の呼び出しです。それ以外の関数は同期関数のままです。
- goroutine が切り替わるのは、待つ箇所だけです。Go のコードが待たずに動いている間、そのスレッドでは他の処理が一切動きません。他の goroutine も、タイマーも、I/O のコールバックも、別の HTTP リクエストも待たされます。ブラウザでは、ページの描画と入力への応答も止まります。
- `runtime.GOMAXPROCS` は、引数に関係なく 1 を返し、何も変えません。`runtime.NumCPU` は 1 を返し、`runtime.LockOSThread` は何もしません。`runtime.Gosched` は、実行可能な他の goroutine には順番を譲りますが、ホストのタイマーと I/O には譲りません。
- 待つ可能性のある export された関数は、JavaScript に Promise を返します。その関数が待っている間は他の JavaScript のコードが動くので、同時に実行中の 2 つの呼び出しは、2 つの goroutine と同じように交互に進みます。
- すべての goroutine が待っていて、ホストにも残りの処理がない場合、Node.js と Bun では Go と同じ `fatal error: all goroutines are asleep - deadlock!` でプログラムが終了します。ブラウザには終了させるプロセスがないので、待っている goroutine はそのまま止まり続けます。

## データ競合

- **値が中途半端な状態で読まれることはありません。** Go では、interface、文字列、スライスに対するデータ競合で書き込み途中の値を読むことがあり、メモリが壊れる場合があります。また、マップに同時に書き込むと、`fatal error: concurrent map writes` でプログラムが終了します。goesm では、各 goroutine が待つ箇所と待つ箇所の間で読み書きし、その間は他の goroutine が動きません。そのため、競合する goroutine が書き込み途中の値を見ることはなく、複数の goroutine が書き込むマップも壊れません。
- **論理的な競合は残ります。** 1 つの goroutine の 2 つの待つ箇所の間では他の goroutine が動き、共有している状態を変えることがあります。残高を読み、スリープしてから残高を書き戻す goroutine は、Go と同じく goesm でも他の goroutine の更新を失います。共有する状態は、Go と同じく `sync.Mutex` やチャネルで守ってください。
- **競合のあるコードが偶然動くことがあります。** 100 個の goroutine がロックなしで共有のカウンタを 10,000 回ずつ増やすプログラムは、ネイティブでは 1 回の実行で 502731 を出力し、goesm では常に 1000000 を出力します。それでもこのコードは誤りです。ネイティブでビルドすると更新を失い、goesm でも読み込みと書き込みの間で goroutine が待つようになった時点で更新を失い始めます。
- **競合検出器はありません。** goesm には `-race` モードがありません。Go のコードは同じなので、テストはネイティブの `go test -race` で実行してください。
- **ポーリングで待つと止まったままになります。** `for !done.Load() {}` のようにフラグをポーリングするループは、そのフラグを別の goroutine、タイマー、I/O が立てる場合には終わりません。ループが回っている間は他の処理が動かないからです。フラグを立てるのがタイマーや I/O の場合は、ループに `runtime.Gosched()` を加えても終わりません。チャネル、`sync.WaitGroup`、`sync.Cond`、`context.Context` で待ってください。

`sync` と `sync/atomic` は Go と同じように動きます。アトミック操作は通常の読み込みと書き込みですが、スレッドが 1 本なのでそれで足ります。

## Worker による並列実行

複数の CPU コアを使う場合や、時間のかかる計算をページのメインスレッドから外す場合は、Go のコードを Worker で実行します。ブラウザでは Web Worker、Node.js と Bun では `worker_threads` を使います。各 Worker は goesm のモジュールを自分で読み込み、プログラムのコピーをそれぞれ持ちます。パッケージ変数、goroutine、メモリも Worker ごとに別々です。Go の値、チャネル、ミューテックスは Worker の間で共有できないので、Worker どうしは他の JavaScript のモジュールと同じく `postMessage` でデータをやり取りします。次の例は `lib/lib.js` を読み込みます。このファイルは、`SumSquares` を export するパッケージから `goesm build -o lib ./lib` でビルドしたものです。

```js
import { Worker, isMainThread, parentPort, workerData } from "node:worker_threads";

if (isMainThread) {
  const results = await Promise.all([1, 2, 3, 4].map((n) => new Promise((resolve, reject) => {
    const w = new Worker(new URL(import.meta.url), { workerData: n });
    w.on("message", resolve);
    w.on("error", reject);
  })));
  console.log(results);
} else {
  const { SumSquares } = await import("./lib/lib.js");
  parentPort.postMessage(await SumSquares(workerData * 1000, 4));
}
```

goesm 自身は goroutine を Worker で実行しません。Go の値を共有するスレッドを実現するには、すべての値を `SharedArrayBuffer` 上のバイト列として持つ必要があります。そうすると、Go の文字列、スライス、構造体を JavaScript の文字列、配列、オブジェクトとして表せなくなり、今の表現の速さと JavaScript から直接使える利点を両方とも失います。

## メモリ安全性

- メモリ安全性は Go と同等です。メモリは JavaScript エンジンのガベージコレクタが管理するので、解放済みのメモリが使われることはありません。範囲外のインデックス、nil ポインタの参照、失敗した型アサーション、整数のゼロ除算は、Go と同じく panic になります。
- `unsafe` で扱える範囲は、[ARCHITECTURE.ja.md §7](../ARCHITECTURE.ja.md#7-reflect--unsafe--メモリ表現の方針) に書いたとおりネイティブより狭くなります。ポインタはアドレスではなく JavaScript のオブジェクトで、ポインタから作った `uintptr` は、同じポインタに戻せるだけの代理の数値です。`unsafe.Add` や `uintptr` によるポインタ演算は、ポインタが指す構造体や配列の中にあるフィールドと要素のオフセットに対してだけ動き、フィールドの途中を指すオフセットは panic になります。メモリを別の型として読めるのは、その節が挙げるレイアウトだけです。それ以外の再解釈は goesm がコンパイル時に報告し、依存パッケージの中ではその関数が、呼ぶと panic するスタブになります。したがって、`unsafe` を使うコードも、ポインタが指す値の外のメモリには届きません。

## 確認方法

`TestKnownGaps` は、別の goroutine が待たずに計算している間に起動した goroutine が、goesm では動かず、ネイティブでは動くことを確認します。このケースは `testdata/semantics/gaps` の `Preemption` です。上で挙げたカウンタ、マップへの同時書き込み、ポーリングのループ、Worker の例は、Node.js と Bun で手作業で確認しました。
