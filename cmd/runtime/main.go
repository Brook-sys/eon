// Command runtime starts the autonomous epistemic runtime process.
//
// Assembly lives in internal/runtime/bootstrap so the entrypoint stays thin:
// parse flags, open the process graph, run HTTP + control loop, then close.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/observability"
	"motor-autonomo/internal/runtime/bootstrap"
)

func main() {
	var (
		listen         = flag.String("listen", "127.0.0.1:8080", "HTTP bind address for inspect/control/dashboard")
		storeBackend   = flag.String("store", "memory", "store backend: memory or sqlite")
		sqlitePath     = flag.String("sqlite-path", "", "SQLite checkpoint path (required for -store=sqlite)")
		missionID      = flag.String("mission-id", "", "mission ID for the continuity control loop (optional)")
		runtimeName    = flag.String("runtime-name", "motor-autonomo", "runtime identity name")
		runtimeVersion = flag.String("runtime-version", "dev", "runtime identity version")
		dashboard      = flag.Bool("dashboard", true, "serve experimental operator dashboard")
		otelEnabled    = flag.Bool("otel", false, "enable optional OpenTelemetry export (derived-only)")
		otelEndpoint   = flag.String("otel-endpoint", "", "OTLP HTTP endpoint host:port (optional)")
		otelInsecure   = flag.Bool("otel-insecure", false, "disable TLS for OTLP HTTP")
		otelSample     = flag.Float64("otel-sample", 1, "trace sample ratio in [0,1]")
		idleMin        = flag.Duration("idle-min", 50*time.Millisecond, "minimum idle sleep after empty control cycle")
		idleMax        = flag.Duration("idle-max", time.Second, "maximum idle sleep after empty control cycle")
		maxInboxBatch  = flag.Int("max-inbox-batch", 8, "max commands/events drained per control cycle")
		deliveryBatch  = flag.Int("delivery-batch", 8, "max outbox deliveries / reminder scans per control cycle")
		deliveryLease  = flag.Duration("delivery-lease", 30*time.Second, "telegram outbox lease duration")
		deliveryRetry  = flag.Duration("delivery-retry", 15*time.Second, "telegram outbox retry delay")
	)
	flag.Parse()

	opts := bootstrap.Options{
		ListenAddr:      *listen,
		StoreBackend:    bootstrap.StorageBackend(*storeBackend),
		SQLitePath:      *sqlitePath,
		MissionID:       domain.MissionID(*missionID),
		RuntimeName:     *runtimeName,
		RuntimeVersion:  *runtimeVersion,
		EnableDashboard: *dashboard,
		IdleMin:         *idleMin,
		IdleMax:         *idleMax,
		MaxInboxBatch:   *maxInboxBatch,
		DeliveryBatch:   *deliveryBatch,
		DeliveryLease:   *deliveryLease,
		DeliveryRetry:   *deliveryRetry,
		Observability: observability.Config{
			Enabled:      *otelEnabled,
			OTLPEndpoint: *otelEndpoint,
			Insecure:     *otelInsecure,
			SampleRatio:  *otelSample,
		},
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rt, err := bootstrap.Open(ctx, opts)
	if err != nil {
		log.Fatalf("runtime open: %v", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := rt.Close(shutdownCtx); err != nil {
			log.Printf("runtime close: %v", err)
		}
	}()

	errCh := make(chan error, 2)
	go func() {
		if err := rt.RunHTTP(ctx); err != nil {
			errCh <- fmt.Errorf("http: %w", err)
			return
		}
		errCh <- nil
	}()
	go func() {
		if err := rt.RunControlLoop(ctx); err != nil {
			errCh <- fmt.Errorf("control loop: %w", err)
			return
		}
		errCh <- nil
	}()

	var first error
	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil && first == nil {
			first = err
			stop()
		}
	}
	if first != nil {
		log.Fatalf("runtime exit: %v", first)
	}
}
