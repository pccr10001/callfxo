# Android App (CallFXO)

Path: `android/`

- Package: `li.power.app.callfxo.android`
- Target SDK: 36
- Min SDK: 32

## Features

- Login with `server address + username + password`
- Long-lived session cookie persisted locally
- WebSocket signaling connects only when:
  - app is foreground, or
  - a call is active
- Dial + Hangup + DTMF keypad
- Call logs view (paged)
- Contacts view:
  - server contacts
  - optional device contacts (READ_CONTACTS)
  - search by name or number
- Foreground call notification service for call stability
- Exit button to fully close app process

## Build

Open `android/` with Android Studio and run app module.

This project currently does not include a committed Gradle wrapper. If you prefer CLI builds, generate one from Android Studio first.

WebRTC dependency:
- `io.getstream:stream-webrtc-android:1.3.10`

## Server URL

Use full URL like:
- `http://192.168.1.10:8080`
- `https://your-domain`

## Backend API expectation

The app calls:
- `POST /api/login`
- `GET /api/me`
- `POST /api/logout`
- `GET /api/fxo`
- `GET /api/contacts?limit=500&q=...`
- `GET /api/calls?page=...&page_size=...`
- WebSocket `/ws/signaling` (cookie-authenticated)

## Permissions

Runtime permissions requested when needed:
- `RECORD_AUDIO`
- `READ_CONTACTS`
- `POST_NOTIFICATIONS` (Android 13+)
