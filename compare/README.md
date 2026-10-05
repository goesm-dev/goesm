# Library comparison

[日本語](README.ja.md)

What does it cost to write in Go, with goesm, what a web app would otherwise get from a popular JavaScript library? Each comparison here does the same job twice: once in a Go package compiled by goesm, and once in JavaScript with the library. Both sides are bundled the same way, and both are timed on the same workload, called from JavaScript. Like the JavaScript side, which imports what its workload calls, the Go side's bundle exports only the functions the workload calls, so that the bundler drops the rest, as it would in an app. The results of the two sides must be identical, or the run fails.

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

For a framework, the comparison takes the part a page depends on most: rendering for React, the Markdown renderer for VitePress and Astro, the reactivity system for Vue, and for Tailwind the compiler that turns a page's class names into CSS. The Go side is ordinary Go code, as one would write it for a native program: `fmt.Errorf` with `%w`, `time.Parse`, the generated protobuf types, `html/template`. Go has no reactivity system to compare, so `reactive` is one written for this comparison with the algorithm of Vue 3.5's: each computation's sources and each source's subscribers are doubly linked lists of the same links, with version numbers, so that a computation that reads the same sources as on its last run reuses its links, and a computed value whose sources did not change is not recomputed. `utility` is likewise written for this comparison. It reads the theme's CSS and builds the utilities from it the way Tailwind does, and takes Tailwind's static utilities, static variants and property order from tables that [js/gen-utility.mjs](js/gen-utility.mjs) generates from the tailwindcss package. It covers the utilities a typical page uses and the variants that do not take a value; `group-*`, `peer-*`, `not-*`, `max-*` and the other compound or functional variants are left out. The JavaScript side is the library's usual API. The Markdown renderers escape a few characters differently, so their HTML is compared with those escapes undone. [js/libs.mjs](js/libs.mjs) holds the inputs and the workloads, and [js/impl/](js/impl) the JavaScript versions.

Three comparisons also have a second Go package under [light/](light), shown as "(light)": the same job written with a bundle in mind, without the standard library packages that make the first package large. [`light/result`](light/result/result.go) builds its messages in an error type of its own instead of with `fmt.Errorf`, which formats any value through `reflect`. [`light/rpc`](light/rpc/rpc.go) encodes and decodes the two messages by hand and calls `fetch` through `syscall/js`, instead of using connect-go, the protobuf runtime and `net/http`. [`light/render`](light/render/render.go) writes the page with one function per component into a `strings.Builder`, escaping with `html.EscapeString`, as templ-generated code does, instead of interpreting a template with `html/template`. Their results, too, must be identical to the library's.

## Results

<!-- compare:start -->
| Library | Go package | goesm gzip | JS gzip | ratio |
| --- | --- | --- | --- | --- |
| luxon | `datetime` | 20.7 KiB | 21.7 KiB | 0.96× |
| neverthrow | `result` | 48.9 KiB | 2.4 KiB | 20.48× |
| neverthrow (light) | `light/result` | 20.0 KiB | 2.4 KiB | 8.39× |
| connect-es | `rpc` | 1239.0 KiB | 32.9 KiB | 37.71× |
| connect-es (light) | `light/rpc` | 14.0 KiB | 32.9 KiB | 0.42× |
| react | `render` | 313.8 KiB | 64.3 KiB | 4.88× |
| react (light) | `light/render` | 15.1 KiB | 64.3 KiB | 0.23× |
| vitepress | `markdown` | 162.8 KiB | 40.4 KiB | 4.03× |
| astro | `markdown` | 162.8 KiB | 47.2 KiB | 3.45× |
| vue | `reactive` | 6.4 KiB | 5.3 KiB | 1.22× |
| tailwind | `utility` | 69.4 KiB | 72.2 KiB | 0.96× |

| Library | node 26.10.0 goesm | node 26.10.0 JS | ratio | bun 1.4.2 goesm | bun 1.4.2 JS | ratio |
| --- | --- | --- | --- | --- | --- | --- |
| luxon | 2.4 ms | 8.9 ms | 0.27× | 4.5 ms | 8.0 ms | 0.57× |
| neverthrow | 0.58 ms | 0.50 ms | 1.17× | 0.69 ms | 0.58 ms | 1.20× |
| neverthrow (light) | 0.57 ms | 0.52 ms | 1.09× | 0.76 ms | 0.57 ms | 1.35× |
| connect-es | 20 ms | 2.7 ms | 7.64× | 23 ms | 2.1 ms | 10.97× |
| connect-es (light) | 1.8 ms | 2.6 ms | 0.70× | 1.4 ms | 2.0 ms | 0.71× |
| react | 3.0 ms | 1.7 ms | 1.74× | 3.2 ms | 2.1 ms | 1.56× |
| react (light) | 0.20 ms | 1.7 ms | 0.12× | 0.23 ms | 2.0 ms | 0.12× |
| vitepress | 0.41 ms | 0.23 ms | 1.79× | 0.66 ms | 0.15 ms | 4.46× |
| astro | 0.38 ms | 2.0 ms | 0.19× | 0.56 ms | 2.7 ms | 0.21× |
| vue | 6.2 ms | 6.5 ms | 0.96× | 6.4 ms | 5.8 ms | 1.11× |
| tailwind | 2.5 ms | 4.5 ms | 0.56× | 3.5 ms | 3.7 ms | 0.94× |
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

`GOESM=/path/to/goesm node build.mjs` uses another goesm binary, and `node build.mjs luxon` or `node run.mjs -quick luxon` limit a run to some comparisons. Run the timings with nothing else busy on the machine, and without `BUN_OPTIONS=--smol`, which changes how Bun collects garbage. The bundles are made with esbuild, minified, as browser ES modules; the Go side's few `node:` imports are left external.

The Go and TypeScript code of [proto/](proto) is generated with `buf generate` ([buf.gen.yaml](buf.gen.yaml) lists the plugins).
