# goesm architecture

[English](ARCHITECTURE.md)

goesm は、現在の Go toolchain を frontend とし、本物の Go package と Go semantics を、Go package ごとに 1 つの ESM としてそのまま build できる TypeScript file の tree (と TypeScript の runtime) へ lowering する基盤の PoC です。この tree は任意の ESM bundler (Vite、Rolldown、esbuild) や TypeScript を扱える runtime (Bun、type stripping を使う Node.js) が読み込みます。`goesm build` は便宜的にそれを esbuild で bundle します。
「Go っぽい言語を JavaScript に変換する」ものではありません。独自 syntax・独自 module system・独自 type system は持ちません。

## 1. Pipeline と責務

```
.go / go.mod / go.sum / go.work
        │  go command + golang.org/x/tools/go/packages   (internal/loader)
        ▼
parse / package load / type check  ── go/parser, go/types (Go が言語仕様の authority)
        │
        ▼
Go semantic lowering                ── internal/lower   (goesm の本体)
        │
        ▼
TypeScript tree + @goesm/runtime    ── goesm emit-ts: <dir>/<import path>.ts, <dir>/@goesm/runtime/*.ts
        │  任意の ESM bundler (Vite / Rolldown / esbuild) または TS を扱える runtime (Bun, Node.js)
        │  goesm build: esbuild Go API (internal/build)、便宜的なもの
        ▼
JavaScript ESM (+ goesm build では .go を指す source map)
```

| layer | 担当 | 担当しないこと |
|---|---|---|
| Go toolchain (`go list` / go/packages / go/types) | module・package 解決、go.mod / go.sum / go.work / GOPROXY、build constraints、parse、type check、定数畳み込み、init order | — |
| goesm (`internal/lower`) | Go の意味論を TS + runtime 呼び出しへ写像、type metadata、blocking 解析、source map の第一段 (TS→Go) | parse、型検査、module 解決、JS printing |
| `@goesm/runtime` | JS にない Go 意味論 (slice / map / pointer / interface / panic / defer / channel / select / 整数 wrap / 型 descriptor) | 型の判定 (すべて compile 時に go/types が済ませている) |
| host の bundler / runtime (`goesm build` では esbuild の Go API) | TS syntax stripping、JS printer、target lowering、bundling、tree shaking、minify、code splitting、最終 source map | Go 意味論の判断 |

## 2. Repository 構成

```
cmd/goesm/            CLI: goesm build / goesm emit-ts
internal/loader/      go/packages による frontend と診断 (go list / go/parser / go/types の layer 付き)
internal/natives/     goesm が持つ stdlib package の Go source 置換 (runtime、reflect、internal/reflectlite、sync、syscall/js)
internal/lower/       typed AST → TypeScript lowering
  program.go          whole-program 解析 (address-taken 変数、blocking 解析)
  emit.go decl.go     package = 1 TS module、型 descriptor、struct class、method table
  func.go stmt.go     関数本体・文 (defer、switch、select、range、range-over-func ...)
  expr.go types.go    式、変換、演算子、型 descriptor / zero value
  writer.go           位置 marker 付き code writer (source location を codegen 中に保持)
internal/sourcemap/   TS→Go の Source Map v3 builder
internal/build/       pipeline 結合、TS tree の書き出し、esbuild Go API 呼び出し (goesm build) と split 用 resolver
runtime/              @goesm/runtime (TypeScript)。goesm binary に embed し、<dir>/@goesm/runtime/ に書き出す
test/                 end-to-end テスト (Node.js 実行、native Go との golden 比較)
testdata/             fixture module (普通の Go module。gofmt / go vet / go test がそのまま通る)
docs/                 GopherJS 比較、生成物の実例
```

## 3. Frontend: Go toolchain をそのまま使う

* `golang.org/x/tools/go/packages` で `NeedSyntax|NeedTypes|NeedTypesInfo|NeedDeps` を読み込みます。module 解決・`go.work`・`GOPROXY`・`go.sum` 検証はすべて go command の仕事で、goesm は一切再実装していません。
* 入力は Go の package pattern (`./main`, `example.com/app/...`)。`import "./foo.go"` のような独自 import はありません。
* `-toolexec prog` (`build` / `emit-ts`、または `GOFLAGS` の `-toolexec`) で、compile が見る source を書き換える tool (OpenTelemetry の compile-time instrumentation など、[docs/otelc.ja.md](docs/otelc.ja.md)) を組み込めます。goesm は target 向けに `go list -deps -export` を `prog` を toolexec program として実行し、compiler の代わりに goesm 自身の recorder を置きます (`internal/toolexec`)。recorder は各 compile の Go file を記録し、build の一時 directory にあるものは内容を保存してから compiler を実行します。package 自身の file と異なる記録済み file が overlay になります。go command は `GOMODCACHE` 以下 (依存 module と、`GOTOOLCHAIN` で download した toolchain の stdlib) の overlay を拒否するため、それらの file については、go command にはディスク上のままの package を list させ、goesm が overlay の file を代わりに parse し、追加 file を package に加え、追加された import を package path で解決して、go/packages と同じ方法で自ら型検査します (`loader/check.go`)。go command は cache 済みの compile では tool を実行しないため、記録は compile の action ID ごとに user cache directory に保存し、goesm は compiler が報告する version に印を付けて、これらの compile を通常の build とは別に cache させます。
* `-overlay file` (`build` / `emit-ts`) は go command 自身の overlay 形式 (`go build -overlay`, `packages.Config.Overlay`) を受け取ります。絶対パスでディスク上のファイルを置き換え・追加でき、存在しないディレクトリにも置けます。Go を別のファイルに埋め込むホスト (Vue SFC 向けの gosfc) は `//line` directive 付きの Go ファイルを生成してこの経路で渡します。位置は常に `//line` を反映する `token.FileSet.Position` 経由で取るため、go/types の診断と goesm の source map はホストのファイル (`Summary.vue:17:21`) を指します。
* target の build constraints は `GOOS=js GOARCH=wasm` (既存 port のうち JS host に最も近いもの)。`int` は 64-bit として型検査されます。
* **Go version を固定しない**: go/parser と go/types は goesm binary にリンクされるため、goesm が理解できる最新 syntax は「goesm を build した toolchain」の syntax です。そこで goesm は `go tool goesm` (go.mod の `tool` directive) や `go run` で、その module が選ぶ toolchain により都度 build される前提にしています。toolchain の方が新しい場合は `loader.VersionHint` がそれを診断します。この PoC 自体 Go 1.27 で build し、Go 1.27 の generic methods を fixture で通しています (`testdata/semantics/generics`)。

## 4. AST から直接 lowering するか、x/tools/go/ssa か

**判断: typed AST (go/ast + go/types) から直接 lowering する。** SSA は採用しません。

| 基準 | typed AST | go/ssa |
|---|---|---|
| 1. 最新 Go syntax への追従 | go/types が受理すれば即使える。新構文は lowering 1 箇所の追加で済む | x/tools の SSA builder 側の対応待ちが発生する (generic methods、range-over-func 等は SSA 側の実装が先に必要) |
| 2. Go semantics の正確性 | 評価順・defer・named result 等は lowering で明示的に扱う必要がある | SSA は評価順を明示化済みで有利 |
| 3. 実装量 | 構造化制御フローを JS の制御構文へそのまま写せる | basic block + phi を JS に戻す relooper/stackifier が必要で大きい |
| 4. source location | AST node の位置をそのまま marker として埋め込める | 命令単位の位置は粗く、構造化後に再対応が必要 |
| 5. runtime semantics の将来実装 | async/await、try/finally、label 付き break が JS の構造に素直に乗る | goroutine の再開点などは state machine 化が前提になる |

2. の不利は、評価順に関わる箇所 (多値代入、defer の引数評価、range 式の一回評価など) を lowering 側で temp に落とすことで補っています。GopherJS も同じく typed AST ベースです。

## 5. Lowering 方式

* **1 Go package = 1 TypeScript module = 1 ES module**。`goesm emit-ts -o <dir>` は Go package `p` の module を `<dir>/<p>.ts` (`example.com/app/main.ts`、`strings.ts`、`internal/bytealg.ts`) に、runtime を `<dir>/@goesm/runtime/index.ts` に書き出します。natives は `natives.ts`、その他の runtime file も同じ directory に並びます。この tree が goesm の主な出力です。
  * module 同士は `.ts` で終わる相対 specifier で import し合います。Go の import は `import * as mathx from "./mathx.ts"`、runtime は `import * as $rt from "../../@goesm/runtime/index.ts"` になります (`internal/bytealg.ts` では `import * as $natives from "../@goesm/runtime/natives.ts"`)。Go の import path は `@` で始まらないので、runtime が package と衝突することはありません。resolver は不要で、TypeScript (`allowImportingTsExtensions`)、Vite、Rolldown、esbuild、Bun、Node.js の type stripping がこの specifier をそのまま解決します。
  * `goesm build` (既定) は esbuild がこの tree を plugin なしで 1 bundle (`dist/main.js`) にまとめます。
  * `goesm build -split` は package ごとに `dist/example.com/app/mathx.js` を出し、runtime は `dist/@goesm/runtime/index.js` と `dist/@goesm/runtime/natives.js` になります。小さな内蔵 resolver が他の entry point (package の module と runtime) の import を external にして `.ts` を `.js` に書き換えるので、Go の import は `import * as mathx from "./mathx.js"` という ESM dependency として残ります。
* 名前: Go の識別子は `$` を含まないので、goesm が導入する名前はすべて `$` を含みます (`User$type`, `User$Adult`, `$rt`, `$t3`)。1 つの関数宣言内の Go object には一意な JS 名を振るため、Go の shadowing を JS の scope 規則で再現する必要がありません。
* 定数式は go/types が評価した値をそのまま出力します (iota、型付き定数、`unsafe.Sizeof` 等)。
* package 変数は `types.Info.InitOrder` の順で初期化し、次に `init()`、entry package に `func main` があれば最後に `main()` を実行します。
* 生成 TS は型を持ちます。Go の型は go/types が決めており、TS の型はそれに従います。tree 全体 (生成 module と runtime) は strict mode に `verbatimModuleSyntax` と `erasableSyntaxOnly` を加えた tsc で型検査が通るので、type stripping を行う runtime (Node.js 22.18 以上、Bun) でもそのまま動きます。`TestTSC` は CI の必須 check です。fixture と examples を、exported な Go API を使う consumer と一緒に型検査します。consumer の `@ts-expect-error` は、exported API が Go の型を持つこと (`Total(items: $rt.S<Item>): number`、block する関数は `Promise<number>` を返す) を確認します。exported な signature と struct の class は正確に型付けし、内部の一時変数や wrapper は `any` です。型 parameter は制約から型付けします (core type、または `number` / `string`)。native Go との結果比較は引き続き意味論の gate です。

### 値の表現

| Go | JS 表現 | 備考 |
|---|---|---|
| bool, float64 | boolean, number | |
| float32 | number (`Math.fround` で丸め) | |
| int8/16/32, uint8/16/32 | number、演算ごとに wrap (`\|0`, `>>>0`, `<<24>>24`, `Math.imul`) | 正確 |
| int64, uint64 | bigint。演算のたびに `BigInt.asIntN` / `asUintN(64, ...)` で wrap | 正確 |
| int, uint, uintptr | number | **2^53 を超えると不正確、64-bit wrap なし** (既知の差分) |
| string | JS string、1 code unit = 1 byte | `len`、index、slice、比較、不正 UTF-8 が Go と一致。JS 境界で `toJSString` / `fromJSString`。64 byte 以上の `[]byte(s)` と `string(b)` は engine の `TextEncoder` と windows-1252 の `TextDecoder` を使い (ASCII の扱いは Go と同じ)、それ以外の byte があれば byte ごとの loop に戻る |
| struct | 生成 class の instance (`$clone` / `$set`) | 値 copy は lowering が挿入。object identity がそのまま address |
| array | JS array | struct と同じく copy は明示的 |
| slice | `Slice{$array,$offset,$length,$capacity}`、nil は `null`。`$array` は JS array で、`make`・`append`・`[]byte(s)` で作った `[]byte` では `Uint8Array` (Go の array の slice と literal は JS array のまま)。65 バイトから 4 KiB のものは共有の 16 KiB slab の view です (V8 はそれより大きい `Uint8Array` の領域を heap の外に 1 個 1〜3 µs かけて確保するため)。値が関数の外に出ない (make・添字アクセス・`len`/`cap` だけに使う) ローカルの `[]bool` 変数もバイト列で持ち (boolean の配列の 4 分の 1 のメモリ)、読み出しを boolean に戻します。`s[i]` の境界チェックはインラインで、添字が構造上範囲内のとき (`for i := range s`、`lo >= 0` の `for i := lo; i < len(s); i++` で `s` と `i` を他で代入しない) は省きます | append / re-slice の aliasing が Go と同じ |
| map | JS `Map` の上の `GoMap`、nil は `null`。bool・整数・string・channel の key はそのまま `Map` の key で値を直接持ち (`Map` の等価性がこれらでは Go と同じ)、それ以外の key は Go equality で hash して `[key, value]` を持つ | struct / interface / NaN key、nil map の panic |
| pointer | `*struct` / `*array` は object 自体。それ以外は `.v` を持つ object (`Cell` / `FieldPtr` / `IndexPtr`) | `&x == &x`、`&s.f == &s.f` を cache で保証 |
| interface | `Iface{t: 型 descriptor, v: 値}`、nil は `null` | 動的型を保持。`MyInt(1)` と `int(1)` を区別、nil `*T` を入れた interface は non-nil |
| func | JS function | |
| chan | runtime `Chan` | |
| 型 parameter | 型引数の表現そのもの (erasure) | 型 descriptor を dictionary 引数で受け取る |

### 64-bit 整数

`int64` と `uint64` は BigInt、`int`・`uint`・`uintptr` は JS の number のままです。64 bit 全体が必要な Go のコード (hash、乱数、`time` の nanosecond、JSON 中の ID、`math.Float64bits`) は明示的に 64-bit 型を使い、`int` は index や個数に使われます。後者は number の方が数倍速く、配列や文字列が持てる範囲では正確です。

演算は inline で書きます (`BigInt.asIntN(64, a * b)`)。V8 はこれを機械語の演算に compile します。除算・剰余・shift は小さな runtime helper を呼びます (0 除算 panic、Go の shift の意味論)。2 つの世界の間の変換は明示的で (`BigInt(i)`、`Number(x)`)、2^53 を超える `int64` を `int` に変換すると丸められます。exported 関数はこれらの型を `bigint` で受け取り・返します。

`bench/int64.mjs` で、goesm が lower し得る表現を、goesm が出力する形で書いた 4 つの workload で比較しました (5 回の最良値、Node 22 / Bun 1.3):

| workload | number (不正確) | BigInt | `{hi, lo}` object | uint32 の local 2 つ | 2^53 を超えたら BigInt の number |
|---|---|---|---|---|---|
| counter と合計 (小さい値) | 13 / 6 ms | 136 / 1003 ms | 260 / 397 ms | 141 / 55 ms | 124 / 30 ms |
| 4 MiB の FNV-1a 64 | 38 / 32 ms | 35 / 498 ms | 170 / 241 ms | 104 / 101 ms | 306 / 849 ms |
| xorshift64* | n/a | 209 / 684 ms | 97 / 273 ms | 113 / 86 ms | n/a |
| Unix nanosecond (加算・除算・剰余) | 13 / 45 ms | 41 / 296 ms | 758 / 978 ms | 306 / 845 ms | 147 / 359 ms |

V8 (Chrome、Node、Deno) では BigInt が 4 つ中 3 つで最速の正確な表現で、hash では number とほぼ同じです。JavaScriptCore (Safari、Bun) では uint32 2 つより 3〜18 倍遅くなります。2 分割は lowering と JS API で、すべての `int64` の変数・field・引数・戻り値を 2 倍にします。BigInt は lowering と exported API を単純に保ち、どこでも正確なので、これを表現として採用しました。hot な local 演算に 2 分割の fast path を入れる余地は、値の表現を変えずに後から残せます。

### 型 metadata

すべての named type は runtime descriptor (`$rt.named(pkgPath, name)`) を持ち、underlying 型、field (名前・pkgPath・tag・embedded)、value / pointer の method set (method 名と signature descriptor) を登録します。複合型 (`[]T`, `map[K]V`, `func(...)`, `struct{...}`, `interface{...}`) は構造で memoize されるため、**descriptor の同一性 = Go の type identity** です。interface 判定はこの method table で行い、TypeScript の structural typing には依存しません。unexported method は pkgPath で修飾されます。interface 経由の呼び出しは動的型の table から method を直接読む (`x.t.mt["M"](x.v, ...)`) ので、呼び出し箇所ごとにそこを通る少数の型に対する inline cache が効きます。各型の table は、その型の method 関数そのもの (receiver の調整が要らないものはラッパーなし) を prototype に持つ空のオブジェクトなので、JS の class の method と同じく hidden class ごとに既知の関数へ解決され、エンジンがインライン化できます。

### Generics: type erasure + runtime type dictionary

* 関数・method のコードは 1 つだけ生成し (erasure)、型引数は runtime 型 descriptor の **dictionary 引数**として先頭に渡します: `First($T_T, values)`。
* descriptor があるので、zero value (`var x T`)、interface 変換 (`any(x)`)、`==`、aggregate の copy、`new(T)`、`make([]T)` を型引数に応じて正しく行えます。
* generic type の instance (`Stack[Pair[string,int]]`) も memoize された descriptor になり、`a.(Pair[string,int])` と `a.(Pair[string,string])` を区別します。
* Go 1.27 の generic methods は、receiver の型引数 → method の型引数の順に dictionary を渡します。interface を満たさないため method table には載りません。
* 理由: specialization はコード量が型引数の数だけ増え、JS bundle では不利です。TS generics に残すだけでは runtime に型情報が無く、zero value・interface 変換・reflect が実装できません。erasure + dictionary は Go の gc (GC-shape stenciling + dictionaries) と同じ考え方で、最も単純かつ reflect に繋がります。TS 側には可読性のため `<T>` を残しますが、意味は持たせていません。

### defer / panic / recover

```ts
function F() {
  let $r0 = zero;                 // 結果変数 (named result はその名前)
  const $d = new $rt.Defers();
  $body: try {
    ...; $r0 = expr; break $body; // return は結果を代入してから defer へ
  } catch ($e) { $d.fail($e); } finally { $d.run(); }
  return $r0;                     // defer が named result を書き換えた値を返す
}
```

* panic は `GoPanic` (JS Error) を throw。値は interface 値として保持し、runtime error は `runtime.Error` を実装する型 (`Error()` / `RuntimeError()`) を持ちます。JS の `TypeError` (nil 参照) は nil pointer dereference の runtime error に変換します。ポインタ経由のフィールドや要素のアクセス (`p.f`、`*p`、`*[N]T` の `p[i]`) は明示的な nil チェックをせずこれに頼ります (明示的なチェックがあると V8 の高速なプロパティアクセスが効かなくなるため)。entry package の exported な関数と method、JS から呼び戻される Go の関数は wrapper で包み、JS からは引き続き `GoPanic` に見えます。
* defer の関数値と引数は defer 文の時点で評価し、closure に閉じ込めます。
* recover() は deferred 関数自身の中でだけ recover します。各 deferred call は、静的に分かる場合 (関数、具象型の method、関数リテラル、`recover` builtin) は呼ぶ関数を記録し、recover() を呼ぶ関数は入口でその呼び出しかどうかを問い合わせます (`const $rf = $rt.recoverFrame("main.F")`。再帰呼び出しは該当しない)。答えは recover() が panic を recover する frame なので、await の後でも効きます。defer された `recover()` は、それを defer した関数が呼んだものとして扱います。
* JS 側から見ると、捕捉されない panic は `GoPanic` 例外になり、`--enable-source-maps` で stack が `.go` の行を指します (テスト済み)。

### goroutine / channel / select

* **blocking 解析** (whole program): channel 操作 (default 付き select の case を除く)、default 無しの select、channel の range、blocking 関数の呼び出し/defer、blocking し得る動的呼び出しを含む関数を blocking とし、`async function` に lowering します。blocking 点はすべて `await`。それ以外は同期関数のままです (await のコストを払わない)。動的呼び出しは保守的に解決します。関数値経由の呼び出しは、同じ signature を持ち、どこかで値として使われる関数 (その場で呼ばれない関数 literal、callee 以外の位置で参照される関数や method) に届き得るとみなします。interface method の呼び出しは、interface 値に格納され得て、その interface を実装する型の同名 method に届き得るとみなします。格納され得る型とは、どこかで interface 型へ変換される型 (代入、引数、return、composite literal の要素、send、map の key、明示的な変換、`append`、`panic`)、すべての instantiation の型引数、そしてそれらから field・要素・pointer で辿れる型 (reflection 用) です。このため block する `io.PipeWriter.Write` があっても `io.Writer.Write` の呼び出しすべてが async になることはなく、program が pipe を `io.Writer` に格納しない限り `fmt.Println` は同期のままです。range-over-func の body は iterator に渡す yield 関数です。block し得る body は async な callback になり、解析はそれを yield 型の関数値として数えるので、それを呼び得る iterator は await します。
* **Mutex**: goroutine が `sync.Mutex` を lock 済みで見つけるのは、保持者が block しているときだけです。そこで `Lock` は同期関数で、待ちません。goesm は block した goroutine が保持し得る critical section を見つけます。`Lock` から同じ block 内の対応する `Unlock` まで (`defer Unlock` なら末尾まで。`Unlock` の無い `if` の branch や block 内の `Lock` なら、`if c { mu.Lock() }; ...; if c { mu.Unlock() }` のように、外側の文の列でそれを unlock する最初の後続の文まで) の間に block する操作があるもの、そして対応する `Unlock` の無い `Lock`、または `Unlock` に届く前に抜ける経路 (`return`、`panic`、`runtime.Goexit` やそれを呼び得る関数の呼び出し、block の外への `break`・`continue`・`goto`) のある `Lock` です。section は mutex を直接 (変数や struct field の path、これを mutex の key と呼びます) か、間接的に (pointer 経由、`sync.Locker` 経由 (それを制約とする型パラメータも含む)、`f().Lock()` のように名前が無いもの) lock します。直接の `Lock` / `RLock` は、その key にそのような section があるとき、または間接的な section があり、かつその key の mutex が escape する (address を取られる、method value や `RLocker` で使われる、interface に格納される型に埋め込まれている) とき、待機する (async な) `lockSlow` / `rLockSlow` になります。間接的なものは、間接的な section があるか、保持される key が escape するときに待ちます。このとき `Locker.Lock` は `lockerSlow` になり、`sync` package の mutex なら待ち、それ以外の Locker は呼び出して、それが埋め込まれた mutex の Lock で lock 済みだった場合は次の unlock の後に再試行します。これを blocking 解析と交互に、変化がなくなるまで繰り返します。`(*sync.Mutex).Lock(&mu)` のような method expression は `mu.Lock()` と同じに扱います (埋め込まれた mutex から promote された method は除く)。文で区切られない section は保持されたものとみなします: `TryLock`、`if` / `switch` の初期化文や defer の中の `Lock`、`Lock` の method value (`lock := mu.Lock`)。同じ規則で待つ可能性のある `Lock` の method value や method expression の値は待機版に bind され、その型の関数値 (`func()`、`func(*sync.Mutex)`) の呼び出しは async になります。`go` 文の関数値と引数は文を実行する goroutine が評価するので、その中の block する操作も数えます。`sync` package 自身の lock (`Cond.Wait`、`RLocker`) は解析の対象外です。
* `go f(x)` は関数値と引数をその場で評価し、`$rt.go(closure)` が microtask として起動します。
* channel は runtime 内の buffer と送受信 wait queue で表現し、即時完了できる場合は同期的に値を返し、block する場合だけ Promise を返します。unbuffered の handoff、close (待機中 sender への panic を含む)、`select` (ready な case から一様ランダム、default、nil channel は永久 block) を実装しています。
* つまり「async/await に変換すれば Go と同じ」とは扱っていません。blocking の意味論は wait queue という runtime 側の境界にあり、async/await は「goroutine を中断・再開する手段」に限定しています。`runtime.Goexit` (deferred 呼び出しは実行され、`recover` では止まらない) と `sync` の置換 (§9) はこの境界の上に実装しました。deadlock 検出、goroutine-local な panic 状態、timer も同様に載せます。
* JS 境界: blocking する exported 関数は Promise を返します (例: `await Example()` は 42)。
* **プログラム**: main package の module は `$rt.runMain` で `main` を実行します。Go と同じく、`main` が return すると (他の goroutine が動いていても) process は終了し、`os.Exit` は deferred 呼び出しを実行せずにその code で終了し、どこでも recover されない panic は `panic: ...` と `goroutine 1 [running]:`、JS の stack を標準エラーに出して status 2 で終了します。package の初期化中の panic も同じです: main module は最初に `@goesm/runtime/program.ts` を import し、これが捕捉されない例外を同じ crash に変えるので、すべての依存 package の変数初期化と `init` 関数も対象になります。`main` 内の `runtime.Goexit` は main goroutine だけを終わらせ、他の goroutine がすべて終わると `fatal error: no goroutines (main called runtime.Goexit) - deadlock!` を出します。`main` が block したまま host の event loop が空になると (Node と Bun の `beforeExit`。JavaScript のコードが求めた終了は含まない)、もう goroutine を起こせるものはないので、Go と同じ `fatal error: all goroutines are asleep - deadlock!` を出して status 2 で終了します。この監視は `program.ts` が始めるので、永久に block する `init` 関数も Node では同じく報告されます。Bun は決着しない top-level `await` で spin し続けるため、そのような program は Bun では止まりません。process の無い browser では、recover されない panic は `reportError` で報告し、block した `main` はそのまま block し続け、`os.Exit` は deferred 呼び出しを実行せずに goroutine を巻き戻します (recover はできません)。

## 6. runtime 構成 (`runtime/src`、`<dir>/@goesm/runtime/*.ts` として出力)

| file | 責務 |
|---|---|
| `types.ts` | 型 descriptor (reflect.Kind 準拠の kind、named / 複合型の memoize、method table、generic instance、`error`) |
| `iface.ts` | interface 値、box / assert / type switch (具体型の case は 1 回読んだ動的型と比較)、`==`、map 用 hash key |
| `slice.ts` | slice、append / copy / bounds check、core type を持たない型 parameter への index |
| `map.ts` | Go map |
| `ptr.ts` | Cell / field pointer / element pointer、型 parameter 経由の load / store |
| `string.ts` | byte string ⇔ UTF-8 / rune、JS 境界変換 |
| `int.ts` | 整数除算・剰余 (0 除算 panic)、shift、64-bit bit 演算、min / max |
| `panic.ts` | GoPanic、runtime error 型、Defers、recover |
| `chan.ts` | channel、select、goroutine の起動と数、`main` の実行 (終了 status、crash 出力、deadlock) |
| `complex.ts` | 複素数 |
| `host.ts` | 標準出力・標準エラー (Node / Bun / Deno の `fs` への write、browser では console)、process の終了 |
| `print.ts` | `print` / `println` builtin (Go runtime の書式) |
| `natives.ts` | Go の body を持たない stdlib 関数の実装 (§9)。それを必要とする stdlib の module だけが import する独立した module で、関数ごとに 1 export なので使われないものは tree shaking で落ちる |
| `fmt.ts`、`json.ts` | `fmt.Sprintf` と `encoding/json` の `Marshal`・`Unmarshal` の高速経路。`natives.ts` が import する。`natives.ts` と同じく runtime を `index.ts` 経由でだけ使うので、`-split` build でも runtime は 1 つ |
| `interop.ts` | 型 descriptor に従う Go 値 → JSON 形 JS 値 (golden テスト・将来の JS ABI) |

fixture を通すのに必要なものから実装しており、scheduler や reflect の先行実装はしていません。

## 7. reflect / unsafe / メモリ表現の方針

* **reflect**: codegen は named type identity、field 名・tag・embedded、method set (名前・signature)、型引数を runtime descriptor として常に残し、`Kind` は reflect と同じ番号です。`reflect` package はその上の Go source で置換しています (§9)。`reflect.Type` は interface 値の descriptor、`reflect.Value` は (descriptor, 値)、addressable なら (descriptor, goesm の pointer) で、`Set` はその pointer 経由で書き込みます。これにより `fmt`、`encoding/json` (内部は Go 1.27 の json v2) を自身の Go source から compile できます。メモリアドレスが必要なもの (`UnsafeAddr`、`NewAt`、`StructOf`) は panic し、block する関数の `Value.Call` も panic します。reflect 経由の channel 操作は block しない場合のみ動きます。
* **pointer 表現**: 現在は「aggregate は object 自体、他は accessor object」。`unsafe.Pointer` との相互変換を将来入れるため、pointer 生成は `ptr.ts` の関数に集約してあり、表現を差し替えられます。
* **unsafe / linear memory**: 「JS に pointer は無いので非対応」とはしません。予定している方向は、(1) `[]byte` 等の数値 slice を TypedArray backing にする (slice の backing store は `slice.ts` の private な表現)、(2) `unsafe.Pointer` を (ArrayBuffer, byte offset) または (object, field) の tagged 表現にし、`unsafe.Slice` / `unsafe.String` / `unsafe.Add` を DataView 上で実装する、(3) 必要な package だけ linear memory (ArrayBuffer) 上に struct を layout する、の段階的導入です。現状は次のとおりです:
  * `unsafe.Pointer` は pointer object そのものを保持するので、`*T` → `unsafe.Pointer` → `*T` は恒等変換です (`strings.Builder`、`sync/atomic.Pointer`、`internal/race` がこれに依存)。
  * `unsafe.String(&b[i], n)`、`unsafe.String(unsafe.SliceData(b), n)`、`unsafe.Slice(&a[i], n)`、`unsafe.SliceData(s)` は slice / array の要素に対して動きます (同じ backing array 上の slice)。`unsafe.Slice(unsafe.StringData(s), n)` は copy しますが、byte は不変なので区別できません。
  * 型 parameter の `unsafe.Sizeof` / `Alignof` は wasm の size で descriptor から計算します。
  * pointer は由来 (provenance) を保持します: `unsafe.StringData(s)` は元の string を、`unsafe.SliceData` は backing array を覚えているので、それらに対する (および単一の変数を指す pointer に対する) `unsafe.String` と `unsafe.Slice` が動きます。
  * メモリの再解釈 (異なる `T` への `(*T)(unsafe.Pointer(&u))`) は、protobuf などの library が頼る layout については動きます (`runtime/src/unsafe.ts`): pointer 変数を別の pointer 型として読む (`sync/atomic` 向けの `(*unsafe.Pointer)(unsafe.Pointer(&p))`)、同じ layout の struct、string (`{data, len}`)・slice (`{data, len, cap}`、読み取り専用)・空 interface (`{type, data}`) を写した header struct で、これらは元の値の view になります。それ以外の再解釈は goesm 診断です (依存 package では関数が stub になります、§9)。`uintptr(unsafe.Pointer(p))` は pointer ごとに安定して異なる代理のアドレスで、表示 (`%p`) や identity の map には足り、pointer が生きている間は同じ pointer に戻せます。
  * protobuf-go の生成 message の高速経路が行う field offset による pointer 演算 (`unsafe.Pointer(uintptr(p) + off)`、`unsafe.Add`): 結果は `p` が指す struct / array への (object, byte offset) のアドレスで、`*T` に変換すると `reflect` が報告する wasm の layout (`StructField.Offset`) でその offset にある field / 要素を、入れ子の struct や array にも降りて見つけます。struct の先頭 field は struct 自身 (protobuf の `messageState` の手法)、`reflect.NewAt` はこのアドレスを受け取り、`[]*T` の field を pointer だけを持つ struct の slice (protobuf の `[]pointer`) として読むと要素を包み/剥がす view になり、`unsafe.Pointer` の比較はアドレスを比較します。field の途中を指す offset や、異なる layout の型として field を読むと panic します。stdlib でこれを行う少数の関数は natives に置き換えています (`math.Float64bits`、`slices.overlaps` など)。

## 8. Source maps と diagnostics

* lowering は式・文の文字列に Go の位置 marker を埋め込み、writer が出力列を確定させた時点で mapping を記録します。codegen の途中で位置情報を捨てません。
* 生成 TS ごとに TS→Go の Source Map v3 を作り、TS に inline で添付します (`emit-ts` は module の隣に `<p>.ts.map` としても書き出します)。`goesm build` では esbuild がそれを読み込んで最終的な **JS→.go** の map を合成します (`sourcesContent` に Go source を含む)。他の bundler が合成するとは限りません。Vite 8.3 と Bun 1.3 では最終的な map は生成された `.ts` file を指します。Node の `--enable-source-maps` で panic の stack が `panics.go:NN` を指すことをテストしています。
* 診断は layer を区別します:
  * `file.go:4:17: ... [go/types]` / `[go/parser]` / `[go list]` — Go frontend の error。元の `.go` 位置。
  * `file.go:6:9: reinterpreting *int64 as *[2]int32 through unsafe.Pointer is not supported [goesm lowering]` — goesm の未対応。Go の compile error とは別物として表示。
  * stdlib package では、goesm がまだ lowering できない関数は error にしません。呼ばれると panic する stub (`goesm: <func> is not supported yet`) になり、CLI がその数を報告します (`-v` で最初の理由とともに一覧)。大半のプログラムはこれらに到達せず (`internal/abi` の gc 型 layout、complex など)、到達しないものは tree shaking で落ちます。
  * `internal error: esbuild rejected TypeScript generated by goesm ... [esbuild]` (`goesm build`) — goesm の bug。Go の error に見せかけません。

## 9. stdlib

方針は「通常の Go source をそのまま compile する」で、TS への手移植はしません。GopherJS の natives と同様に、gc runtime に結び付いた少数の package には target 固有の置換が必要ですが、goesm ではそれも **Go source** として持ちます (`internal/natives/goroot/<import path>/`):

| 置換する package | 置換の内容 |
|---|---|
| `runtime` | 他の package が使う exported API (`GOOS`、`Error`、`Goexit`、`Gosched`、`KeepAlive`、`Caller`、`MemStats` など)。scheduling とメモリは `@goesm/runtime` 側 |
| `reflect` | runtime の型 descriptor の上の全 API (type、value、`Set*`、`Call`、`MakeFunc`、map、slice、`Convert`、`DeepEqual`、`VisibleFields` など)。`fmt` と `encoding/json` に足りる範囲 |
| `internal/reflectlite` | `Type` = runtime の型 descriptor、`Value` = (descriptor, 値 or pointer)。`errors.Is` / `errors.As`、`sort.Slice`、`context` に足りる範囲 |
| `sync` | `Mutex` と `RWMutex` は同期的に lock し、block を跨いで保持される mutex (§5) だけ待つ (async)。`WaitGroup` と `Cond` は channel で待つ。`Once`、`Map`、`Pool` は普通の Go |
| `syscall/js` | js/wasm の `syscall/js` API を JS の値そのものの上に実装 (`Value` が値を持つ)。goesm は stdlib を js/wasm 向けに compile するので、`os`・`syscall`・`time` はこれを通じて host に届く。`js.Global().Get("fs")` は `globalThis.fs` が何であっても常に goesm 自身の file system で、`syscall` package が期待する callback API を持ち、return する前に callback を呼ぶ (Node・Bun・Deno では `node:fs` の上に、browser では console を使う代替)。そのため `os.Stdout` と `os.Stderr` はどこでも動き、呼び出し側を async にしない。`process` は host のもの、browser では最小限の代替 |

仕組み:

* loader は `packages.Config.ParseFile` を設定します。go/packages が `$GOROOT/src` 以下の置換対象 package の file を parse するとき、最初の file が置換 source になり、残りは空 file になります。したがって go/types はすべての importer を置換後の package に対して型検査し、go command が解決した package graph は変わりません。`packages.Config.Overlay` ではできません: `go tool goesm` の toolchain がある module cache 以下の file は go command が overlay を拒否します。
* 型検査済みの import (置換後) で到達できる package だけを lowering するので、gc runtime の内部 (`internal/runtime/*` など) は外れます。
* package は build tag `purego` と `math_big_pure_go` 付きで load します。stdlib が assembly も持つところでは portable な Go のコードが選ばれます (gc が assembly 無しで build するときと同じ)。
* stdlib の Go の body を持たない関数 (assembly、`//go:linkname` 宣言、置換で body を省いたもの) は `runtime/src/natives.ts` に、`types.Func.FullName` から決まる名前の export として実装します。export の有無は goesm が compile 時に確認します。メモリを再解釈する Go body を持つ関数も、短い固定 list (`natives.Override`) で natives に置き換えます: `math.Float64bits` など、64-bit の `math/bits` 関数 (BigInt の半分ずつで計算)、`internal/strconv.formatBits` (Go の body が `uint64` の桁を `uint` 経由で狭めるため)、`slices.overlaps`、`internal/abi.NoEscape`。結果が IEEE 754 で決まる math の関数 (`Floor`、`Ceil`、`Trunc`、`Round`、`RoundToEven`、`Sqrt`、`Abs`、`Signbit`、`Copysign`、`Inf`) も JS の builtin にしています。Go の body の bit 操作より数倍速くなります。
* **patch** (`internal/natives/patch/<import path>/<file>.go`) は、それ以外は自身の source から compile する package の一部の宣言だけを変えます。patch の宣言は package 内の同名の宣言を置き換え (元の宣言はその場で `_` に rename するので、型検査はされますが出力はされません)、import とともに同名の file に追加されます。`time` はこの方法で patch しています: gc runtime が実装する `Sleep`、`Timer`、`Ticker` と timer 関数を host の `setTimeout` (natives.ts の `armTimer`) の上で動かし、timer の関数は発火時に新しい goroutine で実行します。その他の patch は、`unsafe.Pointer` 経由で memory を読むコードや、gc が compiler で実装するものを置き換えます: `sync/atomic.Value`、`hash/maphash` (runtime が hash)、`log/slog.Value` の string と group、`crypto/internal/fips140/subtle.XORBytes`、`alias.AnyOverlap` と SHA-3 の置換関数、`crypto/internal/fips140/check` の自己検証 (FIPS 140 mode は有効にできない)、`crypto/internal/constanttime`、`internal/abi` の escape hint、`context` の `cancelCtx.Err` (閉じ済みの channel を待つので `Context.Err` が async になってしまう)。package を host につなぐ patch もあります: `os/signal` は `process.on` で signal を受け、`os.Executable` は Node.js と Bun では script を返し、`net/http` の client は常に `fetch` を使います (js/wasm port は Go 自身のテストのため Node.js 上では process 内の偽 network を使う)。dialer が素の `net.Dialer` のものである Transport (OpenTelemetry の OTLP/HTTP exporter が作る) も、port ならそれで process 内の network に dial しますが、`fetch` を使います。この patch は otelc などの instrumentation が書き換える元の `RoundTrip` を残して呼びます: `//goesm:original name` directive を持つ patch 関数は、置き換える宣言を `_` ではなく `name` に rename します。`unique` は置換しています: 1 つの Go map が正準値を保持し、解放はしません。`math/big` の word 関数は 32-bit word に patch し、`crypto/internal/fips140/bigmod` と `crypto/internal/boring/bbig` は 32-bit limb 版に置換し、`crypto/internal/fips140/nistec` の P-256 の表は alias せずに decode し、`iter` の coroutine は goroutine 上で動かします。速度のため、`fmt.Sprintf` は文字列・bool・整数・float64 の `%v %d %s %t %x %X %f %F` を、1 回だけ解析した書式で整形し (`runtime/src/fmt.ts`)、それ以外のよく使う書式 (文字列・bool・事前宣言型の数値の `%v %d %s %t %x %X %f %e %g`、`-`・`0`・幅・浮動小数点数の精度) を文字列の連結で組み、数値は engine (`toString`・`toFixed`・`toExponential`) で整形し、それ以外 (他の verb やフラグ、名前付き型、引数の過不足) は fmt 自身のコードに任せます。`encoding/json` の `Marshal` と `Unmarshal` は、単純な値 (bool・数値・文字列・slice・配列・key が文字列の map・pointer・`any`、tag が名前と `omitempty` だけの struct。どこにも method がないもの) なら `runtime/src/json.ts` で処理し (`Marshal` は値を JS の値に変換して engine の `JSON.stringify` に渡し、Go と同じく後から HTML の文字を escape する。`Unmarshal` は型ごとに作る decoder で入力を 1 回だけ読む)、それ以外 (marshaler、埋め込み field、他の tag option、decode 先の nil でない slice・map・pointer、エラー) は最初から package 自身のコードに任せます。ASCII 文字列の `strings` の `ToUpper` と `ToLower` は engine の `toUpperCase` と `toLowerCase`、区切りありの `Split` と `Join` は engine の `split` と `join`、`strings.Builder` は文字列の連結で伸ばし (engine 内では rope。`Cap` は `[]byte` だった場合の値)、整数と文字列の `slices.Sort` (つまり `sort.Ints` と `sort.Strings` も) は engine 組み込みの sort です (等しい要素は区別できないため。浮動小数点数は NaN と -0 のため Go の pdqsort のまま)。`internal/strconv` の `FormatFloat` と `AppendFloat` は float64 の桁を engine の `toExponential` と `toFixed` から取り (natives.ts の `ftoaDigits`。engine はちょうど中間の値を切り上げるので、Go と同じく偶数に丸め直す)、`fmt` の `fmtInteger` は 2^53 未満の値の桁を `uint64` ではなく `uint` で計算し、`internal/strconv` の多倍長 decimal は 32-bit platform と同じく 1 回に最大 28 bit ずつ shift します (`uint` 上の 60 bit の shift では桁が失われる)。
* 置換・patch・override・natives は goesm に compile される固定の集合です。適用されるのは `$GOROOT/src` 以下の file だけで、依存 package の内容がこれを増やすことはできません。
* goesm が lower できない third-party 依存の関数は、stdlib の関数と同じく panic する stub になり、警告が出ます。program 自身の package は完全に lower できる必要があります。
* Go package 間の `//go:linkname`: target 付きの body の無い宣言 (compile-time instrumentation が hook を呼ぶために生成する "pull") は、その名前の Go 関数を呼びます (`internal/lower/linkname.go`)。呼び出しは ES module の import ではなく `@goesm/runtime` の symbol table を経由するので、link された package 同士でも Go の初期化順序が保たれます: 提供側の package は変数の初期化後に関数を登録し、それより前の結果を返さない呼び出しはそれまで延期されます。
* `//go:embed` は `string`、`[]byte`、`embed.FS` の変数を package directory の file で、package の他の変数より先に初期化します。
* instrumentation 向けの goroutine-local storage: otelc が追加する `runtime` の関数 (`GetTraceContextFromGLS` など) を program が使うと、各 goroutine が 2 つの値を持ち、新しい goroutine は生成元の値のコピー (otelc の `newproc1` patch と同じく `OtelContextCloner` で clone) で始まり、各 `await` は再開時に実行中の goroutine を戻します (`$rt.resumeG`)。使わない program にはコストがかかりません。

現状 (`go test ./test -run TestStdlibStatus -v`、golden テストは `testdata/semantics/stdlibuse`):

* Go source のまま compile でき native Go と一致: `errors` (`Is`、`As`、`Join`、`Unwrap`)、`strings` (検索、split、fields、大文字小文字、`Builder`、`Replacer`、`EqualFold`)、`strconv` (整数の format、`Atoi`、quote、`NumError`)、`sort`、`slices`、`maps`、`sync`、`unicode`、`unicode/utf8`、`math/bits`、`strconv` の 64-bit parse と最短表現の float format (`testdata/semantics/int64s`)。
* `fmt` (verb、flag、幅と精度、`Stringer` / `error` / `Formatter` / `GoStringer`、`%w` 付き `Errorf`、`Sscanf`)、`reflect`、`encoding/json` (struct tag、embedded、map、`RawMessage`、`Marshaler` / `TextMarshaler`、`Decoder` の stream、`UseNumber`、error) は `testdata/programs/fmtverbs`、`reflection`、`jsoncodec` で native Go と一致します。
* `time` (`Sleep`、`Stop` / `Reset` 付きの `Timer`、`Ticker`、`AfterFunc`、`select` 内の `After`、時計、`Duration`、format と parse) は `testdata/programs/timers` で native Go と一致します。
* `crypto` の hash と暗号 (MD5、SHA-1、SHA-2、SHA-3、HMAC、AES-GCM、CTR)、`crypto/rand` (host の `crypto.getRandomValues`)、`math/rand`、`math/rand/v2`、`hash/maphash`、`sync/atomic.Value`、slice から配列 pointer への変換は `testdata/programs/stdlibmisc` で native Go と一致します。
* `math/big` (`testdata/programs/mathbig`) と公開鍵暗号 (`crypto/rsa`、`crypto/ecdsa`、`crypto/ecdh`、`crypto/ed25519`、`crypto/x509`。`testdata/programs/cryptopk`) は native Go と一致します。`uint` が JS の number なので、goesm では `math/big.Word` は `uint32` (32-bit platform と同じ `_W = 32`)、`crypto/internal/fips140/bigmod` は 32-bit limb を使います。乗算の内側ループと `bits.{Add,Sub,Mul,Div,Rem}32` は native です。P-384、P-521、Ed25519 の 64-bit limb の体演算は BigInt 上で動くので遅いです。
* `iter.Pull` / `Pull2` (`testdata/programs/iterpull`): coroutine を goroutine 上で動かし、`coroswitch` は channel で制御を渡します (patch)。それらの関数リテラルは、`Pull` か `Pull2` を参照する program でだけ blocking 解析上の関数値として数えます。そうした program では `func()`、`func(T) bool`、`func() (T, bool)` の値の動的呼び出しが async になります。

## 10. Tooling compatibility と security

* `.go` file は普通の Go で、goesm 専用 syntax・directive・magic comment はありません。fixture は `go vet` / `go build` / `go run` がそのまま通り、golden テストはまさに native Go 実行と比較しています。package graph は go command が解決したもので、govulncheck 等の call graph も変わりません。
* 依存 package を import しても goesm 側でコードは実行されません。compiler plugin や third-party の extension 機構はありません。esbuild の plugin は `goesm build -split` で使う goesm 自身の resolver だけで、出力した tree には plugin は不要です。stdlib の置換、patch、natives (§9) は goesm 内の固定の集合で、`$GOROOT/src` にだけ適用されます。stdlib 以外で body の無い Go 関数は、`//go:linkname` が program 内の別の Go 関数を指す場合を除いて error であり、goesm への hook にはなりません。`-toolexec` は `go build -toolexec` と同じく、user が指定した program を実行します。
* 懸念点: (1) go/packages は `go list` を実行するので、`GOFLAGS` などの環境、`go.work`、`GOPROXY` からの module 取得について go command と同じ trust 境界を継承します (goesm がそれを広げることはありません)。(2) 生成コードは Go の型安全性に依存しており、goesm の lowering bug は JS 上の memory safety ではなく誤動作として現れます (JS 自体は memory safe)。(3) 生成 ESM は `globalThis.reportError` 等の host API を使いますが、DOM API binding は未実装です。(4) `GoPanic` の message や source map の `sourcesContent` は Go source を含むため、公開 bundle に Go source が載ります (`SourcesContent` を外すオプションは未実装)。

## 11. 実装済み / 未実装 / native Go との差分

**実装済み (native Go との golden テストで確認)**: package import、関数、多値返却、named result、closure、struct (値 copy、method、pointer method、embedding と promotion、複合リテラルの key としての promote された field、比較)、array、slice (aliasing、append、copy、re-slice、nil)、map (struct / interface key、comma-ok、delete、nil map、range)、pointer (変数・field・要素・`new`、identity)、defer (評価順・named result の変更・LIFO)、panic / recover (runtime error、re-panic)、interface (dispatch、type assertion、type switch、比較、nil interface と nil pointer の区別)、generics (generic 関数、制約と制約の method、interface 経由も含む generic type、型引数に従う演算子と変換、Go 1.27 generic methods、型 identity、generic 関数の local type (gc と同じく関数の型 parameter を先頭の暗黙の型引数として取る))、method value / method expression、switch / fallthrough / label 付き break・continue、`goto` (後方への jump は state machine になる)、range over int、range-over-func (入れ子の文からの break / continue / return、label 付き branch、body 内の `defer` (外側の関数の frame に積む) と `goto`、body 内の blocking 操作と select、yield を誤用する iterator に対する Go と同じ panic)、Go 1.22 の per-iteration loop 変数 (それより前の Go version の file では共有)、Go package 間の `//go:linkname`、`//go:embed`、`-toolexec` による compile-time instrumentation、8/16/32-bit 整数の wrap、整数 0 除算 panic、UTF-8 string と rune、goroutine、unbuffered / buffered channel、close、channel の range、select (default 含む)、`runtime.Goexit` / `Gosched`、package 変数の init order と `init()`、§9 に挙げた stdlib package。

**未実装** (goesm 診断になるか、動作しないもの):
* 64-bit の `int` と `uint` の正確な表現 (number のまま、§5 参照)
* §7 を超える `unsafe` と `reflect`
* Go が知らない未完了の処理 (JavaScript の timer、I/O) が host に残っている間の deadlock 検出、goroutine の preemption、goroutine-local な recover 状態
* JS からの呼び出し ABI (Go の値 ⇔ JS 値の自動変換)

**native Go との既知の差分** (最初の 2 項目は `TestKnownGaps` の `IntWrap`・`UintWrap`・`AppendCap` で差分が存在することを固定。残りは決定的に比較できないため文書のみ):
* `int` と `uint` は 2^53 を超えると不正確、64-bit overflow で wrap しない (`uint(0)-1` が `-1`)。`int64` と `uint64` は正確。ただし負の `i` の変換 `uint(i)` を直接大小比較する場合 (bounds check の慣用句 `uint(i) < uint(len(s))`) は wrap した値として比較する。
* 変数宣言を越える後方 `goto` は、Go なら新しい変数を作るところで同じ変数を再利用する。違いが分かるのは jump の前に作った closure だけ。
* `append` の capacity 拡張は近似 (size class の丸めなし)。`cap()` の値が gc と異なることがある。
* map の range 順は挿入順 (Go はランダム)。どちらも仕様上未定義。
* deferred 関数が実行時にしか分からない場合 (関数値、interface の method) は、そこから呼んだ関数の中の `recover()` も効く (Go では直接呼んだときだけ)。
* goroutine は blocking 点でしか切り替わらない (協調的)。blocking する exported 関数は JS からは Promise を返す。
* 動的呼び出しの blocking 判定は保守的 (§5) なので、不要な `await` が入ることがある (意味は変わらない)。
* `print` / `println` は Go ランタイムと同じ書式で stderr に出力するが、ポインタ・map・channel・func・スライス・interface の値は実アドレスではなく固定のアドレスを表示する。
* `sync`: 解析 (§5) が同期のままにした `Lock` は、待つ必要があると panic する。解析はこれを起こさないはずなので、goesm の bug である。最初の呼び出しの関数が block している間に 2 回目の `Once.Do` を呼ぶと、待たずに panic する。unlock 済み `Mutex` の unlock などの誤用は fatal error ではなく recover できる panic。`runtime.Caller` / `Callers` / `Stack` は何も報告せず、`SetFinalizer` は何もしない。
* Bun では JavaScriptCore が NaN の payload を保たないので、NaN の `math.Float64bits` が Go と異なることがある。
* `time`: timer の channel は `GODEBUG=asynctimerchan=1` と同じく buffer が 1 つある (`len(t.C)` が 1 になり得る)。`Stop` と `Reset` は channel を空にするので、Go 1.23 と同じくその後に古い値を受信することはない。`Timer` と `Ticker` には unexported field が 1 つ多く、`%+v` で表示される。timer は host の event loop が処理できたときに発火するので、走り続ける goroutine があると遅れる。

## 12. 次に実装すべき 3 項目

1. **goroutine runtime の完成**: goroutine-local な panic / recover 状態 (async 境界を跨ぐ recover)、JS 呼び出し ABI (exported 関数の引数・戻り値の変換)。
2. **64-bit 演算の速度**: `int64`/`uint64` は BigInt なので、64-bit limb の上に作られたコード (P-384 / P-521 の体演算、`crypto/ed25519` など) は native よりかなり遅い。goesm で `math/big` と `crypto/internal/fips140/bigmod` が使う 32-bit word のように、重いものから 32-bit や number ベースの経路にする。
3. **bundle size**: `unicode` の表のような package-level の表は eager に構築され、tree shaking で落ちません (`fmt` の hello world は gzip で約 210 KB)。
