# CallFXO

CallFXO is a lightweight SIP + WebRTC bridge server for dialing PSTN through FXO gateways.

It provides:
- SIP registrar for FXO boxes (REGISTER)
- SIP UAC call control for outbound INVITE to a selected FXO box
- WebRTC audio bridge in the browser (PCMU passthrough)
- Optional TURN / TURNS ICE server support for NAT traversal
- Web UI for users, FXO boxes, dialing, DTMF, call logs, and personal contacts
- Password change:
  - user can change only their own password
  - admin can change any user's password
- Role-based access control (`admin` / `user`)
- SQLite persistence
- WebSocket signaling and live FXO online/offline status updates

## Tested FXO Devices

Confirmed working with:
- Cisco SPA3000
- Cisco SPA232D

For these devices, minimum setup is:
1. Set **Proxy** to your CallFXO SIP address (for example `192.168.1.10:5060`).
  - If it's not working, try `TCP `instead of `UDP` as SIP protocol.
2. Set SIP **username/password** to values created in CallFXO FXO box management.
3. Enable **PSTN Line**.
4. Enable **Register**.

## Requirements

- Go 1.25+
- A browser with WebRTC + PCMU support
- Network reachability between browser, CallFXO server, and FXO box

## Quick Start

1. Copy the example config and edit it:

```bash
cp config.yaml.example config.yaml
```

2. Set at least:
  - `sip.advertised_ip` and `media.public_ip` to the IP reachable by FXO boxes and browsers
  - `database.path` to your SQLite file
  - `auth.access_ttl_minutes` / `auth.refresh_ttl_hours` to your desired session lifetime
  - `media.ice_stun_urls` and optionally `media.ice_turn_urls` for browser / app ICE traversal
  - `fcm.*` only if you want Android / browser push notifications

3. Start server:

```bash
go run . -config config.yaml
```

4. Open web UI:

```text
http://<server-ip>:8080
```

5. Login with bootstrap admin:
  - first run (DB not exists): check console output for generated password
  - existing DB: use your existing admin password

## Docker

`Dockerfile` and `docker-compose.yml` are included.

1. In `config.yaml`, set:
  - `database.path: /data/callfxo.db`
  - `sip.advertised_ip` and `media.public_ip` to your host/server reachable IP
2. Start:

```bash
docker compose up -d --build
```

Exposed ports:
- `8080/tcp` (web)
- `5060/tcp` and `5060/udp` (SIP)
- `12000-12100/udp` (RTP dynamic range, default compose profile)

Note: compose sets `net.ipv4.ip_local_port_range=12000 12100` so RTP ephemeral ports stay inside mapped UDP range.

## Default Bootstrap Account

When the database file does not exist (first run), the server auto-generates:
- bootstrap admin password
- auth session secret (cookie signing secret)

Both are persisted in SQLite (`app_settings` for secret) and loaded into memory on startup.

The generated bootstrap admin password is printed to the server console once at first initialization.

Change this in production.

## Core Config

`config.yaml` example fields:

```yaml
http:
  listen: ":8080"

sip:
  transport: "tcp"
  listen: "0.0.0.0:5060"
  realm: "callfxo"
  domain: "callfxo.local"
  advertised_ip: "192.168.26.160"
  contact_user: "callfxo"

media:
  rtp_bind_ip: "0.0.0.0"
  public_ip: "192.168.26.160"
  ice_stun_urls:
    - "stun:stun.l.google.com:19302"
  ice_turn_urls:
    - "turn:turn.example.com:3478?transport=udp"
    - "turn:turn.example.com:3478?transport=tcp"
    - "turns:turn.example.com:5349?transport=tcp"
  ice_turn_username: ""
  ice_turn_credential: ""
  ice_turn_shared_secret: ""
  ice_turn_credential_ttl_minutes: 60

database:
  path: "./callfxo.db"

auth:
  cookie_name: "callfxo_access"
  access_ttl_minutes: 60
  refresh_ttl_hours: 720

fcm:
  enabled: false
  project_id: ""
  app_id: ""
  api_key: ""
  messaging_sender_id: ""
  auth_domain: ""
  storage_bucket: ""
  measurement_id: ""
  vapid_key: ""
  service_account_json: ""
```

Use `config.yaml.example` as the authoritative template. Legacy `auth.session_ttl_hours` is still read for compatibility, but new deployments should use `access_ttl_minutes` and `refresh_ttl_hours`.

## TURN / TURNS

For double-NAT or restrictive network environments, configure TURN or TURNS in `media.ice_turn_urls`.

CallFXO supports both:

- fixed TURN credentials via `media.ice_turn_username` / `media.ice_turn_credential`
- coturn shared-secret dynamic credentials via `media.ice_turn_shared_secret`

If `media.ice_turn_shared_secret` is set, CallFXO generates short-lived credentials for Web and Android clients automatically.

An example coturn deployment is included in [coturn/README.md](/C:/Users/pccr10001/go/src/github.com/pccr10001/callfxo/coturn/README.md).

## FCM Push

- The server is the source of truth for Firebase config. Android and web clients fetch `/api/push/config` after login and cache it locally.
- Set `fcm.enabled: true` only when you want push notifications for incoming calls.
- `fcm.service_account_json` must point to a readable Firebase service account JSON file on the server. This is required for sending FCM messages.
- Browser push also needs `fcm.vapid_key`.
- If FCM is disabled or not configured, foreground web / app calling still works; only push wake-up notifications are skipped.

## Roles

- `admin`: manage users + FXO boxes, and place calls
- `user`: place calls only

## Dialing UX

- Choose FXO box + enter number + click **Dial**
- DTMF keypad available during call
- Click a phone number in **Call Logs** or **Contacts** to auto-fill the dial input
- Call logs and contacts are per-user

## Notes

- This project is designed for **PCMU passthrough** `G711u` (no server-side transcoding).
- If your browser/device path cannot negotiate PCMU, add a transcoder externally.
