# Deploy do wa-evolution

Serviço HTTP em Go (sobre a lib `wa-go`) compatível com o estilo Evolution API.
Binário **self-contained** (CGo-free, SQLite puro-Go) — não precisa de runtime.

## Opção A — binário + systemd (mais simples)
```sh
# build (na máquina dev, na raiz que contém wa-go/ e wa-evolution/)
cd wa-evolution
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags='-s -w' -o wa-server ./cmd/wa-server

# no servidor (ex. cpx43):
sudo useradd -r -s /usr/sbin/nologin wa || true
sudo mkdir -p /opt/wa-evolution/instances && sudo chown -R wa:wa /opt/wa-evolution
scp wa-server cpx43:/opt/wa-evolution/wa-server
scp deploy/wa-evolution.service cpx43:/etc/systemd/system/
scp deploy/wa-evolution.env.example cpx43:/opt/wa-evolution/wa-evolution.env   # edite o APIKEY!
sudo systemctl daemon-reload && sudo systemctl enable --now wa-evolution
curl -H "apikey: SEU_SEGREDO" http://127.0.0.1:8091/instance/fetchInstances    # -> []
```

## Opção B — Docker / Dokploy
O `go.mod` usa `replace .. => ../wa-go`, então o **contexto de build é o diretório-pai**
(que contém `wa-go/` e `wa-evolution/`):
```sh
docker build -f wa-evolution/deploy/Dockerfile -t wa-evolution .
docker run -d --name wa-evolution -p 8091:8091 \
  -e APIKEY=seu-segredo -v wa-data:/opt/wa-evolution/instances wa-evolution \
  -addr :8091 -apikey "$APIKEY" -dir /opt/wa-evolution/instances
```
No Dokploy: app do tipo Dockerfile, **build context = repo-pai**, Dockerfile path =
`wa-evolution/deploy/Dockerfile`, porta 8091, volume em `/opt/wa-evolution/instances`.

## Parear uma conta (headless, sem QR)
1. `POST /instance/create {"instanceName":"conta1"}`
2. `GET  /instance/connect/conta1` → começa o login
3. (pairing-code headless: ver `cmd/wa-paircode` / `ConnectWithPairingCode`) — gera um
   código de 8 chars que você digita no celular em Aparelhos conectados → Conectar com
   número de telefone. Não precisa de câmera/QR.

## Webhooks (Chatwoot/workers)
`POST /webhook/set/conta1 {"url":"https://.../webhook"}` — eventos no formato Evolution
(`messages.upsert`, etc.). Rotas: instance/message/chat/group (27 endpoints).

## Notas
- Pareamento (QR+código), receber, enviar texto/mídia/reação e **grupos** provados live.
- NÃO re-parear/remover a mesma conta em loop (queima o device-management no servidor).
