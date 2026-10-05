# Library comparison

[日本語](README.ja.md)

What does it cost to write in Go, with goesm, what a web app would otherwise get from a popular JavaScript library? Each comparison here does the same job twice: once in a Go package compiled by goesm, and once in JavaScript with the library. Both sides are bundled the same way, and both are timed on the same workload, called from JavaScript. The results of the two sides must be identical, or the run fails.

| Library | Go package | What both sides do |
| --- | --- | --- |
| [luxon](https://moment.github.io/luxon/) 3.7.2 | [`datetime`](datetime/datetime.go) with `time` | parse RFC 3339 timestamps with their offsets, shift them by calendar days, count the days between two, format a label, lay out the 42 days of a calendar month |
| [neverthrow](https://github.com/supermacro/neverthrow) 8.2.0 | [`result`](result/result.go) with `errors`, `fmt`, `strconv` | validate 500 order lines, each step returning an error wrapped with its context, and total the valid ones |
| [connect-es](https://connectrpc.com/docs/web/) 2.2.0 | [`rpc`](rpc/rpc.go) with connect-go and protobuf-go | 50 unary calls of a Connect service in the binary protobuf encoding, over `fetch` |
| [React](https://react.dev/) 19.3.0 | [`render`](render/render.go) with `html/template` | render a page of 100 products to HTML: React's `renderToString`, Go's templates |
| [VitePress](https://vitepress.dev/) | [`markdown`](markdown/markdown.go) with [goldmark](https://github.com/yuin/goldmark) | render a CommonMark document to HTML with markdown-it 15.0.2, VitePress's renderer |
| [Astro](https://astro.build/) | the same `markdown` | the same document with remark and rehype (unified 11), Astro's renderer, built for Node.js where Astro renders Markdown |
| [Vue](https://vuejs.org/) | [`reactive`](reactive/reactive.go), refs, computed values and effects written in Go | build 50 chains of 20 computed values, a diamond and an effect over them, then change the refs 2,000 times, against @vue/reactivity 3.5.43 |

For a framework, the comparison takes the part a page depends on most: rendering for React, the Markdown renderer for VitePress and Astro, the reactivity system for Vue. The Go side is ordinary Go code, as one would write it for a native program: `fmt.Errorf` with `%w`, `time.Parse`, the generated protobuf types, `html/template`. Go has no reactivity system to compare, so `reactive` is one written for this comparison, in the style of Vue's: like Vue, a computation that reads the same sources as on its last run keeps its subscriptions. The JavaScript side is the library's usual API. The Markdown renderers escape a few characters differently, so their HTML is compared with those escapes undone. [js/libs.mjs](js/libs.mjs) holds the inputs and the workloads, and [js/impl/](js/impl) the JavaScript versions.

## Results

<!-- compare:start -->
| Library | Go package | goesm gzip | JS gzip | ratio |
| --- | --- | --- | --- | --- |
| luxon | `datetime` | 25.2 KiB | 21.7 KiB | 1.17× |
| neverthrow | `result` | 102.8 KiB | 2.4 KiB | 43.03× |
| connect-es | `rpc` | 1313.7 KiB | 32.9 KiB | 39.98× |
| react | `render` | 336.1 KiB | 64.3 KiB | 5.22× |
| vitepress | `markdown` | 241.9 KiB | 40.4 KiB | 5.99× |
| astro | `markdown` | 241.9 KiB | 47.2 KiB | 5.12× |
| vue | `reactive` | 11.4 KiB | 5.3 KiB | 2.16× |

| Library | node 26.10.0 goesm | node 26.10.0 JS | ratio | bun 1.4.2 goesm | bun 1.4.2 JS | ratio |
| --- | --- | --- | --- | --- | --- | --- |
| luxon | 2.4 ms | 9.3 ms | 0.26× | 4.6 ms | 8.2 ms | 0.56× |
| neverthrow | 0.91 ms | 0.53 ms | 1.73× | 1.3 ms | 0.49 ms | 2.59× |
| connect-es | 21 ms | 2.7 ms | 7.77× | 23 ms | 2.0 ms | 11.39× |
| react | 2.9 ms | 1.7 ms | 1.68× | 3.3 ms | 2.0 ms | 1.65× |
| vitepress | 0.42 ms | 0.25 ms | 1.68× | 0.66 ms | 0.16 ms | 4.12× |
| astro | 0.39 ms | 2.0 ms | 0.19× | 0.58 ms | 2.6 ms | 0.22× |
| vue | 16 ms | 6.3 ms | 2.51× | 15 ms | 5.6 ms | 2.73× |
<!-- compare:end -->

Sizes are of the minified ES module bundle, compressed with gzip at level 9. Times are the median of one workload run, after a warmup. The connect-es workload answers `fetch` in-process with encoded responses, so it times the client alone: encoding the request, the protocol and decoding the response.

## Running

```sh
cd compare/js
npm ci
node build.mjs                      # builds goesm from this checkout, then both sides of each comparison into ../out
node run.mjs > ../results/node.jsonl
bun run.mjs > ../results/bun.jsonl
node report.mjs ../results/node.jsonl ../results/bun.jsonl   # updates the tables above
```

`GOESM=/path/to/goesm node build.mjs` uses another goesm binary, and `node build.mjs luxon` or `node run.mjs -quick luxon` limit a run to some comparisons. Run the timings with nothing else busy on the machine. The bundles are made with esbuild, minified, as browser ES modules; the Go side's few `node:` imports are left external.

The Go and TypeScript code of [proto/](proto) is generated with `buf generate` ([buf.gen.yaml](buf.gen.yaml) lists the plugins).
