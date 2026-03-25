#!/bin/sh
set -eu

APP_DIR="/app"
CONF_FILE="${APP_DIR}/turnserver.conf"
CERT_FILE="${APP_DIR}/cert.pem"
KEY_FILE="${APP_DIR}/key.pem"
CERTBOT_ENV_FILE="${APP_DIR}/certbot.env"
CRON_FILE="/etc/crontabs/root"

log() {
  printf '%s\n' "$*"
}

require_env() {
  var_name="$1"
  eval "var_value=\${${var_name}:-}"
  if [ -z "${var_value}" ]; then
    printf 'missing required env: %s\n' "${var_name}" >&2
    exit 1
  fi
}

is_true() {
  case "$(printf '%s' "${1:-}" | tr '[:upper:]' '[:lower:]')" in
    1|true|yes|on)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

write_certbot_env() {
  cat > "${CERTBOT_ENV_FILE}" <<EOF
TLS_HOST=${TLS_HOST:-}
CERTBOT_EMAIL=${CERTBOT_EMAIL:-}
EOF
}

copy_live_cert() {
  live_dir="/etc/letsencrypt/live/${TLS_HOST}"
  if [ ! -f "${live_dir}/fullchain.pem" ] || [ ! -f "${live_dir}/privkey.pem" ]; then
    return 1
  fi

  cp "${live_dir}/fullchain.pem" "${CERT_FILE}"
  cp "${live_dir}/privkey.pem" "${KEY_FILE}"
  chmod 600 "${CERT_FILE}" "${KEY_FILE}"
}

obtain_initial_cert() {
  if [ -n "${CERTBOT_EMAIL:-}" ]; then
    certbot certonly \
      --standalone \
      --preferred-challenges http \
      --http-01-port 80 \
      --non-interactive \
      --agree-tos \
      --keep-until-expiring \
      -m "${CERTBOT_EMAIL}" \
      -d "${TLS_HOST}"
  else
    certbot certonly \
      --standalone \
      --preferred-challenges http \
      --http-01-port 80 \
      --non-interactive \
      --agree-tos \
      --register-unsafely-without-email \
      --keep-until-expiring \
      -d "${TLS_HOST}"
  fi
  copy_live_cert
}

setup_certbot_cron() {
  write_certbot_env
  schedule="${CERTBOT_RENEW_SCHEDULE:-17 3 * * *}"
  cat > "${CRON_FILE}" <<EOF
SHELL=/bin/sh
PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
${schedule} /opt/coturn/certbot-renew.sh >> /var/log/certbot-renew.log 2>&1
EOF
  chmod 0644 "${CRON_FILE}"
  touch /var/log/certbot-renew.log
}

write_config() {
  cat > "${CONF_FILE}" <<EOF
fingerprint
use-auth-secret
static-auth-secret=${SHARED_SECRET}
realm=${REALM}
server-name=${REALM}
external-ip=${EXTERNAL_IP}
listening-ip=0.0.0.0
listening-port=${EXTERNAL_PORT}
lt-cred-mech
min-port=${RELAY_MIN_PORT}
max-port=${RELAY_MAX_PORT}
simple-log
no-cli
EOF

  if [ "${TCP_PORT}" != "${EXTERNAL_PORT}" ]; then
    cat >> "${CONF_FILE}" <<EOF
# Plain TCP is exposed by docker-compose host port ${TCP_PORT} -> container port ${EXTERNAL_PORT}.
EOF
  fi

  if [ -n "${TLS_HOST:-}" ] && [ -n "${TLS_PORT:-}" ]; then
    cat >> "${CONF_FILE}" <<EOF
cert=${CERT_FILE}
pkey=${KEY_FILE}
tls-listening-port=${TLS_PORT}
EOF
  fi
}

stop_all() {
  if [ -n "${TURN_PID:-}" ]; then
    kill -TERM "${TURN_PID}" 2>/dev/null || true
  fi
  if [ -n "${CRON_PID:-}" ]; then
    kill -TERM "${CRON_PID}" 2>/dev/null || true
  fi
  wait "${TURN_PID:-}" 2>/dev/null || true
  wait "${CRON_PID:-}" 2>/dev/null || true
  exit 0
}

require_env SHARED_SECRET
require_env EXTERNAL_IP
require_env EXTERNAL_PORT
require_env TCP_PORT
require_env REALM

TLS_ENABLED=0
if [ -n "${TLS_HOST:-}" ] && [ -n "${TLS_PORT:-}" ]; then
  TLS_ENABLED=1
fi

if [ "${TLS_ENABLED}" -eq 1 ]; then
  if is_true "${CERTBOT_ENABLED:-false}"; then
    if ! copy_live_cert; then
      log "No existing certificate found for ${TLS_HOST}, requesting one with certbot."
      obtain_initial_cert
    fi
    setup_certbot_cron
  fi

  if [ ! -f "${CERT_FILE}" ] || [ ! -f "${KEY_FILE}" ]; then
    printf 'TLS is enabled but %s or %s is missing\n' "${CERT_FILE}" "${KEY_FILE}" >&2
    exit 1
  fi
fi

write_config

trap stop_all INT TERM HUP

if [ "${TLS_ENABLED}" -eq 1 ] && is_true "${CERTBOT_ENABLED:-false}"; then
  crond -f -l 2 &
  CRON_PID=$!
fi

turnserver -c "${CONF_FILE}" &
TURN_PID=$!

wait "${TURN_PID}"
exit_code=$?

if [ -n "${CRON_PID:-}" ]; then
  kill -TERM "${CRON_PID}" 2>/dev/null || true
  wait "${CRON_PID}" 2>/dev/null || true
fi

exit "${exit_code}"
