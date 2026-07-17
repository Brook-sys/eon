package bootstrap

import (
	"fmt"
	"path/filepath"
	"strings"

	"motor-autonomo/internal/ingest"
	"motor-autonomo/internal/kernel"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/provider/web/httpfetch"
	"motor-autonomo/internal/provider/web/searxng"
	"motor-autonomo/internal/runtime/source"
)

// buildWeb assembles an optional WebExecutor. Returns nil when Web is disabled.
// Search and fetch adapters may be independently present; missing adapters
// surface as execute-time errors for the corresponding capability only.
func buildWeb(
	opts Options,
	store port.Store,
	clock source.Clock,
	ids source.IDGenerator,
) (*kernel.WebExecutor, error) {
	if opts.Web == nil || !opts.Web.Enabled {
		return nil, nil
	}
	var searcher port.WebSearcher
	if base := strings.TrimSpace(opts.Web.SearchBaseURL); base != "" {
		s, err := searxng.New(searxng.Config{
			BaseURL:          base,
			MaxResponseBytes: opts.Web.SearchMaxResponseBytes,
		})
		if err != nil {
			return nil, fmt.Errorf("web search provider: %w", err)
		}
		searcher = s
	}
	var fetcher port.WebFetcher
	if opts.Web.EnableFetch {
		f, err := httpfetch.New(httpfetch.Config{
			MaxBytes:             opts.Web.FetchMaxBytes,
			AllowPrivateNetworks: opts.Web.FetchAllowPrivate,
		})
		if err != nil {
			return nil, fmt.Errorf("web fetch provider: %w", err)
		}
		fetcher = f
	}
	if searcher == nil && fetcher == nil {
		return nil, fmt.Errorf("web enabled but no search or fetch adapter configured")
	}
	policy := opts.Web.PolicyVersion
	if strings.TrimSpace(policy) == "" {
		policy = "policy@runtime"
	}
	authorizer, err := kernel.NewMVPCapabilityAuthorizer(store, clock, policy)
	if err != nil {
		return nil, fmt.Errorf("web capability authorizer: %w", err)
	}
	exec := &kernel.WebExecutor{
		Store:              store,
		Clock:              clock,
		IDs:                ids,
		Searcher:           searcher,
		Fetcher:            fetcher,
		Authorizer:         authorizer,
		LeaseTTL:           opts.Web.LeaseTTL,
		DefaultSearchLimit: opts.Web.DefaultSearchLimit,
	}
	if opts.Web.IngestFetched && fetcher != nil {
		exec.Ingest = &ingest.Ingester{
			Store: store,
			Clock: clock,
			IDs:   ids,
		}
	}
	return exec, nil
}

// buildFile assembles an optional FileExecutor under authorized roots.
func buildFile(
	opts Options,
	store port.Store,
	clock source.Clock,
	ids source.IDGenerator,
) (*kernel.FileExecutor, error) {
	if opts.File == nil || !opts.File.Enabled {
		return nil, nil
	}
	if len(opts.File.Roots) == 0 {
		return nil, fmt.Errorf("file enabled requires authorized roots")
	}
	roots := make([]kernel.FileRoot, 0, len(opts.File.Roots))
	for _, r := range opts.File.Roots {
		path := filepath.Clean(strings.TrimSpace(r.Path))
		if !filepath.IsAbs(path) {
			return nil, fmt.Errorf("file root %q path must be absolute", r.Name)
		}
		roots = append(roots, kernel.FileRoot{
			Name: strings.TrimSpace(r.Name),
			Path: path,
		})
	}
	policy := opts.File.PolicyVersion
	if strings.TrimSpace(policy) == "" {
		policy = "policy@runtime"
	}
	authorizer, err := kernel.NewMVPCapabilityAuthorizer(store, clock, policy)
	if err != nil {
		return nil, fmt.Errorf("file capability authorizer: %w", err)
	}
	return &kernel.FileExecutor{
		Store:              store,
		Clock:              clock,
		IDs:                ids,
		Roots:              roots,
		Authorizer:         authorizer,
		LeaseTTL:           opts.File.LeaseTTL,
		MaxReadBytes:       opts.File.MaxReadBytes,
		MaxDiscoverEntries: opts.File.MaxDiscoverEntries,
	}, nil
}
