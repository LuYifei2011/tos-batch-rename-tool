#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
export GOTOOLCHAIN=local
python3 scripts/toolchain.py
mkdir -p build
go build -o build/batch-rename ./server
exec ./build/batch-rename -mode demo -data .local/data -web dist
