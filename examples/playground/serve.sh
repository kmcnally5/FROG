#!/bin/bash
# serve.sh — build the kLex Playground WASM bundle and serve it.
#
# Usage: from examples/playground/, run ./serve.sh
# Open:  http://localhost:8768/
#
# To use a pre-built binary instead of rebuilding:
#   SKIP_BUILD=1 ./serve.sh

set -e
cd "$(dirname "$0")"
REPO_ROOT="$(cd ../.. && pwd)"

if [ -z "$SKIP_BUILD" ]; then
    echo "==> regenerating embedded stdlib..."
    (cd "$REPO_ROOT" && go run ./tools/stdlibgen)

    echo "==> regenerating VM builtin table..."
    if [ ! -x "$REPO_ROOT/bin/vmbuiltins" ]; then
        echo "    bin/vmbuiltins missing — building it..."
        (cd "$REPO_ROOT" && go build -o bin/vmbuiltins ./vm/cmd/vmbuiltins)
    fi
    (cd "$REPO_ROOT" && bin/vmbuiltins)

    echo "==> building klex.wasm..."
    (cd "$REPO_ROOT" && GOOS=js GOARCH=wasm CGO_ENABLED=0 go build -o bin/klex.wasm ./cmd/wasm)
fi

if [ ! -f "$REPO_ROOT/bin/klex.wasm" ]; then
    echo "error: bin/klex.wasm not found — run without SKIP_BUILD to build it"
    exit 1
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

echo "==> starting kLex Playground — http://localhost:8768/"
exec env KLEX_PATH="$REPO_ROOT" "$KLEX" serve.lex
