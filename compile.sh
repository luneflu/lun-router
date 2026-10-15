#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

echo "==> Building Svelte frontend (SPA, no SSR)..."
if command -v pnpm &> /dev/null; then
    pnpm build
else
    npm run build
fi

echo "==> Compiling Go server with embedded frontend..."
go build -o lun-router .

echo "==> Done. Run ./lun-router to start server."
