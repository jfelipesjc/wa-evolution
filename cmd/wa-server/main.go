// Command wa-server runs an Evolution-API-compatible HTTP service over the
// multi-session WhatsApp stack (internal/manager + internal/client +
// internal/store). It is the network-facing process the user's Chatwoot/workers
// talk to; like wa-pair/wa-manager it connects to REAL WhatsApp and is run by a
// human, not by `go test`.
//
// Usage:
//
//	go run ./cmd/wa-server -addr :8080 -apikey secret -dir ./instances
//
// Every route requires the apikey header. Instances are created via
// POST /instance/create and persisted as ./instances/<name>.db. Inbound events
// are POSTed to each instance's configured webhookUrl in Evolution shape.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jfelipesjc/wa-evolution/internal/api"
	wa "github.com/jfelipesjc/wa-go/wa"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	apikey := flag.String("apikey", "", "global API key required in the apikey header (empty disables auth — dev only)")
	dir := flag.String("dir", "./instances", "directory for per-instance SQLite stores")
	flag.Parse()

	if os.Getenv("WA_DEBUG") != "" {
		wa.EnableDebug(os.Stderr)
		fmt.Fprintln(os.Stderr, "wa-server: pairing debug ENABLED")
	}

	if err := run(*addr, *apikey, *dir); err != nil {
		fmt.Fprintf(os.Stderr, "wa-server: %v\n", err)
		os.Exit(1)
	}
}

// numeroDaInstancia devolve os dígitos do número pareado na instância ("" se
// ainda não pareou).
func numeroDaInstancia(backend *api.ManagerBackend, instance string) string {
	numero, _ := backend.OwnProfile(instance)
	return numero
}

// mesmoNumero compara dois endereços do WhatsApp só pelos dígitos, ignorando o
// sufixo de aparelho (":12") e o domínio (@s.whatsapp.net / @lid).
func mesmoNumero(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return soDigitosJID(a) == soDigitosJID(b)
}

func soDigitosJID(jid string) string {
	if i := strings.IndexAny(jid, ":@"); i >= 0 {
		jid = jid[:i]
	}
	return jid
}

func run(addr, apikey, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Fetch the live WhatsApp Web version before any instance connects, so an
	// expired hardcoded version never silently breaks pairing/login. Best-effort.
	func() {
		vctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := wa.RefreshVersion(vctx); err != nil {
			fmt.Fprintf(os.Stderr, "wa-server: version refresh failed (fallback %v): %v\n", wa.CurrentVersion(), err)
		} else {
			fmt.Fprintf(os.Stderr, "wa-server: WhatsApp Web version %v\n", wa.CurrentVersion())
		}
	}()

	// Keep the version fresh on long-running processes (WhatsApp expires Web
	// versions ~every 60 days); reconnects then pick up the new version.
	go func() {
		t := time.NewTicker(12 * time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				rctx, cancel := context.WithTimeout(ctx, 10*time.Second)
				_ = wa.RefreshVersion(rctx)
				cancel()
			}
		}
	}()

	mgr := wa.NewManager()
	mgr.Start(ctx)

	backend := api.NewManagerBackend(mgr, dir)

	// Restore previously-paired instances from disk so sessions survive restarts
	// (the store reloads saved creds; the manager reconnects without a re-pair).
	if restored, rerr := backend.Restore(); rerr != nil {
		fmt.Fprintf(os.Stderr, "wa-server: restore warning: %v\n", rerr)
	} else if len(restored) > 0 {
		fmt.Fprintf(os.Stderr, "wa-server: restored %d instance(s): %v\n", len(restored), restored)
	}

	srv := api.New(api.Options{
		APIKey:     apikey,
		Backend:    backend,
		WebhookDir: dir,
	})

	// Pump manager events -> webhooks + per-instance ChatStore + QR capture +
	// Chatwoot inbound bridge.
	feed := func(instance string, ev wa.Event) {
		if cs := backend.ChatStore(instance); cs != nil {
			cs.Consume(ev)
		}
		// Inbound bridge: a received WhatsApp message -> Chatwoot (no-op unless the
		// instance has chatwoot enabled). Skip group messages (the bridge also
		// drops @g.us, but this avoids the work). Media is fetched lazily from the
		// ChatStore (already populated by Consume above) only if the bridge needs it.
		if mev, ok := ev.(wa.MessageEvent); ok && !mev.IsGroup {
			text := mev.Text
			if mev.Reaction != nil {
				text = mev.Reaction.Text // bridge the reaction emoji ("" un-react -> bridge drops)
			}
			// QUAL CONVERSA recebe a mensagem no painel.
			//
			// 1) Resposta dada no CELULAR da loja chega espelhada com o nosso
			//    próprio número como remetente; a conversa certa é a do
			//    DESTINATÁRIO (DestinationJID), nunca a da loja com ela mesma.
			// 2) O WhatsApp passou a endereçar contatos por LID ("196...@lid"),
			//    que não é telefone de ninguém: preferimos o número real
			//    (SenderPN) para o contato do painel nascer com o telefone certo
			//    e a resposta do atendente voltar para o cliente.
			// 3) Nas mensagens espelhadas o SenderPN pode vir sendo o número da
			//    PRÓPRIA loja — usá-lo jogaria as respostas de vários clientes
			//    numa única conversa da loja consigo mesma (foi o que aconteceu
			//    na conversa 337 do painel).
			// O download da mídia continua usando mev.From, que é a chave sob a
			// qual o ChatStore guardou a mensagem.
			bridgeJID := mev.From
			if mev.FromMe && mev.DestinationJID != "" {
				bridgeJID = mev.DestinationJID
			}
			if mev.SenderPN != "" && !mesmoNumero(mev.SenderPN, numeroDaInstancia(backend, instance)) {
				bridgeJID = mev.SenderPN
			}
			// O pushName de uma mensagem espelhada é o nome da PRÓPRIA loja, não o
			// do cliente: usá-lo batizava o contato novo de "Ski In Chile" (o
			// painel tem dezenas de contatos assim). Sem nome, a ponte usa o
			// número, que a equipe reconhece.
			nomeExibido := mev.PushName
			if mev.FromMe {
				nomeExibido = ""
			}
			im := api.InboundMessage{
				JID: bridgeJID, MsgID: mev.ID, PushName: nomeExibido, Text: text,
				IsMedia: mev.Media != nil,
				// Resposta dada no CELULAR da loja: o WhatsApp espelha para cá e,
				// sem esta flag, ela entrava no Chatwoot como "incoming" — a
				// resposta da equipe aparecia como se o cliente tivesse escrito.
				FromMe: mev.FromMe,
			}
			if mev.Quoted != nil {
				im.QuotedWAID = mev.Quoted.StanzaID // reply linkage (in_reply_to)
			}
			if mev.Media != nil {
				im.Mimetype = mev.Media.Mimetype
				im.FileName = mev.Media.FileName
				jid, id := mev.From, mev.ID
				im.Download = func() ([]byte, string, error) {
					return backend.GetBase64FromMedia(context.Background(), instance, jid, id)
				}
			}
			go srv.HandleChatwootInbound(context.Background(), instance, im)
		}
		if qr, ok := ev.(wa.QREvent); ok {
			backend.SetQR(instance, qr.Code)
		}
		if pc, ok := ev.(wa.PairingCodeEvent); ok {
			backend.SetPairingCode(instance, pc.Code)
			fmt.Fprintf(os.Stderr, "[evt] %s PAIRING CODE %s\n", instance, pc.Code)
		}
		// Connection-lifecycle visibility: surface login/disconnect (with the
		// stream:error reason) so session drops are diagnosable from the log.
		switch e := ev.(type) {
		case wa.LoggedInEvent:
			// Clear any cached QR / pairing code so a reconnect never serves a
			// stale (now-invalid) code/QR from /instance/connect.
			backend.SetQR(instance, "")
			backend.SetPairingCode(instance, "")
			fmt.Fprintf(os.Stderr, "[evt] %s LOGGED IN\n", instance)
		case wa.DisconnectedEvent:
			fmt.Fprintf(os.Stderr, "[evt] %s DISCONNECTED: %s\n", instance, e.Reason)
		case wa.QREvent:
			fmt.Fprintf(os.Stderr, "[evt] %s QR\n", instance)
		}
	}
	go api.RunEventPump(ctx, mgr, srv.Dispatcher(), feed)

	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutCtx)
	}()

	fmt.Fprintf(os.Stderr, "wa-server: listening on %s (instances dir %s)\n", addr, dir)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		mgr.Stop()
		return err
	}
	mgr.Stop()
	return nil
}
