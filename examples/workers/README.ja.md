# workers

[English](README.md)

goroutine と channel の例です。`SumSquares` は channel で worker goroutine の pool に仕事を渡し、`sync.WaitGroup` で結果を集めます。`Count` は 8 個の goroutine から `sync.Mutex` で守ったカウンタを増やします。

```sh
../bin/goesm build -o workers/dist ./workers && node workers/index.mjs   # examples/ で実行
```

goesm の blocking 解析は block し得る関数だけを `async` にします。JS からは `SumSquares` と `Count` が Promise を返し、`Square` は同期関数のままです。goroutine は協調的に (blocking 点で) 切り替わるので、ここでの結果は決定的です。
