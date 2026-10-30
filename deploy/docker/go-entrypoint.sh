#!/bin/sh
set -eu
exec su-exec spx /app/service "$@"
