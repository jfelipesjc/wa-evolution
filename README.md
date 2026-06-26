# wa-evolution

[![CI](https://github.com/jfelipesjc/wa-evolution/actions/workflows/ci.yml/badge.svg)](https://github.com/jfelipesjc/wa-evolution/actions/workflows/ci.yml)
[![Release](https://github.com/jfelipesjc/wa-evolution/actions/workflows/release.yml/badge.svg)](https://github.com/jfelipesjc/wa-evolution/actions/workflows/release.yml)
[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

An **Evolution-API-compatible** WhatsApp HTTP service, written in Go on top of
[**wa-go**](https://github.com/jfelipesjc/wa-go) — a WhatsApp Web protocol stack
implemented from scratch (no whatsmeow, no Baileys).

> **The analogy:** `wa-go` is to Baileys what **`wa-evolution` is to the Evolution
> API** — a multi-instance REST + webhook service that a Chatwoot / CRM / worker
> fleet talks to. If you already speak Evolution, the routes will feel familiar.

## Why this over the original Evolution API

The reference Evolution API is Node.js and needs **Postgres + Redis** alongside it.
This one is a **single static Go binary with embedded SQLite** — no external
services, no database to provision.

| | Evolution API (original) | wa-evolution |
|---|---|---|
| Runtime | Node.js | Single static Go binary |
| Database | PostgreSQL **(required)** | SQLite (embedded file) |
| Cache | Redis **(required)** | none |
| Deploy | `docker compose` (3 services) | `docker run` (**1 container**) |
| Image size | hundreds of MB | **~20 MB** |
| Sessions | survive restart | survive restart (reloaded from disk) |

Sessions are persisted per instance as `instances/<name>.db` and **restored on
boot**, so a restart reconnects without re-pairing.

## Quick start

### Docker (recommended — one container)

```sh
docker run -d --name wa-evolution \
  -p 8080:8080 \
  -e WA_APIKEY=change-me \
  -v wa-data:/data \
  ghcr.io/jfelipesjc/wa-evolution:latest
```

### Prebuilt binary

Grab one from [Releases](https://github.com/jfelipesjc/wa-evolution/releases)
(`linux-amd64`, `linux-arm64`, `darwin-arm64`, `windows-amd64`) and run:

```sh
./wa-server -addr :8080 -apikey change-me -dir ./instances
```

### From source

```sh
go run ./cmd/wa-server -addr :8080 -apikey change-me -dir ./instances
```

### Configuration

| Flag | Env (Docker) | Default | Meaning |
|------|--------------|---------|---------|
| `-addr` | `WA_ADDR` | `:8080` | HTTP listen address |
| `-apikey` | `WA_APIKEY` | _(empty)_ | required in the `apikey` header — **empty disables auth (dev only)** |
| `-dir` | `WA_DIR` | `./instances` (`/data/instances` in Docker) | per-instance SQLite stores |

## Pairing an instance

Every route (except the Chatwoot webhook and the `/manager` UI) requires the
`apikey` header.

```sh
# 1. create an instance
curl -X POST localhost:8080/instance/create \
  -H 'apikey: change-me' -H 'content-type: application/json' \
  -d '{"instanceName":"chip1"}'

# 2a. pair by QR — returns a base64 PNG you scan in WhatsApp > Linked devices
curl localhost:8080/instance/connect/chip1 -H 'apikey: change-me'

# 2b. or pair by 8-char code: create with a phone number, then read the code
#     from /instance/connect and type it in WhatsApp > Linked devices > with code

# 3. send a message
curl -X POST localhost:8080/message/sendText/chip1 \
  -H 'apikey: change-me' -H 'content-type: application/json' \
  -d '{"number":"5512999999999","text":"hello from wa-evolution"}'
```

A built-in web dashboard is served at **`/manager`** (no auth) for eyeballing
instance state and QR codes.

## API surface (~69 routes)

Evolution-shaped paths, grouped by area. All take the `apikey` header; the
`{instance}` segment selects the session.

- **Instances** — `POST /instance/create`, `GET /instance/connect/{i}`,
  `GET /instance/fetchInstances`, `GET /instance/connectionState/{i}`,
  `GET /instance/logout/{i}`, `DELETE /instance/delete/{i}`,
  `PUT /instance/restart/{i}`, `POST /instance/setPresence/{i}`
- **Messages** — `sendText`, `sendMedia`, `sendWhatsAppAudio`, `sendPtv`,
  `sendSticker`, `sendReaction`, `sendLocation`, `sendContact`, `sendPoll`,
  `sendButtons`, `sendList`, `sendStatus`, `editMessage`, `deleteMessage`,
  `markMessageAsRead` (all `POST /message/<op>/{i}`)
- **Chats** — `findMessages`, `findChats`, `findContacts`, `whatsappNumbers`,
  `archiveChat`, `markChatUnread`, `sendPresence` (typing),
  `fetchProfilePictureUrl`, `getBase64FromMediaMessage`
- **Groups** — `create`, `updateParticipant` (add/remove/promote/demote),
  `groupMetadata`, `fetchAllGroups`, `participants`, `leave`, `inviteCode`,
  `revokeInviteCode`, `acceptInviteCode`, `sendInvite`, `updateGroupSubject`,
  `updateGroupDescription`, `updateGroupPicture`, `toggleEphemeral`,
  `updateSetting` (announcement/locked)
- **Profile & privacy** — `updateProfileName`, `updateProfileStatus`,
  `updateProfilePicture`, `removeProfilePicture`, `fetchPrivacySettings`,
  `updatePrivacySettings`, `updateBlockStatus`
- **Status / Newsletters / Business** — `message/sendStatus`,
  `chat/findStatusMessage`, `newsletter/create`, `newsletter/follow`,
  `business/getCatalog`, `business/getCollections`
- **Labels, Calls, Settings, Proxy** — `label/findLabels`, `label/handleLabel`,
  `call/offer`, `settings/{set,find}`, `proxy/{set,find}`
- **Webhook / Chatwoot** — `webhook/{set,find}`, `chatwoot/{set,find}`,
  `POST /chatwoot/webhook/{i}` (no apikey — receives Chatwoot replies)

> Sending a text to a **group JID** routes through the group sender-key path, and
> `archiveChat` drives the app-state (LTHash) machinery — the "advanced" protocol
> internals are exercised through these ordinary routes.

## Webhooks

Granular library events are POSTed to each instance's configured `webhookUrl` in
**Evolution shape**: `messages.upsert`, `messages.update`, `connection.update`,
`qrcode.updated`, `presence.update`, `group-participants.update`.

```sh
curl -X POST localhost:8080/webhook/set/chip1 \
  -H 'apikey: change-me' -H 'content-type: application/json' \
  -d '{"url":"https://your-app.example/webhook"}'
```

## Chatwoot

Native two-way Chatwoot bridge: inbound WhatsApp → Chatwoot conversations, and
Chatwoot agent replies → WhatsApp (via `POST /chatwoot/webhook/{instance}`).
Configure with `POST /chatwoot/set/{instance}`.

## Relationship to wa-go

This service is a thin HTTP/Chatwoot shell. **All WhatsApp protocol work** —
Noise handshake, Signal E2E, group sender keys, app-state, media — lives in the
library [`wa-go`](https://github.com/jfelipesjc/wa-go), imported via its public
`wa/` facade. Improve the protocol there; this service inherits it.

## License

[MIT](LICENSE) © José Felipe Leal
