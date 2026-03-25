#!/bin/sh
set -eu

if [ -z "${RENEWED_LINEAGE:-}" ]; then
  printf 'RENEWED_LINEAGE is required\n' >&2
  exit 1
fi

cp "${RENEWED_LINEAGE}/fullchain.pem" /app/cert.pem
cp "${RENEWED_LINEAGE}/privkey.pem" /app/key.pem
chmod 600 /app/cert.pem /app/key.pem

kill -TERM 1
