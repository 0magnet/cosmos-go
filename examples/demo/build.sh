#!/bin/sh
# Build the demo with the standard Go toolchain or TinyGo.
#   ./build.sh        - standard Go
#   ./build.sh tinygo - TinyGo (much smaller binary)
set -e
cd "$(dirname "$0")"

if [ "$1" = "tinygo" ]; then
    # TinyGo trails each new Go release by some weeks. The helper reports the
    # newest Go that this TinyGo accepts, or "auto" when the system one is fine.
    GOTOOLCHAIN=$(sh ../../scripts/tinygo-toolchain.sh) \
        tinygo build -o demo.wasm -target wasm -no-debug .
    cp "$(tinygo env TINYGOROOT)/targets/wasm_exec.js" .
else
    GOOS=js GOARCH=wasm go build -o demo.wasm .
    cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" .
fi

echo "built demo.wasm ($(du -h demo.wasm | cut -f1))"
echo "serve with e.g.: python3 -m http.server 8080"
