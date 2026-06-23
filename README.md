# wa-evolution

Serviço HTTP estilo **Evolution API** construído sobre a biblioteca
[`wa-go`](../wa-go) (a "Baileys em Go" feita do zero).

Análogo: **wa-go = Baileys** (a lib), **wa-evolution = Evolution** (o serviço
multi-instância com API REST + webhooks que o Chatwoot/workers consomem).

Importa a lib via a fachada pública `github.com/felipeleal/wa-go/wa` (replace
local em `go.mod` apontando para `../wa-go`).

## Rodar
```sh
go run ./cmd/wa-server -addr :8080 -apikey SUACHAVE -dir ./instances
```

## Rotas (header `apikey`)
- `POST /instance/create` {instanceName, webhookUrl?}
- `GET  /instance/connect/{instance}` → {code: QR base64 PNG}
- `GET  /instance/fetchInstances`
- `DELETE /instance/delete/{instance}` · `GET /instance/logout/{instance}`
- `POST /webhook/set/{instance}` {url}
- `POST /message/sendText/{instance}` {number, text}
- `POST /message/sendMedia/{instance}` {number, mediatype, media, caption}
- `POST /message/sendReaction/{instance}`
- `POST /chat/findMessages/{instance}` · `POST /chat/whatsappNumbers/{instance}`
- `GET  /group/fetchAllGroups/{instance}` · `GET /group/groupMetadata/{instance}?groupJid=`

## Webhook
Eventos granulares da lib → POST JSON no `webhookUrl` no formato Evolution
(`messages.upsert`, `messages.update`, `connection.update`, `qrcode.updated`,
`presence.update`, `group-participants.update`).

Ver `docs/` para o design.
