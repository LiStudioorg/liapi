#!/usr/bin/env bash
# Build the Nuxt admin SPA into server/adminui so the Go binary can embed it.
# The generated assets ARE committed (CI/Docker build Go without Node), so run
# this whenever the frontend changes and commit the result.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
web="$here/web"

if [ ! -d "$web/node_modules" ]; then
  echo "==> installing frontend deps (npm ci)"
  (cd "$web" && npm ci)
fi

echo "==> building admin SPA (nuxt generate)"
(cd "$web" && npm run build)

echo "==> done. Output embedded from server/adminui/"
echo "    Remember to commit server/adminui/ changes."
