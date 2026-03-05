#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
NODEJS_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
TSX_LOADER="$NODEJS_DIR/node_modules/tsx/dist/loader.mjs"
export NODE_PATH="$NODEJS_DIR/node_modules"
exec node --import "$TSX_LOADER" "$SCRIPT_DIR/amqp-tck-adapter.ts" "$@"
