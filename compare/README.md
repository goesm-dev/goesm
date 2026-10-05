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
| [Tailwind CSS](https://tailwindcss.com/) 4.3.3 | [`utility`](utility/utility.go), a utility-first CSS generator written in Go | build the stylesheet for the 154 class names of a landing page ([js/page.html](js/page.html)) from Tailwind's default theme; the Go side's CSS must be byte for byte Tailwind's |

For a framework, the comparison takes the part a page depends on most: rendering for React, the Markdown renderer for VitePress and Astro, the reactivity system for Vue, and for Tailwind the compiler that turns a page's class names into CSS. The Go side is ordinary Go code, as one would write it for a native program: `fmt.Errorf` with `%w`, `time.Parse`, the generated protobuf types, `html/template`. Go has no reactivity system to compare, so `reactive` is one written for this comparison, in the style of Vue's: like Vue, a computation that reads the same sources as on its last run keeps its subscriptions. `utility` is likewise written for this comparison. It reads the theme's CSS and builds the utilities from it the way Tailwind does, and takes Tailwind's static utilities, static variants and property order from tables that [js/gen-utility.mjs](js/gen-utility.mjs) generates from the tailwindcss package. It covers the utilities a typical page uses and the variants that do not take a value; `group-*`, `peer-*`, `not-*`, `max-*` and the other compound or functional variants are left out. The JavaScript side is the library's usual API. The Markdown renderers escape a few characters differently, so their HTML is compared with those escapes undone. [js/libs.mjs](js/libs.mjs) holds the inputs and the workloads, and [js/impl/](js/impl) the JavaScript versions.

## Results

<!-- compare:start -->
| Library | Go package | goesm gzip | JS gzip | ratio |
| --- | --- | --- | --- | --- |
| luxon | `datetime` | 25.2 KiB | 21.7 KiB | 1.17× |
| neverthrow | `result` | 102.8 KiB | 2.4 KiB | 43.03× |
| connect-es | `rpc` | 1312.5 KiB | 32.9 KiB | 39.95× |
| react | `render` | 336.1 KiB | 64.3 KiB | 5.22× |
| vitepress | `markdown` | 241.9 KiB | 40.4 KiB | 5.99× |
| astro | `markdown` | 241.9 KiB | 47.2 KiB | 5.12× |
| vue | `reactive` | 11.4 KiB | 5.3 KiB | 2.16× |
| tailwind | `utility` | 91.8 KiB | 72.2 KiB | 1.27× |

| Library | node 26.10.0 goesm | node 26.10.0 JS | ratio | bun 1.4.2 goesm | bun 1.4.2 JS | ratio |
| --- | --- | --- | --- | --- | --- | --- |
| luxon | 2.2 ms | 8.5 ms | 0.25× | 4.5 ms | 8.0 ms | 0.57× |
| neverthrow | 0.88 ms | 0.52 ms | 1.68× | 1.2 ms | 0.50 ms | 2.47× |
| connect-es | 20 ms | 2.5 ms | 8.22× | 20 ms | 2.0 ms | 10.36× |
| react | 2.7 ms | 1.7 ms | 1.58× | 3.2 ms | 1.9 ms | 1.66× |
| vitepress | 0.46 ms | 0.24 ms | 1.94× | 0.61 ms | 0.14 ms | 4.33× |
| astro | 0.38 ms | 2.0 ms | 0.19× | 0.59 ms | 2.6 ms | 0.23× |
| vue | 14 ms | 6.1 ms | 2.31× | 15 ms | 5.6 ms | 2.72× |
| tailwind | 2.4 ms | 4.4 ms | 0.55× | 3.2 ms | 3.5 ms | 0.90× |
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
