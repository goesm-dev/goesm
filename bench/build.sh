#!/bin/sh
# Builds the kernels with each compiler into out/ (see README.md):
#
#   out/goesm/kernels.js      goesm build -minify ./kernels (this checkout's goesm)
#   out/gopherjs/bench.js     GopherJS -m ./jsmain
#   out/gowasm/bench.wasm     GOOS=js GOARCH=wasm go build ./jsmain (+ wasm_exec.js)
#   out/tinygo/bench.wasm     tinygo build -target=wasm -opt=2 ./jsmain (+ wasm_exec.js)
#
# Needs the tools of mise.toml and ../mise.toml on PATH (mise install; mise
# exec -- sh build.sh). GopherJS is installed into out/bin by `go install`.
set -eu
cd "$(dirname "$0")"

GOPHERJS_VERSION=v1.21.0
GOPHERJS_GO=go1.21.13 # the Go GopherJS 1.21 compiles against

mkdir -p out/bin out/goesm out/gopherjs out/gowasm out/tinygo
out=$(pwd)/out

echo "goesm: $(cd .. && git describe --always --dirty)"
(cd .. && go build -o "$out/bin/goesm" ./cmd/goesm)
out/bin/goesm build -minify -o out/goesm ./kernels

echo "GopherJS $GOPHERJS_VERSION"
if [ ! -x out/bin/gopherjs ] || ! out/bin/gopherjs version 2>/dev/null | grep -q "${GOPHERJS_VERSION#v}"; then
	GOTOOLCHAIN=$GOPHERJS_GO GOBIN="$out/bin" go install "github.com/gopherjs/gopherjs@$GOPHERJS_VERSION"
fi
GOTOOLCHAIN=$GOPHERJS_GO GOFLAGS=-modfile=gopherjs.mod \
	GOPHERJS_GOROOT="$(GOTOOLCHAIN=$GOPHERJS_GO go env GOROOT)" \
	out/bin/gopherjs build -m -o out/gopherjs/bench.js ./jsmain

echo "$(go version)"
GOOS=js GOARCH=wasm go build -trimpath -ldflags=-s -o out/gowasm/bench.wasm ./jsmain
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" out/gowasm/

echo "$(tinygo version)"
tinygo build -target=wasm -opt=2 -no-debug -o out/tinygo/bench.wasm ./jsmain
cp "$(tinygo env TINYGOROOT)/targets/wasm_exec.js" out/tinygo/
