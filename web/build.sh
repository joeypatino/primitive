#!/bin/sh
# Builds the web demo into web/: the WebAssembly module, Go's JS glue, and
# the sample images. Serve the directory with any static server, e.g.
#   ./web/build.sh && python3 -m http.server -d web 8000
set -eu
cd "$(dirname "$0")/.."

GOOS=js GOARCH=wasm go build -ldflags="-s -w" -o web/primitive.wasm ./web/wasm
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" web/wasm_exec.js

mkdir -p web/samples
for f in examples/*.png; do
	name=$(basename "$f" .png)
	if command -v sips >/dev/null; then # macOS: shrink the large examples
		sips -Z 900 -s format jpeg -s formatOptions 85 "$f" --out "web/samples/$name.jpg" >/dev/null
	else
		convert "$f" -resize 900x900 -quality 85 "web/samples/$name.jpg"
	fi
done
