#!/bin/sh
set -eu

project_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$project_root"
command -v node >/dev/null 2>&1 || { echo 'Node.js is required.' >&2; exit 1; }
exec node scripts/start.mjs
