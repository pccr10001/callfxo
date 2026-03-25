#!/bin/sh
set -eu

if [ ! -f /app/certbot.env ]; then
  exit 0
fi

. /app/certbot.env

if [ -z "${TLS_HOST:-}" ]; then
  exit 0
fi

certbot renew \
  --standalone \
  --preferred-challenges http \
  --http-01-port 80 \
  --deploy-hook /opt/coturn/on-cert-renew.sh
