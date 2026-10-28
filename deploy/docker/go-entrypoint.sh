#!/bin/sh
set -eu
# Only TLS requires mounted files. Copy into private tmpfs before dropping root;
# host certificate/key permissions remain unchanged.
if [ -d /run/secrets ]; then
  mkdir -p /run/spx-certs
  chmod 700 /run/spx-certs
  for source in /run/secrets/*; do
    [ -f "$source" ] || continue
    cp "$source" "/run/spx-certs/$(basename "$source")"
  done
  chown -R spx:spx /run/spx-certs
  chmod 400 /run/spx-certs/*
  for variable in SPX_IAM_TLS_CERT_FILE SPX_IAM_TLS_KEY_FILE SPX_CONSOLE_IAM_CA_FILE; do
    value=$(printenv "$variable" || true)
    case "$value" in /run/secrets/*) export "$variable=/run/spx-certs/$(basename "$value")" ;; esac
  done
fi
exec su-exec spx /app/service "$@"
