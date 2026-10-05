# Concurrency and memory safety

[日本語](concurrency.ja.md)

goesm runs goroutines as Go does with `GOMAXPROCS=1`, without preemption. Code that is correct in Go is correct under goesm. The differences are that goroutines never run in parallel, that a goroutine which never waits keeps the others from running, and that code with data races may happen to work. Memory safety is the same as Go's, and `unsafe` reaches less than it does natively.

## How goroutines run

- A goroutine is a JavaScript `async` function. All goroutines of a program share one JavaScript thread: the page's main thread in a browser, the process's thread under Node.js, Bun and Deno, or the thread of the Worker that loaded the program.
- A function that may wait becomes an `async` function, and each place where it may wait becomes an `await`. Channel operations, `select`, `time.Sleep`, I/O, locking a `sync.Mutex` that another goroutine may hold across a wait, and calls of such functions are waiting points. Every other function stays synchronous.
- Goroutines switch only at waiting points. While Go code runs without waiting, nothing else runs on its thread: no other goroutine, no timer, no I/O callback and no other HTTP request. In a browser, the page also stops rendering and responding to input.
- `runtime.GOMAXPROCS` returns 1 whatever its argument is and changes nothing. `runtime.NumCPU` returns 1, and `runtime.LockOSThread` has no effect. `runtime.Gosched` lets the other runnable goroutines run first, and then the host's timers and I/O callbacks that are due.
- An exported function that may wait returns a Promise to JavaScript. Other JavaScript code runs while the function waits, so two calls in flight interleave as two goroutines do.
- When every goroutine waits and the host has nothing left to do, Node.js and Bun end the program with Go's `fatal error: all goroutines are asleep - deadlock!`. A browser has no process to end, so the waiting goroutines simply stay blocked.

## Data races

- **No torn values.** In Go, a data race on an interface, a string or a slice can read a half-written value and corrupt memory, and maps written concurrently end the program with `fatal error: concurrent map writes`. Under goesm, each goroutine reads and writes between waiting points while no other goroutine runs. Racing goroutines therefore never see a half-written value, and a map that several goroutines write stays consistent.
- **Logical races remain.** Between two waiting points of one goroutine, other goroutines run and may change shared state. A goroutine that reads a balance, sleeps and then writes the balance back loses the other goroutines' updates under goesm as it does in Go. Protect shared state with `sync.Mutex` or channels, as in Go.
- **Racy code can happen to work.** 100 goroutines that increment a shared counter 10,000 times each without a lock printed 502731 in one native run and print exactly 1000000 under goesm. The same code is still wrong: built natively it loses updates, and under goesm it starts losing them as soon as a goroutine waits between the read and the write.
- **No race detector.** goesm has no `-race` mode. The Go code is the same, so run its tests with `go test -race` natively.
- **Polling without yielding hangs.** A loop that polls a flag, such as `for !done.Load() {}`, never ends when another goroutine, a timer or I/O is supposed to set the flag, because nothing else runs while the loop spins. A loop that calls `runtime.Gosched()` on each iteration works as in Go. Waiting on a channel, a `sync.WaitGroup`, a `sync.Cond` or a `context.Context` is better still.

The `sync` and `sync/atomic` packages behave as in Go. The atomic operations are plain loads and stores, which is enough on one thread.

## Parallelism with Workers

To use more than one CPU core, or to keep a long computation off a page's main thread, run the Go code in a Worker: a Web Worker in browsers, or `worker_threads` under Node.js and Bun. Each Worker loads the goesm module itself and gets its own copy of the program, with its own package variables, goroutines and memory. Go values, channels and mutexes do not cross from one Worker to another, so the Workers exchange data through `postMessage`, as with any other JavaScript module. The example below loads `lib/lib.js`, which `goesm build -o lib ./lib` builds from a package that exports `SumSquares`.

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

goesm does not run goroutines on Workers itself. Threads that share Go values would need every value to live as bytes in a `SharedArrayBuffer`, and then Go strings, slices and structs could no longer be JavaScript strings, arrays and objects. That would give up both the speed of the current representation and its direct use from JavaScript.

## Memory safety

- Memory safety is the same as Go's. Memory is managed by the JavaScript engine's garbage collector, so nothing is used after it is freed. An index out of range, a nil pointer dereference, a failed type assertion and an integer division by zero panic as in Go.
- `unsafe` reaches less than natively, as [ARCHITECTURE.md §7](../ARCHITECTURE.md#7-reflect--unsafe--memory-representation) describes. A pointer is a JavaScript object, not an address, and a `uintptr` made from a pointer is a stand-in number that only converts back to the same pointer. Pointer arithmetic with `unsafe.Add` or `uintptr` works only on the offsets of fields and elements inside the struct or array a pointer points into, and an offset into the middle of a field panics. Reading memory as another type works only for the layouts that section lists. goesm reports any other reinterpretation when it compiles, and in a dependency the function becomes a stub that panics when called. Code that uses `unsafe` therefore cannot reach memory outside the value it points into.

## How this is checked

`TestKnownGaps` checks that a goroutine started while another one computes without waiting does not run under goesm and does run natively (`Preemption` in `testdata/semantics/gaps`). The `gosched` program in `testdata/programs` polls flags that a timer and a sleeping goroutine set, with `runtime.Gosched`, and `TestPrograms` compares its output with native Go under Node.js and Bun. The counter, the concurrent map writes, the polling loop without `Gosched` and the Worker example above were checked by hand under Node.js and Bun.
