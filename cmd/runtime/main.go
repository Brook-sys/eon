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
	"strings"
	"syscall"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/observability"
	"motor-autonomo/internal/runtime/bootstrap"
)

func main() {
	var (
		listen             = flag.String("listen", "127.0.0.1:8080", "HTTP bind address for inspect/control/dashboard")
		storeBackend       = flag.String("store", "memory", "store backend: memory or sqlite")
		sqlitePath         = flag.String("sqlite-path", "", "SQLite checkpoint path (required for -store=sqlite)")
		missionID          = flag.String("mission-id", "", "mission ID for the continuity control loop (optional)")
		runtimeName        = flag.String("runtime-name", "motor-autonomo", "runtime identity name")
		runtimeVersion     = flag.String("runtime-version", "dev", "runtime identity version")
		dashboard          = flag.Bool("dashboard", true, "serve experimental operator dashboard")
		modelPresetCatalog = flag.String("model-preset-catalog", "", "evidence-backed model preset catalog path (optional)")
		otelEnabled        = flag.Bool("otel", false, "enable optional OpenTelemetry export (derived-only)")
		otelEndpoint       = flag.String("otel-endpoint", "", "OTLP HTTP endpoint host:port (optional)")
		otelInsecure       = flag.Bool("otel-insecure", false, "disable TLS for OTLP HTTP")
		otelSample         = flag.Float64("otel-sample", 1, "trace sample ratio in [0,1]")
		// Export-buffer retention (disposable queues only; not store GC).
		otelTraceQueue     = flag.Int("otel-trace-queue", 0, "OTLP span queue size (0 = default 2048)")
		otelTraceBatch     = flag.Int("otel-trace-batch", 0, "OTLP span export batch size (0 = default 512)")
		otelTraceFlush     = flag.Duration("otel-trace-flush", 0, "OTLP span batch timeout (0 = default 5s)")
		otelTraceExport    = flag.Duration("otel-trace-export-timeout", 0, "OTLP span export timeout (0 = default 30s)")
		otelMetricEvery    = flag.Duration("otel-metric-interval", 0, "OTLP metric export interval (0 = default 60s)")
		otelMetricExport   = flag.Duration("otel-metric-export-timeout", 0, "OTLP metric export timeout (0 = default 30s)")
		idleMin            = flag.Duration("idle-min", 50*time.Millisecond, "minimum idle sleep after empty control cycle")
		idleMax            = flag.Duration("idle-max", time.Second, "maximum idle sleep after empty control cycle")
		maxInboxBatch      = flag.Int("max-inbox-batch", 8, "max commands/events drained per control cycle")
		memoryCompactBatch = flag.Int("memory-compaction-batch", 8, "max expired semantic memories compacted per control cycle")
		deliveryBatch      = flag.Int("delivery-batch", 8, "max outbox deliveries / reminder scans per control cycle")
		deliveryLease      = flag.Duration("delivery-lease", 30*time.Second, "telegram outbox lease duration")
		deliveryRetry      = flag.Duration("delivery-retry", 15*time.Second, "telegram outbox retry delay")
		// Optional READ_ONLY web acquisition (FR-RES-001/002). Deploy-level egress
		// controls remain outside the process; no secrets on flags.
		webEnabled       = flag.Bool("web", false, "enable web.search / web.fetch path")
		webSearchBaseURL = flag.String("web-search-base-url", "", "SearXNG base URL (enables web.search)")
		webFetch         = flag.Bool("web-fetch", false, "enable hostile-by-default HTTP fetcher")
		webFetchMaxBytes = flag.Int64("web-fetch-max-bytes", 0, "fetch body cap (0 = 1 MiB default)")
		webAllowPrivate  = flag.Bool("web-fetch-allow-private", false, "allow private network targets (tests only)")
		webIngestFetched = flag.Bool("web-ingest-fetched", true, "materialize Source lineage after fetch")
		webSearchLimit   = flag.Int("web-search-limit", 0, "default search hit limit (0 = executor default)")
		// Optional READ_ONLY file path under authorized absolute roots.
		fileEnabled     = flag.Bool("file", false, "enable file.discover / file.read path")
		fileRoots       = flag.String("file-roots", "", "comma-separated name=/abs/path authorized roots")
		fileMaxRead     = flag.Int64("file-max-read-bytes", 0, "file.read cap (0 = 1 MiB default)")
		allowExec       = flag.Bool("allow-exec", false, "enable execution of commands locally via tool execution")
		enableSubagents = flag.Bool("subagents", false, "enable spawn/ orchestration tools")
		p2pEnabled      = flag.Bool("p2p", false, "enable experimental peer-to-peer subsystem")
		p2pBind         = flag.String("p2p-bind", "127.0.0.1:8443", "local bind address for P2P network")
		p2pMDNS         = flag.Bool("p2p-mdns", false, "enable mDNS beacon on P2P network")
		// Optional OpenAI-compatible provider for non-local PROPOSE_ONLY ops.
		// Secrets never appear as flags: pass -model-api-key-env=NAME only.
		modelEnabled   = flag.Bool("model", false, "enable OpenAI-compatible PROPOSE_ONLY model path")
		modelBaseURL   = flag.String("model-base-url", "", "OpenAI-compatible base URL (e.g. http://127.0.0.1:11434)")
		modelName      = flag.String("model-name", "", "provider model name")
		modelAPIKeyEnv = flag.String("model-api-key-env", "", "env var name holding API key (empty = no Authorization)")
		modelMaxField  = flag.String("model-max-output-field", "max_tokens", "max_tokens or max_completion_tokens")
		modelContext   = flag.Int("model-context-tokens", 8000, "provider context window for prompt budget")
		modelPolicy    = flag.String("model-policy-version", "policy@runtime", "changeset policy version stamp")
		modelLeaseTTL  = flag.Duration("model-lease-ttl", 15*time.Minute, "lease TTL for model-backed operations")
		// Optional FR-MODEL-004 step-7 alternate provider (one Complete only).
		modelFallbackEnabled   = flag.Bool("model-fallback", false, "enable alternate OpenAI-compatible fallback provider")
		modelFallbackBaseURL   = flag.String("model-fallback-base-url", "", "fallback OpenAI-compatible base URL")
		modelFallbackName      = flag.String("model-fallback-name", "", "fallback provider model name")
		modelFallbackAPIKeyEnv = flag.String("model-fallback-api-key-env", "", "env var name for fallback API key")
		modelFallbackMaxField  = flag.String("model-fallback-max-output-field", "", "fallback max_tokens dialect (empty = primary)")
		modelFallbackContext   = flag.Int("model-fallback-context-tokens", 0, "fallback context window (0 = primary)")
		// Telegram adapter/ingress remain opt-in through bootstrap.Options.Telegram
		// (token/allowlists/ingress mode). cmd/runtime stays free of secrets and chat IDs.
	)
	flag.Parse()

	opts := bootstrap.Options{
		ListenAddr:             *listen,
		StoreBackend:           bootstrap.StorageBackend(*storeBackend),
		SQLitePath:             *sqlitePath,
		MissionID:              domain.MissionID(*missionID),
		RuntimeName:            *runtimeName,
		RuntimeVersion:         *runtimeVersion,
		EnableDashboard:        *dashboard,
		ModelPresetCatalogPath: *modelPresetCatalog,
		AllowExec:              *allowExec,
		IdleMin:                *idleMin,
		IdleMax:                *idleMax,
		MaxInboxBatch:          *maxInboxBatch,
		MemoryCompactionBatch:  *memoryCompactBatch,
		DeliveryBatch:          *deliveryBatch,
		DeliveryLease:          *deliveryLease,
		DeliveryRetry:          *deliveryRetry,
		Observability: observability.Config{
			Enabled:      *otelEnabled,
			OTLPEndpoint: *otelEndpoint,
			Insecure:     *otelInsecure,
			SampleRatio:  *otelSample,
			Retention: observability.ExportRetention{
				TraceMaxQueueSize:       *otelTraceQueue,
				TraceMaxExportBatchSize: *otelTraceBatch,
				TraceBatchTimeout:       *otelTraceFlush,
				TraceExportTimeout:      *otelTraceExport,
				MetricInterval:          *otelMetricEvery,
				MetricExportTimeout:     *otelMetricExport,
			},
		},
	}
	if *webEnabled {
		opts.Web = &bootstrap.WebOptions{
			Enabled:            true,
			SearchBaseURL:      *webSearchBaseURL,
			EnableFetch:        *webFetch,
			FetchMaxBytes:      *webFetchMaxBytes,
			FetchAllowPrivate:  *webAllowPrivate,
			DefaultSearchLimit: *webSearchLimit,
			IngestFetched:      *webIngestFetched,
		}
	}
	if *fileEnabled {
		roots, err := parseFileRootsFlag(*fileRoots)
		if err != nil {
			log.Fatalf("file-roots: %v", err)
		}
		opts.File = &bootstrap.FileOptions{
			Enabled:      true,
			Roots:        roots,
			MaxReadBytes: *fileMaxRead,
		}
	}
	opts.Network = &bootstrap.NetworkOptions{
		Enabled:     *p2pEnabled,
		BindAddr:    *p2pBind,
		MDNSEnabled: *p2pMDNS,
	}

	opts.Subagent = &bootstrap.SubagentOptions{
		Enabled: *enableSubagents,
	}
	if *modelEnabled {
		mopts := &bootstrap.ModelOptions{
			Enabled:        true,
			BaseURL:        *modelBaseURL,
			Model:          *modelName,
			APIKeyEnv:      *modelAPIKeyEnv,
			MaxOutputField: bootstrap.ModelMaxOutputField(*modelMaxField),
			ContextTokens:  *modelContext,
			PolicyVersion:  *modelPolicy,
			LeaseTTL:       *modelLeaseTTL,
		}
		if *modelFallbackEnabled {
			mopts.Fallback = &bootstrap.ModelFallbackOptions{
				Enabled:        true,
				BaseURL:        *modelFallbackBaseURL,
				Model:          *modelFallbackName,
				APIKeyEnv:      *modelFallbackAPIKeyEnv,
				MaxOutputField: bootstrap.ModelMaxOutputField(*modelFallbackMaxField),
				ContextTokens:  *modelFallbackContext,
			}
		}
		opts.Model = mopts
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

// parseFileRootsFlag accepts "name=/abs/path,other=/abs/other" entries.
// A single bare absolute path becomes root name "default".
func parseFileRootsFlag(raw string) ([]bootstrap.FileRootConfig, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("at least one name=/abs/path root is required")
	}
	parts := strings.Split(raw, ",")
	out := make([]bootstrap.FileRootConfig, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.HasPrefix(part, "/") && !strings.Contains(part, "=") {
			out = append(out, bootstrap.FileRootConfig{Name: "default", Path: part})
			continue
		}
		name, path, ok := strings.Cut(part, "=")
		if !ok {
			return nil, fmt.Errorf("root %q must be name=/abs/path", part)
		}
		name = strings.TrimSpace(name)
		path = strings.TrimSpace(path)
		if name == "" || path == "" {
			return nil, fmt.Errorf("root %q needs non-empty name and path", part)
		}
		out = append(out, bootstrap.FileRootConfig{Name: name, Path: path})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("at least one name=/abs/path root is required")
	}
	return out, nil
}
