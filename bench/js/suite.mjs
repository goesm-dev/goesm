// The benchmark suite: each kernel of package kernels with the size it is
// run at. Sizes are chosen so that native Go takes a few milliseconds or
// more per call, which keeps the cost of crossing from JS into the compiled
// code negligible. The kernels marked `calls` are the exception: they
// measure that crossing together with small pieces of work, as `arg` calls
// from a JS loop (see harness.mjs), and report the time per call.
export const SUITE = [
  { name: "Fib", arg: 30, what: "recursive calls, int arithmetic", ja: "再帰呼び出し、int 演算" },
  { name: "Sieve", arg: 2_000_000, what: "[]bool, tight loops", ja: "[]bool、密なループ" },
  { name: "Mandelbrot", arg: 400, what: "float64 loops", ja: "float64 のループ" },
  { name: "NBody", arg: 100_000, what: "float64 struct fields via pointers", ja: "ポインタ経由の構造体の float64 フィールド" },
  { name: "FNV32", arg: 1_000_000, what: "uint32 multiply and xor", ja: "uint32 の乗算と xor" },
  { name: "FNV64", arg: 1_000_000, what: "uint64 multiply and xor", ja: "uint64 の乗算と xor" },
  { name: "BinaryTrees", arg: 14, what: "allocation, GC", ja: "メモリ確保、GC" },
  { name: "Interfaces", arg: 1_000_000, what: "interface method calls", ja: "インターフェースのメソッド呼び出し" },
  { name: "MapInt", arg: 200_000, what: "map[int]int insert, lookup, delete", ja: "map[int]int の挿入・検索・削除" },
  { name: "MapString", arg: 500_000, what: "map[string]int counting", ja: "map[string]int での集計" },
  { name: "Strings", arg: 100_000, what: "strings.Builder, strconv, Split, Join", ja: "strings.Builder、strconv、Split、Join" },
  { name: "Sort", arg: 100_000, what: "sort.Ints, sort.Strings", ja: "sort.Ints、sort.Strings" },
  { name: "JSON", arg: 2_000, what: "encoding/json Marshal + Unmarshal", ja: "encoding/json の Marshal + Unmarshal" },
  { name: "Sprintf", arg: 50_000, what: "fmt.Sprintf", ja: "fmt.Sprintf" },
  { name: "Channels", arg: 100_000, what: "goroutines, unbuffered channels", ja: "goroutine、バッファなしチャネル" },
  { name: "Add", arg: 100_000, calls: true, what: "calls from JS: two numbers in, one out", ja: "JS からの呼び出し: 数値 2 つを渡して 1 つ受け取る" },
  { name: "Upper", arg: 100_000, calls: true, what: "calls from JS: strings.ToUpper, a string in and out", ja: "JS からの呼び出し: strings.ToUpper、文字列を渡して受け取る" },
  { name: "Handle", arg: 10_000, calls: true, what: "calls from JS: a JSON request handler, a string in and out", ja: "JS からの呼び出し: JSON のリクエストハンドラ、文字列を渡して受け取る" },
];

// The implementations, in the order the report shows them. "js" is the
// reference: the kernels hand-written in JavaScript (handwritten.mjs).
export const IMPLS = [
  { id: "js", label: "Hand-written JS", ja: "手書き JS", reference: true },
  { id: "goesm", label: "goesm" },
  { id: "gopherjs", label: "GopherJS" },
  { id: "gowasm", label: "Go wasm" },
  { id: "tinygo", label: "TinyGo wasm" },
];

// callInputs returns the strings a string-calling kernel (Upper, Handle) is
// called with in turn; package kernels builds the same (UpperInputs,
// HandleInputs) for native Go.
export function callInputs(name) {
  const inputs = [];
  for (let j = 0; j < 100; j++) {
    if (name === "Upper") {
      inputs.push(`user-${j} visited /items/${(j * 7919) % 10007}?ref=home`);
    } else {
      const items = [];
      for (let k = 0; k <= j % 8; k++) {
        items.push({ sku: `S${j}-${k}`, price: 100 + ((j * 37 + k * 101) % 5000), qty: 1 + ((j + k) % 5) });
      }
      inputs.push(JSON.stringify({ user: `user-${j}`, items }));
    }
  }
  return inputs;
}
