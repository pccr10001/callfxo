# CallFXO

CallFXO is a lightweight SIP + WebRTC bridge server for dialing PSTN through FXO gateways.

It provides:
- SIP registrar for FXO boxes (REGISTER)
- SIP UAC call control for outbound INVITE to a selected FXO box
- WebRTC audio bridge in the browser (PCMU passthrough)
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

1. Edit `config.yaml`.
2. Start server:

```bash
go run . -config config.yaml
```

3. Open web UI:

```text
http://<server-ip>:8080
```

4. Login with bootstrap admin:
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

database:
  path: "./callfxo.db"

auth:
  cookie_name: "callfxo_session"
  session_ttl_hours: 24
```

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
