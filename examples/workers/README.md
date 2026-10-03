# workers

[日本語](README.ja.md)

Goroutines and channels. `SumSquares` feeds a pool of worker goroutines through a channel and collects results with `sync.WaitGroup`; `Count` increments a counter from 8 goroutines under a `sync.Mutex`.

```sh
../bin/goesm build -o workers/dist ./workers && node workers/index.mjs   # in examples/
```

goesm's blocking analysis makes exactly the functions that may block `async`: `SumSquares` and `Count` return Promises in JS, `Square` stays synchronous. Goroutines are scheduled cooperatively (they switch at blocking points), so results are deterministic here.
