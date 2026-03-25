# coturn for CallFXO

This folder contains a standalone `coturn` container setup for CallFXO.

It is intended for running TURN/TURNS on the same public host as CallFXO, while still keeping TURN as a separate service.

## Files

- `Dockerfile`: Alpine-based coturn image with `certbot`
- `docker-compose.yml`: single-container compose for coturn
- `entrypoint.sh`: generates `turnserver.conf` from environment variables and starts coturn
- `certbot-renew.sh`: cron job target for certificate renewal
- `on-cert-renew.sh`: copies renewed certs into `/app` and restarts the container

## What It Does

At startup, the container:

1. Reads these env vars:
   - `SHARED_SECRET`
   - `EXTERNAL_IP`
   - `EXTERNAL_PORT`
   - `TLS_PORT`
   - `TCP_PORT`
   - `REALM`
   - `TLS_HOST`
   - `CERTBOT_ENABLED`
   - `CERTBOT_EMAIL`
   - `CERTBOT_RENEW_SCHEDULE`
   - `RELAY_MIN_PORT`
   - `RELAY_MAX_PORT`
2. Generates `/app/turnserver.conf`
3. If both `TLS_HOST` and `TLS_PORT` are set:
   - reads `/app/cert.pem` and `/app/key.pem`
   - or, if `CERTBOT_ENABLED=true`, obtains certs with `certbot --standalone`
4. Starts `turnserver`
5. If certbot is enabled, starts `crond` for renewal

Renewed certs are copied into `/app/cert.pem` and `/app/key.pem`, then the container exits so Docker Compose restarts it.

## Ports

The compose file exposes:

- `80/tcp`: used by certbot HTTP-01 challenge
- `${EXTERNAL_PORT}/udp`: TURN over UDP
- `${TCP_PORT}/tcp`: TURN over TCP
- `${TLS_PORT}/tcp`: TURNS over TLS
- `${RELAY_MIN_PORT}-${RELAY_MAX_PORT}/udp`: TURN relay media range

## Volumes

The compose file persists:

- `/app`: generated config and active cert/key copy
- `/etc/letsencrypt`: certbot certificates
- `/var/lib/letsencrypt`: certbot working state

This allows certificates to survive container recreation.

## Example `.env`

Create `coturn/.env`:

```env
SHARED_SECRET=replace-with-long-random-secret
EXTERNAL_IP=203.0.113.10
EXTERNAL_PORT=53478
TCP_PORT=53478
TLS_PORT=55349
REALM=callfxo.local
RELAY_MIN_PORT=59160
RELAY_MAX_PORT=59200

TLS_HOST=turn.example.com
CERTBOT_ENABLED=true
CERTBOT_EMAIL=admin@example.com
CERTBOT_RENEW_SCHEDULE=17 3 * * *
```

## Start

From the `coturn` folder:

```bash
docker compose up -d --build
```

## TLS Notes

- If `TLS_HOST` and `TLS_PORT` are both set, TURNS is enabled.
- If `CERTBOT_ENABLED=true`, the container will try to obtain a certificate automatically using HTTP-01.
- Port `80/tcp` must be reachable from the internet for certbot HTTP validation.
- If you do not want automatic certbot issuance, place these files into the `/app` volume before startup:

```text
/app/cert.pem
/app/key.pem
```

## coturn Auth Mode

This setup uses coturn shared-secret auth:

- `use-auth-secret`
- `static-auth-secret=${SHARED_SECRET}`

That means CallFXO should use the same shared secret and generate short-lived TURN credentials dynamically.

## CallFXO Config

In the main CallFXO `config.yaml`, point ICE TURN settings at this coturn instance:

```yaml
media:
  ice_stun_urls:
    - "stun:turn.example.com:53478"
  ice_turn_urls:
    - "turn:turn.example.com:53478?transport=udp"
    - "turn:turn.example.com:53478?transport=tcp"
    - "turns:turn.example.com:55349?transport=tcp"
  ice_turn_shared_secret: "replace-with-long-random-secret"
  ice_turn_credential_ttl_minutes: 60
```

If you are not using shared-secret auth, you can instead set:

```yaml
media:
  ice_turn_username: "user"
  ice_turn_credential: "password"
```

## Troubleshooting

- If `turnserver.conf` appears correct but coturn ignores it, make sure coturn is started without `-n`.
- If TURNS does not work, confirm the client is using a `turns:` URL, not just `turn:`.
- To verify the TLS listener directly:

```bash
openssl s_client -connect turn.example.com:55349 -servername turn.example.com
```

- If certbot renewal is enabled, `CERTBOT_RENEW_SCHEDULE` must not start with `-`.
  Correct:

```env
CERTBOT_RENEW_SCHEDULE=17 3 * * *
```

## Security Notes

- Use a long random `SHARED_SECRET`
- Restrict relay UDP range with `RELAY_MIN_PORT` / `RELAY_MAX_PORT`
- Expose only the ports you actually use
- Prefer `turns:` for browser-facing deployments when you have a valid certificate
