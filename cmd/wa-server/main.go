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

	// Pump manager events -> webhooks + per-instance ChatStore + QR capture.
	feed := func(instance string, ev wa.Event) {
		if cs := backend.ChatStore(instance); cs != nil {
			cs.Consume(ev)
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
