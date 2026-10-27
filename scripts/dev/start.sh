#!/bin/sh
set -eu

project_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$project_root"

command -v node >/dev/null 2>&1 || { echo 'Node.js is required.' >&2; exit 1; }
command -v pnpm >/dev/null 2>&1 || { echo 'pnpm is required.' >&2; exit 1; }

pnpm install --frozen-lockfile
printf '%s\n' \
  'Starting SecPlexus development environment: http://127.0.0.1:5173' \
  'Web uses React Fast Refresh. Ctrl+C stops the development server.' \
  'IAM and Console are not implemented yet; the template uses development-only mock data.'
exec pnpm dev
