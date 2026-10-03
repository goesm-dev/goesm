# GopherJS との比較

[English](gopherjs-comparison.md)

GopherJS は Go→JavaScript の最も成熟した参考実装です。goesm は GopherJS の clone ではなく、「TypeScript を中間 target にし、esbuild を backend にする」ことで GopherJS が自前で持っている部分を外部に委ね、残りを現代の JS primitive で単純化することを狙っています。

GopherJS 側の記述は upstream の README / compatibility 文書と compiler / prelude の設計に基づく要約です (2026-10 時点の master。GopherJS の各 release は特定の Go release に対応し、現行は Go 1.21 系)。

| 観点 | GopherJS | goesm (PoC) | goesm で簡略化 / 変化した点 |
|---|---|---|---|
| package loading | 独自の build package (go/build ベース、modules 対応)。stdlib は natives overlay と組み合わせて自前で読み込む | `golang.org/x/tools/go/packages` に全面委譲 (go command が module / go.work / GOPROXY を解決)。置換する stdlib package は go/packages の parse 時 (`ParseFile`) に差し替える | module・build 解決のコードを持たない |
| Go version | release ごとに 1 つの Go version に対応 (natives が stdlib version に依存) | goesm 自身は version を固定しない。goesm を build した toolchain の go/types がそのまま frontend になる (`go tool goesm` で module の toolchain に追従)。Go 1.27 の generic methods を通している | 新 syntax は go/types が受理すれば入力可能。lowering の追加だけで済む。Go source 置換 (`runtime`、`internal/reflectlite`、`sync`) が追従するのは他の stdlib が使う API だけで、内部実装ではない |
| AST / 型情報 | go/ast + go/types から直接 JS を生成 (独自の解析: blocking、escape など) | 同じく typed AST から直接 lowering (SSA 不採用。理由は ARCHITECTURE.md §4) | 出力は JS ではなく TS IR。JS printer・minifier・bundler を持たない |
| 出力形式 | 1 本の script (独自の `$packages` registry)。自前の dead code elimination | 1 Go package = 1 ES module。`go:` specifier → esbuild が bundle、または `-split` で package ごとの ESM | ESM、tree shaking、code splitting、minify、target lowering を esbuild に委譲 |
| 整数 | `int` は 32-bit (32-bit 環境を emulate)、`int64`/`uint64` は high/low の 2 要素で正確 | `int` は Go の wasm 型検査どおり 64-bit。8〜32-bit は正確、64-bit は JS number (2^53 超で不正確) | **GopherJS の方が正確**。goesm の次の課題 (BigInt or hi/lo) |
| strings | JS string を byte 列として扱う | 同じ (1 code unit = 1 byte) | — |
| maps | runtime の `$keyFor` で key を文字列化し、JS の連想構造に `{k, v}` を格納 | JS `Map` + 型 descriptor による hash key (primitive はそのまま key、struct / interface / NaN は Go equality 用に直列化) | primitive key は文字列化不要 |
| pointers | 非 struct は getter / setter closure を持つ pointer object、struct pointer は struct object 自体 | ほぼ同じ発想。`.v` accessor の Cell / FieldPtr / IndexPtr、struct / array は object 自体。identity を WeakMap cache で保証 | 生成は runtime の数関数に集約 (将来の unsafe 用に差し替え可能) |
| interfaces | 非 struct 値は `T.wrapped` で包み、struct は constructor から動的型を得る。method は JS prototype | 常に `Iface{t, v}` (動的型 descriptor + 値)。dispatch は descriptor の method table | 表現が一様。prototype の継承関係に依存しない |
| goroutines | blocking 解析で印を付けた関数を、再開可能な state machine (`$s` による switch と frame 保存) に変換し、独自 scheduler で再開 | 同様の blocking 解析で印を付けた関数を **`async function`** にし、blocking 点を `await` に。goroutine は microtask で起動 | state machine 生成と stack 保存が不要。JS engine の async stack trace がそのまま使える。preemption 無しは両者共通 |
| channels | runtime の send / recv queue と `$select` | runtime の wait queue。即時完了は同期、block 時のみ Promise | blocking の意味論を runtime 境界に保ったまま Promise で中断を表現 |
| defer / panic / recover | goroutine ごとの defer stack と panic 状態を runtime で管理、state machine と連携 | 関数ごとに `Defers` frame と JS の `try/catch/finally` + label 付き `break` | named result の書き換えや return 後の defer を JS の制御構造で表現。goroutine-local な recover 状態は未実装 |
| reflect | natives で reflect を実装。型 object に詳細な metadata | 型 descriptor (kind、field、tag、method set、型引数) を常に出力。その上に `internal/reflectlite` を Go source で置換 (`errors`、`sort` に足りる範囲)。`reflect` 自体は未実装 | reflect も同じく descriptor 上の Go source として載せる予定 |
| runtime | 大きな手書き JS prelude + natives による stdlib の置換 | 小さな TS runtime (`@goesm/runtime`、fixture に必要な分だけ)。`runtime`・`internal/reflectlite`・`sync` は Go source で置換。Go の body を持たない関数は natives (関数ごとに 1 つの ES export) | runtime も natives も esbuild で tree shaking される |
| source maps | 自前の JS printer が JS→Go の map を直接出力 | goesm は TS→Go map だけを作り、esbuild が JS→Go に合成 | 最終 map の生成・合成・minify 後の追従を esbuild に委譲 |
| unsafe | ごく一部のみ | stdlib に必要なパターンのみ: `unsafe.Pointer` の往復変換、slice 要素上の `unsafe.String` / `unsafe.Slice`。メモリの再解釈は診断 (ArrayBuffer / DataView ベースの表現を予定、ARCHITECTURE.ja.md §7) | — |
| generics | 対応 (型引数を runtime に渡す方式) | erasure + 型 descriptor の dictionary 引数 | — |

## まとめ

* GopherJS から引き継いだ考え方: typed AST から直接 lowering、blocking 解析で goroutine を必要な関数だけに限定、string を byte 列として扱う、struct pointer = struct object。
* TypeScript + esbuild で簡略化したもの: JS printer、bundler / 出力形式、minify、tree shaking、source map の最終生成、ES target 対応。
* 現代の JS primitive で簡略化したもの: goroutine の state machine → async/await、defer → try/finally、map → JS `Map`、package registry → ES modules。
* GopherJS の方が進んでいるもの: 64-bit 整数、reflect、stdlib の網羅、`syscall/js`、deadlock 検出。
