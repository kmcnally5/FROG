#!/bin/bash
# serve.sh — serve the UI widgets WASM test.
#
# Usage: from tests/wasm_ui/, run ./serve.sh
# Open:  http://localhost:8767/
#
# Requires a pre-built klex.wasm in bin/. Build it once:
#   cd examples/playground && ./serve.sh   (Ctrl-C after the build completes)
#
# To point at a different binary:
#   KLEX_WASM=/path/to/klex.wasm ./serve.sh

set -e
cd "$(dirname "$0")"
REPO_ROOT="$(cd ../.. && pwd)"

KLEX_WASM="${KLEX_WASM:-$REPO_ROOT/bin/klex.wasm}"
if [ ! -f "$KLEX_WASM" ]; then
    echo "error: klex.wasm not found at $KLEX_WASM"
    echo "  Build it first:  cd examples/playground && ./serve.sh"
    echo "  Or set:          KLEX_WASM=/path/to/klex.wasm ./serve.sh"
    exit 1
fi

if [ "$KLEX_WASM" != "$REPO_ROOT/bin/klex.wasm" ]; then
    cp "$KLEX_WASM" "$REPO_ROOT/bin/klex.wasm"
fi

KLEX="$REPO_ROOT/klex"
if [ ! -x "$KLEX" ]; then
    if command -v klex >/dev/null 2>&1; then
        KLEX="$(command -v klex)"
    else
        echo "==> klex binary missing — building it once at $KLEX ..."
        (cd "$REPO_ROOT" && go build -o klex .)
    fi
fi

echo "==> starting kLex UI test — http://localhost:8767/"
exec env KLEX_PATH="$REPO_ROOT" "$KLEX" serve.lex
