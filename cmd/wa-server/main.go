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
	"syscall"
	"time"

	"github.com/felipeleal/wa-evolution/internal/api"
	wa "github.com/felipeleal/wa-go/wa"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	apikey := flag.String("apikey", "", "global API key required in the apikey header (empty disables auth — dev only)")
	dir := flag.String("dir", "./instances", "directory for per-instance SQLite stores")
	flag.Parse()

	if err := run(*addr, *apikey, *dir); err != nil {
		fmt.Fprintf(os.Stderr, "wa-server: %v\n", err)
		os.Exit(1)
	}
}

func run(addr, apikey, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

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
			im := api.InboundMessage{
				JID: mev.From, MsgID: mev.ID, PushName: mev.PushName, Text: text,
				IsMedia: mev.Media != nil,
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
